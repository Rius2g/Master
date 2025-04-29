// File: pkg/ContractInteractionInterface/dependency_tracker.go

package ContractInteraction

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// DependencyTracker manages dependencies between messages
type DependencyTracker struct {
	// Maps message hash to a boolean indicating if it's confirmed
	confirmedMessages map[[32]byte]bool

	// Maps message hash to list of dependent messages waiting for it
	waitingOn map[[32]byte][]PendingMessage

	// Mutex for thread safety
	mu sync.RWMutex

	// Contract interface reference for sending queued messages
	contract *ContractInteractionInterface
}

// PendingMessage represents a message waiting for its dependencies
type PendingMessage struct {
	Data         string
	Owner        string
	DataName     string
	Dependencies [][32]byte
	RetryCount   int
	LastTry      time.Time
}

// NewDependencyTracker creates a new dependency tracker
func NewDependencyTracker(contract *ContractInteractionInterface) *DependencyTracker {
	dt := &DependencyTracker{
		confirmedMessages: make(map[[32]byte]bool),
		waitingOn:         make(map[[32]byte][]PendingMessage),
		contract:          contract,
	}

	// Start background cleanup process
	go dt.periodicCleanup()

	return dt
}

// QueueMessage adds a message to the queue, tracking its dependencies
func (dt *DependencyTracker) QueueMessage(data, owner, dataName string, dependencies [][32]byte) error {
	// If no dependencies, publish immediately
	if len(dependencies) == 0 {
		payloadBytes := []byte(data)
		input, err := dt.contract.GetPackedInput(data, owner, dataName, dependencies...)
		if err != nil {
			return fmt.Errorf("failed to pack input data: %v", err)
		}
		return dt.contract.Upload(payloadBytes, owner, dataName, 0, dependencies, input)
	}

	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Check if all dependencies are already confirmed
	allConfirmed := true
	var missingDeps [][32]byte

	for _, dep := range dependencies {
		if !dt.confirmedMessages[dep] {
			allConfirmed = false
			missingDeps = append(missingDeps, dep)
		}
	}

	if allConfirmed {
		payloadBytes := []byte(data)
		input, err := dt.contract.GetPackedInput(data, owner, dataName, dependencies...)
		if err != nil {
			return fmt.Errorf("failed to pack input data: %v", err)
		}
		return dt.contract.Upload(payloadBytes, owner, dataName, 0, dependencies, input)
	}

	// Otherwise, queue the message to wait for dependencies
	pendingMsg := PendingMessage{
		Data:         data,
		Owner:        owner,
		DataName:     dataName,
		Dependencies: dependencies,
		RetryCount:   0,
		LastTry:      time.Now(),
	}

	for _, dep := range missingDeps {
		dt.waitingOn[dep] = append(dt.waitingOn[dep], pendingMsg)
	}

	return nil
}

// IsConfirmed checks if a message is confirmed
func (dt *DependencyTracker) IsConfirmed(messageHash [32]byte) bool {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	return dt.confirmedMessages[messageHash]
}

// ConfirmMessage marks a message as confirmed and processes dependent messages
func (dt *DependencyTracker) ConfirmMessage(messageHash [32]byte) {
	fmt.Printf("Confirming message: %x\n", messageHash)
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Mark this message as confirmed
	dt.confirmedMessages[messageHash] = true

	// Get messages waiting for this dependency
	waitingMsgs, exists := dt.waitingOn[messageHash]
	if !exists {
		return
	}

	// Remove this dependency from the waiting list
	delete(dt.waitingOn, messageHash)

	for _, msg := range waitingMsgs {
		allConfirmed := true
		var stillMissingDeps [][32]byte

		for _, dep := range msg.Dependencies {
			if !dt.confirmedMessages[dep] {
				allConfirmed = false
				stillMissingDeps = append(stillMissingDeps, dep)
			}
		}

		if allConfirmed {
			// Publish immediately because all dependencies are now confirmed
			go func(m PendingMessage) {
				payloadBytes := []byte(m.Data)
				input, err := dt.contract.GetPackedInput(m.Data, m.Owner, m.DataName, m.Dependencies...)
				if err != nil {
					log.Printf("Error packing input for message %s: %v", m.DataName, err)
					return
				}
				err = dt.contract.Upload(payloadBytes, m.Owner, m.DataName, 0, m.Dependencies, input)
				if err != nil {
					log.Printf("Error publishing queued message %s: %v", m.DataName, err)
				} else {
					log.Printf("Published previously queued message: %s", m.DataName)
				}
			}(msg)
		} else {
			// Still missing other dependencies, requeue
			for _, dep := range stillMissingDeps {
				dt.waitingOn[dep] = append(dt.waitingOn[dep], msg)
			}
		}
	}
}

// periodicCleanup runs cleanup and retry logic periodically
func (dt *DependencyTracker) periodicCleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		dt.cleanupAndRetry()
	}
}

// cleanupAndRetry handles retrying queued messages and cleaning up
func (dt *DependencyTracker) cleanupAndRetry() {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	now := time.Now()
	retryThreshold := 60 * time.Second // Retry messages after 1 minute
	maxRetries := 5                    // Give up after 5 retries

	// Track dependencies to remove
	var depsToRemove [][32]byte

	for dep, messages := range dt.waitingOn {
		var remainingMessages []PendingMessage

		for _, msg := range messages {
			// Check if enough time has passed for a retry
			if now.Sub(msg.LastTry) > retryThreshold {
				if msg.RetryCount < maxRetries {
					// Try again with incremented retry count
					msg.RetryCount++
					msg.LastTry = now

					go func(m PendingMessage) {
						payloadBytes := []byte(m.Data)
						input, err := dt.contract.GetPackedInput(m.Data, m.Owner, m.DataName, m.Dependencies...)
						if err != nil {
							log.Printf("Error packing input for retry message %s: %v", m.DataName, err)
							return
						}
						err = dt.contract.Upload(payloadBytes, m.Owner, m.DataName, 0, m.Dependencies, input)
						if err != nil {
							log.Printf("Retry %d failed for message %s: %v", m.RetryCount, m.DataName, err)
						} else {
							log.Printf("Retry succeeded for message %s on attempt %d", m.DataName, m.RetryCount)
						}
					}(msg)
				} else {
					// Too many retries, give up on this message
					log.Printf("Abandoning message %s after %d retries",
						msg.DataName, msg.RetryCount)
				}
			} else {
				// Not time to retry yet, keep in queue
				remainingMessages = append(remainingMessages, msg)
			}
		}

		if len(remainingMessages) == 0 {
			// No messages left waiting on this dependency
			depsToRemove = append(depsToRemove, dep)
		} else {
			dt.waitingOn[dep] = remainingMessages
		}
	}

	// Remove empty dependency entries
	for _, dep := range depsToRemove {
		delete(dt.waitingOn, dep)
	}
}
