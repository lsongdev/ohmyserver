package server

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/config"
)

// AppTemplate represents an app template from the apps directory
type AppTemplate struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	Available              bool            `json:"available"`
	Exposable              bool            `json:"exposable"`
	DynamicConfig          bool            `json:"dynamic_config"`
	Port                   int             `json:"port"`
	PlatformVersion        int             `json:"platform_version"`
	Version                string          `json:"version"`
	Categories             []string        `json:"categories"`
	Description            string          `json:"description"`
	ShortDesc              string          `json:"short_desc"`
	Author                 string          `json:"author"`
	Source                 string          `json:"source"`
	FormFields             json.RawMessage `json:"form_fields"`
	SupportedArchitectures []string        `json:"supported_architectures"`
	CreatedAt              int64           `json:"created_at"`
	UpdatedAt              int64           `json:"updated_at"`
	ForcePull              bool            `json:"force_pull"`
	MinVersion             string          `json:"min_version"`
	Icon                   string          `json:"icon"`
	ComposeFile            string          `json:"-"`
}

// AppStatus represents the runtime status of an app
type AppStatus struct {
	State     string `json:"state"` // not_installed, installing, installed, running, stopped
	Container string `json:"container,omitempty"`
}

// initAppsDB creates the in-memory app catalog schema
func (s *Server) initAppsDB() error {
	schema := `
	CREATE TABLE IF NOT EXISTS app_templates (
		id TEXT PRIMARY KEY,
		name TEXT,
		available INTEGER,
		exposable INTEGER,
		dynamic_config INTEGER,
		port INTEGER,
		platform_version INTEGER,
		version TEXT,
		categories TEXT,
		description TEXT,
		short_desc TEXT,
		author TEXT,
		source TEXT,
		form_fields TEXT,
		supported_architectures TEXT,
		created_at INTEGER,
		updated_at INTEGER,
		force_pull INTEGER,
		min_version TEXT,
		icon TEXT,
		compose_file TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_apps_name ON app_templates(name);
	CREATE INDEX IF NOT EXISTS idx_apps_available ON app_templates(available);
	`
	_, err := s.appsDB.Exec(schema)
	return err
}

// loadAppTemplates scans the apps directory and loads all manifests into memory
func (s *Server) loadAppTemplates() error {
	appsDir := config.AppsDir
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return fmt.Errorf("failed to read apps directory: %w", err)
	}

	tx, err := s.appsDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO app_templates (
			id, name, available, exposable, dynamic_config, port, platform_version, version,
			categories, description, short_desc, author, source, form_fields,
			supported_architectures, created_at, updated_at, force_pull, min_version, icon, compose_file
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".git" {
			continue
		}
		appID := entry.Name()
		manifestPath := filepath.Join(appsDir, appID, "manifest.json")
		composePath := filepath.Join(appsDir, appID, "docker-compose.yml")

		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // skip apps without manifest
		}

		var app AppTemplate
		if err := json.Unmarshal(manifestData, &app); err != nil {
			continue // skip malformed manifests
		}
		app.ID = appID

		composeData, err := os.ReadFile(composePath)
		if err != nil {
			continue // skip apps without compose file
		}
		app.ComposeFile = string(composeData)

		categoriesJSON, _ := json.Marshal(app.Categories)
		archsJSON, _ := json.Marshal(app.SupportedArchitectures)
		formFieldsJSON, _ := json.Marshal(app.FormFields)

		_, err = stmt.Exec(
			app.ID, app.Name, boolToInt(app.Available), boolToInt(app.Exposable),
			boolToInt(app.DynamicConfig), app.Port, app.PlatformVersion, app.Version,
			string(categoriesJSON), app.Description, app.ShortDesc, app.Author, app.Source,
			string(formFieldsJSON), string(archsJSON), app.CreatedAt, app.UpdatedAt,
			boolToInt(app.ForcePull), app.MinVersion, app.Icon, app.ComposeFile,
		)
		if err != nil {
			continue
		}
	}

	return tx.Commit()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// scanAppTemplate scans a single row into AppTemplate
