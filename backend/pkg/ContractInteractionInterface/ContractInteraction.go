package ContractInteraction

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	h "github.com/rius2g/Master/backend/helper"
	t "github.com/rius2g/Master/backend/pkg/types"
)

type txMeta struct {
	Seq  uint64
	Node string
}

type txInfo struct {
	meta         txMeta
	dependencies [][32]byte
	gasPrice     *big.Int
}

const NetworkEndpoint = "wss://api.avax-test.network/ext/bc/C/ws"

type ContractInteractionInterface struct {
	contractAddress common.Address
	contractABI     abi.ABI
	client          *ethclient.Client
	rpcClient       *rpc.Client // raw RPC for batch calls
	privateKey      string

	dataIdCounter *big.Int
	data          map[string][]byte
	dependencies  map[string]t.DependencyInfo

	nonceManager *NonceManager
	txLock       sync.Mutex

	dependencyTracker *DependencyTracker

	txHashMap     map[string][32]byte
	txHashMapLock sync.RWMutex

	fastEventID  common.Hash      // selector of FastBroadcast
	receiptQueue chan common.Hash // async confirmation

	pending   map[common.Hash]txInfo // pending transactions
	confirmed int64
	sent      int64
}

func Init(contractAddress, privateKey string) (*ContractInteractionInterface, error) {
	log.Println("Initializing contract interaction…")

	// RPC + WS
	if contractAddress == "" || privateKey == "" {
		log.Fatalf("Contract address and private key are required")
	}
	rpcCli, err := rpc.Dial(NetworkEndpoint)
	if err != nil {
		return nil, fmt.Errorf("dial RPC: %w", err)
	}
	client := ethclient.NewClient(rpcCli)

	// ABI
	contractABI, err := h.LoadABI()
	if err != nil {
		return nil, fmt.Errorf("load ABI: %w", err)
	}

	// Key / nonce manager
	pk, err := crypto.HexToECDSA(strings.TrimPrefix(privateKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("parse key: %w", err)
	}
	addr := crypto.PubkeyToAddress(pk.PublicKey)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	startNonce, err := client.PendingNonceAt(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("pending nonce: %w", err)
	}

	ci := &ContractInteractionInterface{
		contractAddress: common.HexToAddress(contractAddress),
		contractABI:     contractABI,
		client:          client,
		rpcClient:       rpcCli,
		privateKey:      privateKey,

		dataIdCounter: big.NewInt(0),
		data:          make(map[string][]byte),
		dependencies:  make(map[string]t.DependencyInfo),

		nonceManager: NewNonceManager(addr, startNonce),

		dependencyTracker: nil, // set below
		txHashMap:         make(map[string][32]byte),

		fastEventID:  contractABI.Events["FastBroadcast"].ID,
		receiptQueue: make(chan common.Hash, 10_000),
		pending:      make(map[common.Hash]txInfo),
		confirmed:    0,
	}
	ci.dependencyTracker = NewDependencyTracker(ci)

	go ci.watchNewHeads()

	if err := ci.RetriveCurrentDataID(); err != nil {
		return nil, err
	}
	return ci, nil
}

func (c *ContractInteractionInterface) Upload(payloadBytes []byte, owner, dataName string, seq uint64, dependencies [][32]byte, input []byte) error {
	if len(payloadBytes) == 0 || len(owner) == 0 || len(dataName) == 0 {
		return fmt.Errorf("invalid input data")
	}

	if c.dependencyTracker != nil {
		messageHash := crypto.Keccak256Hash(payloadBytes)
		var hash32 [32]byte
		copy(hash32[:], messageHash[:])

		if len(dependencies) > 0 {
			for _, dep := range dependencies {
				if !c.dependencyTracker.IsConfirmed(dep) {
					c.dependencyTracker.QueueMessage(string(payloadBytes), owner, dataName, dependencies)
					return fmt.Errorf("dependency not confirmed, message queued")
				}
			}
		}
	}

	// Validate dependencies
	if len(dependencies) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		depInput, err := c.contractABI.Pack("getDependencyTimestamps", dependencies)
		if err != nil {
			return fmt.Errorf("failed to pack input data: %v", err)
		}

		result, err := c.client.CallContract(ctx, ethereum.CallMsg{
			To:   &c.contractAddress,
			Data: depInput,
		}, nil)
		if err != nil {
			return fmt.Errorf("failed to call contract: %v", err)
		}

		var depTimestamps []*big.Int
		err = c.contractABI.UnpackIntoInterface(&depTimestamps, "getDependencyTimestamps", result)
		if err != nil {
			return fmt.Errorf("failed to unpack return value: %v", err)
		}

		for i, timestamp := range depTimestamps {
			if timestamp.Int64() == 0 {
				return fmt.Errorf("dependency %d not found", i)
			}
		}
	}

	// Send transaction using the input you already packed externally
	if err := c.executeTransaction(payloadBytes, input, dependencies, owner, seq); err != nil {
		return fmt.Errorf("failed to execute transaction: %v", err)
	}

	log.Printf("Message published successfully: %s", dataName)
	return nil
}

