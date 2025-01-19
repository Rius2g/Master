package types 


import (
    "math/big"
    "time"
    "encoding/json"

)


type Contract struct {
    ABI json.RawMessage `json:"abi"`
}



type Message struct {
    Content string 
    Time time.Time 
    Owner string 
    DataName string
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
    releaseTime *big.Int `abi:"releaseTime"`
    DataId uint `abi:"dataId"`
}



type KeyReleaseRequestedEvent struct {
    Index *big.Int `abi:"index"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
    DataId uint `abi:"dataId"`
}

