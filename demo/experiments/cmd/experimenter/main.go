package main

import (
    "fmt"
    "log"
    "os"
    "strconv"
    "time"

    "github.com/joho/godotenv"
    "experiments/pkg/experimenttypes"
    "experiments/pkg/runner"
)

func main() {
    fmt.Println("🚀 Experimenter node starting up...")

    if err := godotenv.Load(); err != nil {
        log.Println("⚠️  .env file not found, falling back to environment variables...")
    }

    config, err := loadExperimentConfig()
    if err != nil {
        log.Fatalf("❌ Failed to load experiment config: %v", err)
    }

    if err := runner.RunExperiment(config); err != nil {
        log.Fatalf("❌ Experiment execution failed: %v", err)
    }

    fmt.Println("🎉 Experiment completed successfully")
}

func loadExperimentConfig() (experimenttypes.ExperimentConfig, error) {
    nodeID := mustGetEnv("NODE_ID")
    region := mustGetEnv("REGION")
    expID := mustGetEnv("EXPERIMENT_ID")

    numNodes, err := strconv.Atoi(getEnv("NUM_NODES", "10"))
    if err != nil {
        return experimenttypes.ExperimentConfig{}, fmt.Errorf("invalid NUM_NODES")
    }

    epochMs, err := strconv.Atoi(getEnv("EPOCH_DURATION_MS", "1000"))
    if err != nil {
        return experimenttypes.ExperimentConfig{}, fmt.Errorf("invalid EPOCH_DURATION_MS")
    }

    msgRate, err := strconv.Atoi(getEnv("MESSAGE_RATE", "5"))
    if err != nil {
        return experimenttypes.ExperimentConfig{}, fmt.Errorf("invalid MESSAGE_RATE")
    }

    durationSec, err := strconv.Atoi(getEnv("TEST_DURATION", "60"))
    if err != nil {
        return experimenttypes.ExperimentConfig{}, fmt.Errorf("invalid TEST_DURATION")
    }

    enableOrdering := getEnv("ENABLE_ORDERING", "false") == "true"
    byzantine := getEnv("BYZANTINE", "false") == "true"
    uploadNode := getEnv("UPLOAD_NODE", "false") == "true"

    return experimenttypes.ExperimentConfig{
        ExperimentID:    experimenttypes.ExperimentID(expID),
        NodeID:          nodeID,
        Region:          region,
        NumNodes:        numNodes,
        EpochDurationMs: epochMs,
        MessageRate:     msgRate,
        TestDuration:    time.Duration(durationSec) * time.Second,
        EnableOrdering:  enableOrdering,
        Byzantine:       byzantine,
        UploadNode:      uploadNode,
    }, nil
}

func mustGetEnv(key string) string {
    val := os.Getenv(key)
    if val == "" {
        log.Fatalf("missing required environment variable: %s", key)
    }
    return val
}

func getEnv(key, fallback string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return fallback
}

