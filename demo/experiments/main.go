package main

import (
	"context"
	"encoding/json"
    "io"
    "net/http"
    "path/filepath"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	// Import your backend packages
	contract "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
	msgProcessor "github.com/rius2g/Master/backend/pkg/messageProcessor"
	t "github.com/rius2g/Master/backend/pkg/types"
)

const (
	defaultResultsDir = "experiment_results"
)

// ExperimentType defines the type of experiment to run
type ExperimentType string

const (
	LatencyTest    ExperimentType = "latency"
	ThroughputTest ExperimentType = "throughput"
	OrderingTest   ExperimentType = "ordering"
	ScalabilityTest ExperimentType = "scalability"
)

// MetricResult stores the measurement result for a single message
type MetricResult struct {
	// Common fields for all experiment types
	MessageID     string    `json:"message_id"`
	MessageSize   int       `json:"message_size"`
	Status        string    `json:"status"`
	SenderNode    string    `json:"sender_node"`
	ReceiverNode  string    `json:"receiver_node,omitempty"`
	SequenceNum   int       `json:"sequence_num"`
	
	// Timing data
	SendTime      time.Time `json:"send_time"`
	UploadTime    time.Time `json:"upload_time"`
	ReceiveTime   time.Time `json:"receive_time,omitempty"`
	ProcessTime   time.Time `json:"process_time,omitempty"`
	
	// Duration measurements
	UploadDuration time.Duration `json:"upload_duration"`
	LatencyDuration time.Duration `json:"latency_duration,omitempty"`
	ProcessDuration time.Duration `json:"process_duration,omitempty"`
	
	// Test-specific data
	ExperimentType ExperimentType `json:"experiment_type"`
	BatchID        string         `json:"batch_id"`
	ErrorMessage   string         `json:"error_message,omitempty"`
}

// Configuration for the experiment
type ExperimentConfig struct {
	// Basic node configuration
	NodeID          string        `json:"node_id"`
	NodeRole        string        `json:"node_role"` // "sender" or "receiver"
	ContractAddress string        `json:"contract_address"`
	PrivateKey      string        `json:"private_key"`
	SecurityLevel   uint          `json:"security_level"`
	
	// Experiment parameters
	ExperimentType  ExperimentType `json:"experiment_type"`
	MessageSizes    []int          `json:"message_sizes"`
	MessageCount    int            `json:"message_count"`
	Interval        time.Duration  `json:"interval"`
	BatchID         string         `json:"batch_id"`
	UseDependencies bool           `json:"use_dependencies"`
	
	// Special test parameters
	BurstMode       bool          `json:"burst_mode"`
    EnableHTTP     bool          `json:"enable_http"`
    HTTPPort       int           `json:"http_port"`
	ResultsDir      string        `json:"results_dir"`
}

