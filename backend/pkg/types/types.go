// Updated types.go
package types

import (
    "math/big"
    "encoding/json"
    "time"
    "github.com/ethereum/go-ethereum/common"
)

type StoredData struct {
    Data []byte `abi:"data"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
    MessageTimestamp *big.Int `abi:"messageTimestamp"`
    DataId uint `abi:"dataId"`
    VectorClocks []VectorClock `abi:"vectorClocks"`
    Dependencies [][]byte `abi:"dependencies"`
}

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

type DependencyInfo struct {
    VectorClocks []VectorClock
    Dependencies [][]byte
}

type BroadcastMessage struct {
    Data []byte `abi:"data"`
    Owner string `abi:"owner"`
    DataName string `abi:"dataName"`
    MessageTimestamp *big.Int `abi:"messageTimestamp"`
    DataId uint `abi:"dataId"`
    VectorClocks []VectorClock `abi:"vectorClocks"`
    Dependencies [][]byte `abi:"dependencies"`
}
