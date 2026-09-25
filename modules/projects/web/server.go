package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Service interface {
	Projects(context.Context) ([]Project, error)
	Project(context.Context, int64) (Project, error)
	Deployments(context.Context, int64) ([]Deployment, error)
	Deployment(context.Context, int64) (Deployment, error)
	DeploymentLogs(context.Context, int64) ([]DeploymentLog, error)
	StartDeploymentAPI(context.Context, int64) (Deployment, error)
	CancelDeploymentAPI(context.Context, int64, int64) error
	ProjectRuntime(context.Context, int64) (RuntimeStatus, error)
	RotateWebhookSecret(context.Context, int64) (string, error)
	CreateProjectAPI(context.Context, Input) (Project, error)
	UpdateProjectAPI(context.Context, int64, Input) error
	DeleteProjectAPI(context.Context, int64) error
	HandleGitHubWebhook(context.Context, int64, string, string, string, []byte) (WebhookResponse, error)
}
type Module struct{ service Service }

type DeploymentService interface {
	StartDeploymentAPI(context.Context, int64) (Deployment, error)
	CancelDeploymentAPI(context.Context, int64, int64) error
	ProjectRuntime(context.Context, int64) (RuntimeStatus, error)
	StartGitHubDeployment(context.Context, *Project, string, string, string, string) (*Deployment, bool, error)
}

type Store interface {
	List(context.Context) ([]Project, error)
	Get(context.Context, int64) (*Project, error)
	Create(context.Context, Project) (*Project, error)
	Update(context.Context, Project) error
	Delete(context.Context, int64) error
	DeploymentsByProject(context.Context, int64) ([]Deployment, error)
	GetDeployment(context.Context, int64) (*Deployment, error)
	DeploymentLogs(context.Context, int64) ([]DeploymentLog, error)
}

func New(repo Store, deployments DeploymentService) *Module {
	return &Module{service: NewLocalService(repo, deployments)}
}
func (*Module) Name() string { return "projects" }

