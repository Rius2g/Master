package collector

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "time"

    "experiments/pkg/experimenttypes"
)

type Client struct {
    CollectorURL string
    HttpClient   *http.Client
}

func NewClient(collectorURL string) *Client {
    return &Client{
        CollectorURL: collectorURL,
        HttpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
    }
}

// SubmitResult sends a single experiment result to the collector API
func (c *Client) SubmitResult(result experimenttypes.ExperimentResult) error {
    jsonValue, err := json.Marshal(result)
    if err != nil {
        return fmt.Errorf("failed to marshal experiment result: %v", err)
    }

    endpoint := fmt.Sprintf("%s/submit-result", c.CollectorURL)
    req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonValue))
    if err != nil {
        return fmt.Errorf("failed to create POST request: %v", err)
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.HttpClient.Do(req)
    if err != nil {
        return fmt.Errorf("failed to submit result to collector API: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("collector API returned non-201 status: %v", resp.Status)
    }

    fmt.Printf("✅ Successfully submitted result to %s\n", c.CollectorURL)
    return nil
}

// Helper function to load collector URL from env or fallback
func GetCollectorURL() string {
    if url := os.Getenv("COLLECTOR_URL"); url != "" {
        return url
    }
    return "http://collector-node:8080"
}

