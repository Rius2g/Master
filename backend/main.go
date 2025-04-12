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
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	contract "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
	msgProcessor "github.com/rius2g/Master/backend/pkg/messageProcessor"
)

var (
	typeFlag        = flag.String("type", "default", "Type of the message processor")
	instanceID      = flag.String("instance", "node1", "Unique instance identifier")
	port            = flag.String("port", "8080", "Port to run the HTTP server on")
	stressMode      = flag.Bool("stress", false, "Enable stress test mode (continuous publishing of messages)")
	publishInterval = flag.Duration("publishInterval", 100*time.Millisecond, "Interval between publishing messages")
	batchMode       = flag.Bool("batch", false, "Enable batch publishing mode")
	batchSize       = flag.Int("batchSize", 10, "Number of messages to send in each batch")
	batchInterval   = flag.Duration("batchInterval", 100*time.Millisecond, "Interval between batches")
	envFile         = flag.String("env-file", ".env", "Path to environment file")
	concurrency     = flag.Int("concurrency", 5, "Number of concurrent message publishers")
)

var (
	messagesPublished int        // Global counter for published messages
	runStartTime      time.Time  // Track the start time of the experiment
	logFile           *os.File   // Global handle for the log file
	publishMutex      sync.Mutex // Mutex to protect the messages published counter
)

// Helper: JSON logging function
func logJSON(record map[string]any) {
	record["timestamp"] = time.Now().Format(time.RFC3339Nano)
	bytes, err := json.Marshal(record)
	if err != nil {
		log.Printf("failed to marshal log record: %v", err)
		return
	}
	log.Println(string(bytes))
}

// Publisher: continuously publishes messages for stress testing
func startPublisher(ctx context.Context, contractClient *contract.ContractInteractionInterface) {
	seq := 1
	ticker := time.NewTicker(*publishInterval)
	defer ticker.Stop()

	// lastDep will hold the hash (converted to [32]byte) of the previous message.
	var lastDep [32]byte

	// Use a worker pool for concurrent publishing
	publishCh := make(chan struct {
		payload []byte
		seq     int
		deps    [][32]byte
		name    string
	}, 100) // Buffer channel to avoid blocking

	// Start worker goroutines
	workerCount := getEnvInt("CONCURRENT_PUBLISHES", *concurrency)
	var wg sync.WaitGroup
    for i := range(workerCount) {
		wg.Add(1)
		go func(workerId int) {
			defer wg.Done()
			for job := range publishCh {
				publishMessage(contractClient, job.payload, job.seq, job.deps, job.name)
			}
		}(i)
	}

	for {
		select {
		case <-ctx.Done():
			close(publishCh) // Stop all workers
			wg.Wait()        // Wait for all workers to finish
			logJSON(map[string]any{
				"event": "publisher_stopped",
				"node":  *instanceID,
			})
			return
		case t := <-ticker.C:
			// Create payload with instanceID, sequence number, timestamp.
			payload := map[string]any{
				"instance":    *instanceID,
				"seq":         seq,
				"publishedAt": t.Format(time.RFC3339Nano),
			}
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				logJSON(map[string]any{
					"event": "payload_marshal_error",
					"node":  *instanceID,
					"error": err.Error(),
				})
				continue
			}

			// Compute hash of the payload; this hash will be used for dependency.
			hash := crypto.Keccak256(payloadBytes)
			var fixedHash [32]byte
			if len(hash) != 32 {
				logJSON(map[string]any{
					"event": "hash_error",
					"node":  *instanceID,
					"error": fmt.Sprintf("hash length %d", len(hash)),
				})
				continue
			}
			copy(fixedHash[:], hash)

			// Build the dependencies slice.
			// In this example, every third message depends on the previous message.
			var deps [][32]byte
			if seq > 1 && seq%3 == 0 {
				deps = append(deps, lastDep)
			}

			// Use seq as part of dataName to help track ordering.
			dataName := fmt.Sprintf("%s-%d", *instanceID, seq)

			// Send to worker pool instead of publishing directly
			job := struct {
				payload []byte
				seq     int
				deps    [][32]byte
				name    string
			}{
				payload: payloadBytes,
				seq:     seq,
				deps:    deps,
				name:    dataName,
			}

			select {
			case publishCh <- job:
				// Successfully queued
			default:
				// Channel full, log warning
				logJSON(map[string]any{
					"event": "publish_queue_full",
					"node":  *instanceID,
					"seq":   seq,
				})
			}

			// Save current message's hash for potential dependency in the next message.
			lastDep = fixedHash
			seq++
		}
	}
}

