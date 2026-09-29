// Package web exposes the Docker module's HTTP API.
package web

import (
	"encoding/json"
	"fmt"
	"io"

	"net/http"
	"strings"
	"time"

	docker "github.com/lsongdev/ohmyserver/modules/docker"
)

type BuildImageRequest = docker.BuildImageRequest
type Service = docker.Service
type ComposeProject = docker.ComposeProject
type ComposeProjectRequest = docker.ComposeProjectRequest
type ComposeProjectDetail = docker.ComposeProjectDetail
type ContainerStats = docker.ContainerStats
type AppListItem = docker.AppListItem
type AppDetail = docker.AppDetail
type AppInstallProgress = docker.AppInstallProgress

type Module struct{ service docker.Service }

func New(service docker.Service) *Module { return &Module{service: service} }

func (*Module) Name() string { return "docker" }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /containers", m.listContainers)
	mux.HandleFunc("POST /containers", m.createContainer)
	mux.HandleFunc("GET /containers/{id}", m.getContainer)
	mux.HandleFunc("GET /containers/{id}/logs", m.containerLogs)
	mux.HandleFunc("GET /containers/{id}/stats", m.containerStats)
	mux.HandleFunc("POST /containers/{id}/start", m.containerAction("start"))
	mux.HandleFunc("POST /containers/{id}/stop", m.containerAction("stop"))
	mux.HandleFunc("POST /containers/{id}/restart", m.containerAction("restart"))
	mux.HandleFunc("DELETE /containers/{id}", m.deleteContainer)
	mux.HandleFunc("GET /images", m.listImages)
	mux.HandleFunc("POST /images/pull", m.pullImage)
	mux.HandleFunc("POST /images/build", m.buildImage)
	mux.HandleFunc("DELETE /images/{id}", m.deleteImage)
	mux.HandleFunc("GET /volumes", m.listVolumes)
	mux.HandleFunc("POST /volumes", m.createVolume)
	mux.HandleFunc("DELETE /volumes/{name}", m.deleteVolume)
	mux.HandleFunc("GET /networks", m.listNetworks)
	mux.HandleFunc("POST /networks", m.createNetwork)
	mux.HandleFunc("DELETE /networks/{id}", m.deleteNetwork)
	mux.HandleFunc("GET /compose", m.listComposeProjects)
	mux.HandleFunc("POST /compose", m.createComposeProject)
	mux.HandleFunc("GET /compose/{name}", m.getComposeProject)
	mux.HandleFunc("POST /compose/{name}/{action}", m.composeAction)
	mux.HandleFunc("GET /apps", m.listApps)
	mux.HandleFunc("GET /apps/categories", m.appCategories)
	mux.HandleFunc("POST /apps/sync", m.syncApps)
	mux.HandleFunc("GET /apps/{id}", m.getApp)
	mux.HandleFunc("GET /apps/{id}/icon", m.appIcon)
	mux.HandleFunc("POST /apps/{id}/install", m.installApp)
	mux.HandleFunc("GET /apps/{id}/install", m.appInstallStatus)
	mux.HandleFunc("POST /apps/{id}/{action}", m.appAction)
	return wrapModuleWeb(mux)
}

