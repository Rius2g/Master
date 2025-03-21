package handler

import (
    "encoding/json"
    "net/http"

    "github.com/gorilla/mux"
    "experiments/pkg/storage"
    "experiments/pkg/experimenttypes"
)

type APIHandler struct {
    Store *storage.InMemoryStore
}

func NewHandler(store *storage.InMemoryStore) *APIHandler {
    return &APIHandler{Store: store}
}

// POST /submit-result
func (h *APIHandler) SubmitResult(w http.ResponseWriter, r *http.Request) {
    var result experimenttypes.ExperimentResult
    if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
        http.Error(w, "Invalid payload", http.StatusBadRequest)
        return
    }
    h.Store.AddResult(result)
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// GET /results
func (h *APIHandler) GetResults(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(h.Store.GetAllResults())
}

// GET /results/{node_id}
func (h *APIHandler) GetResultsByNode(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    nodeID := vars["node_id"]
    json.NewEncoder(w).Encode(h.Store.GetResultsByNode(nodeID))
}

