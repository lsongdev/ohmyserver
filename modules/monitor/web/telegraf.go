package web

import (
	"encoding/json"
	"io"
	"net/http"

	monitor "github.com/lsongdev/ohmyserver/modules/monitor"
	telegraflib "github.com/lsongdev/ohmyserver/modules/monitor/telegraf"
)

// TelegrafHandler accepts the existing external ingestion protocol and stores
// parsed samples through the monitor module's repository.
func TelegrafHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		batch, err := telegraflib.ParseMetrics(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		stats := monitor.StatsFromTelegraf(batch)
		if stats == nil {
			http.Error(w, "hostname not found in telegraf metrics", http.StatusBadRequest)
			return
		}
		if err = repo.Insert(r.Context(), stats); err != nil {
			http.Error(w, "failed to store stats", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
