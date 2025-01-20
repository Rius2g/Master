package types 


import (
    "math/big"
    "time"
    "encoding/json"
    "github.com/ethereum/go-ethereum/common"

)


type Contract struct {
    ABI json.RawMessage `json:"abi"`
}


type VectorClock struct {
    Process common.Address `abi:"process"`
    TimeStamp *big.Int `abi:"timeStamp"`
}



type Message struct {
    Content string 
    Time time.Time 
    Owner string 
    DataName string
    VectorClocks []VectorClock 
    Dependencies [][]byte
}


type KeyReleaseRequested struct {
    Index *big.Int `abi:"index"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
}


type PushEncryptedDataEvent struct {
    EncryptedData []byte
    Owner string
    DataName string
    ReleaseTime *big.Int
}

type StoredData struct {
    EncryptedData []byte `abi:"encryptedData"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
    ReleaseTime *big.Int `abi:"releaseTime"`
    KeyReleased bool `abi:"keyReleased"`
    ReleasePhase *big.Int `abi:"releasePhase"`
    DataId uint `abi:"dataId"`
    VectorClocks []VectorClock `abi:"vectorClocks"`
    Dependencies [][]byte `abi:"dependencies"`
    SecurityLevel *big.Int `abi:"securityLevel"`
}


type DependencyInfo struct {
    VectorClocks []VectorClock
    Dependencies [][]byte
}



type KeyReleasedEvent struct {
    PrivateKey []byte
    Owner string 
    DataName string 
    DataId uint
}

type ReleaseEncryptedData struct {
    EncryptedData []byte `abi:"encryptedData"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
    ReleaseTime *big.Int `abi:"releaseTime"`
    DataId uint `abi:"dataId"`
    Dependencies [][]byte `abi:"dependencies"`
    VectorClocks []VectorClock `abi:"vectorClocks"`
}



type KeyReleaseRequestedEvent struct {
    Index *big.Int `abi:"index"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
    DataId uint `abi:"dataId"`
}

