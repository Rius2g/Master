package ContractInteraction

import (
    "context"
    "fmt"
    "log"
    "strings"
    "time"
    h "Master/helper"
    t "Master/types"
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
    dataIdCounter   uint
    PrivateKeys     map[string][]byte 
    EncryptedData   map[string][]byte
}



func Init(contractAddress, PrivateKey string)(*ContractInteractionInterface, error) {
    //init etc etc, should also ask if they want to deploy own instance or use existing one  
    client, err := ethclient.Dial(NetworkEndpoint)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to the network: %v", err)
    }

    contractABI, err := h.LoadABI()
    if err != nil {
        return nil, fmt.Errorf("failed to load contract ABI: %v", err)
    }


    continterface := ContractInteractionInterface{
        ContractAddress: common.HexToAddress(contractAddress),
        ContractABI:     contractABI,
        Client:          client,
        PrivateKey:      PrivateKey,
        dataIdCounter:   0,
        PrivateKeys:     make(map[string][]byte),
        EncryptedData:   make(map[string][]byte),
    }

    if err := continterface.RetriveCurrentDataID(); err != nil { //init to the current dataId
        return nil, fmt.Errorf("failed to retrieve current data ID: %v", err)
    }

    return &continterface, nil
}





func (c *ContractInteractionInterface) Upload(data, owner, dataName string, releaseTime int64) error {
    if(len(data) == 0) || (len(owner) == 0) || (len(dataName) == 0) || (releaseTime < time.Now().Unix()) {
        return fmt.Errorf("invalid input data") 
    }

    encryptedData, privateKeyBytes, err := h.EncryptData(data) 
    if err != nil {
        return fmt.Errorf("failed to encrypt data: %v", err)
    }

    c.PrivateKeys[dataName] = privateKeyBytes
    //hash := sha256.Sum256(encryptedData)
    
    privKey, err := crypto.HexToECDSA(strings.TrimPrefix(c.PrivateKey, "0x"))
    if err != nil {
        return fmt.Errorf("failed to parse private key: %v", err)
    }

    auth, err := bind.NewKeyedTransactorWithChainID(privKey, big.NewInt(43113))
    if err != nil {
        return fmt.Errorf("failed HexToECDSA: %v", err)
    }

    input, err := c.ContractABI.Pack("pushEncryptedData", encryptedData, owner, dataName, big.NewInt(releaseTime))
    if err != nil {
        return fmt.Errorf("failed to pack input data: %v", err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    gasPrice, err := c.Client.SuggestGasPrice(ctx)
    if err != nil {
        return fmt.Errorf("failed to suggest gas price: %v", err) 
    }
     
    gasLimit, err := c.Client.EstimateGas(ctx, ethereum.CallMsg{
        From: auth.From,
        To:   &c.ContractAddress,
        Gas:  0,
        GasPrice: gasPrice,
        Value: big.NewInt(0),
        Data: input,
    })
    if err != nil {
        return fmt.Errorf("failed to estimate gas: %v", err)
    }

    gasLimit = uint64(float64(gasLimit) * 1.2)

    nonce, err := c.Client.PendingNonceAt(ctx, auth.From)
    if err != nil {
        return fmt.Errorf("failed to retrieve nonce: %v", err)
    }

    tx := types.NewTransaction(nonce, c.ContractAddress, big.NewInt(0), gasLimit, gasPrice, input)

    signedTx, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(43113)), privKey)
    if err != nil {
        return fmt.Errorf("failed to sign transaction: %v", err)
    }


    err = c.Client.SendTransaction(ctx, signedTx)
    if err != nil {
        return fmt.Errorf("failed to send transaction: %v", err)
    }

    receipt, err := bind.WaitMined(ctx, c.Client, signedTx)
    if err != nil {
        return fmt.Errorf("failed to wait for transaction to be mined: %v", err)
    }

    log.Printf("Transaction sent: %s", receipt.TxHash.Hex())

    return nil
}

func (c *ContractInteractionInterface) RetrieveMissing() error {
    //this should ping the getMissingDataItems function in the contract and return the data that is missing
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    input, err := c.ContractABI.Pack("getMissingDataItems", big.NewInt(int64(c.dataIdCounter)))
    if err != nil {
        return fmt.Errorf("failed to pack input data: %v", err)
    }

    result, err := c.Client.CallContract(ctx, ethereum.CallMsg{
        To:   &c.ContractAddress,
        Data: input,
    }, nil) // nil for latest block
    if err != nil {
        return fmt.Errorf("failed to call contract: %v", err)
    }

    var returnVal []t.StoredData
    err = c.ContractABI.UnpackIntoInterface(&returnVal, "getMissingDataItems", result)
    if err != nil {
        return fmt.Errorf("failed to unpack return value: %v", err)
    }

    for _, data := range returnVal {
        c.EncryptedData[data.DataName] = data.EncryptedData
    }

    return nil
}

func (c *ContractInteractionInterface) RetriveCurrentDataID() error {
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    // Pack the function call data
    input, err := c.ContractABI.Pack("getCurrentDataId")
    if err != nil {
        return fmt.Errorf("failed to pack input data: %v", err)
    }

    // Make a call instead of a transaction
    result, err := c.Client.CallContract(ctx, ethereum.CallMsg{
        To:   &c.ContractAddress,
        Data: input,
    }, nil) // nil for latest block
    if err != nil {
        return fmt.Errorf("failed to call contract: %v", err)
    }

    // Unpack the result - fixing the Unpack call
    var returnVal *big.Int
    err = c.ContractABI.UnpackIntoInterface(&returnVal, "getCurrentDataId", result)
    if err != nil {
        return fmt.Errorf("failed to unpack return value: %v", err)
    }

    c.dataIdCounter = uint(returnVal.Uint64())
    return nil
}