func scanAppTemplate(row interface{ Scan(dest ...any) error }) (*AppTemplate, error) {
	var app AppTemplate
	var categoriesJSON, archsJSON, formFieldsJSON string
	var available, exposable, dynamicConfig, forcePull int

	scanDest := []any{
		&app.ID, &app.Name, &available, &exposable, &dynamicConfig, &app.Port,
		&app.PlatformVersion, &app.Version, &categoriesJSON, &app.Description,
		&app.ShortDesc, &app.Author, &app.Source, &formFieldsJSON, &archsJSON,
		&app.CreatedAt, &app.UpdatedAt, &forcePull, &app.MinVersion, &app.Icon, &app.ComposeFile,
	}

	var err error
	switch r := row.(type) {
	case *sql.Row:
		err = r.Scan(scanDest...)
	case *sql.Rows:
		err = r.Scan(scanDest...)
	}
	if err != nil {
		return nil, err
	}

	app.Available = available == 1
	app.Exposable = exposable == 1
	app.DynamicConfig = dynamicConfig == 1
	app.ForcePull = forcePull == 1
	json.Unmarshal([]byte(categoriesJSON), &app.Categories)
	json.Unmarshal([]byte(archsJSON), &app.SupportedArchitectures)
	json.Unmarshal([]byte(formFieldsJSON), &app.FormFields)

	return &app, nil
}

// GetAppTemplate retrieves an app template by ID
func (s *Server) GetAppTemplate(id string) (*AppTemplate, error) {
	query := `SELECT id, name, available, exposable, dynamic_config, port, platform_version, version,
		categories, description, short_desc, author, source, form_fields,
		supported_architectures, created_at, updated_at, force_pull, min_version, icon, compose_file
		FROM app_templates WHERE id = ?`
	return scanAppTemplate(s.appsDB.QueryRow(query, id))
}

// ListAppTemplates lists all app templates optionally filtered by category
func (s *Server) ListAppTemplates(category string) ([]*AppTemplate, error) {
	query := `SELECT id, name, available, exposable, dynamic_config, port, platform_version, version,
		categories, description, short_desc, author, source, form_fields,
		supported_architectures, created_at, updated_at, force_pull, min_version, icon, compose_file
		FROM app_templates WHERE available = 1`
	args := []any{}
	if category != "" && category != "all" {
		query += ` AND categories LIKE ?`
		args = append(args, `%"`+category+`"%`)
	}
	query += ` ORDER BY name COLLATE NOCASE`

	rows, err := s.appsDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []*AppTemplate
	for rows.Next() {
		app, err := scanAppTemplate(rows)
		if err != nil {
			continue
		}
		apps = append(apps, app)
	}
	return apps, rows.Err()
}

// GetAllAppStatuses returns a map of appID -> status by querying Docker once
func (s *Server) GetAllAppStatuses() (map[string]*AppStatus, error) {
	statuses := make(map[string]*AppStatus)

	// Get all app IDs first
	rows, err := s.appsDB.Query(`SELECT id, name FROM app_templates WHERE available = 1`)
	if err != nil {
		return statuses, err
	}
	defer rows.Close()

	var appIDs []string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err == nil {
			appIDs = append(appIDs, id)
			statuses[id] = &AppStatus{State: "not_installed"}
		}
	}
	rows.Close()

	// Track which apps are currently installing
	installingApps := make(map[string]bool)
	s.appInstallMu.RLock()
	for appID, task := range s.appInstalls {
		task.mu.RLock()
		if task.Status == "installing" {
			statuses[appID] = &AppStatus{State: "installing"}
			installingApps[appID] = true
		}
		task.mu.RUnlock()
	}
	s.appInstallMu.RUnlock()

	// Query Docker containers (skip apps that are installing)
	containers, err := s.docker.ListContainers()
	if err != nil {
		return statuses, err
	}

	for _, c := range containers {
		// Check if container has compose project label matching any app ID
		if project, ok := c.Labels["com.docker.compose.project"]; ok {
			for _, appID := range appIDs {
				if project == appID {
					// Don't override installing status
					if installingApps[appID] {
						continue
					}
					state := "installed"
					if c.State == "running" {
						state = "running"
					}
					statuses[appID] = &AppStatus{State: state, Container: c.ID}
				}
			}
		}
	}

	return statuses, nil
}

