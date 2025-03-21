package experimenttypes

import (
    "time"
)

type ExperimentID string

const (
    ThroughputVsOrdering   ExperimentID = "throughput_vs_ordering"
    FaultToleranceTest     ExperimentID = "fault_tolerance"
    GlobalVsLocalTest      ExperimentID = "global_vs_local"
    EpochGranularityTest   ExperimentID = "epoch_granularity"
    BlockchainOverheadTest ExperimentID = "blockchain_overhead"
)

type ExperimentConfig struct {
    ExperimentID    ExperimentID `json:"experiment_id"`
    NodeID          string       `json:"node_id"`
    Region          string       `json:"region"`
    NumNodes        int          `json:"num_nodes"`
    EpochDurationMs int          `json:"epoch_duration_ms"`
    MessageRate     int          `json:"message_rate"` // messages per second
    TestDuration    time.Duration `json:"test_duration"` // how long to run the test
    EnableOrdering  bool         `json:"enable_ordering"`
    Byzantine       bool         `json:"byzantine"` // should this node behave incorrectly
    UploadNode      bool         `json:"upload_node"` // if this node uploads new causal chains
}

type ExperimentResult struct {
    NodeID         string             `json:"node_id"`
    Region         string             `json:"region"`
    ExperimentID   ExperimentID       `json:"experiment_id"`
    Metrics        map[string]float64 `json:"metrics"`
    Timestamp      string             `json:"timestamp"`
    RawLogs        string             `json:"raw_logs,omitempty"`
}

