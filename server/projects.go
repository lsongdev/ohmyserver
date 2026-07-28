package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var ErrDeploymentInProgress = errors.New("deployment already in progress")

func (s *Server) ProjectsView(w http.ResponseWriter, r *http.Request) {
	projects, err := s.GetAllProjects()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "projects/index", H{
		"projects": projects,
	})
}

func (s *Server) DeploymentsView(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		idStr := r.PathValue("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			s.Error(w, err)
			return
		}

		// Get project to access repo URL
		project, err := s.GetProjectByID(id)
		if err != nil {
			s.Error(w, fmt.Errorf("failed to get project: %w", err))
			return
		}

		if project.Repo == "" {
			s.Error(w, &Error{Message: "Repository URL is required. Please set it in project settings."})
			return
		}

		deployment, err := s.StartProjectDeployment(project, "Manual")
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrDeploymentInProgress) {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
			return
		}

		// Redirect to deployment detail page
		http.Redirect(w, r, "/projects/"+idStr+"/deployments/"+strconv.FormatInt(deployment.ID, 10), http.StatusSeeOther)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	project, err := s.GetProjectByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Get deployment history for this project
	deployments, err := s.GetDeploymentsByProject(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Convert deployments to template format
	deploymentList := make([]H, len(deployments))
	for i, d := range deployments {
		deploymentList[i] = H{
			"ID":        d.ID,
			"Name":      project.Name,
			"Repo":      project.Repo,
			"Status":    d.Status,
			"UpdatedAt": d.CreatedAt,
		}
	}

	s.Render(w, "projects/deployments", H{
		"project":     project,
		"deployments": deploymentList,
	})
}

func (s *Server) DeploymentDetailView(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	deploymentIdStr := r.PathValue("deploymentId")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	deploymentId, err := strconv.ParseInt(deploymentIdStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	project, err := s.GetProjectByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Get deployment from database
	deployment, err := s.GetDeployment(deploymentId)
	if err != nil {
		s.Error(w, err)
		return
	}
	if deployment.ProjectID != id {
		http.NotFound(w, r)
		return
	}

	// Get logs for this deployment
	logs, err := s.GetLogsByDeployment(deploymentId)
	if err != nil {
		logs = []DeploymentLog{}
	}

	logMessages := make([]string, len(logs))
	var maxLogID int64 = 0
	for i, log := range logs {
		logMessages[i] = log.Message
		if log.ID > maxLogID {
			maxLogID = log.ID
		}
	}

	// Format times for display
	var startedAt, duration string
	if deployment.StartedAt != nil {
		startedAt = deployment.StartedAt.Format("2006-01-02 15:04")
		if deployment.CompletedAt != nil {
			duration = deployment.CompletedAt.Sub(*deployment.StartedAt).Round(time.Second).String()
		} else {
			duration = time.Since(*deployment.StartedAt).Round(time.Second).String()
		}
	}

	s.Render(w, "projects/deployment_detail", H{
		"project": project,
		"deployment": H{
			"ID":          deployment.ID,
			"Status":      deployment.Status,
			"Commit":      deployment.Commit,
			"CommitMsg":   deployment.CommitMsg,
			"Author":      deployment.Author,
			"TriggeredBy": deployment.TriggeredBy,
			"StartedAt":   startedAt,
			"Duration":    duration,
			"Logs":        logMessages,
			"LastLogID":   maxLogID,
		},
	})
}

func (s *Server) ProjectDetailView(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodDelete {
		s.DeleteProjectHandler(w, r)
		return
	}

	// Extract project ID from URL path using Go 1.22+ path parameters
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	project, err := s.GetProjectByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Get runtime status
	runtimeStatus, err := s.GetProjectRuntimeStatus(id)
	if err != nil {
		runtimeStatus = &ProjectRuntimeStatus{Status: "unknown", Error: err.Error()}
	}

	// Get last deployment
	lastDeployment, err := s.GetLatestDeploymentByProject(id)
	if err != nil {
		lastDeployment = nil
	}

	s.Render(w, "projects/detail", H{
		"project":        project,
		"runtimeStatus":  runtimeStatus,
		"lastDeployment": lastDeployment,
	})
}

func (s *Server) NewProject(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "projects/new", H{})
		return
	}

	// Handle POST request
	err := r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := r.FormValue("description")
	repo := strings.TrimSpace(r.FormValue("repo"))

	if name == "" {
		s.Error(w, &Error{Message: "Project name is required"})
		return
	}

	_, err = s.CreateProject(name, description, repo)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

