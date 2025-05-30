package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	contract "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
	msgProcessor "github.com/rius2g/Master/backend/pkg/messageProcessor"
)

var (
	instanceID      = flag.String("instance", "node1", "Unique instance identifier")
	stressMode      = flag.Bool("stress", false, "Enable stress test mode (continuous publishing of messages)")
	publishInterval = flag.Duration("publishInterval", 1*time.Millisecond, "Interval between publishing messages")
	envFile         = flag.String("env-file", ".env.high_throughput", "Path to environment file")
	concurrency     = flag.Int("concurrency", 1, "Number of concurrent message publishers")
	numberOfNodes   = flag.Int("numberOfNodes", 1, "Number of nodes in the network")
)

var (
	runStartTime time.Time  // Track the start time of the experiment
	logFile      *os.File   // Global handle for the log file
	publishMutex sync.Mutex // Mutex to protect the messages published counter
)

func startPublisher(ctx context.Context, contractClient *contract.ContractInteractionInterface, pubID int) {
	seq := uint64(1)
	var lastDep [32]byte

	ticker := time.NewTicker(*publishInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Build payload
			payload := map[string]any{
				"instance":    *instanceID,
				"seq":         seq,
				"publisherID": pubID,
				"publishedAt": time.Now().Format(time.RFC3339Nano),
			}
			payloadBytes, _ := json.Marshal(payload)

			// Dependencies
			var deps [][32]byte
			if seq > 1 {
				deps = append(deps, lastDep)
			}

			dataName := fmt.Sprintf("%s-%d-%d", *instanceID, pubID, seq)
			input, err := contractClient.GetPackedInput(string(payloadBytes), *instanceID, dataName, deps...)
			if err != nil {
				log.Printf("[pub %d] pack error: %v", pubID, err)
				continue
			}

			// Precompute message hash
			newHash := crypto.Keccak256Hash(payloadBytes)
			var fixedHash [32]byte
			copy(fixedHash[:], newHash[:])

			// Upload
			if err := contractClient.Upload(payloadBytes, *instanceID, dataName, seq, deps, input); err != nil {
				continue
			}

			contract.LogJSON(map[string]any{
				"event":        "message_published",
				"node":         *instanceID,
				"seq":          seq,
				"timestamp":    time.Now().UTC().Format(time.RFC3339),
				"dependencies": encodeDeps(deps),
			})

			lastDep = fixedHash
			seq++
		}
	}
}

func encodeDeps(deps [][32]byte) []string {
	out := make([]string, len(deps))
	for i, dep := range deps {
		out[i] = hex.EncodeToString(dep[:])
	}
	return out
}

// Setup logging to a file
func setupLogging() {
	var err error
	numberOfNodesStr := fmt.Sprintf("%d", *numberOfNodes)
	logFilePath := fmt.Sprintf("logs/experiment_results_%s.log", numberOfNodesStr)
	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	// Redirect global logger output to the log file
	log.SetOutput(logFile)
}

// Close the log file on exit
func cleanupLogging() {
	if logFile != nil {
		logFile.Close()
	}
}

// main – modified to include results logging to file
func main() {
	flag.Parse()
	runStartTime = time.Now()

	contract.LogJSON(map[string]any{
		"event":     "application_start",
		"node":      *instanceID,
		"timestamp": runStartTime.UTC().Format(time.RFC3339),
	})

	// Set GOMAXPROCS and setup logging as usual.
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	setupLogging()
	defer cleanupLogging()

	// Load environment settings (including RPC URL, PRIVATE_KEY, etc.)
	if err := godotenv.Load(*envFile); err != nil {
		contract.LogJSON(map[string]any{"event": "env_load_error", "error": err.Error()})
		if *envFile != ".env" {
			if err := godotenv.Load(); err != nil {
				log.Fatalf("Warning: could not load default .env file: %v", err)
			}
		}
	}

	// Initialize your contract as before.
	contractAddress := "0x215acfdA10D7877C4486ca7bB89Db093bfF15Fe0"
	privateKeyHex := os.Getenv("PRIVATE_KEY")
	c, err := contract.Init(contractAddress, privateKeyHex)
	if err != nil {
		contract.LogJSON(map[string]any{"event": "contract_init_error", "error": err.Error()})
		log.Fatalf("failed to initialize contract: %v", err)
	}
	contract.LogJSON(map[string]any{"event": "contract_initialized", "node": *instanceID})

	// Initialize and start your message processor.
	mp, err := msgProcessor.NewMessageProcessor(c)
	if err != nil {
		contract.LogJSON(map[string]any{"event": "message_processor_init_error", "error": err.Error()})
		log.Fatalf("failed to create message processor: %v", err)
	}
	contract.LogJSON(map[string]any{"event": "message_processor_initialized", "node": *instanceID})

	mp.StartStatsReporting(10 * time.Second) // Report every 10 seconds

	mp.Start()
	defer mp.Stop()

	go func() {
		for err := range mp.Errors() {
			contract.LogJSON(map[string]any{"event": "processor_error", "node": *instanceID, "error": err.Error()})
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *stressMode {
		for i := 0; i < *concurrency; i++ {
			go startPublisher(ctx, c, i+1)
		}
	}

	go func() {
		statsTicker := time.NewTicker(10 * time.Second)
		defer statsTicker.Stop()
		var lastCount int64 = 0
		lastTime := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-statsTicker.C:
				publishMutex.Lock()
				currentCount := c.Confirmed()
				sent := c.Sent()
				fmt.Printf("Current count: %d\n", currentCount)
				ratePerSecond := float64(currentCount-lastCount) / t.Sub(lastTime).Seconds()
				publishMutex.Unlock()

				// Log detailed stats to JSON
				statsData := map[string]any{
					"event":            "periodic_stats",
					"node":             *instanceID,
					"messages_per_sec": ratePerSecond,
					"messages_sent":    sent,
					"total_messages":   currentCount,
					"elapsed_seconds":  t.Sub(runStartTime).Seconds(),
				}

				contract.LogJSON(statsData)

				lastCount = currentCount
				lastTime = t
			}
		}
	}()

	// Listen for termination signals.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	runEndTime := time.Now()
	durationSec := runEndTime.Sub(runStartTime).Seconds()

	var messagesReceived int
	var procRate float64
	if mp != nil {
		messagesReceived = mp.GetMessagesReceived()
		procRate = float64(messagesReceived) / durationSec

	}

	confirmed := c.Confirmed()
	fmt.Printf("Confirmed messages: %d\n", confirmed)

	// Log the summary of the experiment with cost details
	summary := map[string]any{
		"event":               "experiment_summary",
		"node":                *instanceID,
		"run_duration_sec":    durationSec,
		"messages_published":  confirmed,
		"messages_received":   messagesReceived,
		"avg_publish_rate":    float64(confirmed) / durationSec,
		"avg_processing_rate": procRate,
		"total_messages_sent": c.Sent(),
	}
	contract.LogJSON(summary)

	fmt.Printf("Total messages published: %d\n", confirmed)
	fmt.Printf("Total messages received: %d\n", messagesReceived)
	fmt.Printf("Run duration: %.2f seconds\n", durationSec)
	fmt.Printf("Average publish rate: %.2f messages/sec\n", float64(confirmed)/durationSec)
	fmt.Printf("Average processing rate: %.2f messages/sec\n", procRate)

	fmt.Println("========================================")

	// Log application shutdown
	contract.LogJSON(map[string]any{
		"event": "application_shutdown",
		"node":  *instanceID,
	})
}
