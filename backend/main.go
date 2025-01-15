package main


import (
    "fmt"
    "encoding/json"
    "os"
    "log"
    "strings"
    "context"
    "math/big" 

    "time"
    h "Master/helper"
    t "Master/types"


   "github.com/ethereum/go-ethereum"
    "github.com/ethereum/go-ethereum/accounts/abi"
    "github.com/ethereum/go-ethereum/accounts/abi/bind"
    "github.com/ethereum/go-ethereum/common"
 //   "github.com/ethereum/go-ethereum/common/hexutil"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/crypto" 
    "github.com/ethereum/go-ethereum/ethclient"
    "github.com/joho/godotenv"
)


type Contract struct {
    ABI json.RawMessage `json:"abi"`
}

var (
    PrivateKey string
    client     *ethclient.Client
    contractABI abi.ABI
)


func LoadABI() (abi.ABI, error) {
    filePath := "TwoPhaseCommit.json"
    abiBytes, err := os.ReadFile(filePath)
    if err != nil {
        return abi.ABI{}, fmt.Errorf("failed to read ABI file: %v", err)
    }

    var contract Contract
    err = json.Unmarshal(abiBytes, &contract)
    if err != nil {
        return abi.ABI{}, fmt.Errorf("failed to unmarshal contract JSON: %v", err)
    }

    parsedABI, err := abi.JSON(strings.NewReader(string(contract.ABI)))
    if err != nil {
        return abi.ABI{}, fmt.Errorf("failed to parse ABI JSON: %v", err)
    }

    return parsedABI, nil
}

//this is the package so we can import these functions in other projects
func main() {
    if err := godotenv.Load(); err != nil {
        log.Fatal("Error loading .env file") 
    }
    fmt.Println("Starting the application...")

    contractABI, err := LoadABI()
    if err != nil {
        log.Fatalf("failed to load contract ABI: %v", err)
    }

    fmt.Println("ABI loaded successfully")
    fmt.Println(contractABI)

}


type ContractInteractionInterface struct {
    ContractAddress common.Address 
    ContractABI     abi.ABI
    Client          *ethclient.Client
    PrivateKey      string 
    dataIdCounter   uint
    PrivateKeys     map[string][]byte 
    EncryptedData   map[string][]byte
}

func Init(contractAddress, PrivateKey string)(ContractInteractionInterface, error) {
    //init etc etc, should also ask if they want to deploy own instance or use existing one  



    return ContractInteractionInterface{}, nil
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

func (c *ContractInteractionInterface) Listen() error {
    return nil
}