func (c *ContractInteractionInterface) RetrieveMissing() error {
	fmt.Println("=== RETRIEVING MISSING DATA ===")
	fmt.Printf("Current DataIdCounter: %d\n", c.dataIdCounter.Int64())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	input, err := c.contractABI.Pack("getMissingDataItems", c.dataIdCounter)
	if err != nil {
		fmt.Printf("ERROR: Failed to pack input data: %v\n", err)
		return fmt.Errorf("failed to pack input data: %v", err)
	}

	fmt.Println("Calling contract to get missing data...")
	result, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &c.contractAddress,
		Data: input,
	}, nil)
	if err != nil {
		fmt.Printf("ERROR: Failed to call contract: %v\n", err)
		return fmt.Errorf("failed to call contract: %v", err)
	}
	fmt.Printf("Got result, length: %d bytes\n", len(result))

	var returnVal []t.StoredData
	err = c.contractABI.UnpackIntoInterface(&returnVal, "getMissingDataItems", result)
	if err != nil {
		fmt.Printf("ERROR: Failed to unpack return value: %v\n", err)
		return fmt.Errorf("failed to unpack return value: %v", err)
	}
	fmt.Printf("Successfully unpacked %d missing data items\n", len(returnVal))

	for i, data := range returnVal {
		fmt.Printf("Processing item %d: DataId=%d, DataName=%s, Owner=%s, DataLen=%d\n",
			i, data.DataId.Int64(), data.DataName, data.Owner, len(data.Data))

		// Convert big.Int to string for map key
		dataIdStr := data.DataId.String()

		c.data[dataIdStr] = data.Data

		c.dependencies[dataIdStr] = t.DependencyInfo{
			Dependencies: data.Dependencies,
		}

		fmt.Printf("Stored data for DataId %s\n", dataIdStr)

		// Update our counter if this is newer
		if data.DataId.Cmp(c.dataIdCounter) > 0 {
			c.dataIdCounter = new(big.Int).Set(data.DataId)
			fmt.Printf("Updated DataIdCounter to %d\n", c.dataIdCounter.Int64())
		}
	}

	fmt.Printf("After retrieval, DataIdCounter: %d\n", c.dataIdCounter.Int64())
	fmt.Printf("Data map size: %d\n", len(c.data))
	return nil
}

func (c *ContractInteractionInterface) RetriveCurrentDataID() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	input, err := c.contractABI.Pack("getCurrentDataId")
	if err != nil {
		return fmt.Errorf("failed to pack input data: %v", err)
	}

	log.Println("Retrieving current data ID...")

	result, err := c.client.CallContract(ctx, ethereum.CallMsg{
		To:   &c.contractAddress,
		Data: input,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to call contract: %v", err)
	}

	var returnVal *big.Int
	err = c.contractABI.UnpackIntoInterface(&returnVal, "getCurrentDataId", result)
	if err != nil {
		return fmt.Errorf("failed to unpack return value: %v", err)
	}

	c.dataIdCounter = returnVal
	return nil
}

