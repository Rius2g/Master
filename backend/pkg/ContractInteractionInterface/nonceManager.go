package ContractInteraction

import (
	"github.com/ethereum/go-ethereum/common"
	"sync"
)

// NonceManager tracks nonces locally to avoid excessive RPC calls
type NonceManager struct {
	nonces       map[common.Address]uint64
	lock         sync.Mutex
	initialNonce uint64
}

// NewNonceManager creates a new nonce manager
func NewNonceManager(address common.Address, initialNonce uint64) *NonceManager {
	nm := &NonceManager{
		nonces:       make(map[common.Address]uint64),
		initialNonce: initialNonce,
	}
	nm.nonces[address] = initialNonce
	return nm
}

// GetNonce gets the next nonce for an address and increments it
func (nm *NonceManager) GetNonce(address common.Address) uint64 {
	nm.lock.Lock()
	defer nm.lock.Unlock()

	nonce, exists := nm.nonces[address]
	if !exists {
		nm.nonces[address] = nm.initialNonce
		return nm.initialNonce
	}

	currentNonce := nonce
	nm.nonces[address] = nonce + 1
	return currentNonce
}

// ResetNonce resets the nonce for an address to a specific value
func (nm *NonceManager) ResetNonce(address common.Address, nonce uint64) {
	nm.lock.Lock()
	defer nm.lock.Unlock()
	nm.nonces[address] = nonce
}

// PeekNonce gets the current nonce without incrementing
func (nm *NonceManager) PeekNonce(address common.Address) uint64 {
	nm.lock.Lock()
	defer nm.lock.Unlock()

	nonce, exists := nm.nonces[address]
	if !exists {
		return nm.initialNonce
	}
	return nonce
}
