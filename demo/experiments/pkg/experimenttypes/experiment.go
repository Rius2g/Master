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
    NodeID            string        `json:"node_id"`
    Region            string        `json:"region"`
    ExperimentID      ExperimentID  `json:"experiment_id"`
    TestDuration      time.Duration `json:"test_duration"`
    MessageRate       int           `json:"message_rate"`
    Byzantine         bool          `json:"byzantine"`
    SecurityLevel     int           `json:"security_level"`
    NumNodes          int           `json:"num_nodes"`
    EpochDurationMs   int           `json:"epoch_duration_ms"`
    EnableOrdering    bool          `json:"enable_ordering"`
    UploadNode        bool `json:"upload_node"`
}


type ExperimentResult struct {
    NodeID         string             `json:"node_id"`
    Region         string             `json:"region"`
    ExperimentID   ExperimentID       `json:"experiment_id"`
    Metrics        map[string]float64 `json:"metrics"`
    Timestamp      string             `json:"timestamp"`
    RawLogs        string             `json:"raw_logs,omitempty"`
}

