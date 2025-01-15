package types 


import (
    "math/big"
)


type KeyReleased struct {
    PrivateKey []byte `abi:"privateKey"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
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
    DataId *big.Int `abi:"dataId"`
}

type KeyReleasedEvent struct {
    PrivateKey []byte
    Owner string 
    DataName string 
}

type ReleaseEncryptedData struct {
    EncryptedData []byte `abi:"encryptedData"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
    releaseTime *big.Int `abi:"releaseTime"`
}



type KeyReleaseRequestedEvent struct {
    Index *big.Int `abi:"index"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
}