// GetAppStatus gets the status of a single app
func (s *Server) GetAppStatus(appID string) (*AppStatus, error) {
	// Check if installation is in progress
	task := s.getOrCreateInstallTask(appID)
	task.mu.RLock()
	installStatus := task.Status
	task.mu.RUnlock()
	if installStatus == "installing" {
		return &AppStatus{State: "installing"}, nil
	}

	// For docker-compose apps, list containers and find by compose project label
	containers, err := s.docker.ListContainers()
	if err != nil {
		return &AppStatus{State: "not_installed"}, nil
	}

	for _, c := range containers {
		// Check if container has the compose project label matching appID
		if project, ok := c.Labels["com.docker.compose.project"]; ok && project == appID {
			// Found the container, get detailed info
			containerJSON, err := s.docker.GetContainer(c.ID)
			if err != nil {
				continue
			}
			state := "installed"
			if containerJSON.State.Running {
				state = "running"
			}
			return &AppStatus{State: state, Container: c.ID}, nil
		}
	}

	// Fallback: try to find container by appID directly (for non-compose apps)
	container, err := s.docker.GetContainer(appID)
	if err == nil {
		state := "installed"
		if container.State.Running {
			state = "running"
		}
		return &AppStatus{State: state, Container: container.ID}, nil
	}

	return &AppStatus{State: "not_installed"}, nil
}

// GetAppDir returns the installation directory for an app
func (s *Server) GetAppDir(appID string) string {
	return filepath.Join(config.DockerComposeDir, appID)
}

// IsAppInstalled checks if an app has been installed (compose dir exists or container exists)
func (s *Server) IsAppInstalled(appID string) bool {
	appDir := s.GetAppDir(appID)
	if _, err := os.Stat(appDir); err == nil {
		return true
	}
	_, err := s.docker.GetContainer(appID)
	return err == nil
}

// InstallApp installs an app by copying compose file and running docker-compose up
func (s *Server) InstallApp(appID string, envVars map[string]string) error {
	app, err := s.GetAppTemplate(appID)
	if err != nil {
		return err
	}

	appDir := s.GetAppDir(appID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("failed to create app directory: %w", err)
	}

	// Write docker-compose.yml
	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(app.ComposeFile), 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %w", err)
	}

	// Build environment
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	// Set defaults
	if envVars["APP_PORT"] == "" {
		env = append(env, fmt.Sprintf("APP_PORT=%d", app.Port))
	}
	if envVars["APP_DOMAIN"] == "" {
		env = append(env, fmt.Sprintf("APP_DOMAIN=%s.localhost", appID))
	}
	if envVars["LOCAL_DOMAIN"] == "" {
		env = append(env, "LOCAL_DOMAIN=local")
	}
	if envVars["APP_DATA_DIR"] == "" {
		env = append(env, fmt.Sprintf("APP_DATA_DIR=%s", appDir))
	}

	// Run docker-compose up
	cmd := exec.Command("docker-compose", "-f", composePath, "up", "-d")
	cmd.Dir = appDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker-compose up failed: %s: %w", string(output), err)
	}
	return nil
}

// UninstallApp removes an app
func (s *Server) UninstallApp(appID string) error {
	appDir := s.GetAppDir(appID)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	// Check if compose file exists
	if _, err := os.Stat(composePath); err == nil {
		cmd := exec.Command("docker-compose", "-f", composePath, "down", "-v")
		cmd.Dir = appDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker-compose down failed: %s: %w", string(output), err)
		}
	} else {
		// Try removing container directly
		if err := s.docker.RemoveContainer(appID); err != nil {
			return fmt.Errorf("failed to remove container: %w", err)
		}
	}

	os.RemoveAll(appDir)
	return nil
}