func (c *ContractInteractionInterface) SubmitPrivateKey(dataName, owner string) error {
    if len(dataName) == 0 || len(owner) == 0 {
        return fmt.Errorf("invalid input data")
    }

    if _, ok := c.PrivateKeys[dataName]; !ok {
        return fmt.Errorf("private key not found for data name: %s", dataName) 
    }


    privKey, err := crypto.HexToECDSA(strings.TrimPrefix(c.PrivateKey, "0x"))
    if err != nil {
        return fmt.Errorf("failed to parse private key: %v", err)
    }

    auth, err := bind.NewKeyedTransactorWithChainID(privKey, big.NewInt(43113))
    if err != nil {
        return fmt.Errorf("failed HexToECDSA: %v", err)
    }

    input, err := c.ContractABI.Pack("releaseKey", c.PrivateKeys[dataName])
    if err != nil {
        return fmt.Errorf("failed to pack input data: %v", err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    gasPrice, err := c.Client.SuggestGasPrice(ctx)
    if err != nil {
        return fmt.Errorf("failed to suggest gas price: %v", err)
    }

    gasLimit, err := c.Client.EstimateGas(ctx, ethereum.CallMsg{
        From: auth.From,
        To:   &c.ContractAddress,
        Gas:  0,
        GasPrice: gasPrice,
        Value: big.NewInt(0),
        Data: input,
    })

    if err != nil {
        return fmt.Errorf("failed to estimate gas: %v", err)
    }

    gasLimit = uint64(float64(gasLimit) * 1.2)
        
    nonce, err := c.Client.PendingNonceAt(ctx, auth.From)
    if err != nil {
        return fmt.Errorf("failed to retrieve nonce: %v", err)
    }

    tx := types.NewTransaction(nonce, c.ContractAddress, big.NewInt(0), gasLimit, gasPrice, input)

    signedTx, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(43113)), privKey)
    if err != nil {
        return fmt.Errorf("failed to sign transaction: %v", err)
    }

    err = c.Client.SendTransaction(ctx, signedTx)
    if err != nil {
        return fmt.Errorf("failed to send transaction: %v", err)
    }

    receipt, err := bind.WaitMined(ctx, c.Client, signedTx)
    if err != nil {
        return fmt.Errorf("failed to wait for transaction to be mined: %v", err)
    }

    log.Printf("Transaction sent: %s", receipt.TxHash.Hex())

    return nil
}


func (c *ContractInteractionInterface) Listen(ctx context.Context) (<-chan t.Message, error) {
    messages := make(chan t.Message)
    
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
                handleEvent(c, vLog, messages)

            case <-ctx.Done():
                return
            }
        }
    }()

    return messages, nil
}

func handleEvent(c *ContractInteractionInterface, vLog types.Log, messages chan<- t.Message) {
    txHash := vLog.TxHash.Hex()
    receiveTime := time.Now()
    fmt.Printf("Received log: %s at %s\n", txHash, receiveTime)

    switch vLog.Topics[0].Hex() {
    case c.ContractABI.Events["ReleaseEncryptedData"].ID.Hex():
        var event t.ReleaseEncryptedData
        if err := c.ContractABI.UnpackIntoInterface(&event, "ReleaseEncryptedData", vLog.Data); err != nil {
            log.Printf("failed to unpack ReleaseEncryptedData event: %v", err)
            return
        }
        
        // Store the encrypted data for later decryption
        c.EncryptedData[event.DataName] = event.EncryptedData
        log.Printf("Stored encrypted data for: %s", event.DataName)

    case c.ContractABI.Events["KeyReleaseRequested"].ID.Hex():
        var event t.KeyReleaseRequested
        if err := c.ContractABI.UnpackIntoInterface(&event, "KeyReleaseRequested", vLog.Data); err != nil {
            log.Printf("failed to unpack KeyReleaseRequested event: %v", err)
            return
        }

        log.Printf("Key release requested for: %s", event.DataName)
        // Submit the private key if we have it
        if err := c.SubmitPrivateKey(event.DataName, event.Owner); err != nil {
            log.Printf("failed to submit private key: %v", err)
        }

    case c.ContractABI.Events["KeyReleased"].ID.Hex():
        var event t.KeyReleased
        if err := c.ContractABI.UnpackIntoInterface(&event, "KeyReleased", vLog.Data); err != nil {
            log.Printf("failed to unpack KeyReleased event: %v", err)
            return
        }

        log.Printf("Received private key for: %s", event.DataName)
        
        // Store the private key
        c.PrivateKeys[event.DataName] = event.PrivateKey

        // Try to decrypt if we have the encrypted data
        if encryptedData, exists := c.EncryptedData[event.DataName]; exists {
            data, err := h.DecryptData(encryptedData, event.PrivateKey)
            if err != nil {
                log.Printf("failed to decrypt data: %v", err)
                return
            }

            // Send decrypted message through channel
            msg := t.Message{
                Content:  data,
                Time:    time.Now(),
                Owner:   event.Owner,
                DataName: event.DataName,
            }

            select {
            case messages <- msg:
                log.Printf("Sent decrypted message for: %s", event.DataName)
                // Clean up stored data after successful processing
                delete(c.EncryptedData, event.DataName)
            default:
                log.Printf("Message channel full or closed, failed to send message for: %s", event.DataName)
            }
        }

    default:
        log.Printf("unknown event: %s", vLog.Topics[0].Hex())
    }
}

