package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	yeelight "github.com/lsongdev/yeelight-go/yeelight"
)

// YeelightDevice represents a Yeelight smart light
type YeelightDevice struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Status    string    `json:"status"` // online, offline
	LastSeen  time.Time `json:"last_seen"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	client    *yeelight.Yeelight
}

// YeelightState represents the current state of a Yeelight device
type YeelightState struct {
	Power     string `json:"power"`
	Bright    int    `json:"bright"`
	CT        int    `json:"ct"`
	RGB       int    `json:"rgb"`
	Hue       int    `json:"hue"`
	Sat       int    `json:"sat"`
	ColorMode int    `json:"color_mode"`
	Name      string `json:"name"`
}

// initYeelightSchema creates the Yeelight device table
func (s *Server) initYeelightSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS yeelight_devices (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER DEFAULT 55443,
		status TEXT NOT NULL DEFAULT 'offline',
		last_seen DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_yeelight_devices_name ON yeelight_devices(name);
	CREATE INDEX IF NOT EXISTS idx_yeelight_devices_host ON yeelight_devices(host);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateYeelightDevice creates a new Yeelight device
func (s *Server) CreateYeelightDevice(name, host string, port int) (*YeelightDevice, error) {

	now := time.Now()
	query := `
	INSERT INTO yeelight_devices (name, host, port, status, created_at, updated_at)
	VALUES (?, ?, ?, 'offline', ?, ?)
	`
	result, err := s.db.Exec(query, name, host, port, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &YeelightDevice{
		ID:        id,
		Name:      name,
		Host:      host,
		Port:      port,
		Status:    "offline",
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// GetAllYeelightDevices retrieves all Yeelight devices
func (s *Server) GetAllYeelightDevices() ([]YeelightDevice, error) {

	query := `
	SELECT id, name, host, port, status, last_seen, created_at, updated_at
	FROM yeelight_devices
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []YeelightDevice
	for rows.Next() {
		var device YeelightDevice
		var lastSeen sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&device.ID, &device.Name, &device.Host, &device.Port, &device.Status, &lastSeen, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if lastSeen.Valid {
			device.LastSeen = parseTime(lastSeen.String)
		}
		device.CreatedAt = parseTime(createdAt)
		device.UpdatedAt = parseTime(updatedAt)
		devices = append(devices, device)
	}

	return devices, rows.Err()
}

// GetYeelightDeviceByID retrieves a Yeelight device by ID
func (s *Server) GetYeelightDeviceByID(id int64) (*YeelightDevice, error) {

	query := `
	SELECT id, name, host, port, status, last_seen, created_at, updated_at
	FROM yeelight_devices
	WHERE id = ?
	`

	var device YeelightDevice
	var lastSeen sql.NullString
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&device.ID, &device.Name, &device.Host, &device.Port, &device.Status, &lastSeen, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if lastSeen.Valid {
		device.LastSeen = parseTime(lastSeen.String)
	}
	device.CreatedAt = parseTime(createdAt)
	device.UpdatedAt = parseTime(updatedAt)

	return &device, nil
}