// StartApp starts an installed app
func (s *Server) StartApp(appID string) error {
	appDir := s.GetAppDir(appID)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	if _, err := os.Stat(composePath); err == nil {
		cmd := exec.Command("docker-compose", "-f", composePath, "start")
		cmd.Dir = appDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker-compose start failed: %s: %w", string(output), err)
		}
		return nil
	}
	return s.docker.StartContainer(appID)
}

// StopApp stops an installed app
func (s *Server) StopApp(appID string) error {
	appDir := s.GetAppDir(appID)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	if _, err := os.Stat(composePath); err == nil {
		cmd := exec.Command("docker-compose", "-f", composePath, "stop")
		cmd.Dir = appDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker-compose stop failed: %s: %w", string(output), err)
		}
		return nil
	}
	return s.docker.StopContainer(appID)
}

// RestartApp restarts an installed app
func (s *Server) RestartApp(appID string) error {
	appDir := s.GetAppDir(appID)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	if _, err := os.Stat(composePath); err == nil {
		cmd := exec.Command("docker-compose", "-f", composePath, "restart")
		cmd.Dir = appDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker-compose restart failed: %s: %w", string(output), err)
		}
		return nil
	}
	return s.docker.RestartContainer(appID)
}

// GetAppCategories returns all unique categories
func (s *Server) GetAppCategories() ([]string, error) {
	rows, err := s.appsDB.Query(`SELECT categories FROM app_templates WHERE available = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categorySet := make(map[string]struct{})
	for rows.Next() {
		var catJSON string
		if err := rows.Scan(&catJSON); err != nil {
			continue
		}
		var cats []string
		if err := json.Unmarshal([]byte(catJSON), &cats); err == nil {
			for _, c := range cats {
				categorySet[c] = struct{}{}
			}
		}
	}

	var categories []string
	for c := range categorySet {
		categories = append(categories, c)
	}
	sort.Strings(categories)
	return categories, rows.Err()
}

// ============ HTTP Handlers ============

// AppsView renders the app store list page
func (s *Server) AppsView(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")

	category := r.URL.Query().Get("category")

	if filter == "" && category == "" {
		filter = "all"
	}

	apps, err := s.ListAppTemplates("")
	if err != nil {
		s.Error(w, err)
		return
	}

	statuses, err := s.GetAllAppStatuses()
	if err != nil {
		statuses = make(map[string]*AppStatus)
	}

	categories, err := s.GetAppCategories()
	if err != nil {
		categories = []string{}
	}

	// Filter by status and category if requested
	var filteredApps []*AppTemplate
	for _, app := range apps {
		status := statuses[app.ID]
		if status == nil {
			status = &AppStatus{State: "not_installed"}
		}
		switch filter {
		case "running":
			if status.State != "running" {
				continue
			}
		case "installed":
			if status.State != "running" && status.State != "installed" {
				continue
			}
		}
		if category != "" {
			found := false
			for _, c := range app.Categories {
				if c == category {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filteredApps = append(filteredApps, app)
	}

	s.Render(w, "docker/apps", H{
		"apps":       filteredApps,
		"statuses":   statuses,
		"categories": categories,
		"filter":     filter,
		"category":   category,
		"count":      len(filteredApps),
	})
}

func generateRandomString(min, max int) string {
	length := 16
	if min > 0 && max > 0 {
		length = min + rand.Intn(max-min+1)
	} else if min > 0 {
		length = min
	} else if max > 0 {
		length = max
	}
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

// AppDetailView renders app detail page
func (s *Server) AppDetailView(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	app, err := s.GetAppTemplate(appID)
	if err != nil {
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	status, err := s.GetAppStatus(appID)
	if err != nil {
		status = &AppStatus{State: "not_installed"}
	}

	// Parse form fields for template
	var formFields []map[string]any
	if len(app.FormFields) > 0 {
		json.Unmarshal(app.FormFields, &formFields)
		for _, field := range formFields {
			if field["type"] == "random" {
				min := 0
				max := 0
				if v, ok := field["min"].(float64); ok {
					min = int(v)
				}
				if v, ok := field["max"].(float64); ok {
					max = int(v)
				}
				field["value"] = generateRandomString(min, max)
			}
		}
	}

	// Read README.md
	readmePath := filepath.Join(config.AppsDir, appID, "README.md")
	readmeContent, _ := os.ReadFile(readmePath)

	s.Render(w, "docker/apps_detail", H{
		"app":           app,
		"status":        status,
		"installStatus": status.State,
		"formFields":    formFields,
		"readme":        string(readmeContent),
	})
}

// AppIconHandler serves app icons
func (s *Server) AppIconHandler(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	iconPath := filepath.Join(config.AppsDir, appID, "logo.jpg")

	// Try logo.jpg first, then any image file
	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		// Try to find any image in the app directory
		entries, err := os.ReadDir(filepath.Join(config.AppsDir, appID))
		if err != nil {
			http.Error(w, "Icon not found", http.StatusNotFound)
			return
		}
		for _, entry := range entries {
			name := strings.ToLower(entry.Name())
			if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".jpeg") || strings.HasSuffix(name, ".svg") {
				iconPath = filepath.Join(config.AppsDir, appID, entry.Name())
				break
			}
		}
	}

	data, err := os.ReadFile(iconPath)
	if err != nil {
		http.Error(w, "Icon not found", http.StatusNotFound)
		return
	}

	contentType := "image/jpeg"
	if strings.HasSuffix(iconPath, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(iconPath, ".svg") {
		contentType = "image/svg+xml"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// getOrCreateInstallTask returns existing install task or creates a new idle one
func (s *Server) getOrCreateInstallTask(appID string) *AppInstallTask {
	s.appInstallMu.Lock()
	defer s.appInstallMu.Unlock()

	if task, ok := s.appInstalls[appID]; ok {
		return task
	}

	task := &AppInstallTask{
		AppID:     appID,
		Status:    "idle",
		Logs:      make([]string, 0),
		clients:   make(map[chan string]struct{}),
		startedAt: time.Now(),
	}
	s.appInstalls[appID] = task
	return task
}

// addInstallLog appends a log line and broadcasts to all SSE clients
func (task *AppInstallTask) addInstallLog(line string) {
	task.mu.Lock()
	task.Logs = append(task.Logs, line)
	task.mu.Unlock()

	task.clientMu.Lock()
	for ch := range task.clients {
		select {
		case ch <- line:
		default:
		}
	}
	task.clientMu.Unlock()
}

// registerInstallClient registers a channel to receive live logs
func (task *AppInstallTask) registerInstallClient(ch chan string) {
	task.clientMu.Lock()
	task.clients[ch] = struct{}{}
	task.clientMu.Unlock()
}

// unregisterInstallClient removes a channel
func (task *AppInstallTask) unregisterInstallClient(ch chan string) {
	task.clientMu.Lock()
	delete(task.clients, ch)
	task.clientMu.Unlock()
}

// runAppInstall executes docker-compose up in background
func (s *Server) runAppInstall(task *AppInstallTask, app *AppTemplate, envVars map[string]string) {
	task.mu.Lock()
	task.Status = "installing"
	task.Logs = make([]string, 0)
	task.startedAt = time.Now()
	task.mu.Unlock()

	appDir := s.GetAppDir(app.ID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		task.addInstallLog(fmt.Sprintf("Failed to create app directory: %v", err))
		task.mu.Lock()
		task.Status = "failed"
		task.mu.Unlock()
		return
	}

	composePath := filepath.Join(appDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(app.ComposeFile), 0644); err != nil {
		task.addInstallLog(fmt.Sprintf("Failed to write compose file: %v", err))
		task.mu.Lock()
		task.Status = "failed"
		task.mu.Unlock()
		return
	}

	task.addInstallLog(fmt.Sprintf("Installing %s...", app.Name))

	env := os.Environ()
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	if envVars["APP_PORT"] == "" {
		env = append(env, fmt.Sprintf("APP_PORT=%d", app.Port))
	}
	if envVars["APP_DOMAIN"] == "" {
		env = append(env, fmt.Sprintf("APP_DOMAIN=%s.localhost", app.ID))
	}
	if envVars["LOCAL_DOMAIN"] == "" {
		env = append(env, "LOCAL_DOMAIN=local")
	}
	if envVars["APP_DATA_DIR"] == "" {
		env = append(env, fmt.Sprintf("APP_DATA_DIR=%s", appDir))
	}

	cmd := exec.Command("docker-compose", "-f", composePath, "up", "-d")
	cmd.Dir = appDir
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		task.addInstallLog(fmt.Sprintf("Failed to create stdout pipe: %v", err))
		task.mu.Lock()
		task.Status = "failed"
		task.mu.Unlock()
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		task.addInstallLog(fmt.Sprintf("Failed to create stderr pipe: %v", err))
		task.mu.Lock()
		task.Status = "failed"
		task.mu.Unlock()
		return
	}

	if err := cmd.Start(); err != nil {
		task.addInstallLog(fmt.Sprintf("Failed to start command: %v", err))
		task.mu.Lock()
		task.Status = "failed"
		task.mu.Unlock()
		return
	}

	// Stream stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			task.addInstallLog(scanner.Text())
		}
	}()
	// Stream stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			task.addInstallLog(scanner.Text())
		}
	}()

	err = cmd.Wait()
	if err != nil {
		task.addInstallLog(fmt.Sprintf("Installation failed: %v", err))
		task.mu.Lock()
		task.Status = "failed"
		task.mu.Unlock()
	} else {
		task.addInstallLog(fmt.Sprintf("%s installed successfully", app.Name))
		task.mu.Lock()
		task.Status = "completed"
		task.mu.Unlock()
	}

	// Close all client channels after completion/failure
	task.clientMu.Lock()
	for ch := range task.clients {
		close(ch)
		delete(task.clients, ch)
	}
	task.clientMu.Unlock()
}

// AppInstallTriggerHandler triggers installation and streams progress via SSE.
// POST: trigger new install with form data, then stream logs.
// GET: reconnect to an in-progress install and stream logs.
func (s *Server) AppInstallTriggerHandler(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	task := s.getOrCreateInstallTask(appID)
	task.mu.RLock()
	currentStatus := task.Status
	task.mu.RUnlock()

	if r.Method == http.MethodGet {
		// GET: just stream existing install progress
		s.streamInstallLogs(w, r, task)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := ParseFormAll(r); err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	app, err := s.GetAppTemplate(appID)
	if err != nil {
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	if currentStatus == "installing" {
		// Already installing, just stream the progress
		s.streamInstallLogs(w, r, task)
		return
	}

	// Collect env vars
	envVars := make(map[string]string)
	for key, values := range r.Form {
		if len(values) > 0 {
			envVars[key] = values[0]
		}
	}

	// Reset task for new install
	task.mu.Lock()
	task.Status = "installing"
	task.Logs = make([]string, 0)
	task.startedAt = time.Now()
	task.mu.Unlock()

	// Start installation in background
	go s.runAppInstall(task, app, envVars)

	// Stream the progress
	s.streamInstallLogs(w, r, task)
}

// streamInstallLogs streams installation logs via SSE
func (s *Server) streamInstallLogs(w http.ResponseWriter, r *http.Request, task *AppInstallTask) {
	sse, err := NewSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	task.mu.RLock()
	status := task.Status
	logs := make([]string, len(task.Logs))
	copy(logs, task.Logs)
	task.mu.RUnlock()

	sse.Send("status", status)

	// Send historical logs
	for _, line := range logs {
		sse.Send("log", line)
	}

	// If already completed or failed, send final event and return
	if status == "completed" {
		sse.Send("complete", "Installation completed")
		return
	}
	if status == "failed" {
		sse.Send("error", "Installation failed")
		return
	}
	if status == "idle" {
		sse.Send("idle", "No active installation")
		return
	}

	// Register for live logs
	ch := make(chan string, 100)
	task.registerInstallClient(ch)
	defer task.unregisterInstallClient(ch)

	// Stream live logs
	for line := range ch {
		sse.Send("log", line)
	}

	// Stream ended, check final status
	task.mu.RLock()
	finalStatus := task.Status
	task.mu.RUnlock()

	if finalStatus == "completed" {
		sse.Send("complete", "Installation completed")
	} else if finalStatus == "failed" {
		sse.Send("error", "Installation failed")
	}
}

// AppActionHandler handles start/stop/restart/uninstall actions
func (s *Server) AppActionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	appID := r.PathValue("id")
	action := r.FormValue("action")

	var err error
	switch action {
	case "start":
		err = s.StartApp(appID)
	case "stop":
		err = s.StopApp(appID)
	case "restart":
		err = s.RestartApp(appID)
	case "uninstall":
		err = s.UninstallApp(appID)
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	redirect := r.FormValue("redirect")
	if redirect == "" {
		redirect = "/apps"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// AppAPIHandler returns JSON list of apps
func (s *Server) AppAPIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	filter := r.URL.Query().Get("filter")
	apps, err := s.ListAppTemplates("")
	if err != nil {
		json.NewEncoder(w).Encode(H{"error": err.Error()})
		return
	}

	statuses, err := s.GetAllAppStatuses()
	if err != nil {
		statuses = make(map[string]*AppStatus)
	}

	type appResponse struct {
		*AppTemplate
		Status string `json:"status"`
	}

	var result []appResponse
	for _, app := range apps {
		status := statuses[app.ID]
		if status == nil {
			status = &AppStatus{State: "not_installed"}
		}
		if filter != "" && filter != "all" && status.State != filter {
			continue
		}
		result = append(result, appResponse{AppTemplate: app, Status: status.State})
	}

	json.NewEncoder(w).Encode(result)
}

// AppLogsHandler streams app logs via SSE
func (s *Server) AppLogsHandler(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	appDir := s.GetAppDir(appID)
	composePath := filepath.Join(appDir, "docker-compose.yml")

	// Use SSE for real-time logs
	sse, err := NewSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var cmd *exec.Cmd
	if _, err := os.Stat(composePath); err == nil {
		cmd = exec.Command("docker-compose", "-f", composePath, "logs", "-f", "--tail", "100")
		cmd.Dir = appDir
	} else {
		cmd = exec.Command("docker", "logs", "-f", "--tail", "100", appID)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sse.Send("error", fmt.Sprintf("Failed to create stdout pipe: %s", err.Error()))
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		sse.Send("error", fmt.Sprintf("Failed to create stderr pipe: %s", err.Error()))
		return
	}

	if err := cmd.Start(); err != nil {
		sse.Send("error", fmt.Sprintf("Failed to start command: %s", err.Error()))
		return
	}

	done := make(chan bool, 1)
	go func() {
		io.Copy(&sseWriter{sse: sse}, stdout)
		done <- true
	}()
	go func() {
		io.Copy(&sseWriter{sse: sse}, stderr)
	}()

	<-done
	cmd.Wait()
}

type sseWriter struct {
	sse *sse
}

func (w *sseWriter) Write(p []byte) (n int, err error) {
	w.sse.Send("log", string(p))
	return len(p), nil
}

func init() {
	RegisterInitHook(func(s *Server) error {
		if err := s.initAppsDB(); err != nil {
			return err
		}
		return s.loadAppTemplates()
	})
}
