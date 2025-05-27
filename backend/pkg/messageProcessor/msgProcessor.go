package messageProcessor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	c "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
	t "github.com/rius2g/Master/backend/pkg/types"
)

type MessageProcessor struct {
	contract           *c.ContractInteractionInterface
	messageChannel     <-chan t.Message
	errChannel         chan error
	ctx                context.Context
	cancel             context.CancelFunc
	ShouldReplyTo      func(t.Message) bool
	messageHashes      map[string][32]byte
	messagesToProcess  int
	messagesProcessed  int
	lastProcessedCount int
	startTime          time.Time
	lastReportTime     time.Time
	statsMutex         sync.RWMutex
}

func NewMessageProcessor(contract *c.ContractInteractionInterface) (*MessageProcessor, error) {
	ctx, cancel := context.WithCancel(context.Background())
	messageChannel, err := contract.Listen(ctx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to listen for messages: %v", err)
	}

	return &MessageProcessor{
		contract:           contract,
		messageChannel:     messageChannel,
		errChannel:         make(chan error),
		ctx:                ctx,
		cancel:             cancel,
		messageHashes:      make(map[string][32]byte),
		messagesToProcess:  0,
		messagesProcessed:  0,
		lastProcessedCount: 0,
		startTime:          time.Now(),
		lastReportTime:     time.Now(),
	}, nil
}

func (mp *MessageProcessor) StartStatsReporting(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				mp.reportStats()
			case <-mp.ctx.Done():
				return
			}
		}
	}()
}

func (mp *MessageProcessor) reportStats() {
	mp.statsMutex.Lock()
	now := time.Now()
	duration := now.Sub(mp.lastReportTime)
	totalDuration := now.Sub(mp.startTime)
	processed := mp.messagesProcessed
	mp.statsMutex.Unlock()

	c.LogJSON(map[string]any{
		"event":                 "node_processing_stats",
		"messages_processed":    processed,
		"messages_per_sec":      float64(processed) / totalDuration.Seconds(),
		"interval_msgs_per_sec": float64(processed-mp.lastProcessedCount) / duration.Seconds(),
		"uptime_seconds":        totalDuration.Seconds(),
	})

	// Update last counts for next interval
	mp.statsMutex.Lock()
	mp.lastReportTime = now
	mp.lastProcessedCount = processed
	mp.statsMutex.Unlock()
}

func (mp *MessageProcessor) Start() {
	fmt.Println("Starting message processor...")
	go func() {
		defer close(mp.errChannel)
		fmt.Println("Message processor goroutine started, waiting for messages...")
		messageCount := 0

		for {
			select {
			case msg, ok := <-mp.messageChannel:
				if !ok {
					fmt.Println("Message channel closed")
					mp.errChannel <- fmt.Errorf("message channel closed")
					return
				}

				messageCount++
				fmt.Printf("\n==== RECEIVED MESSAGE #%d ====\n", messageCount)
				fmt.Printf("DataName: %s\n", msg.DataName)
				fmt.Printf("Owner: %s\n", msg.Owner)
				fmt.Printf("Content: %s\n", msg.Content)

				if err := mp.processMessage(msg); err != nil {
					fmt.Printf("Error processing message: %v\n", err)
					mp.errChannel <- err
				}

			case <-mp.ctx.Done():
				fmt.Println("Message processor context done")
				return
			}
		}
	}()
}

func (mp *MessageProcessor) processMessage(message t.Message) error {
	log.Printf("Processing message: %v", message)

	// Store the message hash for future reference
	hash := crypto.Keccak256([]byte(message.Content))

	var depHash [32]byte
	if len(hash) != 32 {
		return fmt.Errorf("invalid hash length: %d", len(hash))
	}

	copy(depHash[:], hash)

	mp.messageHashes[message.DataName] = depHash

	// Check if we should reply
	if mp.ShouldReplyTo == nil {
		return nil
	}

	if mp.ShouldReplyTo(message) {
		dependencies := [][32]byte{depHash}

		// --- Build reply payload ---
		replyContent := fmt.Sprintf("Reply to %s", message.DataName)
		payloadBytes := []byte(replyContent)
		input, err := mp.contract.GetPackedInput(replyContent, "ReplyOwner", fmt.Sprintf("Reply-%s", message.DataName), dependencies...)
		if err != nil {
			return fmt.Errorf("failed to pack input for reply: %v", err)
		}

		err = mp.contract.Upload(payloadBytes, "ReplyOwner", fmt.Sprintf("Reply-%s", message.DataName), 0, dependencies, input)
		if err != nil {
			return fmt.Errorf("failed to upload reply: %v", err)
		}
	}

	mp.statsMutex.Lock()
	mp.messagesProcessed++
	mp.statsMutex.Unlock()

	if sendTimeStr := extractTimestampFromMessage(message); sendTimeStr != "" {
		if sendTime, err := time.Parse(time.RFC3339Nano, sendTimeStr); err == nil {
			latencyMs := time.Since(sendTime).Milliseconds()

			c.LogJSON(map[string]any{
				"event":      "message_latency",
				"data_name":  message.DataName,
				"latency_ms": latencyMs,
				"sender":     message.Owner,
			})
		}
	}
	return nil
}

func (mp *MessageProcessor) GetMessagesReceived() int {
	mp.statsMutex.RLock()
	defer mp.statsMutex.RUnlock()
	return mp.messagesProcessed
}

func extractTimestampFromMessage(message t.Message) string {
	// Try to parse the message as JSON and extract publishedAt field
	var payload map[string]any
	if err := json.Unmarshal([]byte(message.Content), &payload); err == nil {
		if timestamp, ok := payload["publishedAt"].(string); ok {
			return timestamp
		}
	}
	return ""
}

func (mp *MessageProcessor) GetDependencyHash(dataName string) ([32]byte, bool) {
	hash, ok := mp.messageHashes[dataName]
	return hash, ok
}

func (mp *MessageProcessor) Stop() {
	mp.cancel()
}

func (mp *MessageProcessor) Errors() <-chan error {
	return mp.errChannel
}