func startHTTPServer(ctx context.Context, config ExperimentConfig) {
	if !config.EnableHTTP {
		return
	}
	
	// Create a router for our endpoints
	mux := http.NewServeMux()
	
	// Register handlers
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
			"node":   config.NodeID,
		})
	})
	
	// Results endpoint returns all experiment results
	mux.HandleFunc("/results", func(w http.ResponseWriter, r *http.Request) {
		// List all result files
		files, err := filepath.Glob(filepath.Join(config.ResultsDir, "*.jsonl"))
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading results: %v", err), http.StatusInternalServerError)
			return
		}
		
		// Collect all results
		allResults := make(map[string][]MetricResult)
		
		for _, file := range files {
			// Read file content
			f, err := os.Open(file)
			if err != nil {
				log.Printf("Error opening file %s: %v", file, err)
				continue
			}
			
			// Parse JSONL file
			var fileResults []MetricResult
			decoder := json.NewDecoder(f)
			for {
				var result MetricResult
				if err := decoder.Decode(&result); err != nil {
					if err != io.EOF {
						log.Printf("Error decoding result: %v", err)
					}
					break
				}
				fileResults = append(fileResults, result)
			}
			f.Close()
			
			// Add to collection
			basename := filepath.Base(file)
			allResults[basename] = fileResults
		}
		
		// Create response
		response := map[string]interface{}{
			"node_id":  config.NodeID,
			"node_role": config.NodeRole,
			"experiment_type": config.ExperimentType,
			"results":  allResults,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
	
	// Summary endpoint returns experiment statistics
	mux.HandleFunc("/summary", func(w http.ResponseWriter, r *http.Request) {
		// Prepare summary statistics
		summary := generateSummaryStats(config.ResultsDir)
		
		// Add node info
		summary["node_id"] = config.NodeID
		summary["node_role"] = config.NodeRole
		summary["experiment_type"] = config.ExperimentType
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summary)
	})
	
	// Create the server
	addr := fmt.Sprintf(":%d", config.HTTPPort)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	
	// Start the server in a goroutine
	go func() {
		log.Printf("Starting HTTP server on port %d", config.HTTPPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	
	// Shutdown gracefully when context is canceled
	go func() {
		<-ctx.Done()
		log.Println("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()
}

// Helper function to generate summary statistics
func generateSummaryStats(resultsDir string) map[string]interface{} {
	// Summary statistics by message size for each experiment type
	summary := make(map[string]interface{})
	
	// Stats by experiment type and message size
	statsByType := make(map[ExperimentType]map[int]struct{
		Count         int
		SuccessCount  int
		FailCount     int
		TotalLatency  time.Duration
		MinLatency    time.Duration
		MaxLatency    time.Duration
		AvgLatency    time.Duration
		TotalUpload   time.Duration
		AvgUpload     time.Duration
	})
	
	// Find all result files
	files, err := filepath.Glob(filepath.Join(resultsDir, "*.jsonl"))
	if err != nil {
		summary["error"] = fmt.Sprintf("Failed to read results directory: %v", err)
		return summary
	}
	
	// Process each file
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			log.Printf("Error opening file %s: %v", file, err)
			continue
		}
		
		// Process each line (result)
		decoder := json.NewDecoder(f)
		for {
			var result MetricResult
			if err := decoder.Decode(&result); err != nil {
				if err != io.EOF {
					log.Printf("Error decoding result: %v", err)
				}
				break
			}
			
			// Initialize stats map for this experiment type if needed
			if _, exists := statsByType[result.ExperimentType]; !exists {
				statsByType[result.ExperimentType] = make(map[int]struct{
					Count         int
					SuccessCount  int
					FailCount     int
					TotalLatency  time.Duration
					MinLatency    time.Duration
					MaxLatency    time.Duration
					AvgLatency    time.Duration
					TotalUpload   time.Duration
					AvgUpload     time.Duration
				})
			}
			
			// Get stats for this experiment type and message size
			stats := statsByType[result.ExperimentType][result.MessageSize]
			
			// Update stats based on the result
			if result.Status == "received" && result.LatencyDuration > 0 {
				stats.Count++
				stats.TotalLatency += result.LatencyDuration
				
				// Update min/max latency
				if stats.MinLatency == 0 || result.LatencyDuration < stats.MinLatency {
					stats.MinLatency = result.LatencyDuration
				}
				if result.LatencyDuration > stats.MaxLatency {
					stats.MaxLatency = result.LatencyDuration
				}
			}
			
			if result.UploadDuration > 0 {
				stats.TotalUpload += result.UploadDuration
			}
			
			if strings.HasPrefix(result.Status, "success") {
				stats.SuccessCount++
			} else if strings.HasPrefix(result.Status, "error") {
				stats.FailCount++
			}
			
			// Update the map
			statsByType[result.ExperimentType][result.MessageSize] = stats
		}
		
		f.Close()
	}
	
	// Calculate averages and format for output
	formattedStats := make(map[string]map[string]interface{})
	
	for expType, sizeStats := range statsByType {
		expTypeStats := make(map[string]interface{})
		
		for size, stats := range sizeStats {
			// Calculate averages
			if stats.Count > 0 {
				stats.AvgLatency = stats.TotalLatency / time.Duration(stats.Count)
			}
			if stats.SuccessCount > 0 {
				stats.AvgUpload = stats.TotalUpload / time.Duration(stats.SuccessCount)
			}
			
			// Format for output
			sizeKey := fmt.Sprintf("%d_bytes", size)
			expTypeStats[sizeKey] = map[string]interface{}{
				"message_count":      stats.Count + stats.SuccessCount + stats.FailCount,
				"received_count":     stats.Count,
				"success_count":      stats.SuccessCount,
				"fail_count":         stats.FailCount,
				"avg_latency_ms":     stats.AvgLatency.Milliseconds(),
				"min_latency_ms":     stats.MinLatency.Milliseconds(),
				"max_latency_ms":     stats.MaxLatency.Milliseconds(),
				"avg_upload_ms":      stats.AvgUpload.Milliseconds(),
				"success_rate":       float64(stats.SuccessCount) / float64(stats.SuccessCount+stats.FailCount) * 100,
			}
		}
		
		formattedStats[string(expType)] = expTypeStats
	}
	
	summary["experiment_stats"] = formattedStats
	summary["generated_at"] = time.Now().Format(time.RFC3339)
	
	return summary
}

func main() {
	// Parse command line flags
	nodeRole := flag.String("role", "receiver", "Node role: 'sender' or 'receiver'")
	expType := flag.String("type", "throughput", "Experiment type: 'latency', 'throughput', 'ordering', 'scalability'")
	nodeID := flag.String("node", fmt.Sprintf("node-%d", time.Now().UnixNano()%1000), "Node identifier")
	sizeStr := flag.String("sizes", "1024,10240,102400,1048576", "Comma-separated message sizes in bytes")
	count := flag.Int("count", 50, "Number of messages to send per size")
	interval := flag.Int("interval", 500, "Milliseconds between messages")
	burstMode := flag.Bool("burst", false, "Send messages in burst mode (minimal delay)")
	useDeps := flag.Bool("deps", false, "Use message dependencies")
	secLevel := flag.Uint("security", 1, "Security level for messages")
	resultsDir := flag.String("results", defaultResultsDir, "Directory for results")	

    enableHTTP := flag.Bool("http", true, "Enable HTTP server for results")
	httpPort := flag.Int("port", 8080, "HTTP server port")

	flag.Parse()
	
	// Parse message sizes
	var messageSizes []int
	for _, s := range strings.Split(*sizeStr, ",") {
		var size int
		fmt.Sscanf(s, "%d", &size)
		if size > 0 {
			messageSizes = append(messageSizes, size)
		}
	}
	
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using environment variables")
	}
	
	contractAddress := os.Getenv("CONTRACT_ADDRESS")
	if contractAddress == "" {
		log.Fatal("CONTRACT_ADDRESS not found in environment")
	}
	
	privateKey := os.Getenv("PRIVATE_KEY")
	if privateKey == "" {
		log.Fatal("PRIVATE_KEY not found in environment")
	}
	
	// Create experiment configuration
	config := ExperimentConfig{
		NodeID:          *nodeID,
		NodeRole:        *nodeRole,
		ContractAddress: contractAddress,
		PrivateKey:      privateKey,
		SecurityLevel:   *secLevel,
		ExperimentType:  ExperimentType(*expType),
		MessageSizes:    messageSizes,
		MessageCount:    *count,
		Interval:        time.Duration(*interval) * time.Millisecond,
		BatchID:         fmt.Sprintf("%s-%d", *expType, time.Now().Unix()),
		UseDependencies: *useDeps,
		BurstMode:       *burstMode,
		ResultsDir:      *resultsDir,
        EnableHTTP:      *enableHTTP,
        HTTPPort:        *httpPort,
	}
	
	// Validate configuration
	if len(config.MessageSizes) == 0 {
		log.Fatal("No valid message sizes specified")
	}
	
	// Adjust configuration based on experiment type
	adjustConfigForExperimentType(&config)
	
	// Create results directory
	if err := os.MkdirAll(config.ResultsDir, 0755); err != nil {
		log.Fatalf("Failed to create results directory: %v", err)
	}
	
	// Initialize contract interface
	c, err := contract.Init(config.ContractAddress, config.PrivateKey, config.SecurityLevel)
	if err != nil {
		log.Fatalf("Failed to initialize contract interface: %v", err)
	}
	log.Printf("Smart Contract interface initialized for node %s", config.NodeID)
	
	// Initialize message processor
	mp, err := msgProcessor.NewMessageProcessor(c)
	if err != nil {
		log.Fatalf("Failed to initialize message processor: %v", err)
	}
	mp.Start()
	defer mp.Stop()
	
	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down gracefully...")
		cancel()
	}()

    if config.EnableHTTP {
        startHTTPServer(ctx, config)
        log.Println("HTTP server started")
        log.Printf("Results available at http://localhost:%d/results", config.HTTPPort)
    }

	
	// Create result storage channel
	results := make(chan MetricResult, config.MessageCount*len(config.MessageSizes))
	
	// Start result processor
	go processResults(ctx, results, config)
	
	// Listen for contract events regardless of role
	messages, encryptedMessages, err := c.Listen(ctx)
	if err != nil {
		log.Fatalf("Failed to listen for contract events: %v", err)
	}
	log.Printf("Listening for contract events on %s", config.ExperimentType)
	
	// Start message handler
	go handleMessages(ctx, messages, encryptedMessages, results, config)
	
	// Run sender role if applicable
	if config.NodeRole == "sender" {
		log.Printf("Starting %s experiment as sender node %s", config.ExperimentType, config.NodeID)
		
		// Use wait group to track experiment completion
		var wg sync.WaitGroup
		wg.Add(1)
		
		// Run appropriate experiment based on type
		switch config.ExperimentType {
		case LatencyTest:
			go runLatencyExperiment(ctx, c, config, results, &wg, mp)
		case ThroughputTest:
			go runThroughputExperiment(ctx, c, config, results, &wg, mp)
		case OrderingTest:
			go runOrderingExperiment(ctx, c, config, results, &wg, mp)
		case ScalabilityTest:
			go runScalabilityExperiment(ctx, c, config, results, &wg, mp)
		default:
			log.Printf("Unknown experiment type: %s, defaulting to throughput", config.ExperimentType)
			go runThroughputExperiment(ctx, c, config, results, &wg, mp)
		}
		
		// Wait for experiment to complete
		wg.Wait()
		log.Printf("Experiment completed, waiting for pending messages...")
		
		// Allow time for pending messages to be processed
		time.Sleep(30 * time.Second)
		
	} else {
		log.Printf("Running as receiver node %s for %s experiment", config.NodeID, config.ExperimentType)
	}
	
	// Handle errors from message processor
	go func() {
		for err := range mp.Errors() {
			log.Printf("Error from message processor: %v", err)
		}
	}()

   
	
	// Keep running until context is cancelled
	<-ctx.Done()
	log.Println("Shutdown complete")

}