// UpdateYeelightDevice updates an existing Yeelight device
func (s *Server) UpdateYeelightDevice(id int64, name, host string, port int) error {

	now := time.Now()
	query := `
	UPDATE yeelight_devices
	SET name = ?, host = ?, port = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, host, port, now, id)
	return err
}

// DeleteYeelightDevice deletes a Yeelight device by ID
func (s *Server) DeleteYeelightDevice(id int64) error {

	query := `DELETE FROM yeelight_devices WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// GetYeelightDeviceState retrieves the current state from the device
func (d *YeelightDevice) GetYeelightDeviceState() (*YeelightState, error) {
	client := yeelight.New(&yeelight.Config{
		Host:    d.Host,
		Port:    d.Port,
		Timeout: 3 * time.Second,
	})

	result, err := client.GetProp("power", "bright", "ct", "rgb", "hue", "sat", "color_mode", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to get device state: %w", err)
	}

	state := &YeelightState{}
	if len(result.Result) >= 8 {
		// All values from GetProp are returned as strings
		if v, ok := result.Result[0].(string); ok {
			state.Power = v
		}
		if v, ok := result.Result[1].(string); ok {
			if val, err := strconv.Atoi(v); err == nil {
				state.Bright = val
			}
		}
		if v, ok := result.Result[2].(string); ok {
			if val, err := strconv.Atoi(v); err == nil {
				state.CT = val
			}
		}
		if v, ok := result.Result[3].(string); ok {
			if val, err := strconv.ParseInt(v, 10, 64); err == nil {
				state.RGB = int(val)
			}
		}
		if v, ok := result.Result[4].(string); ok {
			if val, err := strconv.Atoi(v); err == nil {
				state.Hue = val
			}
		}
		if v, ok := result.Result[5].(string); ok {
			if val, err := strconv.Atoi(v); err == nil {
				state.Sat = val
			}
		}
		if v, ok := result.Result[6].(string); ok {
			if val, err := strconv.Atoi(v); err == nil {
				state.ColorMode = val
			}
		}
		if v, ok := result.Result[7].(string); ok {
			state.Name = v
		}
	}

	return state, nil
}

// YeelightListView renders the Yeelight device list page
func (s *Server) YeelightListView(w http.ResponseWriter, r *http.Request) {
	devices, err := s.GetAllYeelightDevices()
	if err != nil {
		log.Printf("Error fetching Yeelight devices: %v", err)
		s.Error(w, err)
		return
	}

	s.Render(w, "yeelight/index", H{
		"devices": devices,
	})
}

// YeelightNewHandler handles creating a new Yeelight device
func (s *Server) YeelightNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "yeelight/new", H{})
		return
	}

	// POST: Create new Yeelight device
	name := r.FormValue("name")
	host := r.FormValue("host")
	port := 55443

	if portStr := r.FormValue("port"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			port = 55443
		}
	}

	if name == "" || host == "" {
		s.Render(w, "yeelight/new", H{
			"error": "Name and host are required",
			"name":  name,
			"host":  host,
			"port":  port,
		})
		return
	}

	_, err := s.CreateYeelightDevice(name, host, port)
	if err != nil {
		s.Render(w, "yeelight/new", H{
			"error": "Failed to create device: " + err.Error(),
			"name":  name,
			"host":  host,
			"port":  port,
		})
		return
	}

	http.Redirect(w, r, "/yeelight", http.StatusSeeOther)
}

// YeelightEditHandler handles editing a Yeelight device
func (s *Server) YeelightEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	device, err := s.GetYeelightDeviceByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Yeelight device not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		s.Render(w, "yeelight/new", H{
			"server": device,
		})
		return
	}

	// POST: Update Yeelight device
	name := r.FormValue("name")
	host := r.FormValue("host")
	port := 55443

	if portStr := r.FormValue("port"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			port = 55443
		}
	}

	err = s.UpdateYeelightDevice(id, name, host, port)
	if err != nil {
		s.Render(w, "yeelight/new", H{
			"error":  "Failed to update device: " + err.Error(),
			"server": device,
		})
		return
	}

	http.Redirect(w, r, "/yeelight", http.StatusSeeOther)
}

