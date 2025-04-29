package ContractInteraction

import (
	"context"
	"fmt"
	"log"
	"math/big"
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
	Seq  int64
	Node string
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

	pending   map[common.Hash]txMeta // pending transactions
	confirmed int64
}

func Init(contractAddress, privateKey string) (*ContractInteractionInterface, error) {
	log.Println("Initializing contract interaction…")

	// RPC + WS
	rpcCli, err := rpc.Dial("https://api.avax-test.network/ext/bc/C/rpc")
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
		pending:      make(map[common.Hash]txMeta),
		confirmed:    0,
	}
	ci.dependencyTracker = NewDependencyTracker(ci)

	go ci.pollReceipts() // background confirmation handler

	if err := ci.RetriveCurrentDataID(); err != nil {
		return nil, err
	}
	return ci, nil
}

func (c *ContractInteractionInterface) Upload(payloadBytes []byte, owner, dataName string, seq int64, dependencies [][32]byte, input []byte) error {
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
	if err := c.executeTransaction(payloadBytes, input, owner, seq); err != nil {
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
	log.Println("Input data:", input)
	log.Println("Contract address:", c.contractAddress.Hex())
	log.Println("Private key:", c.privateKey)
	log.Println("Client:", c.client)
	log.Println("Context:", ctx)

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

func (c *ContractInteractionInterface) getStoredData(id *big.Int) (t.StoredData, error) {
	var out t.StoredData
	in, err := c.contractABI.Pack("getStoredData", id)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ret, err := c.client.CallContract(ctx, ethereum.CallMsg{To: &c.contractAddress, Data: in}, nil)
	if err != nil {
		return out, err
	}
	if err := c.contractABI.UnpackIntoInterface(&out, "getStoredData", ret); err != nil {
		return out, err
	}
	return out, nil
}

func (c *ContractInteractionInterface) handleEvent(vLog types.Log, sink chan<- t.Message) {
	if len(vLog.Topics) == 0 || vLog.Topics[0] != c.fastEventID {
		return
	}
	var ev struct {
		DataHash [32]byte
		DataId   *big.Int
	}
	if err := c.contractABI.UnpackIntoInterface(&ev, "FastBroadcast", vLog.Data); err != nil {
		log.Printf("unpack FastBroadcast: %v", err)
		return
	}

	stored, err := c.getStoredData(ev.DataId)
	if err != nil {
		log.Printf("getStoredData: %v", err)
		return
	}

	hash := crypto.Keccak256Hash(stored.Data)
	var h32 [32]byte
	copy(h32[:], hash[:])

	if h32 != ev.DataHash {
		log.Printf("hash mismatch id %s", ev.DataId)
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
}
func (c *ContractInteractionInterface) pollReceipts() {
	for {
		// Build a batch of up to 100 tx hashes
		elems := make([]rpc.BatchElem, 0, 100)
		for i := 0; i < cap(elems); i++ {
			select {
			case h := <-c.receiptQueue:
				elems = append(elems, rpc.BatchElem{
					Method: "eth_getTransactionReceipt",
					Args:   []any{h},
					Result: &types.Receipt{},
				})
			default:
				i = cap(elems) // break outer for
			}
		}
		if len(elems) == 0 {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err := c.rpcClient.BatchCall(elems); err != nil {
			log.Printf("batch receipts: %v", err)

		}
		for _, be := range elems {
			r, ok := be.Result.(*types.Receipt)
			if !ok || r == nil || r.Status != types.ReceiptStatusSuccessful {
				continue
			}
			h := r.TxHash

			c.txHashMapLock.Lock()
			meta, present := c.pending[h]
			if present {
				delete(c.pending, h)
			}
			c.txHashMapLock.Unlock()

			if present {
				atomic.AddInt64(&c.confirmed, 1)

				LogJSON(map[string]any{ // use your helper
					"event": "message_published",
					"node":  meta.Node,
					"seq":   meta.Seq,
				})
			}
		}

		// Optional: handle receipts here (update cost metrics, etc.)
	}
}

func (c *ContractInteractionInterface) Confirmed() int64 {
	return atomic.LoadInt64(&c.confirmed)
}

// Modified executeTransaction function to wait for transaction confirmation
func (c *ContractInteractionInterface) executeTransaction(payloadBytes []byte, input []byte, owner string, seq int64) error {
	// Lock to synchronize transaction submissions
	c.txLock.Lock()
	defer c.txLock.Unlock()

	privkey, err := crypto.HexToECDSA(strings.TrimPrefix(c.privateKey, "0x"))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privkey, big.NewInt(43113))
	if err != nil {
		return fmt.Errorf("failed HexToECDSA: %v", err)
	}

	// Use a shorter context timeout for throughput
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get gas price with a separate context
	gasPriceCtx, gasPriceCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer gasPriceCancel()

	gasPrice, err := c.client.SuggestGasPrice(gasPriceCtx)
	if err != nil {
		gasPrice = big.NewInt(25000000000) // 25 Gwei
		log.Printf("Using fallback gas price: %s", gasPrice.String())
	}

	// Add a cap to prevent extremely high gas prices
	if gasPrice.Cmp(big.NewInt(100000000000)) > 0 { // If more than 100 Gwei
		gasPrice = big.NewInt(100000000000) // 100 Gwei
		log.Printf("Gas price suspiciously high, capping at 100 Gwei")
	}

	// Use reasonable gas limit to avoid estimation
	gasLimit := uint64(8000000)
	// Get nonce from local manager instead of RPC call
	nonce := c.nonceManager.GetNonce(auth.From)

	// Every 50 transactions, try to resync our nonce with the network
	if nonce%50 == 0 {
		go c.resyncNonce(auth.From)
	}

	tx := types.NewTransaction(nonce, c.contractAddress, big.NewInt(0), gasLimit, gasPrice, input)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(43113)), privkey)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}

	err = c.client.SendTransaction(ctx, signedTx)
	if err != nil {
		if strings.Contains(err.Error(), "nonce too low") ||
			strings.Contains(err.Error(), "replacement transaction underpriced") {
			// If nonce error, try to recover by fetching from network
			log.Printf("Nonce error detected: %v, attempting to resync nonce", err)
			if resyncErr := c.resyncNonce(auth.From); resyncErr == nil {
				// Retry with new nonce
				newNonce := c.nonceManager.GetNonce(auth.From)
				newTx := types.NewTransaction(newNonce, c.contractAddress, big.NewInt(0), gasLimit, gasPrice, input)
				newSignedTx, signErr := types.SignTx(newTx, types.NewEIP155Signer(big.NewInt(43113)), privkey)
				if signErr != nil {
					return fmt.Errorf("failed to sign retry transaction: %v", signErr)
				}

				if retryErr := c.client.SendTransaction(ctx, newSignedTx); retryErr != nil {
					return fmt.Errorf("failed to send retry transaction: %v", retryErr)
				}

				txHash := newSignedTx.Hash().Hex()
				log.Printf("Retry transaction sent successfully with nonce %d: %s", newNonce, txHash)

				return nil
			}
		}
		return fmt.Errorf("failed to send transaction: %v", err)
	}
	meta := txMeta{Seq: seq, Node: owner}
	c.txHashMapLock.Lock()
	c.pending[signedTx.Hash()] = meta
	c.txHashMapLock.Unlock()
	c.receiptQueue <- signedTx.Hash() // enqueue for async confirmation

	txHash := signedTx.Hash().Hex()
	log.Printf("Transaction sent: %s with nonce %d", txHash, nonce)

	// Store message hash for dependency tracking as before
	if len(input) > 4 {
		dataPart := input[4:]
		if len(dataPart) > 32 {
			messageHash := crypto.Keccak256Hash(payloadBytes)
			var hash32 [32]byte
			copy(hash32[:], messageHash[:])

			c.txHashMapLock.Lock()
			c.txHashMap[txHash] = hash32
			c.txHashMapLock.Unlock()

			log.Printf("Stored transaction hash %s with message hash %x", txHash, hash32)
			if c.dependencyTracker != nil {
				c.dependencyTracker.ConfirmMessage(hash32)
				log.Printf("Confirmed message hash %x", hash32)
			}

		}
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
