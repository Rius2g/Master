package ContractInteraction

import (
    "context"
    "fmt"
    "log"
    "strings"
    "sync"
    "time"
    h "github.com/rius2g/Master/backend/helper"
    t "github.com/rius2g/Master/backend/pkg/types"
    "github.com/ethereum/go-ethereum/accounts/abi"

    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/ethclient"
    "github.com/ethereum/go-ethereum/accounts/abi/bind"
    "github.com/ethereum/go-ethereum/crypto"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum"

    "math/big"
)

var (
    PrivateKey string
    client     *ethclient.Client
    contractABI abi.ABI
    NetworkEndpoint string = "wss://api.avax-test.network/ext/bc/C/ws"
)

type ContractInteractionInterface struct {
    ContractAddress common.Address 
    ContractABI     abi.ABI
    Client          *ethclient.Client
    PrivateKey      string 
    DataIdCounter   *big.Int 
    Data            map[*big.Int][]byte
    ProcessID       common.Address
    Dependencies    map[*big.Int]t.DependencyInfo
    VectorClock     map[string]*big.Int
    nonceManager   *NonceManager
    txLock      sync.Mutex
}

func Init(contractAddress, privateKey string)(*ContractInteractionInterface, error) {
    client, err := ethclient.Dial(NetworkEndpoint)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to the network: %v", err)
    }

    contractABI, err := h.LoadABI()
    if err != nil {
        return nil, fmt.Errorf("failed to load contract ABI: %v", err)
    }

    privKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKey, "0x"))
    if err != nil {
        return nil, fmt.Errorf("failed to parse private key: %v", err)
    }

    processAddress := crypto.PubkeyToAddress(privKey.PublicKey)

    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    initialNonce, err := client.PendingNonceAt(ctx, processAddress)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve initial nonce: %v", err) 
    }

    nonceManager := NewNonceManager(processAddress, initialNonce) 



    contInterface := ContractInteractionInterface{
        ContractAddress: common.HexToAddress(contractAddress),
        ContractABI:     contractABI,
        Client:          client,
        PrivateKey:      privateKey,
        DataIdCounter:   big.NewInt(0),
        Data:            make(map[*big.Int][]byte),
        ProcessID:       processAddress,
        Dependencies:    make(map[*big.Int]t.DependencyInfo),
        VectorClock:     make(map[string]*big.Int),
        nonceManager:   nonceManager,
    }

    if err := contInterface.RetriveCurrentDataID(); err != nil {
        return nil, fmt.Errorf("failed to retrieve current data ID: %v", err)
    }

    return &contInterface, nil
}

func (c *ContractInteractionInterface) Upload(data, owner, dataName string, dependencies [][32]byte) error {
    if len(data) == 0 || len(owner) == 0 || len(dataName) == 0 {
        return fmt.Errorf("invalid input data")
    }

    // Validate dependencies
    if len(dependencies) > 0 {
        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()

        input, err := c.ContractABI.Pack("getDependencyTimestamps", dependencies)
        if err != nil {
            return fmt.Errorf("failed to pack input data: %v", err)
        }

        result, err := c.Client.CallContract(ctx, ethereum.CallMsg{
            To:   &c.ContractAddress,
            Data: input,
        }, nil)
        if err != nil {
            return fmt.Errorf("failed to call contract: %v", err)
        }

        var depTimestamps []*big.Int
        err = c.ContractABI.UnpackIntoInterface(&depTimestamps, "getDependencyTimestamps", result)
        if err != nil {
            return fmt.Errorf("failed to unpack return value: %v", err)
        }

        for i, timestamp := range depTimestamps {
            if timestamp.Int64() == 0 {
                return fmt.Errorf("dependency %d not found", i)
            }
        }
    }

    // Pack the inputs for the message data
    input, err := c.ContractABI.Pack(
        "publishMessage",
        []byte(data),
        owner,
        dataName,
        dependencies,
    )
    if err != nil {
        return fmt.Errorf("failed to pack input data: %v", err)
    }

    if err := c.executeTransaction(input); err != nil {
        return fmt.Errorf("failed to execute transaction: %v", err)
    }

    log.Printf("Message published successfully: %s", dataName)
    return nil
}