func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/projects/", http.StripPrefix("/v1/projects", m.Handler()))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, moduleWeb, "assets/index.html")
	})
	return mux
}
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/github/{id}", m.githubWebhook)
	mux.HandleFunc("GET /projects", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.service.Projects(r.Context())
		if err != nil {
			projectProblem(w, err)
			return
		}
		projectJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("GET /projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := projectID(r)
		if err != nil {
			projectFail(w, 400, "invalid_id", err.Error())
			return
		}
		item, err := m.service.Project(r.Context(), id)
		if err != nil {
			projectProblem(w, err)
			return
		}
		projectJSON(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("GET /projects/{id}/runtime", func(w http.ResponseWriter, r *http.Request) {
		id, err := projectID(r)
		if err != nil {
			projectFail(w, 400, "invalid_id", err.Error())
			return
		}
		status, err := m.service.ProjectRuntime(r.Context(), id)
		if err != nil {
			projectProblem(w, err)
			return
		}
		projectJSON(w, 200, map[string]any{"data": status})
	})
	mux.HandleFunc("POST /projects/{id}/webhook-secret", func(w http.ResponseWriter, r *http.Request) {
		id, err := projectID(r)
		if err != nil {
			projectFail(w, 400, "invalid_id", err.Error())
			return
		}
		secret, err := m.service.RotateWebhookSecret(r.Context(), id)
		if err != nil {
			projectProblem(w, err)
			return
		}
		projectJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"webhook_secret_once": secret}})
	})
	mux.HandleFunc("POST /projects", func(w http.ResponseWriter, r *http.Request) {
		var in Input
		if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" {
			projectFail(w, 400, "invalid_project", "project name is required")
			return
		}
		item, err := m.service.CreateProjectAPI(r.Context(), in)
		if err != nil {
			projectProblem(w, err)
			return
		}
		projectJSON(w, 201, map[string]any{"data": map[string]any{"project": item, "webhook_secret_once": item.WebhookSecret}})
	})
	mux.HandleFunc("PUT /projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := projectID(r)
		if err != nil {
			projectFail(w, 400, "invalid_id", err.Error())
			return
		}
		var in Input
		if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" {
			projectFail(w, 400, "invalid_project", "project name is required")
			return
		}
		if err = m.service.UpdateProjectAPI(r.Context(), id, in); err != nil {
			projectProblem(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := projectID(r)
		if err != nil {
			projectFail(w, 400, "invalid_id", err.Error())
			return
		}
		if err = m.service.DeleteProjectAPI(r.Context(), id); err != nil {
			projectProblem(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /projects/{id}/deployments", func(w http.ResponseWriter, r *http.Request) {
		id, err := projectID(r)
		if err != nil {
			projectFail(w, 400, "invalid_id", err.Error())
			return
		}
		items, err := m.service.Deployments(r.Context(), id)
		if err != nil {
			projectProblem(w, err)
			return
		}
		projectJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /projects/{id}/deployments", func(w http.ResponseWriter, r *http.Request) {
		id, err := projectID(r)
		if err != nil {
			projectFail(w, 400, "invalid_id", err.Error())
			return
		}
		item, err := m.service.StartDeploymentAPI(r.Context(), id)
		if err != nil {
			projectProblem(w, err)
			return
		}
		projectJSON(w, 202, map[string]any{"data": item})
	})
	mux.HandleFunc("GET /deployments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := projectID(r)
		if err != nil {
			projectFail(w, 400, "invalid_id", err.Error())
			return
		}
		item, err := m.service.Deployment(r.Context(), id)
		if err != nil {
			projectProblem(w, err)
			return
		}
		projectJSON(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("GET /deployments/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		id, err := projectID(r)
		if err != nil {
			projectFail(w, 400, "invalid_id", err.Error())
			return
		}
		items, err := m.service.DeploymentLogs(r.Context(), id)
		if err != nil {
			projectProblem(w, err)
			return
		}
		projectJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /projects/{projectID}/deployments/{deploymentID}/cancel", func(w http.ResponseWriter, r *http.Request) {
		projectID, err := strconv.ParseInt(r.PathValue("projectID"), 10, 64)
		if err != nil || projectID < 1 {
			projectFail(w, 400, "invalid_id", "project id must be positive")
			return
		}
		deploymentID, err := strconv.ParseInt(r.PathValue("deploymentID"), 10, 64)
		if err != nil || deploymentID < 1 {
			projectFail(w, 400, "invalid_id", "deployment id must be positive")
			return
		}
		if err = m.service.CancelDeploymentAPI(r.Context(), projectID, deploymentID); err != nil {
			projectProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /projects/{projectID}/deployments/{deploymentID}/events", func(w http.ResponseWriter, r *http.Request) {
		pid, err := strconv.ParseInt(r.PathValue("projectID"), 10, 64)
		if err != nil || pid < 1 {
			projectFail(w, 400, "invalid_id", "project id must be positive")
			return
		}
		did, err := strconv.ParseInt(r.PathValue("deploymentID"), 10, 64)
		if err != nil || did < 1 {
			projectFail(w, 400, "invalid_id", "deployment id must be positive")
			return
		}
		deployment, err := m.service.Deployment(r.Context(), did)
		if err != nil {
			projectProblem(w, err)
			return
		}
		if deployment.ProjectID != pid {
			projectFail(w, 404, "not_found", "deployment not found")
			return
		}
		serveDeploymentEvents(w, r, m.service, did)
	})
	return wrapModuleWeb(mux)
}
func serveDeploymentEvents(w http.ResponseWriter, r *http.Request, service Service, deploymentID int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	lastID, _ := strconv.ParseInt(r.URL.Query().Get("lastLogID"), 10, 64)
	if lastID == 0 {
		lastID, _ = strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	}
	sendWithID := func(event string, data any, id int64) bool {
		payload, err := json.Marshal(data)
		if err != nil {
			return false
		}
		if id > 0 {
			if _, err = fmt.Fprintf(w, "id: %d\n", id); err != nil {
				return false
			}
		}
		if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	send := func(event string, data any) bool { return sendWithID(event, data, 0) }
	if !send("connected", map[string]int64{"lastLogId": lastID}) {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			deployment, err := service.Deployment(r.Context(), deploymentID)
			if err != nil {
				send("problem", map[string]string{"message": "deployment not found"})
				return
			}
			if !send("status", map[string]string{"status": string(deployment.Status)}) {
				return
			}
			logs, err := service.DeploymentLogs(r.Context(), deploymentID)
			if err != nil {
				continue
			}
			for _, entry := range logs {
				if entry.ID > lastID {
					if !sendWithID("log", map[string]any{"id": entry.ID, "message": entry.Message}, entry.ID) {
						return
					}
					lastID = entry.ID
				}
			}
			switch deployment.Status {
			case DeploymentCompleted, DeploymentFailed, DeploymentCancelled:
				send("complete", map[string]string{"status": string(deployment.Status)})
				return
			}
		}
	}
}
func projectID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errInvalidProjectID
	}
	return id, nil
}

var errInvalidProjectID = &projectIDError{}

type projectIDError struct{}

func (*projectIDError) Error() string { return "project id must be a positive integer" }
func projectJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func projectFail(w http.ResponseWriter, status int, code, msg string) {
	projectJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}
func projectProblem(w http.ResponseWriter, err error) {
	status, code := 500, "projects_error"
	if errors.Is(err, sql.ErrNoRows) {
		status, code = 404, "not_found"
	}
	if errors.Is(err, ErrDeploymentInProgress) {
		status, code = 409, "deployment_in_progress"
	}
	projectFail(w, status, code, err.Error())
}