// Improve your connection and event handling
func (c *ContractInteractionInterface) Listen(ctx context.Context) (<-chan t.Message, error) {
	out := make(chan t.Message, 500)

	wsCli, err := ethclient.Dial(NetworkEndpoint)
	if err != nil {
		return nil, err
	}

	query := ethereum.FilterQuery{Addresses: []common.Address{c.contractAddress}}
	logs := make(chan types.Log, 500)
	sub, err := wsCli.SubscribeFilterLogs(ctx, query, logs)
	if err != nil {
		return nil, err
	}

	go func() {
		defer sub.Unsubscribe()
		defer close(out)

		for {
			select {
			case err := <-sub.Err():
				log.Printf("event sub error: %v", err)
				return
			case vLog := <-logs:
				go c.handleEvent(vLog, out)
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (c *ContractInteractionInterface) GetPackedInput(data, owner, dataName string, deps ...[32]byte) ([]byte, error) {
	return c.contractABI.Pack("publishMessage", []byte(data), owner, dataName, deps)
}

func (c *ContractInteractionInterface) watchNewHeads() {
	const (
		maxBatch = 100
		maxStale = 50
	)

	type pendingInfo struct {
		meta    txMeta
		addedAt uint64
	}

	staleIndex := make(map[uint64][]common.Hash)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wsCli, err := ethclient.Dial(NetworkEndpoint)
	if err != nil {
		log.Fatalf("newHeads dial: %v", err)
	}

	// subscribe to heads
	headers := make(chan *types.Header, 64)
	sub, err := wsCli.SubscribeNewHead(ctx, headers)
	if err != nil {
		log.Fatalf("newHeads subscribe: %v", err)
	}

	log.Println("newHeads watcher started")

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("newHeads subscription error: %v", err)

		case hdr := <-headers:
			number := hdr.Number.Uint64()
			c.txHashMapLock.RLock()
			pendingNow := make([]common.Hash, 0, len(c.pending))
			for h := range c.pending {
				pendingNow = append(pendingNow, h)
			}
			c.txHashMapLock.RUnlock()

			if len(pendingNow) == 0 {
				continue // nothing to do for this block
			}
			for start := 0; start < len(pendingNow); start += maxBatch {
				end := start + maxBatch
				if end > len(pendingNow) {
					end = len(pendingNow)
				}

				batch := make([]rpc.BatchElem, 0, end-start)
				for _, h := range pendingNow[start:end] {
					batch = append(batch, rpc.BatchElem{
						Method: "eth_getTransactionReceipt",
						Args:   []any{h},
						Result: new(types.Receipt),
					})
				}
				if err := c.rpcClient.BatchCallContext(ctx, batch); err != nil {
					log.Printf("batch receipt call: %v", err)
					continue
				}

				for _, be := range batch {
					r := be.Result.(*types.Receipt)
					if r == nil || r.BlockNumber == nil {
						continue
					}
					if r.Status != types.ReceiptStatusSuccessful {
						c.removePending(r.TxHash, false)
						continue
					}
					c.removePending(r.TxHash, true)
				}
			}

			staleBlock := number - maxStale
			for h := range c.pending {
				info := c.pending[h]
				staleIndex[info.meta.Seq] = append(staleIndex[info.meta.Seq], h)
			}
			if hashes := staleIndex[staleBlock]; len(hashes) > 0 {
				for _, h := range hashes {
					c.removePending(h, false)
				}
				delete(staleIndex, staleBlock)
			}
		}
	}
}

func (c *ContractInteractionInterface) removePending(h common.Hash, confirmed bool) {
	c.txHashMapLock.Lock()
	info, ok := c.pending[h]
	if ok {
		delete(c.pending, h)
	}
	c.txHashMapLock.Unlock()

	if ok && confirmed {
		newTotal := atomic.AddInt64(&c.confirmed, 1)
		LogJSON(map[string]any{
			"event":     "tx_confirmed",
			"node":      info.meta.Node,
			"seq":       info.meta.Seq,
			"confirmed": newTotal,
		})
	}
}

func (c *ContractInteractionInterface) getStoredData(id *big.Int) (t.StoredData, error) {
	var out t.StoredData

	in, err := c.contractABI.Pack("getStoredData", id)
	if err != nil {
		return out, err
	}
	ret, err := c.client.CallContract(
		context.Background(),
		ethereum.CallMsg{To: &c.contractAddress, Data: in},
		nil,
	)
	if err != nil {
		return out, err
	}

	vals, err := c.contractABI.Unpack("getStoredData", ret)
	if err != nil {
		return out, err
	}
	if len(vals) != 1 {
		return out, fmt.Errorf("expected 1 output, got %d", len(vals))
	}

	normalizeDeps := func(raw any) ([][32]byte, error) {
		if arr32, ok := raw.([][32]byte); ok {
			return arr32, nil
		}
		if bb, ok := raw.([][]byte); ok {
			deps := make([][32]byte, len(bb))
			for i, b := range bb {
				copy(deps[i][:], b)
			}
			return deps, nil
		}
		return nil, fmt.Errorf("cannot normalize deps type %T", raw)
	}

	v := vals[0]

	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Struct {
		data := rv.FieldByName("Data").Bytes()
		owner := rv.FieldByName("Owner").String()
		name := rv.FieldByName("DataName").String()
		ts := rv.FieldByName("MessageTimestamp").Interface().(*big.Int)
		did := rv.FieldByName("DataId").Interface().(*big.Int)

		rawDepVal := rv.FieldByName("Dependencies").Interface()
		deps, err := normalizeDeps(rawDepVal)
		if err != nil {
			return out, err
		}

		return t.StoredData{
			Data:             data,
			Owner:            owner,
			DataName:         name,
			MessageTimestamp: ts,
			DataId:           did,
			Dependencies:     deps,
		}, nil
	}

	tuple, ok := v.([]any)
	if !ok || len(tuple) != 6 {
		return out, fmt.Errorf("unexpected return shape: %T", v)
	}
	data := tuple[0].([]byte)
	owner := tuple[1].(string)
	name := tuple[2].(string)
	ts := tuple[3].(*big.Int)
	did := tuple[4].(*big.Int)

	deps, err := normalizeDeps(tuple[5])
	if err != nil {
		return out, err
	}

	return t.StoredData{
		Data:             data,
		Owner:            owner,
		DataName:         name,
		MessageTimestamp: ts,
		DataId:           did,
		Dependencies:     deps,
	}, nil
}

func (c *ContractInteractionInterface) handleEvent(vLog types.Log, sink chan<- t.Message) {
	if len(vLog.Topics) == 0 || vLog.Topics[0] != c.fastEventID {
		fmt.Printf("Invalid event ID: %s\n", vLog.Topics[0].Hex())
		return
	}

	if len(vLog.Topics) < 3 {
		return
	}

	dataHash := common.BytesToHash(vLog.Topics[1].Bytes())
	dataId := new(big.Int).SetBytes(vLog.Topics[2].Bytes())

	stored, err := c.getStoredData(dataId)
	if err != nil {
		if strings.Contains(err.Error(), "invalid id") {
			// retry up to 2 more times
			for i := 0; i < 2; i++ {
				time.Sleep(200 * time.Millisecond)
				stored, err = c.getStoredData(dataId)
				if err == nil {
					break
				}
				if !strings.Contains(err.Error(), "invalid id") {
					// some other error—log and bail
					log.Printf("[handleEvent] unexpected error fetching stored data: %v", err)
					return
				}
			}
			if err != nil {
				// still invalid after retries—drop the event
				log.Printf("[handleEvent] dataId %s still not indexed, skipping", dataId)
				return
			}
		} else {
			// completely different error—log and bail
			log.Printf("[handleEvent] error calling getStoredData(%s): %v", dataId, err)
			return
		}
	}
	hash := crypto.Keccak256Hash(stored.Data)
	var h32 [32]byte
	copy(h32[:], hash[:])

	if h32 != dataHash {
		log.Printf("hash mismatch id %s", dataId)
		return
	}

	deps := make([][32]byte, len(stored.Dependencies))
	copy(deps, stored.Dependencies)

	timestamp := stored.MessageTimestamp.Int64()

	msg := t.Message{
		Content:      string(stored.Data),
		Time:         time.Unix(timestamp, 0),
		DataId:       stored.DataId,
		Owner:        stored.Owner,
		DataName:     stored.DataName,
		Dependencies: deps,
	}
	sink <- msg

	orderingKey := (uint64(vLog.BlockNumber) << 32) | uint64(vLog.TxIndex)

	LogJSON(map[string]any{
		"event":        "message_received",
		"node":         stored.Owner,
		"seq":          stored.DataId,
		"ordering_key": orderingKey,
	})
}

func (c *ContractInteractionInterface) Confirmed() int64 {
	return atomic.LoadInt64(&c.confirmed)
}

func (c *ContractInteractionInterface) Sent() int64 {
	return atomic.LoadInt64(&c.sent)
}

func (c *ContractInteractionInterface) executeTransaction(
	payloadBytes []byte,
	input []byte,
	deps [][32]byte,
	owner string,
	seq uint64,
) error {
	c.txLock.Lock()
	defer c.txLock.Unlock()

	privkey, err := crypto.HexToECDSA(strings.TrimPrefix(c.privateKey, "0x"))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	chainID := big.NewInt(43113)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gasPriceCtx, gasPriceCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer gasPriceCancel()

	gasPrice, err := c.client.SuggestGasPrice(gasPriceCtx)
	if err != nil {
		gasPrice = big.NewInt(25_000_000_000)
		log.Printf("Using fallback gas price: %s", gasPrice)
	}

	capPrice := big.NewInt(100_000_000_000)
	if gasPrice.Cmp(capPrice) > 0 {
		gasPrice = capPrice
	}

	auth, _ := bind.NewKeyedTransactorWithChainID(privkey, chainID)
	nonce := c.nonceManager.GetNonce(auth.From)
	if nonce%50 == 0 {
		go c.resyncNonce(auth.From)
	}

	gasLimit := uint64(8_000_000)
	tx := types.NewTransaction(nonce, c.contractAddress, big.NewInt(0), gasLimit, gasPrice, input)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privkey)
	if err != nil {
		return fmt.Errorf("failed to sign tx: %w", err)
	}

	err = c.client.SendTransaction(ctx, signedTx)
	var finalTx *types.Transaction
	var finalNonce uint64
	if err != nil && strings.Contains(err.Error(), "replacement transaction underpriced") {

		// resync nonce
		if resyncErr := c.resyncNonce(auth.From); resyncErr != nil {
			return fmt.Errorf("resync after underpriced failed: %w", resyncErr)
		}

		// bump gas price +10%
		bumped := new(big.Int).Mul(gasPrice, big.NewInt(110))
		gasPrice = bumped.Div(bumped, big.NewInt(100))
		log.Printf("Bumping gas price to %s and retrying", gasPrice)

		// rebuild & re-sign
		finalNonce = c.nonceManager.GetNonce(auth.From)
		bumpedTx := types.NewTransaction(finalNonce, c.contractAddress, big.NewInt(0), gasLimit, gasPrice, input)
		newSigned, bumpErr := types.SignTx(bumpedTx, types.NewEIP155Signer(chainID), privkey)
		if bumpErr != nil {
			return fmt.Errorf("failed to sign bumped tx: %w", bumpErr)
		}
		if bumpErr = c.client.SendTransaction(ctx, newSigned); bumpErr != nil {
			return fmt.Errorf("failed to send bumped tx: %w", bumpErr)
		}
		log.Printf("Bumped tx sent: %s (nonce %d)", newSigned.Hash().Hex(), finalNonce)

		finalTx = newSigned
	} else if err != nil {
		return fmt.Errorf("send transaction failed: %w", err)
	} else {
		// first attempt succeeded
		finalTx = signedTx
		finalNonce = nonce
		log.Printf("Transaction sent: %s (nonce %d)", signedTx.Hash().Hex(), nonce)
	}

	atomic.AddInt64(&c.sent, 1)
	txHash := finalTx.Hash()
	c.txHashMapLock.Lock()
	c.pending[txHash] = txInfo{
		meta:         txMeta{Seq: seq, Node: owner},
		dependencies: deps,
		gasPrice:     gasPrice,
	}
	c.txHashMapLock.Unlock()

	c.receiptQueue <- txHash

	if len(input) > 4 && c.dependencyTracker != nil {
		h := crypto.Keccak256Hash(payloadBytes)
		var h32 [32]byte
		copy(h32[:], h[:])
		c.dependencyTracker.ConfirmMessage(h32)
	}

	return nil
}

func (c *ContractInteractionInterface) resyncNonce(address common.Address) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nonce, err := c.client.PendingNonceAt(ctx, address)
	if err != nil {
		return err
	}

	c.nonceManager.ResetNonce(address, nonce)
	log.Printf("Resynced nonce for %s to %d", address.Hex(), nonce)
	return nil
}
