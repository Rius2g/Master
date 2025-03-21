package metrics

import (
    "sort"
    "sync"
    "time"
)

type MetricCollector struct {
    mu              sync.Mutex
    messageTimes    []time.Time
    messageLatency  []float64
    causalViolations int
    startTime       time.Time
}

// Create a new collector
func NewMetricCollector() *MetricCollector {
    return &MetricCollector{
        messageTimes:   []time.Time{},
        messageLatency: []float64{},
        startTime:      time.Now(),
    }
}

// Simulate recording a message receive/send event
func (mc *MetricCollector) TrackMessage(t time.Time) {
    mc.mu.Lock()
    defer mc.mu.Unlock()
    mc.messageTimes = append(mc.messageTimes, t)
}

// Optional: simulate recording a latency value (e.g., difference from first node)
func (mc *MetricCollector) TrackLatency(deltaMs float64) {
    mc.mu.Lock()
    defer mc.mu.Unlock()
    mc.messageLatency = append(mc.messageLatency, deltaMs)
}

// Optional: simulate recording a causal violation (for byzantine tests)
func (mc *MetricCollector) RecordCausalViolation() {
    mc.mu.Lock()
    defer mc.mu.Unlock()
    mc.causalViolations++
}

// Calculate and aggregate metrics
func (mc *MetricCollector) Compute() map[string]float64 {
    mc.mu.Lock()
    defer mc.mu.Unlock()

    metrics := make(map[string]float64)

    // Latency Spread (∆L)
    if len(mc.messageLatency) > 0 {
        min, max := mc.messageLatency[0], mc.messageLatency[0]
        for _, l := range mc.messageLatency {
            if l < min {
                min = l
            }
            if l > max {
                max = l
            }
        }
        metrics["latency_spread_ms"] = max - min
    }

    // Throughput (messages/sec)
    durationSec := time.Since(mc.startTime).Seconds()
    if durationSec > 0 {
        throughput := float64(len(mc.messageTimes)) / durationSec
        metrics["throughput_msgs_per_sec"] = throughput
    }

    // Causal ordering violations
    metrics["causal_violations"] = float64(mc.causalViolations)

    // Optional - 95th percentile latency spread
    if len(mc.messageLatency) >= 5 {
        sorted := append([]float64{}, mc.messageLatency...)
        sort.Float64s(sorted)
        idx := int(0.95 * float64(len(sorted)))
        metrics["p95_latency_ms"] = sorted[idx]
    }

    return metrics
}