// YeelightDetailHandler handles viewing/deleting a Yeelight device
func (s *Server) YeelightDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	device, err := s.GetYeelightDeviceByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Yeelight device not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	// Check for method override
	method := r.Method
	if r.Method == http.MethodPost {
		overrideMethod := r.FormValue("_method")
		if overrideMethod != "" {
			method = overrideMethod
		}
	}

	switch method {
	case http.MethodGet:
		// Get device state on-demand
		var deviceState *YeelightState
		var connectionError string

		state, err := device.GetYeelightDeviceState()
		if err != nil {
			connectionError = err.Error()
			// Update status to offline

			s.db.Exec("UPDATE yeelight_devices SET status = 'offline', last_seen = ?, updated_at = ? WHERE id = ?", time.Now(), time.Now(), id)

		} else {
			deviceState = state
			// Update status to online

			s.db.Exec("UPDATE yeelight_devices SET status = 'online', last_seen = ?, updated_at = ? WHERE id = ?", time.Now(), time.Now(), id)

			// Refresh device data
			device, _ = s.GetYeelightDeviceByID(id)
		}

		s.Render(w, "yeelight/detail", H{
			"device":          device,
			"state":           deviceState,
			"connectionError": connectionError,
		})

	case http.MethodPut:
		// Update Yeelight device
		name := r.FormValue("name")
		host := r.FormValue("host")
		port := 55443

		if portStr := r.FormValue("port"); portStr != "" {
			var err error
			port, err = strconv.Atoi(portStr)
			if err != nil {
				port = 55443
			}
		}

		err = s.UpdateYeelightDevice(id, name, host, port)
		if err != nil {
			s.Render(w, "yeelight/detail", H{
				"error":  "Failed to update device: " + err.Error(),
				"device": device,
			})
			return
		}

		http.Redirect(w, r, "/yeelight", http.StatusSeeOther)

	case http.MethodDelete:
		err = s.DeleteYeelightDevice(id)
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/yeelight", http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// YeelightPowerControlHandler handles power control
func (s *Server) YeelightPowerControlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	device, err := s.GetYeelightDeviceByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Yeelight device not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	action := r.FormValue("action")
	if action == "" {
		s.Error(w, &Error{Message: "Power action is required"})
		return
	}

	client := yeelight.New(&yeelight.Config{
		Host:    device.Host,
		Port:    device.Port,
		Timeout: 3 * time.Second,
	})

	effect := &yeelight.Effect{
		Effect:   "smooth",
		Duration: 500,
	}

	var result *yeelight.CommandResult
	switch action {
	case "on":
		result, err = client.SetPower("on", effect, yeelight.Normal)
	case "off":
		result, err = client.SetPower("off", effect, yeelight.Normal)
	case "toggle":
		result, err = client.Toggle()
	default:
		s.Error(w, &Error{Message: "Unknown action: " + action})
		return
	}

	if err != nil {
		s.Error(w, &Error{Message: "Failed to control power: " + err.Error()})
		return
	}

	if result.Error != nil {
		s.Error(w, &Error{Message: fmt.Sprintf("Device error: %s", result.Error.Message)})
		return
	}

	// Redirect back to detail page
	http.Redirect(w, r, fmt.Sprintf("/yeelight/%d", id), http.StatusSeeOther)
}

// YeelightBrightnessControlHandler handles brightness control
func (s *Server) YeelightBrightnessControlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	device, err := s.GetYeelightDeviceByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Yeelight device not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	brightness, err := strconv.Atoi(r.FormValue("brightness"))
	if err != nil || brightness < 1 || brightness > 100 {
		s.Error(w, &Error{Message: "Brightness must be between 1 and 100"})
		return
	}

	client := yeelight.New(&yeelight.Config{
		Host:    device.Host,
		Port:    device.Port,
		Timeout: 3 * time.Second,
	})

	effect := &yeelight.Effect{
		Effect:   "smooth",
		Duration: 500,
	}

	result, err := client.SetBright(brightness, effect)
	if err != nil {
		s.Error(w, &Error{Message: "Failed to set brightness: " + err.Error()})
		return
	}

	if result.Error != nil {
		s.Error(w, &Error{Message: fmt.Sprintf("Device error: %s", result.Error.Message)})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/yeelight/%d", id), http.StatusSeeOther)
}

