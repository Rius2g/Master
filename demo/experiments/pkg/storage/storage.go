package storage

import (
    "sync"

    "experiments/pkg/experimenttypes"
)

type InMemoryStore struct {
    mu      sync.Mutex
    Results []experimenttypes.ExperimentResult
}

func NewStore() *InMemoryStore {
    return &InMemoryStore{}
}

func (s *InMemoryStore) AddResult(res experimenttypes.ExperimentResult) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.Results = append(s.Results, res)
}

func (s *InMemoryStore) GetAllResults() []experimenttypes.ExperimentResult {
    s.mu.Lock()
    defer s.mu.Unlock()
    return append([]experimenttypes.ExperimentResult{}, s.Results...)
}

func (s *InMemoryStore) GetResultsByNode(nodeID string) []experimenttypes.ExperimentResult {
    s.mu.Lock()
    defer s.mu.Unlock()

    var filtered []experimenttypes.ExperimentResult
    for _, r := range s.Results {
        if r.NodeID == nodeID {
            filtered = append(filtered, r)
        }
    }
    return filtered
}

