package runner

import (
    "fmt"
    "log"
    "time"

    "experiments/pkg/collector"
    "experiments/pkg/experimenttypes"
    "experiments/pkg/metrics"
)

// Main runner entry point
func RunExperiment(config experimenttypes.ExperimentConfig) error {
    log.Printf("Node %s starting experiment: %s\n", config.NodeID, config.ExperimentID)

    switch config.ExperimentID {
    case experimenttypes.ThroughputVsOrdering:
        return runThroughputVsOrdering(config)
    case experimenttypes.FaultToleranceTest:
        return runFaultTolerance(config)
    case experimenttypes.GlobalVsLocalTest:
        return runGlobalVsLocal(config)
    case experimenttypes.EpochGranularityTest:
        return runEpochGranularity(config)
    case experimenttypes.BlockchainOverheadTest:
        return runBlockchainOverhead(config)
    default:
        return fmt.Errorf("unknown experiment ID: %s", config.ExperimentID)
    }
}

// ----------------------
// Experiment 2
func runThroughputVsOrdering(config experimenttypes.ExperimentConfig) error {
    log.Printf("Running Throughput vs Ordering tradeoff...")

    results := metrics.NewMetricCollector()

    for i := 0; i < config.MessageRate*int(config.TestDuration.Seconds()); i++ {
        // Simulate message sending here...
        // (your actual logic would upload via ContractInteractionInterface)
        time.Sleep(time.Duration(1000/config.MessageRate) * time.Millisecond)
        results.TrackMessage(time.Now())
    }

    finalResults := results.Compute()
    return submitResult(config, finalResults)
}

// ----------------------
// Experiment 3
func runFaultTolerance(config experimenttypes.ExperimentConfig) error {
    log.Printf("Running Fault Tolerance and Byzantine Resilience...")

    results := metrics.NewMetricCollector()

    if config.Byzantine {
        log.Printf("Simulating Byzantine node behavior...")
        // Introduce randomized misordering or dropping logic here...
    }

    // Simulate causal-chain propagation, listening, etc.
    time.Sleep(config.TestDuration)

    finalResults := results.Compute()
    return submitResult(config, finalResults)
}

// ----------------------
// Experiment 4
func runGlobalVsLocal(config experimenttypes.ExperimentConfig) error {
    log.Printf("Running Global vs Local Dissemination Impact...")

    results := metrics.NewMetricCollector()

    // Simulate encrypted-data + private-key phase w/ geo considerations
    time.Sleep(config.TestDuration)

    finalResults := results.Compute()
    return submitResult(config, finalResults)
}

// ----------------------
// Experiment 5
func runEpochGranularity(config experimenttypes.ExperimentConfig) error {
    log.Printf("Running Epoch Granularity Sensitivity...")

    results := metrics.NewMetricCollector()

    // Simulate different epoch duration effects here
    time.Sleep(config.TestDuration)

    finalResults := results.Compute()
    return submitResult(config, finalResults)
}

// ----------------------
// Experiment 6
func runBlockchainOverhead(config experimenttypes.ExperimentConfig) error {
    log.Printf("Running Blockchain Overhead Profiling...")

    results := metrics.NewMetricCollector()

    // Compare native multicast to blockchain event-based delivery here
    time.Sleep(config.TestDuration)

    finalResults := results.Compute()
    return submitResult(config, finalResults)
}

// ----------------------
func submitResult(config experimenttypes.ExperimentConfig, data map[string]float64) error {
    res := experimenttypes.ExperimentResult{
        NodeID:       config.NodeID,
        Region:       config.Region,
        ExperimentID: config.ExperimentID,
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

