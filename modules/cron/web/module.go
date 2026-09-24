package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	cronmodule "github.com/lsongdev/ohmyserver/modules/cron"
)

type Job = cronmodule.Job
type Execution = cronmodule.Execution
type Input = cronmodule.Input

type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "cron" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /jobs", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.service.CronJobs(r.Context())
		if err != nil {
			cronError(w, err)
			return
		}
		cronJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /jobs", func(w http.ResponseWriter, r *http.Request) {
		var in Input
		if decodeJSON(w, r, &in) != nil || !validInput(in) {
			cronProblem(w, 400, "invalid_job", "name, type, schedule and command are required; type must be cron or once and timeout cannot be negative")
			return
		}
		item, err := m.service.CreateCronJobAPI(r.Context(), in)
		if err != nil {
			cronError(w, err)
			return
		}
		cronJSON(w, 201, map[string]any{"data": item})
	})
	mux.HandleFunc("GET /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := cronID(r)
		if !ok {
			cronProblem(w, 400, "invalid_id", "job id must be a positive integer")
			return
		}
		item, err := m.service.CronJob(r.Context(), id)
		if err != nil {
			cronError(w, err)
			return
		}
		cronJSON(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("PUT /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := cronID(r)
		if !ok {
			cronProblem(w, 400, "invalid_id", "job id must be a positive integer")
			return
		}
		var in Input
		if decodeJSON(w, r, &in) != nil || !validInput(in) {
			cronProblem(w, 400, "invalid_job", "name, type, schedule and command are required; type must be cron or once and timeout cannot be negative")
			return
		}
		if err := m.service.UpdateCronJobAPI(r.Context(), id, in); err != nil {
			cronError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := cronID(r)
		if !ok {
			cronProblem(w, 400, "invalid_id", "job id must be a positive integer")
			return
		}
		if err := m.service.DeleteCronJobAPI(r.Context(), id); err != nil {
			cronError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /jobs/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		id, ok := cronID(r)
		if !ok {
			cronProblem(w, 400, "invalid_id", "job id must be a positive integer")
			return
		}
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if decodeJSON(w, r, &body) != nil || body.Enabled == nil {
			cronProblem(w, 400, "invalid_body", "enabled boolean is required")
			return
		}
		if err := m.service.SetCronJobEnabled(r.Context(), id, *body.Enabled); err != nil {
			cronError(w, err)
			return
		}
		cronJSON(w, 200, map[string]any{"data": map[string]bool{"enabled": *body.Enabled}})
	})
	mux.HandleFunc("POST /jobs/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		id, ok := cronID(r)
		if !ok {
			cronProblem(w, 400, "invalid_id", "job id must be a positive integer")
			return
		}
		if err := m.service.RunCronJobNow(r.Context(), id); err != nil {
			cronError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /jobs/{id}/executions", func(w http.ResponseWriter, r *http.Request) {
		id, ok := cronID(r)
		if !ok {
			cronProblem(w, 400, "invalid_id", "job id must be a positive integer")
			return
		}
		items, err := m.service.CronJobExecutions(r.Context(), id, 50)
		if err != nil {
			cronError(w, err)
			return
		}
		cronJSON(w, 200, map[string]any{"data": items})
	})
	return wrapModuleWeb(mux)
}

func validInput(in Input) bool {
	return strings.TrimSpace(in.Name) != "" && strings.TrimSpace(in.Schedule) != "" && strings.TrimSpace(in.Command) != "" && (in.Type == "cron" || in.Type == "once") && in.Timeout >= 0
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("request must contain one JSON value")
	}
	return nil
}
func cronID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}
func cronJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func cronProblem(w http.ResponseWriter, status int, code, message string) {
	cronJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func cronError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, sql.ErrNoRows) {
		status = http.StatusNotFound
	} else if errors.Is(err, ErrInvalidInput) {
		status = http.StatusBadRequest
	}
	cronProblem(w, status, "cron_error", err.Error())
}