func (m *Module) buildImage(w http.ResponseWriter, r *http.Request) {
	var input BuildImageRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		writeError(w, 422, "validation_failed", "image name is required")
		return
	}
	if input.SourceType != "dockerfile" && input.SourceType != "directory" && input.SourceType != "repository" {
		writeError(w, 422, "validation_failed", "unsupported build source")
		return
	}
	if input.SourceType == "dockerfile" && strings.TrimSpace(input.Dockerfile) == "" || input.SourceType == "directory" && strings.TrimSpace(input.BuildDir) == "" || input.SourceType == "repository" && strings.TrimSpace(input.Repository) == "" {
		writeError(w, 422, "validation_failed", "required build source is missing")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming_unavailable", "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	send := func(event, message string) error {
		payload, _ := json.Marshal(map[string]string{"message": message})
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	err := m.service.BuildDockerImage(r.Context(), input, func(message string) error { return send("log", message) })
	if err != nil {
		_ = send("error", err.Error())
		return
	}
	_ = send("done", "镜像构建完成")
}

func (m *Module) listApps(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListDockerApps(r.Context(), r.URL.Query().Get("filter"), r.URL.Query().Get("category"))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "app_catalog_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []AppListItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}
func (m *Module) appCategories(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListDockerAppCategories(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "app_categories_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}
func (m *Module) syncApps(w http.ResponseWriter, r *http.Request) {
	if err := m.service.SyncDockerApps(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "app_sync_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (m *Module) getApp(w http.ResponseWriter, r *http.Request) {
	item, err := m.service.GetDockerApp(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "app_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}
func (m *Module) appIcon(w http.ResponseWriter, r *http.Request) {
	data, contentType, err := m.service.GetDockerAppIcon(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}
func (m *Module) installApp(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Environment map[string]string `json:"environment"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := m.service.StartDockerAppInstall(r.Context(), r.PathValue("id"), input.Environment); err != nil {
		writeError(w, http.StatusConflict, "app_install_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
func (m *Module) appInstallStatus(w http.ResponseWriter, r *http.Request) {
	item, err := m.service.GetDockerAppInstall(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "app_install_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}
func (m *Module) appAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	switch action {
	case "start", "stop", "restart", "uninstall":
	default:
		writeError(w, http.StatusBadRequest, "invalid_action", "unsupported app action")
		return
	}
	if err := m.service.DockerAppAction(r.Context(), r.PathValue("id"), action); err != nil {
		writeError(w, http.StatusConflict, "app_action_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) containerStats(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unavailable", "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	writeEvent := func(event string, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		stats, err := m.service.GetDockerContainerStats(r.Context(), r.PathValue("id"))
		if err != nil {
			_ = writeEvent("error", map[string]string{"message": err.Error()})
			return
		}
		if err := writeEvent("stats", stats); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Module) createComposeProject(w http.ResponseWriter, r *http.Request) {
	var input ComposeProjectRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := m.service.CreateDockerComposeProject(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "compose_create_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": item})
}

func (m *Module) listComposeProjects(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListDockerComposeProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "compose_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []ComposeProject{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (m *Module) getComposeProject(w http.ResponseWriter, r *http.Request) {
	item, err := m.service.GetDockerComposeProject(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, "compose_project_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (m *Module) composeAction(w http.ResponseWriter, r *http.Request) {
	name, action := r.PathValue("name"), r.PathValue("action")
	switch action {
	case "up", "down", "restart", "pull":
	default:
		writeError(w, http.StatusBadRequest, "invalid_action", "unsupported compose action")
		return
	}
	if err := m.service.DockerComposeAction(r.Context(), name, action); err != nil {
		writeError(w, http.StatusConflict, "compose_action_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) containerLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := m.service.GetDockerContainerLogs(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "container_logs_unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"logs": logs}})
}

type pullImageRequest struct {
	Name string `json:"name"`
}

func (m *Module) pullImage(w http.ResponseWriter, r *http.Request) {
	var input pullImageRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "image name is required")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unavailable", "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	send := func(event string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte("event: " + event + "\ndata: " + string(encoded) + "\n\n")); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	err := m.service.PullDockerImage(r.Context(), strings.TrimSpace(input.Name), func(message string) error {
		return send("progress", map[string]string{"message": message})
	})
	if err != nil {
		_ = send("error", map[string]string{"message": err.Error()})
		return
	}
	_ = send("done", map[string]string{"message": "镜像拉取完成"})
}

type createContainerRequest struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	Command string `json:"command"`
}

func (m *Module) createContainer(w http.ResponseWriter, r *http.Request) {
	var input createContainerRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Image) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "name and image are required")
		return
	}
	id, err := m.service.CreateDockerContainer(r.Context(), strings.TrimSpace(input.Name), strings.TrimSpace(input.Image), input.Command)
	if err != nil {
		writeError(w, http.StatusConflict, "container_create_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"id": id}})
}

type createVolumeRequest struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
}

func (m *Module) createVolume(w http.ResponseWriter, r *http.Request) {
	var input createVolumeRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "name is required")
		return
	}
	if err := m.service.CreateDockerVolume(r.Context(), strings.TrimSpace(input.Name), strings.TrimSpace(input.Driver)); err != nil {
		writeError(w, http.StatusConflict, "volume_create_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

type createNetworkRequest struct {
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`
	IPRange string `json:"ip_range"`
}

func (m *Module) createNetwork(w http.ResponseWriter, r *http.Request) {
	var input createNetworkRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(input.Name) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", "name is required")
		return
	}
	if err := m.service.CreateDockerNetwork(r.Context(), strings.TrimSpace(input.Name), strings.TrimSpace(input.Driver), strings.TrimSpace(input.Subnet), strings.TrimSpace(input.Gateway), strings.TrimSpace(input.IPRange)); err != nil {
		writeError(w, http.StatusConflict, "network_create_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		return io.ErrUnexpectedEOF
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return io.ErrUnexpectedEOF
	}
	return nil
}

type containerSummary struct {
	ID      string   `json:"id"`
	Names   []string `json:"names"`
	Image   string   `json:"image"`
	State   string   `json:"state"`
	Status  string   `json:"status"`
	Created int64    `json:"created"`
}

type containerDetail struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Created string `json:"created"`
}

func (m *Module) listContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := m.service.ListDockerContainers(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", err.Error())
		return
	}
	data := make([]containerSummary, 0, len(containers))
	for _, c := range containers {
		data = append(data, containerSummary{ID: c.ID, Names: c.Names, Image: c.Image, State: c.State, Status: c.Status, Created: c.Created})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (m *Module) getContainer(w http.ResponseWriter, r *http.Request) {
	c, err := m.service.InspectDockerContainer(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "container_not_found", err.Error())
		return
	}
	detail := containerDetail{ID: c.ID, Name: strings.TrimPrefix(c.Name, "/"), Created: c.Created}
	if c.Config != nil {
		detail.Image = c.Config.Image
	}
	if c.State != nil {
		detail.State = c.State.Status
		detail.Status = c.State.Error
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": detail})
}

func (m *Module) containerAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var err error
		switch action {
		case "start":
			err = m.service.StartDockerContainer(r.Context(), id)
		case "stop":
			err = m.service.StopDockerContainer(r.Context(), id)
		case "restart":
			err = m.service.RestartDockerContainer(r.Context(), id)
		}
		if err != nil {
			writeError(w, http.StatusConflict, "container_action_failed", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (m *Module) deleteContainer(w http.ResponseWriter, r *http.Request) {
	if err := m.service.RemoveDockerContainer(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, "container_delete_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) listImages(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListDockerImages(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", err.Error())
		return
	}
	data := make([]map[string]any, 0, len(items))
	for _, item := range items {
		data = append(data, map[string]any{"id": item.ID, "repo_tags": item.RepoTags, "size": item.Size, "created": item.Created})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (m *Module) deleteImage(w http.ResponseWriter, r *http.Request) {
	if err := m.service.RemoveDockerImage(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, "image_delete_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) listVolumes(w http.ResponseWriter, r *http.Request) {
	result, err := m.service.ListDockerVolumes(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", err.Error())
		return
	}
	data := make([]map[string]any, 0, len(result.Volumes))
	for _, item := range result.Volumes {
		if item != nil {
			data = append(data, map[string]any{"name": item.Name, "driver": item.Driver, "scope": item.Scope, "created_at": item.CreatedAt})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (m *Module) deleteVolume(w http.ResponseWriter, r *http.Request) {
	if err := m.service.RemoveDockerVolume(r.Context(), r.PathValue("name")); err != nil {
		writeError(w, http.StatusConflict, "volume_delete_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) listNetworks(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListDockerNetworks(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", err.Error())
		return
	}
	data := make([]map[string]any, 0, len(items))
	for _, item := range items {
		data = append(data, map[string]any{"id": item.ID, "name": item.Name, "driver": item.Driver, "scope": item.Scope, "internal": item.Internal, "attachable": item.Attachable})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (m *Module) deleteNetwork(w http.ResponseWriter, r *http.Request) {
	if err := m.service.RemoveDockerNetwork(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, "network_delete_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	if message == "" {
		message = code
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