func (s *Server) EditProject(w http.ResponseWriter, r *http.Request) {
	// Extract project ID from URL path using Go 1.22+ path parameters
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		project, err := s.GetProjectByID(id)
		if err != nil {
			s.Error(w, err)
			return
		}
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		s.Render(w, "projects/settings", H{
			"project":    project,
			"webhookURL": fmt.Sprintf("%s://%s/hooks/github/%d", scheme, r.Host, project.ID),
		})
		return
	}

	// Handle POST request
	err = r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := r.FormValue("description")
	repo := strings.TrimSpace(r.FormValue("repo"))
	webhookBranch := strings.TrimSpace(r.FormValue("webhook_branch"))
	webhookSecret := strings.TrimSpace(r.FormValue("webhook_secret"))

	if name == "" {
		s.Error(w, &Error{Message: "Project name is required"})
		return
	}
	if webhookBranch == "" {
		webhookBranch = "main"
	}
	if !validGitHubBranch(webhookBranch) {
		s.Error(w, &Error{Message: "Invalid GitHub webhook branch"})
		return
	}
	if webhookSecret == "" {
		project, err := s.GetProjectByID(id)
		if err != nil {
			s.Error(w, err)
			return
		}
		webhookSecret = project.WebhookSecret
	}

	err = s.UpdateProject(id, name, description, repo, webhookBranch, webhookSecret)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/projects/"+idStr, http.StatusSeeOther)
}

func (s *Server) DeleteProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract project ID from URL path using Go 1.22+ path parameters
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	err = s.DeleteProject(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

// Error is a simple error type with a message
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// DeploymentLogsHandler handles SSE for real-time deployment logs
func (s *Server) DeploymentLogsHandler(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}
	deploymentIdStr := r.PathValue("deploymentId")
	deploymentId, err := strconv.ParseInt(deploymentIdStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}
	deployment, err := s.GetDeployment(deploymentId)
	if err != nil || deployment.ProjectID != projectID {
		http.NotFound(w, r)
		return
	}

	// Get lastLogId from query parameter
	lastLogID, err := strconv.ParseInt(r.URL.Query().Get("lastLogId"), 10, 64)
	if err != nil {
		lastLogID = 0
	}

	sse, err := NewSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send initial connection message with last log ID
	sse.Send("connected", fmt.Sprintf(`{"lastLogId":%d}`, lastLogID))

	// Poll for updates every 500ms
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Get deployment status
			deployment, err := s.GetDeployment(deploymentId)
			if err != nil {
				sse.Send("error", "Deployment not found")
				return
			}

			// Send status update
			sse.Send("status", string(deployment.Status))

			// Get logs for this deployment
			logs, err := s.GetLogsByDeployment(deploymentId)
			if err != nil {
				continue
			}

			// Send new logs only
			for _, log := range logs {
				if log.ID > lastLogID {
					sse.Send("log", fmt.Sprintf("%d:%s", log.ID, log.Message))
					lastLogID = log.ID
				}
			}

			// Send completion signal
			if deployment.Status == DeploymentStatusCompleted ||
				deployment.Status == DeploymentStatusFailed ||
				deployment.Status == DeploymentStatusCancelled {
				sse.Send("complete", string(deployment.Status))
				return
			}
		}
	}
}