// Helper function to publish a message
func publishMessage(contractClient *contract.ContractInteractionInterface, payloadBytes []byte, seq int, deps [][32]byte, dataName string) {
	startTime := time.Now()
	
	err := contractClient.Upload(string(payloadBytes), *instanceID, dataName, deps)
	if err != nil {
		logJSON(map[string]any{
			"event":        "publish_error",
			"node":         *instanceID,
			"seq":          seq,
			"error":        err.Error(),
			"publish_time": time.Now().Sub(startTime).Milliseconds(),
		})
	} else {
		logJSON(map[string]any{
			"event":         "message_published",
			"node":          *instanceID,
			"seq":           seq,
			"dataName":      dataName,
			"publishTime":   startTime.Format(time.RFC3339Nano),
			"publish_latency_ms": time.Now().Sub(startTime).Milliseconds(),
			"payload":       string(payloadBytes),
			"dependencies":  deps, // Log dependencies for debugging.
		})
		
		// Thread-safe increment of messages published
		publishMutex.Lock()
		messagesPublished++
		publishMutex.Unlock()
	}
}

// Add this function to your main.go file to implement batch publishing
// This will significantly increase throughput by sending multiple messages at once

func startBatchPublisher(ctx context.Context, contractClient *contract.ContractInteractionInterface, batchSize int, batchInterval time.Duration) {
    seq := 1
    ticker := time.NewTicker(batchInterval)
    defer ticker.Stop()

    // lastDep will hold the hash (converted to [32]byte) of the previous message.
    var lastDep [32]byte

    for {
        select {
        case <-ctx.Done():
            logJSON(map[string]any{
                "event": "publisher_stopped",
                "node":  *instanceID,
            })
            return
        case t := <-ticker.C:
            // Create a batch of messages
            for i := range(batchSize) {
                // Create payload with instanceID, sequence number, timestamp.
                payload := map[string]any{
                    "instance":    *instanceID,
                    "seq":         seq,
                    "batch":       i,
                    "publishedAt": t.Add(time.Duration(i) * time.Microsecond).Format(time.RFC3339Nano),
                }
                payloadBytes, err := json.Marshal(payload)
                if err != nil {
                    logJSON(map[string]any{
                        "event": "payload_marshal_error",
                        "node":  *instanceID,
                        "error": err.Error(),
                    })
                    continue
                }

                // Compute hash of the payload; this hash will be used for dependency.
                hash := crypto.Keccak256(payloadBytes)
                var fixedHash [32]byte
                if len(hash) != 32 {
                    logJSON(map[string]any{
                        "event": "hash_error",
                        "node":  *instanceID,
                        "error": fmt.Sprintf("hash length %d", len(hash)),
                    })
                    continue
                }
                copy(fixedHash[:], hash)

                // Build the dependencies slice.
                // In this example, every third message depends on the previous message.
                var deps [][32]byte
                if seq > 1 && seq%3 == 0 {
                    deps = append(deps, lastDep)
                }

                // Use seq as part of dataName to help track ordering.
                dataName := fmt.Sprintf("%s-%d-%d", *instanceID, seq, i)

                // Launch goroutine to publish the message asynchronously
                go func(payload []byte, dataName string, deps [][32]byte, seqNum int) {
                    err := contractClient.Upload(string(payload), *instanceID, dataName, deps)
                    if err != nil {
                        logJSON(map[string]any{
                            "event": "publish_error",
                            "node":  *instanceID,
                            "seq":   seqNum,
                            "error": err.Error(),
                        })
                    } else {
                        logJSON(map[string]any{
                            "event":         "message_published",
                            "node":          *instanceID,
                            "seq":           seqNum,
                            "dataName":      dataName,
                            "publishTime":   t.Format(time.RFC3339Nano),
                            "payload":       string(payload),
                            "dependencies":  deps, // Log dependencies for debugging.
                        })
                        messagesPublished++
                    }
                }(payloadBytes, dataName, deps, seq)

                // Save current message's hash for potential dependency in the next message.
                lastDep = fixedHash
                seq++
            }
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

// Helper to get integer from environment with default
func getEnvInt(key string, defaultVal int) int {
	if val, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

// Helper to get bool from environment with default
func getEnvBool(key string, defaultVal bool) bool {
	if val, exists := os.LookupEnv(key); exists {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			return boolVal
		}
	}
	return defaultVal
}

// main – modified to include results logging to file
func main() {
	flag.Parse()
	runStartTime = time.Now()

	// Set GOMAXPROCS to use all available cores
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)

	// Set up logging to file
	setupLogging()
	defer cleanupLogging()

	// Load environment settings
	if err := godotenv.Load(*envFile); err != nil {
		logJSON(map[string]any{
			"event": "env_load_error",
			"error": err.Error(),
		})
		// Try to load default .env if custom one fails
		if *envFile != ".env" {
			if err := godotenv.Load(); err != nil {
				log.Printf("Warning: could not load default .env file: %v", err)
			}
		}
	}
	
	logJSON(map[string]any{
		"event":      "application_start",
		"node":       *instanceID,
		"batch_mode": *batchMode,
		"concurrency": *concurrency,
	})

	// Initialize smart contract and message processor
	contractAddress := os.Getenv("CONTRACT_ADDRESS")
	privateKey := os.Getenv("PRIVATE_KEY")
	
	c, err := contract.Init(contractAddress, privateKey)
	if err != nil {
		logJSON(map[string]any{
			"event": "contract_init_error",
			"error": err.Error(),
		})
		log.Fatalf("failed to initialize contract: %v", err)
	}
	logJSON(map[string]any{
		"event": "contract_initialized",
		"node":  *instanceID,
	})

	mp, err := msgProcessor.NewMessageProcessor(c)
	if err != nil {
		logJSON(map[string]any{
			"event": "message_processor_init_error",
			"error": err.Error(),
		})
		log.Fatalf("failed to create message processor: %v", err)
	}
	logJSON(map[string]any{
		"event": "message_processor_initialized",
		"node":  *instanceID,
	})

	// Start the message processor (listening for events)
	mp.Start()
	defer mp.Stop()

	// Log any errors from the message processor
	go func() {
		for err := range mp.Errors() {
			logJSON(map[string]any{
				"event": "processor_error",
				"node":  *instanceID,
				"error": err.Error(),
			})
		}
	}()

	// Start publishers based on mode
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	if *stressMode {
		if *batchMode || getEnvBool("ENABLE_BATCHING", false) {
			// Use batch publisher for higher throughput
			go startBatchPublisher(ctx, c, *batchSize, *batchInterval)
		} else {
			// Use regular publisher with worker pool
			go startPublisher(ctx, c)
		}
	}

	logJSON(map[string]any{
		"event": "ordering_check_enabled",
		"node":  *instanceID,
	})

	// Print periodic stats
	go func() {
		statsTicker := time.NewTicker(10 * time.Second)
		defer statsTicker.Stop()
		
		lastCount := 0
		lastTime := time.Now()
		
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-statsTicker.C:
				publishMutex.Lock()
				currentCount := messagesPublished
				currentTime := t
				ratePerSecond := float64(currentCount-lastCount) / currentTime.Sub(lastTime).Seconds()
				publishMutex.Unlock()
				
				fmt.Printf("[STATS] Messages per second: %.2f, Total: %d\n", ratePerSecond, currentCount)
				
				lastCount = currentCount
				lastTime = currentTime
			}
		}
	}()

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	// Compute summary metrics on shutdown
	runEndTime := time.Now()
	duration := runEndTime.Sub(runStartTime).Seconds()
	summary := map[string]any{
		"event":              "experiment_summary",
		"node":               *instanceID,
		"run_duration_sec":   duration,
		"messages_published": messagesPublished,
		"avg_publish_rate":   float64(messagesPublished) / duration, // messages per second
	}
	logJSON(summary)

	logJSON(map[string]any{
		"event": "application_shutdown",
		"node":  *instanceID,
	})
}
