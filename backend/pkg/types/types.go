// Updated types.go
package types

import (
	"encoding/json"
	"math/big"
	"time"
)

type StoredData struct {
	Data             []byte     `abi:"data"`
	Owner            string     `abi:"owner"`
	DataName         string     `abi:"dataName"`
	MessageTimestamp *big.Int   `abi:"messageTimestamp"`
	DataId           *big.Int   `abi:"dataId"`
	Dependencies     [][32]byte `abi:"dependencies"`
}

type Contract struct {
	ABI json.RawMessage `json:"abi"`
}

type Message struct {
	Content      string
	Time         time.Time
	DataId       *big.Int
	Owner        string
	DataName     string
	Dependencies [][32]byte
}

type DependencyInfo struct {
	Dependencies [][32]byte
}

type BroadcastMessage struct {
	Data             []byte     `abi:"data"`
	Owner            string     `abi:"owner"`
	DataName         string     `abi:"dataName"`
	MessageTimestamp *big.Int   `abi:"messageTimestamp"`
	DataId           *big.Int   `abi:"dataId"`
	Dependencies     [][32]byte `abi:"dependencies"`
}
