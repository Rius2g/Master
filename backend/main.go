package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
    "github.com/ethereum/go-ethereum/crypto"
	"log"
	"net/http"
	"os"
	"os/signal"
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
)

var messagesPublished int      // Global counter for published messages
var runStartTime time.Time       // Track the start time of the experiment
var logFile *os.File             // Global handle for the log file

//
// Helper: JSON logging function
//
func logJSON(record map[string]any) {
	record["timestamp"] = time.Now().Format(time.RFC3339Nano)
	bytes, err := json.Marshal(record)
	if err != nil {
		log.Printf("failed to marshal log record: %v", err)
		return
	}
	log.Println(string(bytes))
}

//
// Publisher: continuously publishes messages for stress testing
//
func startPublisher(ctx context.Context, contractClient *contract.ContractInteractionInterface) {
	seq := 1
	ticker := time.NewTicker(*publishInterval)
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

			// Publish the message via the contract interface with dependencies.
			err = contractClient.Upload(string(payloadBytes), *instanceID, dataName, deps)
			if err != nil {
				logJSON(map[string]any{
					"event": "publish_error",
					"node":  *instanceID,
					"seq":   seq,
					"error": err.Error(),
				})
			} else {
				logJSON(map[string]any{
					"event":         "message_published",
					"node":          *instanceID,
					"seq":           seq,
					"dataName":      dataName,
					"publishTime":   t.Format(time.RFC3339Nano),
					"payload":       string(payloadBytes),
					"dependencies":  deps, // Log dependencies for debugging.
				})
				messagesPublished++
			}

			// Save current message's hash for potential dependency in the next message.
			lastDep = fixedHash
			seq++
		}
	}
}

//
// HTTP handler for reporting in-memory metrics
//
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]any{
		"node":              *instanceID,
		"port":              *port,
		"messagesPublished": messagesPublished,
		"timestamp":         time.Now().Format(time.RFC3339Nano),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

//
// Start the HTTP server (useful for health checks if needed)
//
func startHTTPServer() {
	http.HandleFunc("/metrics", metricsHandler)
	logJSON(map[string]any{
		"event": "http_server_start",
		"node":  *instanceID,
		"port":  *port,
	})
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatalf("failed to start HTTP server: %v", err)
	}
}

//
// Setup logging to a file
//
func setupLogging() {
	var err error
	logFile, err = os.OpenFile("experiment_results.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	// Redirect global logger output to the log file
	log.SetOutput(logFile)
}

//
// Close the log file on exit
//
func cleanupLogging() {
	if logFile != nil {
		logFile.Close()
	}
}

//
// main – modified to include results logging to file
//
func main() {
    fmt.Println("Starting the application...")
	flag.Parse()
	runStartTime = time.Now()

	// Set up logging to file
	setupLogging()
	defer cleanupLogging()

	// Load .env settings
	if err := godotenv.Load(); err != nil {
		logJSON(map[string]any{
			"event": "env_load_error",
			"error": err.Error(),
		})
		log.Fatalf("Error loading .env file: %v", err)
	}
	logJSON(map[string]any{
		"event": "application_start",
		"node":  *instanceID,
	})

	// Initialize smart contract and message processor
	contractAddress := os.Getenv("CONTRACT_ADDRESS")
	privateKey := os.Getenv("PRIVATE_KEY")
    fmt.Println("Contract Address:", contractAddress) 
    fmt.Println("Private Key:", privateKey)
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

	// If stress mode is enabled, start the publisher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if *stressMode {
		go startPublisher(ctx, c)
	}

	// Start the HTTP server in a goroutine
	go startHTTPServer()

	logJSON(map[string]any{
		"event": "ordering_check_enabled",
		"node":  *instanceID,
	})

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	// Compute summary metrics on shutdown
	runEndTime := time.Now()
	duration := runEndTime.Sub(runStartTime).Seconds()
	summary := map[string]any{
		"event":             "experiment_summary",
		"node":              *instanceID,
		"run_duration_sec":  duration,
		"messages_published": messagesPublished,
		"avg_publish_rate":  float64(messagesPublished) / duration, // messages per second
	}
	logJSON(summary)

	logJSON(map[string]any{
		"event": "application_shutdown",
		"node":  *instanceID,
	})
}

