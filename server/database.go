package server

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lsongdev/ohmyserver/cron"
	_ "modernc.org/sqlite"
)

// parseTime parses a time string from database, handling multiple formats
func parseTime(timeStr string) time.Time {
	if timeStr == "" {
		return time.Time{}
	}

	// Try multiple time formats
	formats := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02T15:04:05-07:00",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t
		}
	}

	return time.Time{}
}

// ptrTime returns a pointer to the given time if it's not zero
func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// Project represents a project in the database
type Project struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Repo        string    `json:"repo"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DeploymentStatus represents the status of a deployment
type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusRunning   DeploymentStatus = "running"
	DeploymentStatusCompleted DeploymentStatus = "completed"
	DeploymentStatusFailed    DeploymentStatus = "failed"
	DeploymentStatusCancelled DeploymentStatus = "cancelled"
)

// Deployment represents a deployment in the database
type Deployment struct {
	ID            int64            `json:"id"`
	ProjectID     int64            `json:"project_id"`
	Status        DeploymentStatus `json:"status"`
	DockerImage   string           `json:"docker_image"`
	Dockerfile    string           `json:"dockerfile"`
	ContainerName string           `json:"container_name"`
	ContainerID   string           `json:"container_id"`
	Command       string           `json:"command"`
	Port          int              `json:"port"`
	Commit        string           `json:"commit"`
	CommitMsg     string           `json:"commit_msg"`
	Author        string           `json:"author"`
	TriggeredBy   string           `json:"triggered_by"`
	StartedAt     *time.Time       `json:"started_at"`
	CompletedAt   *time.Time       `json:"completed_at"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// DeploymentLog represents a log entry for a deployment
type DeploymentLog struct {
	ID           int64     `json:"id"`
	DeploymentID int64     `json:"deployment_id"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"created_at"`
}

// initSchema creates the necessary tables via registered hooks
func (s *Server) initSchema() error {
	if err := s.runInitSchemaHooks(); err != nil {
		return err
	}
	if err := s.runMigrateHooks(); err != nil {
		return err
	}
	return nil
}

// CreateProject creates a new project in the database
func (s *Server) CreateProject(name, description, repo string) (*Project, error) {

	now := time.Now()
	query := `
	INSERT INTO projects (name, description, repo, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, description, repo, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Project{
		ID:          id,
		Name:        name,
		Description: description,
		Repo:        repo,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetProjectByID retrieves a project by its ID
func (s *Server) GetProjectByID(id int64) (*Project, error) {

	query := `
	SELECT id, name, description, repo, created_at, updated_at
	FROM projects
	WHERE id = ?
	`

	var project Project
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&project.ID, &project.Name, &project.Description, &project.Repo, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	project.CreatedAt = parseTime(createdAt)
	project.UpdatedAt = parseTime(updatedAt)

	return &project, nil
}

// GetAllProjects retrieves all projects from the database
func (s *Server) GetAllProjects() ([]Project, error) {

	query := `
	SELECT id, name, description, repo, created_at, updated_at
	FROM projects
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		var createdAt, updatedAt string
		if err := rows.Scan(&project.ID, &project.Name, &project.Description, &project.Repo, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		project.CreatedAt = parseTime(createdAt)
		project.UpdatedAt = parseTime(updatedAt)
		projects = append(projects, project)
	}

	return projects, rows.Err()
}