// adjustConfigForExperimentType modifies the configuration based on the experiment type
func adjustConfigForExperimentType(config *ExperimentConfig) {
	switch config.ExperimentType {
	case LatencyTest:
		// Latency tests need consistent timing and medium-sized messages
		if len(config.MessageSizes) > 2 {
			// Use medium sizes for latency tests
			config.MessageSizes = config.MessageSizes[1:3]
		}
		// Ensure consistent interval
		config.BurstMode = false
		log.Printf("Configured for latency test with %d message sizes", len(config.MessageSizes))
		
	case ThroughputTest:
		// Throughput tests need minimal delay between messages
		if config.BurstMode {
			config.Interval = 50 * time.Millisecond
		}
		log.Printf("Configured for throughput test with %d message sizes", len(config.MessageSizes))
		
	case OrderingTest:
		// Ordering tests focus on message sequence
		// Use medium size messages for ordering tests
		if len(config.MessageSizes) > 1 {
			config.MessageSizes = []int{config.MessageSizes[1]}
		}
		log.Printf("Configured for ordering test with message size %d", config.MessageSizes[0])
		
	case ScalabilityTest:
		// Scalability tests focus on large messages
		// Use the larger message sizes
		if len(config.MessageSizes) > 2 {
			config.MessageSizes = config.MessageSizes[len(config.MessageSizes)-2:]
		}
		// Adjust count for large messages
		if config.MessageSizes[0] > 100000 {
			config.MessageCount = config.MessageCount / 2
		}
		log.Printf("Configured for scalability test with %d messages per size", config.MessageCount)
	}
}

