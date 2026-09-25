package telegraf

import (
	"encoding/json"
	"fmt"
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