func (c *ContractInteractionInterface) RetrieveMissing() error {
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    input, err := c.ContractABI.Pack("getMissingDataItems", c.DataIdCounter)
    if err != nil {
        return fmt.Errorf("failed to pack input data: %v", err)
    }

    result, err := c.Client.CallContract(ctx, ethereum.CallMsg{
        To:   &c.ContractAddress,
        Data: input,
    }, nil)
    if err != nil {
        return fmt.Errorf("failed to call contract: %v", err)
    }

    var returnVal []t.StoredData
    err = c.ContractABI.UnpackIntoInterface(&returnVal, "getMissingDataItems", result)
    if err != nil {
        return fmt.Errorf("failed to unpack return value: %v", err)
    }

    for _, data := range returnVal {
        c.Data[data.DataId] = data.Data
        
        c.Dependencies[data.DataId] = t.DependencyInfo{
            VectorClocks: data.VectorClocks,
            Dependencies: data.Dependencies,
        }
    }

    return nil
}

func (c *ContractInteractionInterface) RetriveCurrentDataID() error {
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    input, err := c.ContractABI.Pack("getCurrentDataId")
    if err != nil {
        return fmt.Errorf("failed to pack input data: %v", err)
    }

    result, err := c.Client.CallContract(ctx, ethereum.CallMsg{
        To:   &c.ContractAddress,
        Data: input,
    }, nil)
    if err != nil {
        return fmt.Errorf("failed to call contract: %v", err)
    }

    var returnVal *big.Int
    err = c.ContractABI.UnpackIntoInterface(&returnVal, "getCurrentDataId", result)
    if err != nil {
        return fmt.Errorf("failed to unpack return value: %v", err)
    }

    c.DataIdCounter = returnVal
    return nil
}

func (c *ContractInteractionInterface) Listen(ctx context.Context) (<-chan t.Message, error) {
    messages := make(chan t.Message, 50)
    
    client, err := ethclient.Dial(NetworkEndpoint)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to the network: %v", err)
    }

    currentBlock, err := client.BlockByNumber(ctx, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve current block: %v", err)
    }

    query := ethereum.FilterQuery{
        Addresses: []common.Address{c.ContractAddress},
        FromBlock: currentBlock.Number(),
    }

    logs := make(chan types.Log)
    sub, err := client.SubscribeFilterLogs(ctx, query, logs)
    if err != nil {
        return nil, fmt.Errorf("failed to subscribe to logs: %v", err)
    }

    go func() {
        defer sub.Unsubscribe()
        defer close(messages)

        for {
            select {
            case err := <-sub.Err():
                log.Printf("subscription error: %v", err)
                sub.Unsubscribe()

                currentBlock, err := client.BlockNumber(ctx)
                if err != nil {
                    log.Printf("failed to retrieve current block: %v", err)
                    return
                }

                query.FromBlock = big.NewInt(int64(currentBlock))
                sub, err = client.SubscribeFilterLogs(ctx, query, logs)
                if err != nil {
                    log.Printf("failed to resubscribe to logs: %v", err)
                    return
                }

            case vLog := <-logs:
                c.handleEvent(vLog, messages)

            case <-ctx.Done():
                return
            }
        }
    }()

    return messages, nil
}

func (c *ContractInteractionInterface) handleEvent(vLog types.Log, messages chan<- t.Message) {
    txHash := vLog.TxHash.Hex()
    receiveTime := time.Now()
    fmt.Printf("Received log: %s at %s\n", txHash, receiveTime)

    switch vLog.Topics[0].Hex() {
    case c.ContractABI.Events["BroadcastMessage"].ID.Hex():
        var event t.BroadcastMessage
        if err := c.ContractABI.UnpackIntoInterface(&event, "BroadcastMessage", vLog.Data); err != nil {
            log.Printf("failed to unpack BroadcastMessage event: %v", err)
            return
        }

        if event.DataId.Int64() > c.DataIdCounter.Int64() {
            // Missing entry, ask for them
            c.RetrieveMissing()
        }

        c.Dependencies[event.DataId] = t.DependencyInfo{ 
            VectorClocks: event.VectorClocks,
            Dependencies: event.Dependencies,
        }

        // Update vector clock
        for _, vc := range event.VectorClocks {
            processStr := vc.Process.String()
            if current, exists := c.VectorClock[processStr]; !exists || current.Cmp(vc.TimeStamp) < 0 {
                c.VectorClock[processStr] = vc.TimeStamp
            }
        }

        // Store the data
        c.Data[event.DataId] = event.Data
        log.Printf("Stored message data for: %s", event.DataName)

        // Create and send message
        msg := t.Message{
            Content:      string(event.Data),
            Time:         time.Unix(event.MessageTimestamp.Int64(), 0),
            Owner:        event.Owner,
            DataName:     event.DataName,
            VectorClocks: event.VectorClocks,
            Dependencies: event.Dependencies,
        }

        select {
        case messages <- msg:
            log.Printf("Sent message for: %s", event.DataName)
        default:
            log.Printf("Message channel full or closed, failed to send message for: %s", event.DataName)
        }

    default:
        log.Printf("unknown event: %s", vLog.Topics[0].Hex())
    }
}