// CancelDeploymentHandler handles deployment cancellation
func (s *Server) CancelDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}
	deploymentIdStr := r.PathValue("deploymentId")
	deploymentId, err := strconv.ParseInt(deploymentIdStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	deployment, err := s.GetDeployment(deploymentId)
	if err != nil || deployment.ProjectID != projectID {
		http.NotFound(w, r)
		return
	}

	s.deploymentsMu.RLock()
	executor, exists := s.deployments[deploymentId]
	s.deploymentsMu.RUnlock()
	if !exists {
		// Deployment not currently running, check if already completed
		if deployment.Status == DeploymentStatusCompleted ||
			deployment.Status == DeploymentStatusFailed ||
			deployment.Status == DeploymentStatusCancelled {
			http.Error(w, "Deployment already finished", http.StatusBadRequest)
			return
		}

		// Deployment exists but executor not found (server restart?)
		if err := s.UpdateDeploymentStatus(deploymentId, DeploymentStatusCancelled); err != nil {
			http.Error(w, "Failed to cancel deployment", http.StatusInternalServerError)
			return
		}
		_ = s.AddDeploymentLog(deploymentId, fmt.Sprintf(
			"[%s] Deployment cancelled without an active executor",
			time.Now().Format("2006-01-02 15:04:05"),
		))
		w.WriteHeader(http.StatusOK)
		return
	}

	// Cancel the executor
	executor.Cancel()

	w.WriteHeader(http.StatusAccepted)
}

// StartProjectDeployment serializes triggers per project and registers the
// executor before returning the deployment to the handler.
func (s *Server) StartProjectDeployment(project *Project, triggeredBy string) (*Deployment, error) {
	s.deploymentsMu.Lock()
	defer s.deploymentsMu.Unlock()

	active, err := s.GetActiveDeploymentByProject(project.ID)
	if err != nil {
		return nil, fmt.Errorf("check active deployment: %w", err)
	}
	if active != nil {
		return nil, fmt.Errorf("%w: deployment #%d is %s", ErrDeploymentInProgress, active.ID, active.Status)
	}

	deployment, err := s.CreateDeployment(project.ID, "", "", "", "", "", "", "", triggeredBy)
	if err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}
	s.launchDeploymentLocked(deployment)
	return deployment, nil
}

func (s *Server) StartGitHubDeployment(project *Project, commit, commitMsg, author, deliveryID string) (*Deployment, bool, error) {
	s.deploymentsMu.Lock()
	defer s.deploymentsMu.Unlock()

	existing, err := s.GetDeploymentByGitHubDeliveryID(deliveryID)
	if err != nil {
		return nil, false, fmt.Errorf("check webhook delivery: %w", err)
	}
	if existing != nil {
		return existing, true, nil
	}
	active, err := s.GetActiveDeploymentByProject(project.ID)
	if err != nil {
		return nil, false, fmt.Errorf("check active deployment: %w", err)
	}
	if active != nil {
		return nil, false, fmt.Errorf("%w: deployment #%d is %s", ErrDeploymentInProgress, active.ID, active.Status)
	}
	deployment, err := s.CreateGitHubDeployment(project.ID, commit, commitMsg, author, deliveryID)
	if err != nil {
		return nil, false, fmt.Errorf("create GitHub deployment: %w", err)
	}
	s.launchDeploymentLocked(deployment)
	return deployment, false, nil
}

// launchDeploymentLocked starts an executor while trigger registration remains
// protected by deploymentsMu.
func (s *Server) launchDeploymentLocked(deployment *Deployment) {
	executor := NewDeploymentExecutor(s, deployment)
	s.deployments[deployment.ID] = executor

	go func() {
		defer func() {
			s.deploymentsMu.Lock()
			delete(s.deployments, deployment.ID)
			s.deploymentsMu.Unlock()
		}()
		if err := executor.Execute(); err != nil {
			log.Printf("Deployment %d failed: %v", deployment.ID, err)
		}
	}()
}

func init() {
	RegisterInitHook((*Server).RecoverInterruptedDeployments)
}