// handleMessages processes incoming messages and records metrics
func handleMessages(ctx context.Context, 
                   messages <-chan t.Message,
                   encryptedMessages <-chan t.EncryptedMessage,
                   results chan<- MetricResult,
                   config ExperimentConfig) {
	
	// Track received messages for ordering consistency
	receivedMessages := make(map[string][]int)
	
	for {
		select {
		case <-ctx.Done():
			return
			
		case msg, ok := <-messages:
			if !ok {
				continue
			}
			
			// Parse the message name format: EXP-{type}-{size}-{seqNum}-{nodeID}-{batchID}-{msgID}
			parts := strings.Split(msg.DataName, "-")
			if len(parts) < 7 || parts[0] != "EXP" {
				// Not our experiment message
				continue
			}
			
			expType := ExperimentType(parts[1])
			size := 0
			seqNum := 0
			fmt.Sscanf(parts[2], "%d", &size)
			fmt.Sscanf(parts[3], "%d", &seqNum)
			senderNodeID := parts[4]
			batchID := parts[5]
			msgID := parts[6]
			
			// Skip if not matching our experiment type
			if expType != config.ExperimentType {
				continue
			}
			
			receiveTime := time.Now()
			
			// Parse embedded timestamp if present
			var sendTime time.Time
			if strings.Contains(msg.Content, "TIMESTAMP:") {
				timeStr := strings.SplitN(msg.Content, "TIMESTAMP:", 2)[1]
				timeStr = strings.SplitN(timeStr, "\n", 2)[0]
				timeStr = strings.TrimSpace(timeStr)
				
				if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
					sendTime = t
				}
			}
			
			result := MetricResult{
				MessageID:       msgID,
				MessageSize:     size,
				Status:          "received",
				SenderNode:      senderNodeID,
				ReceiverNode:    config.NodeID,
				SequenceNum:     seqNum,
				ReceiveTime:     receiveTime,
				SendTime:        sendTime,
				ExperimentType:  expType,
				BatchID:         batchID,
			}
			
			// Calculate latency if we have send time
			if !sendTime.IsZero() {
				result.LatencyDuration = receiveTime.Sub(sendTime)
				log.Printf("Message %s latency: %v", msgID, result.LatencyDuration)
			}
			
			// For ordering tests, track sequence numbers
			if expType == OrderingTest {
				key := fmt.Sprintf("%s-%s", senderNodeID, batchID)
				
				if _, exists := receivedMessages[key]; !exists {
					receivedMessages[key] = make([]int, 0, config.MessageCount)
				}
				
				receivedMessages[key] = append(receivedMessages[key], seqNum)
				
				// Check for end of sequence marker
				if strings.Contains(msg.Content, "END_SEQUENCE") {
					// Verify ordering
					ordered := true
					for i := 1; i < len(receivedMessages[key]); i++ {
						if receivedMessages[key][i] <= receivedMessages[key][i-1] {
							ordered = false
							break
						}
					}
					
					log.Printf("Ordering test from %s batch %s: received %d messages, ordered=%v", 
						senderNodeID, batchID, len(receivedMessages[key]), ordered)
					
					// Clear this sequence
					delete(receivedMessages, key)
				}
			}
			
			results <- result
			log.Printf("Received message: %s from %s (%d bytes)", msg.DataName, msg.Owner, size)
			
		case encMsg, ok := <-encryptedMessages:
			if !ok {
				continue
			}
			
			// Similar parsing for encrypted messages
			parts := strings.Split(encMsg.DataName, "-")
			if len(parts) < 7 || parts[0] != "EXP" {
				continue
			}
			
			expType := ExperimentType(parts[1])
			size := 0
			seqNum := 0
			fmt.Sscanf(parts[2], "%d", &size)
			fmt.Sscanf(parts[3], "%d", &seqNum)
			senderNodeID := parts[4]
			batchID := parts[5]
			msgID := parts[6]
			
			// Skip if not matching our experiment type
			if expType != config.ExperimentType {
				continue
			}
			
			result := MetricResult{
				MessageID:      msgID,
				MessageSize:    size,
				Status:         "encrypted_received",
				SenderNode:     senderNodeID,
				ReceiverNode:   config.NodeID,
				SequenceNum:    seqNum,
				ReceiveTime:    time.Now(),
				ExperimentType: expType,
				BatchID:        batchID,
			}
			
			results <- result
			log.Printf("Received encrypted message: %s from %s", encMsg.DataName, encMsg.Owner)
		}
	}
}

