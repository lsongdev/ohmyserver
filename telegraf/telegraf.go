package telegraf

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ---------- Types ----------

// Metric represents a single measurement from Telegraf
type Metric struct {
	Name      string            `json:"name"`
	Tags      map[string]string `json:"tags"`
	Fields    map[string]any    `json:"fields"`
	Timestamp int64             `json:"timestamp"`
}

// Batch wraps the Telegraf JSON format: {"metrics": [...]}
type Batch struct {
	Metrics []Metric `json:"metrics"`
}

// ---------- Parsing ----------

// ParseMetrics decodes Telegraf's native JSON format into a Batch.
// Expected format: {"metrics": [{"name":"cpu","tags":{...},"fields":{...},"timestamp":...}, ...]}
func ParseMetrics(body []byte) (*Batch, error) {
	var batch Batch
	if err := json.Unmarshal(body, &batch); err != nil {
		return nil, fmt.Errorf("invalid telegraf json: %w", err)
	}
	if len(batch.Metrics) == 0 {
		return nil, fmt.Errorf("empty metrics")
	}
	return &batch, nil
}

// ---------- Helpers ----------

// ToFloat64 safely extracts a float64 from an any value
func ToFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint64:
		return float64(val), true
	case json.Number:
		f, _ := val.Float64()
		return f, true
	default:
		return 0, false
	}
}

// ToUint64 safely extracts a uint64 from an any value
func ToUint64(v any) (uint64, bool) {
	switch val := v.(type) {
	case float64:
		return uint64(val), true
	case uint64:
		return val, true
	case int64:
		return uint64(val), true
	case int:
		return uint64(val), true
	case json.Number:
		i, _ := val.Int64()
		return uint64(i), true
	default:
		return 0, false
	}
}

// ---------- Handler interface (Telegraf plugin compatibility) ----------

// Handler defines the interface for Telegraf metric handlers
type Handler interface {
	HandleMetrics(hostname string, metrics map[string]interface{}) error
}

// MetricsPayload is the JSON format accepted by the telegraf endpoint
type MetricsPayload struct {
	Hostname  string                 `json:"hostname"`
	Metrics   map[string]interface{} `json:"metrics"`
	Timestamp time.Time              `json:"timestamp"`
	Token     string                 `json:"token,omitempty"`
}

type handler struct {
	token    string
	callback func(hostname string, payload MetricsPayload) error
}

// NewHandler creates a new Telegraf handler
func NewHandler(token string, cb func(hostname string, payload MetricsPayload) error) Handler {
	return &handler{token: token, callback: cb}
}

func (h *handler) HandleMetrics(hostname string, metrics map[string]interface{}) error {
	return h.callback(hostname, MetricsPayload{
		Hostname:  hostname,
		Metrics:   metrics,
		Timestamp: time.Now(),
	})
}

// ListenAndServe starts an HTTP server that accepts Telegraf metrics
func ListenAndServe(addr string, h Handler) error {
	if h == nil {
		return fmt.Errorf("handler is nil")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload MetricsPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

		if payload.Hostname == "" {
			http.Error(w, "hostname is required", http.StatusBadRequest)
			return
		}

		if err := h.HandleMetrics(payload.Hostname, payload.Metrics); err != nil {
			log.Printf("telegraf handle error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	log.Printf("Telegraf listener starting on %s", addr)
	return http.ListenAndServe(addr, mux)
}