// UpdateProject updates an existing project
func (s *Server) UpdateProject(id int64, name, description, repo string) error {

	now := time.Now()
	query := `
	UPDATE projects
	SET name = ?, description = ?, repo = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, description, repo, now, id)
	return err
}

// DeleteProject deletes a project by its ID
func (s *Server) DeleteProject(id int64) error {

	query := `DELETE FROM projects WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// CreateDeployment creates a new deployment
func (s *Server) CreateDeployment(projectID int64, dockerImage, dockerfile, containerName, command, commit, commitMsg, author, triggeredBy string) (*Deployment, error) {

	now := time.Now()
	query := `
	INSERT INTO deployments (project_id, status, docker_image, dockerfile, container_name, container_id, command, "commit", commit_msg, author, triggered_by, created_at, updated_at)
	VALUES (?, 'pending', ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, projectID, dockerImage, dockerfile, containerName, command, commit, commitMsg, author, triggeredBy, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Deployment{
		ID:            id,
		ProjectID:     projectID,
		Status:        DeploymentStatusPending,
		DockerImage:   dockerImage,
		Dockerfile:    dockerfile,
		ContainerName: containerName,
		ContainerID:   "",
		Command:       command,
		Commit:        commit,
		CommitMsg:     commitMsg,
		Author:        author,
		TriggeredBy:   triggeredBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// GetDeployment retrieves a deployment by ID
func (s *Server) GetDeployment(id int64) (*Deployment, error) {

	query := `
	SELECT id, project_id, status, docker_image, dockerfile, container_name, container_id, command, "commit", commit_msg, author, triggered_by, started_at, completed_at, created_at, updated_at
	FROM deployments
	WHERE id = ?
	`

	var deployment Deployment
	var startedAt, completedAt, createdAt, updatedAt sql.NullString
	err := s.db.QueryRow(query, id).Scan(
		&deployment.ID, &deployment.ProjectID, &deployment.Status, &deployment.DockerImage, &deployment.Dockerfile,
		&deployment.ContainerName, &deployment.ContainerID, &deployment.Command, &deployment.Commit, &deployment.CommitMsg, &deployment.Author,
		&deployment.TriggeredBy, &startedAt, &completedAt, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		deployment.StartedAt = ptrTime(parseTime(startedAt.String))
	}
	if completedAt.Valid {
		deployment.CompletedAt = ptrTime(parseTime(completedAt.String))
	}
	if createdAt.Valid {
		deployment.CreatedAt = parseTime(createdAt.String)
	}
	if updatedAt.Valid {
		deployment.UpdatedAt = parseTime(updatedAt.String)
	}

	return &deployment, nil
}

// GetDeploymentsByProject retrieves all deployments for a project
func (s *Server) GetDeploymentsByProject(projectID int64) ([]Deployment, error) {

	query := `
	SELECT id, project_id, status, docker_image, dockerfile, container_name, container_id, command, "commit", commit_msg, author, triggered_by, started_at, completed_at, created_at, updated_at
	FROM deployments
	WHERE project_id = ?
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []Deployment
	for rows.Next() {
		var deployment Deployment
		var startedAt, completedAt, createdAt, updatedAt sql.NullString
		err := rows.Scan(
			&deployment.ID, &deployment.ProjectID, &deployment.Status, &deployment.DockerImage, &deployment.Dockerfile,
			&deployment.ContainerName, &deployment.ContainerID, &deployment.Command, &deployment.Commit, &deployment.CommitMsg, &deployment.Author,
			&deployment.TriggeredBy, &startedAt, &completedAt, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if startedAt.Valid {
			deployment.StartedAt = ptrTime(parseTime(startedAt.String))
		}
		if completedAt.Valid {
			deployment.CompletedAt = ptrTime(parseTime(completedAt.String))
		}
		if createdAt.Valid {
			deployment.CreatedAt = parseTime(createdAt.String)
		}
		if updatedAt.Valid {
			deployment.UpdatedAt = parseTime(updatedAt.String)
		}

		deployments = append(deployments, deployment)
	}

	return deployments, rows.Err()
}

// UpdateDeploymentStatus updates the status of a deployment
func (s *Server) UpdateDeploymentStatus(id int64, status DeploymentStatus) error {

	now := time.Now()
	query := `UPDATE deployments SET status = ?, updated_at = ?`
	args := []interface{}{status, now}

	if status == DeploymentStatusRunning {
		query += `, started_at = ?`
		args = append(args, now)
	}

	if status == DeploymentStatusCompleted || status == DeploymentStatusFailed || status == DeploymentStatusCancelled {
		query += `, completed_at = ?`
		args = append(args, now)
	}

	query += ` WHERE id = ?`
	args = append(args, id)

	_, err := s.db.Exec(query, args...)
	return err
}

// UpdateDeploymentContainerID updates the container ID for a deployment
func (s *Server) UpdateDeploymentContainerID(id int64, containerID string) error {

	now := time.Now()
	query := `UPDATE deployments SET container_id = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, containerID, now, id)
	return err
}

// GetLatestDeploymentByProject retrieves the most recent deployment for a project
func (s *Server) GetLatestDeploymentByProject(projectID int64) (*Deployment, error) {

	query := `
	SELECT id, project_id, status, docker_image, dockerfile, container_name, container_id, command, "commit", commit_msg, author, triggered_by, started_at, completed_at, created_at, updated_at
	FROM deployments
	WHERE project_id = ?
	ORDER BY created_at DESC
	LIMIT 1
	`

	var deployment Deployment
	var startedAt, completedAt, createdAt, updatedAt sql.NullString
	err := s.db.QueryRow(query, projectID).Scan(
		&deployment.ID, &deployment.ProjectID, &deployment.Status, &deployment.DockerImage, &deployment.Dockerfile,
		&deployment.ContainerName, &deployment.ContainerID, &deployment.Command, &deployment.Commit, &deployment.CommitMsg, &deployment.Author,
		&deployment.TriggeredBy, &startedAt, &completedAt, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if startedAt.Valid {
		deployment.StartedAt = ptrTime(parseTime(startedAt.String))
	}
	if completedAt.Valid {
		deployment.CompletedAt = ptrTime(parseTime(completedAt.String))
	}
	if createdAt.Valid {
		deployment.CreatedAt = parseTime(createdAt.String)
	}
	if updatedAt.Valid {
		deployment.UpdatedAt = parseTime(updatedAt.String)
	}

	return &deployment, nil
}

// ProjectRuntimeStatus represents the runtime status of a project
type ProjectRuntimeStatus struct {
	Status        string // running, stopped, error, not_deployed
	ContainerID   string
	ContainerName string
	IsRunning     bool
	Error         string
}

// GetProjectRuntimeStatus gets the runtime status of a project by checking its latest deployment's container
func (s *Server) GetProjectRuntimeStatus(projectID int64) (*ProjectRuntimeStatus, error) {
	status := &ProjectRuntimeStatus{
		Status: "not_deployed",
	}

	// Get latest deployment
	deployment, err := s.GetLatestDeploymentByProject(projectID)
	if err != nil {
		return nil, err
	}

	if deployment == nil {
		return status, nil
	}

	status.ContainerID = deployment.ContainerID
	status.ContainerName = deployment.ContainerName

	// If no container ID, check deployment status
	if deployment.ContainerID == "" {
		// If we have a container name but no ID, try to find the container by name
		if deployment.ContainerName != "" {
			container, err := s.docker.GetContainer(deployment.ContainerName)
			if err == nil && container.State.Running {
				status.IsRunning = true
				status.Status = "running"
				status.ContainerID = container.ID
				return status, nil
			}
		}

		if deployment.Status == DeploymentStatusCompleted {
			status.Status = "stopped"
		} else if deployment.Status == DeploymentStatusFailed {
			status.Status = "error"
			status.Error = "Deployment failed"
		} else if deployment.Status == DeploymentStatusRunning {
			status.Status = "starting"
		} else {
			status.Status = "not_deployed"
		}
		return status, nil
	}

	// Check if container exists and is running
	// Try by container ID first, then by name
	container, err := s.docker.GetContainer(deployment.ContainerID)
	if err != nil && deployment.ContainerName != "" {
		// Try by container name if ID fails
		container, err = s.docker.GetContainer(deployment.ContainerName)
	}

	if err != nil {
		// Container not found
		if deployment.Status == DeploymentStatusCompleted {
			status.Status = "stopped"
		} else {
			status.Status = "error"
			status.Error = "Container not found"
		}
		return status, nil
	}

	status.IsRunning = container.State.Running
	if container.State.Running {
		status.Status = "running"
	} else if container.State.Dead || container.State.OOMKilled {
		status.Status = "error"
		status.Error = "Container died unexpectedly"
	} else if container.State.ExitCode != 0 {
		status.Status = "error"
		status.Error = fmt.Sprintf("Container exited with code %d", container.State.ExitCode)
	} else {
		status.Status = "stopped"
	}

	return status, nil
}

// AddDeploymentLog adds a log entry to a deployment
func (s *Server) AddDeploymentLog(deploymentID int64, message string) error {

	now := time.Now()
	query := `INSERT INTO deployment_logs (deployment_id, message, created_at) VALUES (?, ?, ?)`
	_, err := s.db.Exec(query, deploymentID, message, now)
	return err
}

// GetLogsByDeployment retrieves all logs for a deployment
func (s *Server) GetLogsByDeployment(deploymentID int64) ([]DeploymentLog, error) {

	query := `
	SELECT id, deployment_id, message, created_at
	FROM deployment_logs
	WHERE deployment_id = ?
	ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []DeploymentLog
	for rows.Next() {
		var log DeploymentLog
		var createdAt sql.NullString
		err := rows.Scan(&log.ID, &log.DeploymentID, &log.Message, &createdAt)
		if err != nil {
			return nil, err
		}

		if createdAt.Valid {
			t, _ := time.Parse("2006-01-02 15:04:05.999999999-07:00", createdAt.String)
			log.CreatedAt = t
		}

		logs = append(logs, log)
	}

	return logs, rows.Err()
}

// DNSProvider represents a connection to a hosted DNS provider.
type DNSProvider struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`   // cloudflare, dnspod, namecom
	Config    string    `json:"config"` // JSON encoded config
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// initDomainSchema creates hosted DNS connection storage. Hosted zones and
// records remain provider-owned and are fetched live rather than mirrored into
// the local DNS server tables.
func (s *Server) initDomainSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS dns_provider_connections (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		config TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_dns_provider_connections_name ON dns_provider_connections(name);
	`
	if _, err := s.db.Exec(query); err != nil {
		return err
	}
	// Preserve connections from installations that used the former /domains
	// feature. These statements are conditional because fresh databases never
	// had the legacy tables.
	var legacy int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='dns_providers'`).Scan(&legacy); err != nil {
		return err
	}
	if legacy != 0 {
		if _, err := s.db.Exec(`
			INSERT OR IGNORE INTO dns_provider_connections
				(id, name, type, config, created_at, updated_at)
			SELECT id, name, type, config, created_at, updated_at FROM dns_providers
		`); err != nil {
			return err
		}
	}
	return nil
}

// CreateDNSProvider creates a new DNS provider
func (s *Server) CreateDNSProvider(name, providerType, config string) (*DNSProvider, error) {

	now := time.Now()
	query := `
	INSERT INTO dns_provider_connections (name, type, config, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, providerType, config, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &DNSProvider{
		ID:        id,
		Name:      name,
		Type:      providerType,
		Config:    config,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// GetDNSProvider retrieves a DNS provider by ID
func (s *Server) GetDNSProvider(id int64) (*DNSProvider, error) {

	query := `
	SELECT id, name, type, config, created_at, updated_at
	FROM dns_provider_connections
	WHERE id = ?
	`

	var provider DNSProvider
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&provider.ID, &provider.Name, &provider.Type, &provider.Config, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	provider.CreatedAt = parseTime(createdAt)
	provider.UpdatedAt = parseTime(updatedAt)

	return &provider, nil
}

// GetAllDNSProviders retrieves all DNS providers
func (s *Server) GetAllDNSProviders() ([]DNSProvider, error) {

	query := `
	SELECT id, name, type, config, created_at, updated_at
	FROM dns_provider_connections
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []DNSProvider
	for rows.Next() {
		var provider DNSProvider
		var createdAt, updatedAt string
		err := rows.Scan(&provider.ID, &provider.Name, &provider.Type, &provider.Config, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		provider.CreatedAt = parseTime(createdAt)
		provider.UpdatedAt = parseTime(updatedAt)
		providers = append(providers, provider)
	}

	return providers, rows.Err()
}

// UpdateDNSProvider updates a DNS provider
func (s *Server) UpdateDNSProvider(id int64, name, providerType, config string) error {

	now := time.Now()
	query := `
	UPDATE dns_provider_connections
	SET name = ?, type = ?, config = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, providerType, config, now, id)
	return err
}

// DeleteDNSProvider deletes a DNS provider
func (s *Server) DeleteDNSProvider(id int64) error {

	query := `DELETE FROM dns_provider_connections WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// TCPListener represents a TCP/UDP listener
type TCPListener struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Protocol    string    `json:"protocol"` // tcp or udp
	ListenPort  int       `json:"listen_port"`
	ForwardHost string    `json:"forward_host"`
	ForwardPort int       `json:"forward_port"`
	Status      string    `json:"status"` // active, inactive
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// initTCPListenerSchema creates the TCP listener tables
func (s *Server) initTCPListenerSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS tcp_listeners (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		protocol TEXT NOT NULL DEFAULT 'tcp',
		listen_port INTEGER NOT NULL UNIQUE,
		forward_host TEXT NOT NULL DEFAULT '127.0.0.1',
		forward_port INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_tcp_listeners_name ON tcp_listeners(name);
	CREATE INDEX IF NOT EXISTS idx_tcp_listeners_port ON tcp_listeners(listen_port);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateTCPListener creates a new TCP/UDP listener
func (s *Server) CreateTCPListener(name, protocol string, listenPort int, forwardHost string, forwardPort int) (*TCPListener, error) {

	if forwardHost == "" {
		forwardHost = "127.0.0.1"
	}

	now := time.Now()
	query := `
	INSERT INTO tcp_listeners (name, protocol, listen_port, forward_host, forward_port, status, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, 'active', ?, ?)
	`
	result, err := s.db.Exec(query, name, protocol, listenPort, forwardHost, forwardPort, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &TCPListener{
		ID:          id,
		Name:        name,
		Protocol:    protocol,
		ListenPort:  listenPort,
		ForwardHost: forwardHost,
		ForwardPort: forwardPort,
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetTCPListener retrieves a TCP listener by ID
func (s *Server) GetTCPListener(id int64) (*TCPListener, error) {

	query := `
	SELECT id, name, protocol, listen_port, forward_host, forward_port, status, created_at, updated_at
	FROM tcp_listeners
	WHERE id = ?
	`

	var listener TCPListener
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&listener.ID, &listener.Name, &listener.Protocol, &listener.ListenPort, &listener.ForwardHost, &listener.ForwardPort, &listener.Status, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	listener.CreatedAt = parseTime(createdAt)
	listener.UpdatedAt = parseTime(updatedAt)

	return &listener, nil
}

// GetAllTCPListeners retrieves all TCP/UDP listeners
func (s *Server) GetAllTCPListeners() ([]TCPListener, error) {

	query := `
	SELECT id, name, protocol, listen_port, forward_host, forward_port, status, created_at, updated_at
	FROM tcp_listeners
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var listeners []TCPListener
	for rows.Next() {
		var listener TCPListener
		var createdAt, updatedAt string
		err := rows.Scan(&listener.ID, &listener.Name, &listener.Protocol, &listener.ListenPort, &listener.ForwardHost, &listener.ForwardPort, &listener.Status, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		listener.CreatedAt = parseTime(createdAt)
		listener.UpdatedAt = parseTime(updatedAt)
		listeners = append(listeners, listener)
	}

	return listeners, rows.Err()
}

// UpdateTCPListener updates a TCP listener
func (s *Server) UpdateTCPListener(id int64, name, protocol string, listenPort int, forwardHost string, forwardPort int) error {

	now := time.Now()
	query := `
	UPDATE tcp_listeners
	SET name = ?, protocol = ?, listen_port = ?, forward_host = ?, forward_port = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, protocol, listenPort, forwardHost, forwardPort, now, id)
	return err
}

// DeleteTCPListener deletes a TCP listener
func (s *Server) DeleteTCPListener(id int64) error {

	query := `DELETE FROM tcp_listeners WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// UpdateTCPListenerStatus updates TCP listener status
func (s *Server) UpdateTCPListenerStatus(id int64, status string) error {

	now := time.Now()
	query := `UPDATE tcp_listeners SET status = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, status, now, id)
	return err
}

// GetTCPListenerByPort retrieves a TCP listener by listen port
func (s *Server) GetTCPListenerByPort(port int) (*TCPListener, error) {

	query := `
	SELECT id, name, protocol, listen_port, forward_host, forward_port, status, created_at, updated_at
	FROM tcp_listeners
	WHERE listen_port = ?
	`

	var listener TCPListener
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, port).Scan(&listener.ID, &listener.Name, &listener.Protocol, &listener.ListenPort, &listener.ForwardHost, &listener.ForwardPort, &listener.Status, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	listener.CreatedAt = parseTime(createdAt)
	listener.UpdatedAt = parseTime(updatedAt)

	return &listener, nil
}

// CronJob represents a scheduled task
type CronJob struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`     // once, cron
	Schedule   string     `json:"schedule"` // cron expression or ISO time for once
	Command    string     `json:"command"`
	WorkingDir string     `json:"working_dir"`
	Timeout    int        `json:"timeout"` // seconds, 0 = no timeout
	Enabled    bool       `json:"enabled"`
	LastRun    *time.Time `json:"last_run"`
	NextRun    *time.Time `json:"next_run"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CronJobExecution represents a job execution record
type CronJobExecution struct {
	ID          int64      `json:"id"`
	JobID       int64      `json:"job_id"`
	Status      string     `json:"status"` // running, success, failed, timeout
	Output      string     `json:"output"`
	Error       string     `json:"error"`
	ExitCode    int        `json:"exit_code"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Implement cron.Job interface for CronJob

// GetID implements cron.Job
func (j *CronJob) GetID() int64 { return j.ID }

// GetName implements cron.Job
func (j *CronJob) GetName() string { return j.Name }

// GetType implements cron.Job
func (j *CronJob) GetType() string { return j.Type }

// GetSchedule implements cron.Job
func (j *CronJob) GetSchedule() string { return j.Schedule }

// GetCommand implements cron.Job
func (j *CronJob) GetCommand() string { return j.Command }

// GetWorkingDir implements cron.Job
func (j *CronJob) GetWorkingDir() string { return j.WorkingDir }

// GetTimeout implements cron.Job
func (j *CronJob) GetTimeout() int { return j.Timeout }

// IsEnabled implements cron.Job
func (j *CronJob) IsEnabled() bool { return j.Enabled }

// initCronJobSchema creates the cron job tables
func (s *Server) initCronJobSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS cron_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'cron',
		schedule TEXT NOT NULL,
		command TEXT NOT NULL,
		working_dir TEXT DEFAULT '',
		timeout INTEGER DEFAULT 0,
		enabled INTEGER DEFAULT 1,
		last_run DATETIME,
		next_run DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_cron_jobs_name ON cron_jobs(name);
	CREATE INDEX IF NOT EXISTS idx_cron_jobs_enabled ON cron_jobs(enabled);
	CREATE TABLE IF NOT EXISTS cron_job_executions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'running',
		output TEXT,
		error TEXT,
		exit_code INTEGER DEFAULT 0,
		started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		completed_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (job_id) REFERENCES cron_jobs(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_cron_executions_job_id ON cron_job_executions(job_id);
	CREATE INDEX IF NOT EXISTS idx_cron_executions_status ON cron_job_executions(status);
	
	-- SSH Servers table
	CREATE TABLE IF NOT EXISTS ssh_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER DEFAULT 22,
		username TEXT NOT NULL,
		auth_type TEXT NOT NULL DEFAULT 'password',
		password TEXT,
		private_key TEXT,
		host_key TEXT DEFAULT '',
		description TEXT,
		enabled INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_ssh_servers_enabled ON ssh_servers(enabled);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateCronJob creates a new cron job
func (s *Server) CreateCronJob(name, jobType, schedule, command, workingDir string, timeout int) (*CronJob, error) {

	now := time.Now()
	query := `
	INSERT INTO cron_jobs (name, type, schedule, command, working_dir, timeout, enabled, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
	`
	result, err := s.db.Exec(query, name, jobType, schedule, command, workingDir, timeout, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Calculate next run time
	var nextRun *time.Time
	if jobType == "cron" {
		nextRun, err = cron.ParseCronExpression(schedule)
		if err != nil {
			return nil, err
		}
	} else if jobType == "once" {
		t, err := time.Parse(time.RFC3339, schedule)
		if err != nil {
			return nil, err
		}
		if t.After(now) {
			nextRun = &t
		}
	}

	// Update next_run if calculated
	if nextRun != nil {
		query = `UPDATE cron_jobs SET next_run = ? WHERE id = ?`
		_, err = s.db.Exec(query, nextRun, id)
		if err != nil {
			return nil, err
		}
	}

	return &CronJob{
		ID:         id,
		Name:       name,
		Type:       jobType,
		Schedule:   schedule,
		Command:    command,
		WorkingDir: workingDir,
		Timeout:    timeout,
		Enabled:    true,
		CreatedAt:  now,
		UpdatedAt:  now,
		NextRun:    nextRun,
	}, nil
}

// GetCronJob retrieves a cron job by ID
func (s *Server) GetCronJob(id int64) (*CronJob, error) {

	query := `
	SELECT id, name, type, schedule, command, working_dir, timeout, enabled, last_run, next_run, created_at, updated_at
	FROM cron_jobs
	WHERE id = ?
	`

	var job CronJob
	var lastRun, nextRun, createdAt, updatedAt sql.NullString
	err := s.db.QueryRow(query, id).Scan(
		&job.ID, &job.Name, &job.Type, &job.Schedule, &job.Command, &job.WorkingDir,
		&job.Timeout, &job.Enabled, &lastRun, &nextRun, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if lastRun.Valid {
		t := parseTime(lastRun.String)
		job.LastRun = &t
	}
	if nextRun.Valid {
		t := parseTime(nextRun.String)
		job.NextRun = &t
	}
	job.CreatedAt = parseTime(createdAt.String)
	job.UpdatedAt = parseTime(updatedAt.String)

	return &job, nil
}

// GetAllCronJobs retrieves all cron jobs
func (s *Server) GetAllCronJobs() ([]CronJob, error) {

	query := `
	SELECT id, name, type, schedule, command, working_dir, timeout, enabled, last_run, next_run, created_at, updated_at
	FROM cron_jobs
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []CronJob
	for rows.Next() {
		var job CronJob
		var lastRun, nextRun, createdAt, updatedAt sql.NullString
		err := rows.Scan(
			&job.ID, &job.Name, &job.Type, &job.Schedule, &job.Command, &job.WorkingDir,
			&job.Timeout, &job.Enabled, &lastRun, &nextRun, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if lastRun.Valid {
			t := parseTime(lastRun.String)
			job.LastRun = &t
		}
		if nextRun.Valid {
			t := parseTime(nextRun.String)
			job.NextRun = &t
		}
		job.CreatedAt = parseTime(createdAt.String)
		job.UpdatedAt = parseTime(updatedAt.String)

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// GetEnabledCronJobs retrieves all enabled cron jobs
func (s *Server) GetEnabledCronJobs() ([]CronJob, error) {

	query := `
	SELECT id, name, type, schedule, command, working_dir, timeout, enabled, last_run, next_run, created_at, updated_at
	FROM cron_jobs
	WHERE enabled = 1
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []CronJob
	for rows.Next() {
		var job CronJob
		var lastRun, nextRun, createdAt, updatedAt sql.NullString
		err := rows.Scan(
			&job.ID, &job.Name, &job.Type, &job.Schedule, &job.Command, &job.WorkingDir,
			&job.Timeout, &job.Enabled, &lastRun, &nextRun, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if lastRun.Valid {
			t := parseTime(lastRun.String)
			job.LastRun = &t
		}
		if nextRun.Valid {
			t := parseTime(nextRun.String)
			job.NextRun = &t
		}
		job.CreatedAt = parseTime(createdAt.String)
		job.UpdatedAt = parseTime(updatedAt.String)

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// UpdateCronJob updates a cron job
func (s *Server) UpdateCronJob(id int64, name, jobType, schedule, command, workingDir string, timeout int) error {

	now := time.Now()
	query := `
	UPDATE cron_jobs
	SET name = ?, type = ?, schedule = ?, command = ?, working_dir = ?, timeout = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, jobType, schedule, command, workingDir, timeout, now, id)
	return err
}

// DeleteCronJob deletes a cron job
func (s *Server) DeleteCronJob(id int64) error {

	query := `DELETE FROM cron_jobs WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// EnableCronJob enables a cron job
func (s *Server) EnableCronJob(id int64) error {

	query := `UPDATE cron_jobs SET enabled = 1, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, time.Now(), id)
	return err
}

// DisableCronJob disables a cron job
func (s *Server) DisableCronJob(id int64) error {

	query := `UPDATE cron_jobs SET enabled = 0, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, time.Now(), id)
	return err
}

// UpdateCronJobLastRun updates the last run time for a cron job
func (s *Server) UpdateCronJobLastRun(id int64, lastRun time.Time) error {

	query := `UPDATE cron_jobs SET last_run = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, lastRun, time.Now(), id)
	return err
}

// UpdateCronJobRunTimes updates both last run and next run times for a cron job
func (s *Server) UpdateCronJobRunTimes(id int64, lastRun time.Time, nextRun *time.Time) error {

	var nextRunVal interface{}
	if nextRun != nil {
		nextRunVal = *nextRun
	} else {
		nextRunVal = nil
	}

	query := `UPDATE cron_jobs SET last_run = ?, next_run = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, lastRun, nextRunVal, time.Now(), id)
	return err
}

// CreateCronJobExecution creates a new execution record
func (s *Server) CreateCronJobExecution(jobID int64) (*CronJobExecution, error) {

	now := time.Now()
	query := `
	INSERT INTO cron_job_executions (job_id, status, started_at, created_at)
	VALUES (?, 'running', ?, ?)
	`
	result, err := s.db.Exec(query, jobID, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &CronJobExecution{
		ID:        id,
		JobID:     jobID,
		Status:    "running",
		StartedAt: now,
		CreatedAt: now,
	}, nil
}

// UpdateCronJobExecution updates an execution record with completion info
func (s *Server) UpdateCronJobExecution(id int64, status, output, errorMsg string, exitCode int) error {

	now := time.Now()
	query := `
	UPDATE cron_job_executions
	SET status = ?, output = ?, error = ?, exit_code = ?, completed_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, status, output, errorMsg, exitCode, now, id)
	return err
}

// GetCronJobExecutions retrieves executions for a job
func (s *Server) GetCronJobExecutions(jobID int64, limit int) ([]CronJobExecution, error) {

	query := `
	SELECT id, job_id, status, output, error, exit_code, started_at, completed_at, created_at
	FROM cron_job_executions
	WHERE job_id = ?
	ORDER BY created_at DESC
	LIMIT ?
	`

	rows, err := s.db.Query(query, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []CronJobExecution
	for rows.Next() {
		var exec CronJobExecution
		var startedAt, completedAt, createdAt sql.NullString
		err := rows.Scan(
			&exec.ID, &exec.JobID, &exec.Status, &exec.Output, &exec.Error, &exec.ExitCode,
			&startedAt, &completedAt, &createdAt,
		)
		if err != nil {
			return nil, err
		}

		exec.StartedAt = parseTime(startedAt.String)
		if completedAt.Valid {
			exec.CompletedAt = ptrTime(parseTime(completedAt.String))
		}
		exec.CreatedAt = parseTime(createdAt.String)

		executions = append(executions, exec)
	}

	return executions, rows.Err()
}

// GetRecentCronJobExecutions retrieves recent executions across all jobs
func (s *Server) GetRecentCronJobExecutions(limit int) ([]CronJobExecution, error) {

	query := `
	SELECT id, job_id, status, output, error, exit_code, started_at, completed_at, created_at
	FROM cron_job_executions
	ORDER BY created_at DESC
	LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []CronJobExecution
	for rows.Next() {
		var exec CronJobExecution
		var startedAt, completedAt, createdAt sql.NullString
		err := rows.Scan(
			&exec.ID, &exec.JobID, &exec.Status, &exec.Output, &exec.Error, &exec.ExitCode,
			&startedAt, &completedAt, &createdAt,
		)
		if err != nil {
			return nil, err
		}

		exec.StartedAt = parseTime(startedAt.String)
		if completedAt.Valid {
			exec.CompletedAt = ptrTime(parseTime(completedAt.String))
		}
		exec.CreatedAt = parseTime(createdAt.String)

		executions = append(executions, exec)
	}

	return executions, rows.Err()
}

// initESXiServerSchema creates the ESXi servers table
func (s *Server) initESXiServerSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS esxi_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		host TEXT NOT NULL,
		username TEXT NOT NULL,
		password TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 443,
		insecure BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_esxi_servers_name ON esxi_servers(name);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateESXiServer creates a new ESXi server
func (s *Server) CreateESXiServer(name, host, username, password string, port int, insecure bool) (*ESXiServer, error) {

	now := time.Now()
	query := `
	INSERT INTO esxi_servers (name, host, username, password, port, insecure, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, host, username, password, port, insecure, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &ESXiServer{
		ID:        id,
		Name:      name,
		Host:      host,
		Username:  username,
		Password:  password,
		Port:      port,
		Insecure:  insecure,
		CreatedAt: now.Format("2006-01-02 15:04:05"),
		UpdatedAt: now.Format("2006-01-02 15:04:05"),
	}, nil
}

// GetESXiServer retrieves an ESXi server by ID
func (s *Server) GetESXiServer(id int64) (*ESXiServer, error) {

	query := `
	SELECT id, name, host, username, password, port, insecure, created_at, updated_at
	FROM esxi_servers
	WHERE id = ?
	`

	var server ESXiServer
	err := s.db.QueryRow(query, id).Scan(
		&server.ID, &server.Name, &server.Host, &server.Username, &server.Password,
		&server.Port, &server.Insecure, &server.CreatedAt, &server.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &server, nil
}

// GetAllESXiServers retrieves all ESXi servers
func (s *Server) GetAllESXiServers() ([]ESXiServer, error) {

	query := `
	SELECT id, name, host, username, password, port, insecure, created_at, updated_at
	FROM esxi_servers
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []ESXiServer
	for rows.Next() {
		var server ESXiServer
		err := rows.Scan(
			&server.ID, &server.Name, &server.Host, &server.Username, &server.Password,
			&server.Port, &server.Insecure, &server.CreatedAt, &server.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}

	return servers, rows.Err()
}

// UpdateESXiServer updates an ESXi server
func (s *Server) UpdateESXiServer(id int64, name, host, username, password string, port int, insecure bool) error {

	now := time.Now()
	query := `
	UPDATE esxi_servers
	SET name = ?, host = ?, username = ?, password = ?, port = ?, insecure = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, host, username, password, port, insecure, now, id)
	return err
}

// DeleteESXiServer deletes an ESXi server
func (s *Server) DeleteESXiServer(id int64) error {

	query := `DELETE FROM esxi_servers WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// ============== ACME Provider Schema ==============

// initACMEProviderSchema creates the ACME providers table
func (s *Server) initACMEProviderSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS acme_providers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		directory_url TEXT NOT NULL,
		terms_of_service TEXT,
		website TEXT,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_acme_providers_name ON acme_providers(name);
	`
	_, err := s.db.Exec(query)
	return err
}

// seedDefaultACMEProviders inserts default ACME providers if they don't already exist
func (s *Server) seedDefaultACMEProviders() error {
	defaultProviders := []struct {
		Name         string
		DirectoryURL string
	}{
		{"Let's Encrypt", "https://acme-v02.api.letsencrypt.org/directory"},
		{"Let's Encrypt Staging", "https://acme-staging-v02.api.letsencrypt.org/directory"},
		{"ZeroSSL", "https://acme.zerossl.com/v2/DV90"},
		{"Buypass", "https://api.buypass.com/acme/directory"},
	}

	for _, p := range defaultProviders {
		_, err := s.db.Exec(
			`INSERT OR IGNORE INTO acme_providers (name, directory_url) VALUES (?, ?)`,
			p.Name, p.DirectoryURL,
		)
		if err != nil {
			return fmt.Errorf("failed to seed provider %s: %w", p.Name, err)
		}
	}
	return nil
}

// CreateACMEProvider creates a new ACME provider
func (s *Server) acme_create_provider(name, directoryURL string) (int64, error) {

	query := `
	INSERT INTO acme_providers (name, directory_url)
	VALUES (?, ?)
	`
	result, err := s.db.Exec(query, name, directoryURL)
	if err != nil {
		return -1, err
	}
	return result.LastInsertId()
}

// GetACMEProvider retrieves an ACME provider by ID
func (s *Server) GetACMEProvider(id int64) (*ACMEProvider, error) {

	query := `
	SELECT id, name, directory_url, created_at, updated_at
	FROM acme_providers
	WHERE id = ?
	`
	var provider ACMEProvider
	err := s.db.QueryRow(query, id).Scan(
		&provider.ID, &provider.Name, &provider.DirectoryURL, &provider.CreatedAt, &provider.UpdatedAt,
	)
	return &provider, err
}

// GetAllACMEProviders retrieves all ACME providers
func (s *Server) acme_get_providers() ([]ACMEProvider, error) {

	query := `
	SELECT id, name, directory_url, created_at, updated_at
	FROM acme_providers
	ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []ACMEProvider
	for rows.Next() {
		var provider ACMEProvider
		err := rows.Scan(
			&provider.ID, &provider.Name, &provider.DirectoryURL, &provider.CreatedAt, &provider.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}

	return providers, rows.Err()
}

// UpdateACMEProvider updates an ACME provider
func (s *Server) UpdateACMEProvider(id int64, name, directoryURL, termsOfService, website string) error {

	now := time.Now()
	query := `
	UPDATE acme_providers
	SET name = ?, directory_url = ?, terms_of_service = ?, website = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, directoryURL, termsOfService, website, now, id)
	return err
}

// DeleteACMEProvider deletes an ACME provider
func (s *Server) DeleteACMEProvider(id int64) error {

	query := `DELETE FROM acme_providers WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// ============== ACME Account Schema ==============

// initACMEAccountSchema creates the ACME accounts table
func (s *Server) initACMEAccountSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS acme_accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider_id INTEGER NOT NULL,
		account_url TEXT NOT NULL UNIQUE,
		email TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'valid',
		key_pem TEXT NOT NULL,
		thumbprint TEXT,
		agreed_terms BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (provider_id) REFERENCES acme_providers(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_acme_accounts_provider_id ON acme_accounts(provider_id);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateACMEAccount creates a new ACME account
func (s *Server) CreateACMEAccount(providerID int64, accountURL, email, keyPem, thumbprint string, agreedTerms bool) (*ACMEAccount, error) {

	now := time.Now()
	query := `
	INSERT INTO acme_accounts (provider_id, account_url, email, status, key_pem, thumbprint, agreed_terms, created_at, updated_at)
	VALUES (?, ?, ?, 'valid', ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, providerID, accountURL, email, keyPem, thumbprint, agreedTerms, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &ACMEAccount{
		ID:          id,
		ProviderID:  providerID,
		AccountURL:  accountURL,
		Email:       email,
		Status:      "valid",
		KeyPem:      keyPem,
		Thumbprint:  thumbprint,
		AgreedTerms: agreedTerms,
		CreatedAt:   now.Format("2006-01-02 15:04:05"),
		UpdatedAt:   now.Format("2006-01-02 15:04:05"),
	}, nil
}

// GetACMEAccount retrieves an ACME account by ID
func (s *Server) GetACMEAccount(id int64) (*ACMEAccount, error) {

	query := `
	SELECT id, provider_id, account_url, email, status, key_pem, thumbprint, agreed_terms, created_at, updated_at
	FROM acme_accounts
	WHERE id = ?
	`

	var account ACMEAccount
	err := s.db.QueryRow(query, id).Scan(
		&account.ID, &account.ProviderID, &account.AccountURL, &account.Email,
		&account.Status, &account.KeyPem, &account.Thumbprint, &account.AgreedTerms,
		&account.CreatedAt, &account.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &account, nil
}

// GetACMEAccountsByProvider retrieves all ACME accounts for a provider
func (s *Server) GetACMEAccounts() ([]ACMEAccount, error) {

	query := `
	SELECT a.id, a.provider_id, p.name, a.account_url, a.email, a.status, a.key_pem, a.thumbprint, a.agreed_terms, a.created_at, a.updated_at
	FROM acme_accounts a
	JOIN acme_providers p ON a.provider_id = p.id
	ORDER BY a.created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []ACMEAccount
	for rows.Next() {
		var account ACMEAccount
		err := rows.Scan(
			&account.ID, &account.ProviderID, &account.ProviderName, &account.AccountURL, &account.Email,
			&account.Status, &account.KeyPem, &account.Thumbprint, &account.AgreedTerms,
			&account.CreatedAt, &account.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}

	return accounts, rows.Err()
}

// UpdateACMEAccount updates an ACME account
func (s *Server) UpdateACMEAccount(id int64, accountURL, email, keyPem, thumbprint string, agreedTerms bool) error {

	now := time.Now()
	query := `
	UPDATE acme_accounts
	SET account_url = ?, email = ?, key_pem = ?, thumbprint = ?, agreed_terms = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, accountURL, email, keyPem, thumbprint, agreedTerms, now, id)
	return err
}

// UpdateACMEAccountStatus updates the status of an ACME account
func (s *Server) UpdateACMEAccountStatus(id int64, status string) error {

	now := time.Now()
	query := `UPDATE acme_accounts SET status = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, status, now, id)
	return err
}

// DeleteACMEAccount deletes an ACME account
func (s *Server) DeleteACMEAccount(id int64) error {

	query := `DELETE FROM acme_accounts WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// ============== ACME Order Schema ==============

// initACMEOrderSchema creates the ACME orders table
func (s *Server) initACMEOrderSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS acme_orders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		order_url TEXT NOT NULL UNIQUE,
		domains TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (account_id) REFERENCES acme_accounts(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_acme_orders_account_id ON acme_orders(account_id);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateACMEOrder creates a new ACME order
func (s *Server) CreateACMEOrder(accountID int64, orderURL, domains string) (*ACMEOrder, error) {

	now := time.Now()
	query := `
	INSERT INTO acme_orders (account_id, order_url, domains, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, accountID, orderURL, domains, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &ACMEOrder{
		ID:        id,
		AccountID: accountID,
		OrderURL:  orderURL,
		Domains:   domains,
		CreatedAt: now.Format("2006-01-02 15:04:05"),
		UpdatedAt: now.Format("2006-01-02 15:04:05"),
	}, nil
}

// GetACMEOrder retrieves an ACME order by ID
func (s *Server) GetACMEOrder(id int64) (*ACMEOrder, error) {

	query := `
	SELECT id, account_id, order_url, domains, created_at, updated_at
	FROM acme_orders
	WHERE id = ?
	`
	var order ACMEOrder
	err := s.db.QueryRow(query, id).Scan(
		&order.ID, &order.AccountID, &order.OrderURL, &order.Domains,
		&order.CreatedAt, &order.UpdatedAt,
	)
	return &order, err
}

// GetACMEOrders retrieves all ACME orders
func (s *Server) GetACMEOrders() ([]ACMEOrder, error) {

	query := `
	SELECT id, account_id, order_url, domains, created_at, updated_at
	FROM acme_orders
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []ACMEOrder
	for rows.Next() {
		var order ACMEOrder
		err := rows.Scan(
			&order.ID, &order.AccountID, &order.OrderURL, &order.Domains,
			&order.CreatedAt, &order.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		orders = append(orders, order)
	}

	return orders, rows.Err()
}

// DeleteACMEOrder deletes an ACME order
func (s *Server) DeleteACMEOrder(id int64) error {

	query := `DELETE FROM acme_orders WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// ============== ACME Authorization Schema ==============

// initACMEAuthorizationSchema creates the ACME authorizations table
func (s *Server) initACMEAuthorizationSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS acme_authorizations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		order_id INTEGER NOT NULL,
		authz_url TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL DEFAULT 'pending',
		identifier_type TEXT NOT NULL,
		identifier_value TEXT NOT NULL,
		expires DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (order_id) REFERENCES acme_orders(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_acme_authorizations_order_id ON acme_authorizations(order_id);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateACMEAuthorization creates a new ACME authorization
func (s *Server) CreateACMEAuthorization(orderID int64, authzURL, status, identifierType, identifierValue string, expires time.Time) (*ACMEAuthorization, error) {

	now := time.Now()
	query := `
	INSERT INTO acme_authorizations (order_id, authz_url, status, identifier_type, identifier_value, expires, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, orderID, authzURL, status, identifierType, identifierValue, expires, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &ACMEAuthorization{
		ID:         id,
		OrderID:    orderID,
		AuthzURL:   authzURL,
		Status:     status,
		Identifier: Identifier{Type: identifierType, Value: identifierValue},
		Expires:    expires,
		CreatedAt:  now.Format("2006-01-02 15:04:05"),
	}, nil
}

// ============== ACME Challenge Schema ==============

// initACMEChallengeSchema creates the ACME challenges table
func (s *Server) initACMEChallengeSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS acme_challenges (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		authorization_id INTEGER NOT NULL,
		challenge_url TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		token TEXT NOT NULL,
		key_auth TEXT,
		validated DATETIME,
		error TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (authorization_id) REFERENCES acme_authorizations(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_acme_challenges_authz_id ON acme_challenges(authorization_id);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateACMEChallenge creates a new ACME challenge
func (s *Server) CreateACMEChallenge(authzID int64, challengeURL, challengeType, status, token, keyAuth string) (*ACMEChallenge, error) {

	now := time.Now()
	query := `
	INSERT INTO acme_challenges (authorization_id, challenge_url, type, status, token, key_auth, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, authzID, challengeURL, challengeType, status, token, keyAuth, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &ACMEChallenge{
		ID:              id,
		AuthorizationID: authzID,
		ChallengeURL:    challengeURL,
		Type:            challengeType,
		Status:          status,
		Token:           token,
		KeyAuth:         keyAuth,
		CreatedAt:       now.Format("2006-01-02 15:04:05"),
	}, nil
}

// GetACMEChallengesByAuthorization retrieves all ACME challenges for an authorization
func (s *Server) GetACMEChallengesByAuthorization(authzID int64) ([]ACMEChallenge, error) {

	query := `
	SELECT id, authorization_id, challenge_url, type, status, token, key_auth, validated, error, created_at
	FROM acme_challenges
	WHERE authorization_id = ?
	ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var challenges []ACMEChallenge
	for rows.Next() {
		var challenge ACMEChallenge
		var validated sql.NullString
		err := rows.Scan(
			&challenge.ID, &challenge.AuthorizationID, &challenge.ChallengeURL,
			&challenge.Type, &challenge.Status, &challenge.Token, &challenge.KeyAuth,
			&validated, &challenge.Error, &challenge.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if validated.Valid {
			challenge.Validated = parseTime(validated.String)
		}

		challenges = append(challenges, challenge)
	}

	return challenges, rows.Err()
}

// UpdateACMEChallenge updates an ACME challenge
func (s *Server) UpdateACMEChallenge(id int64, status, keyAuth, errorMsg string, validated time.Time) error {

	now := time.Now()
	query := `
	UPDATE acme_challenges
	SET status = ?, key_auth = ?, error = ?, validated = ?, created_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, status, keyAuth, errorMsg, validated, now, id)
	return err
}

// ============== CSR Schema ==============

// initCertificateSchema creates the certificates table
func (s *Server) initCertificateSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS certificates (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		order_id INTEGER DEFAULT 0,
		domains TEXT NOT NULL,
		csr_pem TEXT NOT NULL,
		private_key_pem TEXT NOT NULL,
		cert_pem TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_certificates_status ON certificates(status);
	CREATE INDEX IF NOT EXISTS idx_certificates_order_id ON certificates(order_id);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateCertificate creates a new certificate (CSR) record
func (s *Server) CreateCertificate(domains, csrPem, privateKeyPem string) (*Certificate, error) {

	now := time.Now()
	query := `
	INSERT INTO certificates (domains, csr_pem, private_key_pem, status, created_at, updated_at)
	VALUES (?, ?, ?, 'pending', ?, ?)
	`
	result, err := s.db.Exec(query, domains, csrPem, privateKeyPem, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Certificate{
		ID:            id,
		Domains:       domains,
		CSRPem:        csrPem,
		PrivateKeyPem: privateKeyPem,
		Status:        "pending",
		CreatedAt:     now.Format("2006-01-02 15:04:05"),
		UpdatedAt:     now.Format("2006-01-02 15:04:05"),
	}, nil
}

// GetCertificate retrieves a certificate by ID
func (s *Server) GetCertificate(id int64) (*Certificate, error) {

	query := `
	SELECT id, order_id, domains, csr_pem, private_key_pem,
		cert_pem, status, created_at, updated_at
	FROM certificates
	WHERE id = ?
	`
	var cert Certificate
	var certPem sql.NullString
	err := s.db.QueryRow(query, id).Scan(
		&cert.ID, &cert.OrderID, &cert.Domains,
		&cert.CSRPem, &cert.PrivateKeyPem, &certPem,
		&cert.Status, &cert.CreatedAt, &cert.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if certPem.Valid {
		cert.CertPem = certPem.String
	}
	return &cert, nil
}

// GetAllCertificates retrieves all certificates
func (s *Server) GetAllCertificates() ([]Certificate, error) {

	query := `
	SELECT id, order_id, domains, csr_pem, private_key_pem,
		cert_pem, status, created_at, updated_at
	FROM certificates
	ORDER BY created_at DESC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []Certificate
	for rows.Next() {
		var cert Certificate
		var certPem sql.NullString
		err := rows.Scan(
			&cert.ID, &cert.OrderID, &cert.Domains,
			&cert.CSRPem, &cert.PrivateKeyPem, &certPem,
			&cert.Status, &cert.CreatedAt, &cert.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if certPem.Valid {
			cert.CertPem = certPem.String
		}
		certs = append(certs, cert)
	}

	return certs, rows.Err()
}

// UpdateCertificate updates certificate information
func (s *Server) UpdateCertificate(id int64, orderID int64, certPem string, status string) error {

	now := time.Now()
	query := `
	UPDATE certificates
	SET order_id = ?, cert_pem = ?, status = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, orderID, certPem, status, now, id)
	return err
}

// UpdateCertificateStatus updates only the certificate status
func (s *Server) UpdateCertificateStatus(id int64, status string) error {

	now := time.Now()
	query := `
	UPDATE certificates
	SET status = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, status, now, id)
	return err
}

// DeleteCertificate deletes a certificate by ID
func (s *Server) DeleteCertificate(id int64) error {

	query := `DELETE FROM certificates WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

func init() {
	// Core schemas: projects, deployments, deployment_logs, system_stats
	RegisterInitSchemaHook(func(s *Server) error {
		schema := `
		CREATE TABLE IF NOT EXISTS system_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			cpu_usage REAL NOT NULL,
			memory_total INTEGER NOT NULL,
			memory_used INTEGER NOT NULL,
			memory_free INTEGER NOT NULL,
			memory_usage REAL NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_timestamp ON system_stats(timestamp);
		CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			repo TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_projects_name ON projects(name);
		CREATE TABLE IF NOT EXISTS deployments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			docker_image TEXT DEFAULT '',
			dockerfile TEXT,
			container_name TEXT DEFAULT '',
			container_id TEXT DEFAULT '',
			command TEXT DEFAULT '',
			"commit" TEXT DEFAULT '',
			commit_msg TEXT DEFAULT '',
			author TEXT DEFAULT '',
			triggered_by TEXT DEFAULT 'Manual',
			started_at DATETIME,
			completed_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_deployments_project_id ON deployments(project_id);
		CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
		CREATE TABLE IF NOT EXISTS deployment_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			deployment_id INTEGER NOT NULL,
			message TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_logs_deployment_id ON deployment_logs(deployment_id);
		`
		_, err := s.db.Exec(schema)
		return err
	})

	// Core schemas with dedicated methods in database.go
	RegisterInitSchemaHook((*Server).initTCPListenerSchema)
	RegisterInitSchemaHook((*Server).initESXiServerSchema)
	RegisterInitSchemaHook((*Server).initCertificateSchema)
	RegisterInitSchemaHook((*Server).initACMEProviderSchema)
	RegisterInitSchemaHook((*Server).initACMEAccountSchema)
	RegisterInitSchemaHook((*Server).initACMEOrderSchema)
	RegisterInitSchemaHook((*Server).initACMEAuthorizationSchema)
	RegisterInitSchemaHook((*Server).initACMEChallengeSchema)
	RegisterInitSchemaHook((*Server).initCronJobSchema)

	// Seed default ACME providers
	RegisterInitHook((*Server).seedDefaultACMEProviders)

	// Migrations
	RegisterMigrateHook(func(s *Server) error {
		migrationSQLs := []string{
			`ALTER TABLE projects ADD COLUMN repo TEXT DEFAULT ''`,
			`ALTER TABLE deployments ADD COLUMN container_id TEXT DEFAULT ''`,
			`ALTER TABLE llm_models ADD COLUMN mapped_name TEXT`,
			`ALTER TABLE system_stats ADD COLUMN hostname TEXT DEFAULT ''`,
			`ALTER TABLE system_stats ADD COLUMN cpu_cores INTEGER DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN disk_total INTEGER DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN disk_used INTEGER DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN disk_free INTEGER DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN disk_usage REAL DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN net_bytes_sent INTEGER DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN net_bytes_recv INTEGER DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN uptime INTEGER DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN load_1 REAL DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN load_5 REAL DEFAULT 0`,
			`ALTER TABLE system_stats ADD COLUMN load_15 REAL DEFAULT 0`,
			`ALTER TABLE ssh_servers ADD COLUMN host_key TEXT DEFAULT ''`,
		}
		for _, sql := range migrationSQLs {
			s.db.Exec(sql)
		}
		_, err := s.db.Exec(`UPDATE projects SET repo = '' WHERE repo IS NULL`)
		return err
	})
}