// YeelightColorTempControlHandler handles color temperature control
func (s *Server) YeelightColorTempControlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	device, err := s.GetYeelightDeviceByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Yeelight device not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ct, err := strconv.Atoi(r.FormValue("ct"))
	if err != nil || ct < 1700 || ct > 6500 {
		s.Error(w, &Error{Message: "Color temperature must be between 1700 and 6500"})
		return
	}

	client := yeelight.New(&yeelight.Config{
		Host:    device.Host,
		Port:    device.Port,
		Timeout: 3 * time.Second,
	})

	effect := &yeelight.Effect{
		Effect:   "smooth",
		Duration: 500,
	}

	result, err := client.SetCT(ct, effect)
	if err != nil {
		s.Error(w, &Error{Message: "Failed to set color temperature: " + err.Error()})
		return
	}

	if result.Error != nil {
		s.Error(w, &Error{Message: fmt.Sprintf("Device error: %s", result.Error.Message)})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/yeelight/%d", id), http.StatusSeeOther)
}

// YeelightColorControlHandler handles RGB color control
func (s *Server) YeelightColorControlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	device, err := s.GetYeelightDeviceByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Yeelight device not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	colorStr := r.FormValue("color")
	if colorStr == "" {
		s.Error(w, &Error{Message: "Color is required"})
		return
	}

	// Parse hex color (remove # if present)
	colorStr = strings.TrimPrefix(colorStr, "#")
	if len(colorStr) != 6 {
		s.Error(w, &Error{Message: "Color must be a 6-digit hex value"})
		return
	}

	colorInt, err := strconv.ParseInt(colorStr, 16, 64)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid color value"})
		return
	}

	client := yeelight.New(&yeelight.Config{
		Host:    device.Host,
		Port:    device.Port,
		Timeout: 3 * time.Second,
	})

	effect := &yeelight.Effect{
		Effect:   "smooth",
		Duration: 500,
	}

	result, err := client.SetRGB(int(colorInt), effect)
	if err != nil {
		s.Error(w, &Error{Message: "Failed to set color: " + err.Error()})
		return
	}

	if result.Error != nil {
		s.Error(w, &Error{Message: fmt.Sprintf("Device error: %s", result.Error.Message)})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/yeelight/%d", id), http.StatusSeeOther)
}

// YeelightTestConnectionHandler tests the connection to a Yeelight device
func (s *Server) YeelightTestConnectionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	device, err := s.GetYeelightDeviceByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Yeelight device not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	// Try to get device state
	_, err = device.GetYeelightDeviceState()
	if err != nil {
		// Update status to offline

		s.db.Exec("UPDATE yeelight_devices SET status = 'offline', last_seen = ?, updated_at = ? WHERE id = ?", time.Now(), time.Now(), id)

		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Connection failed: " + err.Error()))
		return
	}

	// Update status to online

	s.db.Exec("UPDATE yeelight_devices SET status = 'online', last_seen = ?, updated_at = ? WHERE id = ?", time.Now(), time.Now(), id)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Connection successful"))
}

// YeelightDiscoverHandler handles device discovery via SSE
func (s *Server) YeelightDiscoverHandler(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	// Discover devices in a goroutine
	go func() {
		// Use yeelight discovery
		lights, err := yeelight.Discover()
		if err != nil {
			log.Printf("Error discovering Yeelight devices: %v", err)
			// Send error event
			fmt.Fprintf(w, "event: error\ndata: {\"error\": \"%s\"}\n\n", err.Error())
			flusher.Flush()
			return
		}

		// Send discovered devices
		for _, light := range lights {
			select {
			case <-ctx.Done():
				return
			default:
				device := H{
					"host": light.GetHost(),
					"port": light.GetPort(),
					"name": "",
				}

				data, err := json.Marshal(device)
				if err != nil {
					continue
				}

				fmt.Fprintf(w, "event: device\ndata: %s\n\n", string(data))
				flusher.Flush()

				// Small delay between devices
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Send completion event
		fmt.Fprintf(w, "event: done\ndata: {}\n\n")
		flusher.Flush()
	}()

	// Keep connection open until context is done
	<-ctx.Done()
}

func init() {
	RegisterInitSchemaHook((*Server).initYeelightSchema)
}