// processResults writes results to files and logs summary statistics
func processResults(ctx context.Context, results <-chan MetricResult, config ExperimentConfig) {
	// Create result files by experiment type and batch
	resultFiles := make(map[string]*os.File)
	
	// Ensure cleanup of files on exit
	defer func() {
		for _, file := range resultFiles {
			file.Close()
		}
	}()
	
	// Initialize JSON encoder for each file
	encoders := make(map[string]*json.Encoder)
	
	// Track statistics by message size
	stats := make(map[int]struct {
		Count        int
		TotalLatency time.Duration
		MinLatency   time.Duration
		MaxLatency   time.Duration
		TotalUpload  time.Duration
		SuccessCount int
		FailCount    int
	})
	
	for {
		select {
		case <-ctx.Done():
			// Log final statistics on shutdown
			logFinalStats(stats)
			return
			
		case result, ok := <-results:
			if !ok {
				continue
			}
			
			// Create key for this batch
			fileKey := fmt.Sprintf("%s-%s", result.ExperimentType, result.BatchID)
			
			// Create file if it doesn't exist
			if _, exists := resultFiles[fileKey]; !exists {
				filename := fmt.Sprintf("%s/%s-%s-%s.jsonl", 
					config.ResultsDir, result.ExperimentType, config.NodeID, result.BatchID)
				
				file, err := os.Create(filename)
				if err != nil {
					log.Printf("Error creating result file: %v", err)
					continue
				}
				
				resultFiles[fileKey] = file
				encoders[fileKey] = json.NewEncoder(file)
				log.Printf("Created result file: %s", filename)
			}
			
			// Write result to file
			if err := encoders[fileKey].Encode(result); err != nil {
				log.Printf("Error writing result: %v", err)
			}
			
			// Update statistics
			size := result.MessageSize
			stat := stats[size]
			
			if result.Status == "received" && result.LatencyDuration > 0 {
				stat.Count++
				stat.TotalLatency += result.LatencyDuration
				
				// Update min/max latency
				if stat.MinLatency == 0 || result.LatencyDuration < stat.MinLatency {
					stat.MinLatency = result.LatencyDuration
				}
				if result.LatencyDuration > stat.MaxLatency {
					stat.MaxLatency = result.LatencyDuration
				}
			}
			
			if result.UploadDuration > 0 {
				stat.TotalUpload += result.UploadDuration
			}
			
			if strings.HasPrefix(result.Status, "success") {
				stat.SuccessCount++
			} else if strings.HasPrefix(result.Status, "error") {
				stat.FailCount++
			}
			
			stats[size] = stat
		}
	}
}

// logFinalStats outputs summary statistics at the end of the experiment
func logFinalStats(stats map[int]struct {
	Count        int
	TotalLatency time.Duration
	MinLatency   time.Duration
	MaxLatency   time.Duration
	TotalUpload  time.Duration
	SuccessCount int
	FailCount    int
}) {
	log.Println("===== Experiment Summary =====")
	
	for size, stat := range stats {
		log.Printf("Message size: %d bytes", size)
		
		if stat.Count > 0 {
			avgLatency := stat.TotalLatency / time.Duration(stat.Count)
			log.Printf("  Latency: avg=%v min=%v max=%v count=%d", 
				avgLatency, stat.MinLatency, stat.MaxLatency, stat.Count)
		}
		
		totalCount := stat.SuccessCount + stat.FailCount
		if totalCount > 0 {
			successRate := float64(stat.SuccessCount) / float64(totalCount) * 100
			log.Printf("  Success rate: %.2f%% (%d/%d)", 
				successRate, stat.SuccessCount, totalCount)
			
			if stat.SuccessCount > 0 {
				avgUpload := stat.TotalUpload / time.Duration(stat.SuccessCount)
				log.Printf("  Average upload time: %v", avgUpload)
			}
		}
		
		log.Println("-----------------------------")
	}
	
	log.Println("==============================")
}

