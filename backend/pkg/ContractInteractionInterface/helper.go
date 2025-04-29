package ContractInteraction

import (
	"encoding/json"
	"log"
	"time"
)

// Helper: JSON logging function
func LogJSON(record map[string]any) {
	record["timestamp"] = time.Now().Format(time.RFC3339Nano)
	bytes, err := json.Marshal(record)
	if err != nil {
		log.Printf("failed to marshal log record: %v", err)
		return
	}
	log.Println(string(bytes))
}
