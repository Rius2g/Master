package main

import (
    "log"
    "net/http"
    "os"

    "github.com/gorilla/mux"
    "experiments/pkg/handler"
    "experiments/pkg/storage"
)

func main() {
    port := getPort()
    store := storage.NewStore()
    apiHandler := handler.NewHandler(store)

    r := mux.NewRouter()

    // Routes
    r.HandleFunc("/submit-result", apiHandler.SubmitResult).Methods("POST")
    r.HandleFunc("/results", apiHandler.GetResults).Methods("GET")
    r.HandleFunc("/results/{node_id}", apiHandler.GetResultsByNode).Methods("GET")

    log.Printf("Collector API running on port %s\n", port)
    log.Fatal(http.ListenAndServe(":"+port, r))
}

func getPort() string {
    port := os.Getenv("PORT")
    if port == "" {
        return "8080"
    }
    return port
}