// runLatencyExperiment focuses on measuring precise message delivery timing
func runLatencyExperiment(ctx context.Context, 
                        c *contract.ContractInteractionInterface,
                        config ExperimentConfig,
                        results chan<- MetricResult,
                        wg *sync.WaitGroup,
                        mp *msgProcessor.MessageProcessor) {
	defer wg.Done()
	
	// Track dependencies if enabled
	var dependencies [][]byte
	if config.UseDependencies {
		dependencies = make([][]byte, 0)
	}
	
	log.Printf("Starting latency experiment with %d sizes, %d messages each", 
		len(config.MessageSizes), config.MessageCount)
	
	for _, size := range config.MessageSizes {
		log.Printf("Testing latency with message size: %d bytes", size)
		
		for i := 0; i < config.MessageCount; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				msgID := fmt.Sprintf("%d", time.Now().UnixNano())
				
				// Precise timestamp for latency measurement
				sendTime := time.Now()
				timeStr := sendTime.Format(time.RFC3339Nano)
				
				// Create message with embedded timestamp
				message := fmt.Sprintf("TIMESTAMP: %s\n%s", 
					timeStr, generateRandomContent(size - 100)) // Reserve space for timestamp
				
				owner := fmt.Sprintf("Sender-%s", config.NodeID)
				dataName := fmt.Sprintf("EXP-%s-%d-%d-%s-%s-%s", 
					LatencyTest, size, i, config.NodeID, config.BatchID, msgID)
				
				// Use shorter release time for latency tests
				releaseTime := time.Now().Add(1 * time.Minute).Unix()
				
				// Upload the message
				uploadStart := time.Now()
				err := c.Upload(
					message,
					owner,
					dataName,
					releaseTime,
					dependencies,
					config.SecurityLevel,
				)
				uploadDuration := time.Since(uploadStart)
				
				// Record result
				result := MetricResult{
					MessageID:      msgID,
					MessageSize:    size,
					SequenceNum:    i,
					SenderNode:     config.NodeID,
					SendTime:       sendTime,
					UploadTime:     uploadStart,
					UploadDuration: uploadDuration,
					ExperimentType: LatencyTest,
					BatchID:        config.BatchID,
				}
				
				if err != nil {
					log.Printf("Error uploading message %s: %v", dataName, err)
					result.Status = "error"
					result.ErrorMessage = err.Error()
				} else {
					log.Printf("Uploaded latency test message %d of %d (%d bytes)", 
						i+1, config.MessageCount, size)
					result.Status = "success"
					
					// Update dependencies if needed
					if config.UseDependencies && mp != nil {
						time.Sleep(200 * time.Millisecond) // Brief wait for processing
						if hash, ok := mp.GetDependencyHash(dataName); ok {
							dependencies = append(dependencies, hash)
							// Keep only the last 5 dependencies
							if len(dependencies) > 5 {
								dependencies = dependencies[len(dependencies)-5:]
							}
						}
					}
				}
				
				results <- result
				
				// Use configured interval
				time.Sleep(config.Interval)
			}
		}
		
		// Pause between different message sizes
		log.Printf("Completed latency test for %d bytes", size)
		time.Sleep(5 * time.Second)
	}
	
	log.Printf("Completed latency experiment")
}

// runThroughputExperiment focuses on maximum message throughput
func runThroughputExperiment(ctx context.Context, 
                           c *contract.ContractInteractionInterface,
                           config ExperimentConfig,
                           results chan<- MetricResult,
                           wg *sync.WaitGroup,
                           mp *msgProcessor.MessageProcessor) {
	defer wg.Done()
	
	// Track dependencies if enabled
	var dependencies [][]byte
	if config.UseDependencies {
		dependencies = make([][]byte, 0)
	}
	
	log.Printf("Starting throughput experiment with %d sizes, %d messages each", 
		len(config.MessageSizes), config.MessageCount)
	
	// Use shorter interval for burst mode
	interval := config.Interval
	if config.BurstMode {
		interval = 50 * time.Millisecond
	}
	
	for _, size := range config.MessageSizes {
		log.Printf("Testing throughput with message size: %d bytes", size)
		
		// Pre-generate messages to minimize generation overhead
		messages := make([]string, config.MessageCount)
		for i := 0; i < config.MessageCount; i++ {
			messages[i] = generateRandomContent(size)
		}
		
		startTime := time.Now()
		
		for i := 0; i < config.MessageCount; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				msgID := fmt.Sprintf("%d", time.Now().UnixNano())
				
				// Use pre-generated message
				message := fmt.Sprintf("TIMESTAMP: %s\n%s", 
					time.Now().Format(time.RFC3339Nano), messages[i])
				
				owner := fmt.Sprintf("Sender-%s", config.NodeID)
				dataName := fmt.Sprintf("EXP-%s-%d-%d-%s-%s-%s", 
					ThroughputTest, size, i, config.NodeID, config.BatchID, msgID)
				
				releaseTime := time.Now().Add(5 * time.Minute).Unix()
				
				// Upload the message
				uploadStart := time.Now()
				err := c.Upload(
					message,
					owner,
					dataName,
					releaseTime,
					dependencies,
					config.SecurityLevel,
				)
				uploadDuration := time.Since(uploadStart)
				
				// Record result
				result := MetricResult{
					MessageID:      msgID,
					MessageSize:    size,
					SequenceNum:    i,
					SenderNode:     config.NodeID,
					SendTime:       time.Now(),
					UploadTime:     uploadStart,
					UploadDuration: uploadDuration,
					ExperimentType: ThroughputTest,
					BatchID:        config.BatchID,
				}
				
				if err != nil {
					result.Status = "error"
					result.ErrorMessage = err.Error()
				} else {
					result.Status = "success"
					
					// Update dependencies if needed (but only occasionally to maintain throughput)
					if config.UseDependencies && mp != nil && i%5 == 0 {
						if hash, ok := mp.GetDependencyHash(dataName); ok {
							dependencies = append(dependencies, hash)
							if len(dependencies) > 3 {
								dependencies = dependencies[len(dependencies)-3:]
							}
						}
					}
				}
				
				results <- result
				
				// Use minimal interval for throughput test
				time.Sleep(interval)
			}
		}
		
		elapsed := time.Since(startTime)
		throughput := float64(config.MessageCount) / elapsed.Seconds()
		throughputBytes := float64(config.MessageCount*size) / elapsed.Seconds() / 1024
		
		log.Printf("Throughput results for %d bytes: %.2f msgs/sec, %.2f KB/sec", 
			size, throughput, throughputBytes)
		
		// Pause between different message sizes
		time.Sleep(5 * time.Second)
	}
	
	log.Printf("Completed throughput experiment")
}

