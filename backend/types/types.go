package types 


import (
    "math/big"
)


type PushEncryptedDataEvent struct {
    EncryptedData []byte
    Owner string
    DataName string
    ReleaseTime *big.Int
}

type KeyReleasedEvent struct {
    PrivateKey []byte
    Owner string 
    DataName string 
}


type KeyReleaseRequestedEvent struct {
    Index *big.Int `abi:"index"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
}