// Optimize the executeTransaction function in ContractInteractionInterface
func (c *ContractInteractionInterface) executeTransaction(input []byte) error {
    // Lock to synchronize transaction submissions
    c.txLock.Lock()
    defer c.txLock.Unlock()
    
    privkey, err := crypto.HexToECDSA(strings.TrimPrefix(c.PrivateKey, "0x"))
    if err != nil {
        return fmt.Errorf("failed to parse private key: %v", err)
    }

    auth, err := bind.NewKeyedTransactorWithChainID(privkey, big.NewInt(43113))
    if err != nil {
        return fmt.Errorf("failed HexToECDSA: %v", err)
    }

    // Use a longer context timeout
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    // Get gas price with a separate context
    gasPriceCtx, gasPriceCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer gasPriceCancel()
    
    gasPrice, err := c.Client.SuggestGasPrice(gasPriceCtx)
    if err != nil {
        // If we can't get gas price, use a fallback value
        gasPrice = big.NewInt(25000000000) // 25 Gwei
        log.Printf("Using fallback gas price: %s", gasPrice.String())
    } else {
        // Increase by 50% instead of 300% to save on costs while still being prioritized
        gasPrice = new(big.Int).Mul(gasPrice, big.NewInt(150))
        gasPrice = new(big.Int).Div(gasPrice, big.NewInt(100))
    }

    // Use fixed gas limit to avoid estimation
    gasLimit := uint64(300000)

    // Get nonce from local manager instead of RPC call
    nonce := c.nonceManager.GetNonce(auth.From)
    
    // Every 50 transactions, try to resync our nonce with the network
    if nonce%50 == 0 {
        go c.resyncNonce(auth.From)
    }

    tx := types.NewTransaction(nonce, c.ContractAddress, big.NewInt(0), gasLimit, gasPrice, input)

    signedTx, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(43113)), privkey)
    if err != nil {
        return fmt.Errorf("failed to sign transaction: %v", err)
    }

    err = c.Client.SendTransaction(ctx, signedTx)
    if err != nil {
        if strings.Contains(err.Error(), "nonce too low") || 
           strings.Contains(err.Error(), "replacement transaction underpriced") {
            // If nonce error, try to recover by fetching from network
            log.Printf("Nonce error detected: %v, attempting to resync nonce", err)
            if resyncErr := c.resyncNonce(auth.From); resyncErr == nil {
                // Retry with new nonce
                newNonce := c.nonceManager.GetNonce(auth.From)
                newTx := types.NewTransaction(newNonce, c.ContractAddress, big.NewInt(0), gasLimit, gasPrice, input)
                newSignedTx, signErr := types.SignTx(newTx, types.NewEIP155Signer(big.NewInt(43113)), privkey)
                if signErr != nil {
                    return fmt.Errorf("failed to sign retry transaction: %v", signErr)
                }
                
                if retryErr := c.Client.SendTransaction(ctx, newSignedTx); retryErr != nil {
                    return fmt.Errorf("failed to send retry transaction: %v", retryErr)
                }
                log.Printf("Retry transaction sent successfully with nonce %d: %s", 
                    newNonce, newSignedTx.Hash().Hex())
                return nil
            }
        }
        return fmt.Errorf("failed to send transaction: %v", err)
    }

    log.Printf("Transaction sent: %s with nonce %d", signedTx.Hash().Hex(), nonce)
    return nil
}


func (c *ContractInteractionInterface) resyncNonce(address common.Address) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    nonce, err := c.Client.PendingNonceAt(ctx, address)
    if err != nil {
        return err
    }
    
    c.nonceManager.ResetNonce(address, nonce)
    log.Printf("Resynced nonce for %s to %d", address.Hex(), nonce)
    return nil
}