// runOrderingExperiment focuses on message delivery order consistency
func runOrderingExperiment(ctx context.Context, 
                         c *contract.ContractInteractionInterface,
                         config ExperimentConfig,
                         results chan<- MetricResult,
                         wg *sync.WaitGroup,
                         mp *msgProcessor.MessageProcessor) {
	defer wg.Done()
	
	// For ordering tests, we run multiple sequences
	numSequences := 5
	size := config.MessageSizes[0] // Use first size for ordering tests
	
	log.Printf("Starting ordering experiment with %d sequences of %d messages each", 
		numSequences, config.MessageCount)
	
	for seq := 0; seq < numSequences; seq++ {
		log.Printf("Starting sequence %d of %d", seq+1, numSequences)
		
		// New batch ID for each sequence
		batchID := fmt.Sprintf("%s-seq%d", config.BatchID, seq)
		
		// Strict dependencies for ordering test
		var dependencies [][]byte
		if config.UseDependencies {
			dependencies = make([][]byte, 0, 1) // Only depend on previous message
		}
		
		for i := 0; i < config.MessageCount; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				msgID := fmt.Sprintf("%d", time.Now().UnixNano())
                // Create message with sequence information
				message := fmt.Sprintf("TIMESTAMP: %s\nSEQUENCE: %d\nORDER: %d\n%s", 
					time.Now().Format(time.RFC3339Nano), seq, i, generateRandomContent(size-200))
				
				// Mark the last message in sequence
				if i == config.MessageCount-1 {
					message += "\nEND_SEQUENCE"
				}
				
				owner := fmt.Sprintf("Sender-%s", config.NodeID)
				dataName := fmt.Sprintf("EXP-%s-%d-%d-%s-%s-%s", 
					OrderingTest, size, i, config.NodeID, batchID, msgID)
				
				// Use increasing release times to enforce ordering
				releaseTime := time.Now().Add(time.Duration(1+i) * time.Minute).Unix()
				
				// Upload the message
				uploadStart := time.Now()
				err := c.Upload(
					message,
					owner,
					dataName,
					releaseTime,
					dependencies,
					config.SecurityLevel,
				)
				uploadDuration := time.Since(uploadStart)
				
				// Record result
				result := MetricResult{
					MessageID:      msgID,
					MessageSize:    size,
					SequenceNum:    i,
					SenderNode:     config.NodeID,
					SendTime:       time.Now(),
					UploadTime:     uploadStart,
					UploadDuration: uploadDuration,
					ExperimentType: OrderingTest,
					BatchID:        batchID,
				}
				
				if err != nil {
					log.Printf("Error uploading ordering message %d: %v", i, err)
					result.Status = "error"
					result.ErrorMessage = err.Error()
				} else {
					log.Printf("Uploaded ordering message %d of %d (sequence %d)", 
						i+1, config.MessageCount, seq+1)
					result.Status = "success"
					
					// For ordering tests, strictly depend on previous message
					if config.UseDependencies && mp != nil {
						time.Sleep(300 * time.Millisecond) // Ensure previous message is processed
						if hash, ok := mp.GetDependencyHash(dataName); ok {
							dependencies = [][]byte{hash} // Only depend on the previous message
						}
					}
				}
				
				results <- result
				
				// Brief interval between messages in same sequence
				time.Sleep(200 * time.Millisecond)
			}
		}
		
		log.Printf("Completed sequence %d of %d", seq+1, numSequences)
		// Pause between sequences
		time.Sleep(10 * time.Second)
	}
	
	log.Printf("Completed ordering experiment")
}

