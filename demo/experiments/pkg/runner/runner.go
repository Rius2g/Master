package runner

import (
    "fmt"
    "log"
    "os"
    "time"

    contract "github.com/rius2g/Master/backend/pkg/ContractInteractionInterface"
    "experiments/pkg/collector"
    "experiments/pkg/experimenttypes"
    "experiments/pkg/metrics"
)

// Main runner entry point
func RunExperiment(config experimenttypes.ExperimentConfig) error {
    log.Printf("Node %s starting experiment: %s\n", config.NodeID, config.ExperimentID)

    contractAddress := os.Getenv("CONTRACT_ADDRESS")
    privateKey := os.Getenv("PRIVATE_KEY")
    
    client, err := contract.Init(contractAddress, privateKey, uint(config.SecurityLevel))
    if err != nil {
        return fmt.Errorf("blockchain client init failed: %v", err)
    }

    switch experimenttypes.ExperimentID(config.ExperimentID) {
    case experimenttypes.ThroughputVsOrdering:
        return runThroughputVsOrdering(config, client)
    case experimenttypes.FaultToleranceTest:
        return runFaultTolerance(config, client)
    case experimenttypes.GlobalVsLocalTest:
        return runGlobalVsLocal(config, client)
    case experimenttypes.EpochGranularityTest:
        return runEpochGranularity(config, client)
    case experimenttypes.BlockchainOverheadTest:
        return runBlockchainOverhead(config, client)
    default:
        return fmt.Errorf("unknown experiment ID: %s", config.ExperimentID)
    }
}

// ----------------------
func runThroughputVsOrdering(config experimenttypes.ExperimentConfig, client *contract.ContractInteractionInterface) error {
    log.Printf("Running Throughput vs Ordering tradeoff...")

    results := metrics.NewMetricCollector()
    releaseTime := time.Now().Add(2 * time.Minute).Unix()

    for i := 0; i < config.MessageRate*int(config.TestDuration.Seconds()); i++ {
        msgName := fmt.Sprintf("node-%s-msg-%d", config.NodeID, i)
        start := time.Now()
        err := client.Upload(
            fmt.Sprintf("payload-%d", i),
            config.NodeID,
            msgName,
            releaseTime,
            nil,
            uint(config.SecurityLevel),
        )
        if err != nil {
            log.Printf("Upload failed: %v", err)
        }
        results.TrackLatency(time.Since(start).Seconds())
        time.Sleep(time.Duration(1000/config.MessageRate) * time.Millisecond)
    }

    return submitResult(config, results.Compute())
}

// ----------------------
func runFaultTolerance(config experimenttypes.ExperimentConfig, client *contract.ContractInteractionInterface) error {
    log.Printf("Running Fault Tolerance and Byzantine Resilience...")

    results := metrics.NewMetricCollector()
    for i := 0; i < config.MessageRate*int(config.TestDuration.Seconds()); i++ {
        msgName := fmt.Sprintf("fault-test-%s-msg-%d", config.NodeID, i)

        payload := fmt.Sprintf("normal-payload-%d", i)
        if config.Byzantine && i%5 == 0 {
            payload = "malformed_data"
        }

        start := time.Now()
        err := client.Upload(
            payload,
            config.NodeID,
            msgName,
            time.Now().Add(1*time.Minute).Unix(),
            nil,
            uint(config.SecurityLevel),
        )
        if err != nil {
            log.Printf("Upload failed: %v", err)
        }

        results.TrackLatency(time.Since(start).Seconds())
        time.Sleep(time.Duration(1000/config.MessageRate) * time.Millisecond)
    }

    return submitResult(config, results.Compute())
}

// ----------------------
func runGlobalVsLocal(config experimenttypes.ExperimentConfig, client *contract.ContractInteractionInterface) error {
    log.Printf("Running Global vs Local Dissemination Impact...")

    results := metrics.NewMetricCollector()
    for i := 0; i < config.MessageRate*int(config.TestDuration.Seconds()); i++ {
        msgName := fmt.Sprintf("geo-test-%s-msg-%d", config.NodeID, i)
        start := time.Now()
        err := client.Upload(
            fmt.Sprintf("geo-payload-%d", i),
            config.NodeID,
            msgName,
            time.Now().Add(90*time.Second).Unix(),
            nil,
            uint(config.SecurityLevel),
        )
        if err != nil {
            log.Printf("Upload failed: %v", err)
        }

        results.TrackLatency(time.Since(start).Seconds())
        time.Sleep(time.Duration(1000/config.MessageRate) * time.Millisecond)
    }

    return submitResult(config, results.Compute())
}

// ----------------------
func runEpochGranularity(config experimenttypes.ExperimentConfig, client *contract.ContractInteractionInterface) error {
    log.Printf("Running Epoch Granularity Sensitivity...")

    results := metrics.NewMetricCollector()
    epochDurations := []int64{10, 30, 60}

    for _, epoch := range epochDurations {
        for i := 0; i < config.MessageRate; i++ {
            msgName := fmt.Sprintf("epoch-%ds-%s-%d", epoch, config.NodeID, i)
            releaseTime := time.Now().Add(time.Duration(epoch) * time.Second).Unix()
            start := time.Now()
            err := client.Upload(
                fmt.Sprintf("epoch-sensitive-%d", i),
                config.NodeID,
                msgName,
                releaseTime,
                nil,
                uint(config.SecurityLevel),
            )
            if err != nil {
                log.Printf("Upload failed: %v", err)
            }

            results.TrackLatency(time.Since(start).Seconds())
            time.Sleep(1 * time.Second)
        }
    }

    return submitResult(config, results.Compute())
}

// ----------------------
func runBlockchainOverhead(config experimenttypes.ExperimentConfig, client *contract.ContractInteractionInterface) error {
    log.Printf("Running Blockchain Overhead Profiling...")

    results := metrics.NewMetricCollector()
    for i := 0; i < config.MessageRate*int(config.TestDuration.Seconds()); i++ {
        msgName := fmt.Sprintf("blockchain-latency-%s-%d", config.NodeID, i)
        start := time.Now()
        err := client.Upload(
            fmt.Sprintf("block-overhead-payload-%d", i),
            config.NodeID,
            msgName,
            time.Now().Add(60*time.Second).Unix(),
            nil,
            uint(config.SecurityLevel),
        )
        if err != nil {
            log.Printf("Upload failed: %v", err)
        }

        results.TrackLatency(time.Since(start).Seconds())
        time.Sleep(1 * time.Second)
    }

    return submitResult(config, results.Compute())
}

// ----------------------
func submitResult(config experimenttypes.ExperimentConfig, data map[string]float64) error {
    res := experimenttypes.ExperimentResult{
        NodeID:       config.NodeID,
        Region:       config.Region,
        ExperimentID: experimenttypes.ExperimentID(config.ExperimentID),
        Metrics:      data,
        Timestamp:    time.Now().Format(time.RFC3339),
    }

    apiClient := collector.NewClient(collector.GetCollectorURL())
    if err := apiClient.SubmitResult(res); err != nil {
        return fmt.Errorf("result submission failed: %v", err)
    }

    log.Printf("✅ Experiment result sent successfully.")
    return nil
}

