package main

import (
	"context"
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
	publishInterval = flag.Duration("publishInterval", 100*time.Millisecond, "Interval between publishing messages")
	envFile         = flag.String("env-file", ".env", "Path to environment file")
	concurrency     = flag.Int("concurrency", 1, "Number of concurrent message publishers")
)

var (
	runStartTime time.Time  // Track the start time of the experiment
	logFile      *os.File   // Global handle for the log file
	publishMutex sync.Mutex // Mutex to protect the messages published counter
)

func startPublisher(ctx context.Context, contractClient *contract.ContractInteractionInterface) {
	var seq int64 = 1
	var lastDep [32]byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Build payload
			payload := map[string]any{
				"instance":    *instanceID,
				"seq":         seq,
				"publishedAt": time.Now().Format(time.RFC3339Nano),
			}
			payloadBytes, _ := json.Marshal(payload)

			// Dependencies
			var deps [][32]byte
			if seq > 1 {
				deps = append(deps, lastDep)
			}

			// Pack the transaction input
			dataName := fmt.Sprintf("%s-%d", *instanceID, seq)
			input, err := contractClient.GetPackedInput(string(payloadBytes), *instanceID, dataName, deps...)
			if err != nil {
				log.Printf("Failed to pack input: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// Precompute message hash
			newMessageHash := crypto.Keccak256Hash(payloadBytes)
			var fixedHash [32]byte
			copy(fixedHash[:], newMessageHash[:])

			fmt.Printf("Hash: %x\n", newMessageHash)

			// Upload using payloadBytes and input
			start := time.Now()
			err = contractClient.Upload(payloadBytes, *instanceID, dataName, seq, deps, input)
			elapsed := time.Since(start)

			if err != nil {
				contract.LogJSON(map[string]any{
					"event": "publish_error",
					"node":  *instanceID,
					"seq":   seq,
					"error": err.Error(),
				})
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// Only update lastDep after successful upload
			lastDep = fixedHash
			seq++

			log.Printf("[Publisher] Successfully uploaded seq %d (took %s)", seq, elapsed)
		}
	}
}

// Setup logging to a file
func setupLogging() {
	var err error
	logFile, err = os.OpenFile("experiment_results.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
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
	contractAddress := os.Getenv("CONTRACT_ADDRESS")
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

	// Start publishers as per your chosen mode.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *stressMode {
		go startPublisher(ctx, c)
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
				ratePerSecond := float64(currentCount-lastCount) / t.Sub(lastTime).Seconds()
				publishMutex.Unlock()

				// Log detailed stats to JSON
				statsData := map[string]any{
					"event":            "periodic_stats",
					"node":             *instanceID,
					"messages_per_sec": ratePerSecond,
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

	// Get cost statistics
	runEndTime := time.Now()
	durationSec := runEndTime.Sub(runStartTime).Seconds()

	var messagesReceived int
	var procRate float64
	if mp != nil {
		messagesReceived = mp.GetMessagesReceived()
		procRate = float64(messagesReceived) / durationSec

	}

	endToEndRatio := 0.0
	confirmed := c.Confirmed()
	if confirmed > 0 {
		endToEndRatio = float64(messagesReceived) / float64(confirmed)
	}

	// Log the summary of the experiment with cost details

	summary := map[string]any{
		"event":               "experiment_summary",
		"node":                *instanceID,
		"run_duration_sec":    durationSec,
		"messages_published":  confirmed,
		"messages_received":   messagesReceived,
		"avg_publish_rate":    float64(confirmed) / durationSec,
		"avg_processing_rate": procRate,
		"end_to_end_ratio":    endToEndRatio,
	}
	contract.LogJSON(summary)

	// Print human-readable summary to stdout
	fmt.Printf("Total messages published: %d\n", confirmed)
	fmt.Printf("Total messages received: %d\n", messagesReceived)
	fmt.Printf("Run duration: %.2f seconds\n", durationSec)
	fmt.Printf("Average publish rate: %.2f messages/sec\n", float64(confirmed)/durationSec)
	fmt.Printf("Average processing rate: %.2f messages/sec\n", procRate)
	fmt.Printf("End-to-end ratio: %.2f%%\n", endToEndRatio*100)

	fmt.Println("========================================")

	// Log application shutdown
	contract.LogJSON(map[string]any{
		"event": "application_shutdown",
		"node":  *instanceID,
	})
}