// runScalabilityExperiment focuses on handling large messages and system capacity
func runScalabilityExperiment(ctx context.Context, 
                             c *contract.ContractInteractionInterface,
                             config ExperimentConfig,
                             results chan<- MetricResult,
                             wg *sync.WaitGroup,
                             mp *msgProcessor.MessageProcessor) {
	defer wg.Done()
	
	// Track dependencies if enabled
	var dependencies [][]byte
	if config.UseDependencies {
		dependencies = make([][]byte, 0)
	}
	
	log.Printf("Starting scalability experiment with %d message sizes", len(config.MessageSizes))
	
	// For each message size (focusing on larger sizes)
	for _, size := range config.MessageSizes {
		log.Printf("Testing scalability with message size: %d bytes", size)
		
		// Adjust count based on message size to avoid blockchain size limits
		adjustedCount := config.MessageCount
		if size > 262144 { // 256KB
			adjustedCount = config.MessageCount / 2
		}
		if size > 524288 { // 512KB
			adjustedCount = config.MessageCount / 4
		}
		
		log.Printf("Using adjusted count of %d messages for size %d bytes", adjustedCount, size)
		
		// Use slightly longer interval for large messages
		interval := config.Interval
		if size > 262144 {
			interval = interval * 2
		}
		
		// Pre-generate content to avoid overhead during sending
		contents := make([]string, adjustedCount)
		for i := 0; i < adjustedCount; i++ {
			contents[i] = generateRandomContent(size)
		}
		
		startTime := time.Now()
		
		for i := 0; i < adjustedCount; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				msgID := fmt.Sprintf("%d", time.Now().UnixNano())
				
				// Use pre-generated content with timestamp
				message := fmt.Sprintf("TIMESTAMP: %s\n%s", 
					time.Now().Format(time.RFC3339Nano), contents[i])
				
				owner := fmt.Sprintf("Sender-%s", config.NodeID)
				dataName := fmt.Sprintf("EXP-%s-%d-%d-%s-%s-%s", 
					ScalabilityTest, size, i, config.NodeID, config.BatchID, msgID)
				
				// Longer release time for larger messages
				releaseTime := time.Now().Add(10 * time.Minute).Unix()
				
				// Upload the message and measure time
				uploadStart := time.Now()
				err := c.Upload(
					message,
					owner,
					dataName,
					releaseTime,
					dependencies,
					config.SecurityLevel,
				)
				uploadDuration := time.Since(uploadStart)
				
				// Record result
				result := MetricResult{
					MessageID:      msgID,
					MessageSize:    size,
					SequenceNum:    i,
					SenderNode:     config.NodeID,
					SendTime:       time.Now(),
					UploadTime:     uploadStart,
					UploadDuration: uploadDuration,
					ExperimentType: ScalabilityTest,
					BatchID:        config.BatchID,
				}
				
				if err != nil {
					log.Printf("Error uploading scalability message %d: %v", i, err)
					result.Status = "error"
					result.ErrorMessage = err.Error()
				} else {
					log.Printf("Uploaded scalability message %d of %d (%d bytes) in %v", 
						i+1, adjustedCount, size, uploadDuration)
					result.Status = "success"
					
					// Update dependencies occasionally
					if config.UseDependencies && mp != nil && i%3 == 0 {
						time.Sleep(200 * time.Millisecond)
						if hash, ok := mp.GetDependencyHash(dataName); ok {
							dependencies = append(dependencies, hash)
							if len(dependencies) > 2 { // Keep fewer dependencies for large messages
								dependencies = dependencies[len(dependencies)-2:]
							}
						}
					}
				}
				
				results <- result
				
				// Use adjusted interval for large messages
				time.Sleep(interval)
			}
		}
		
		elapsed := time.Since(startTime)
		throughput := float64(adjustedCount) / elapsed.Seconds()
		throughputMB := float64(adjustedCount*size) / elapsed.Seconds() / (1024 * 1024)
		
		log.Printf("Scalability results for %d bytes: sent %d messages in %v", 
			size, adjustedCount, elapsed)
		log.Printf("Average throughput: %.2f msgs/sec, %.2f MB/sec", 
			throughput, throughputMB)
		
		// Longer pause between size changes for large messages
		pauseTime := 10 * time.Second
		if size > 262144 {
			pauseTime = 30 * time.Second
		}
		time.Sleep(pauseTime)
	}
	
	log.Printf("Completed scalability experiment")
}

// generateRandomContent creates random content of specified size
func generateRandomContent(size int) string {
	if size <= 0 {
		return ""
	}
	
	// Reserve 100 bytes for timestamp and other metadata
	if size < 100 {
		size = 100
	}
	
	// Character set for random data
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const charsetLen = len(charset)
	
	// Create a buffer for efficiency
	buffer := make([]byte, size)
	
	// Generate random content
	for i := 0; i < size; i++ {
		buffer[i] = charset[rand.Intn(charsetLen)]
	}
	
	return string(buffer)
}
