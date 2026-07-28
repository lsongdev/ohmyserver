package server

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Setting represents a setting in the database
type Setting struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"` // string, number, boolean, json
	Value       string    `json:"value"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// initSettingsSchema creates the settings table
func (s *Server) initSettingsSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS settings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL DEFAULT 'string',
		value TEXT NOT NULL,
		description TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_settings_name ON settings(name);
	CREATE INDEX IF NOT EXISTS idx_settings_type ON settings(type);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateSetting creates a new setting
func (s *Server) CreateSetting(name, settingType, value, description string) (*Setting, error) {

	// Validate type
	validTypes := map[string]bool{
		"string":  true,
		"number":  true,
		"boolean": true,
		"json":    true,
	}
	if !validTypes[settingType] {
		return nil, fmt.Errorf("invalid setting type: %s", settingType)
	}

	now := time.Now()
	query := `
	INSERT INTO settings (name, type, value, description, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, settingType, value, description, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Setting{
		ID:          id,
		Name:        name,
		Type:        settingType,
		Value:       value,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetAllSettings retrieves all settings
func (s *Server) GetAllSettings() ([]Setting, error) {

	query := `
	SELECT id, name, type, value, description, created_at, updated_at
	FROM settings
	ORDER BY name ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []Setting
	for rows.Next() {
		var setting Setting
		var createdAt, updatedAt string
		if err := rows.Scan(&setting.ID, &setting.Name, &setting.Type, &setting.Value, &setting.Description, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		setting.CreatedAt = parseTime(createdAt)
		setting.UpdatedAt = parseTime(updatedAt)
		settings = append(settings, setting)
	}

	return settings, rows.Err()
}

// GetSettingByID retrieves a setting by ID
func (s *Server) GetSettingByID(id int64) (*Setting, error) {

	query := `
	SELECT id, name, type, value, description, created_at, updated_at
	FROM settings
	WHERE id = ?
	`

	var setting Setting
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&setting.ID, &setting.Name, &setting.Type, &setting.Value, &setting.Description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	setting.CreatedAt = parseTime(createdAt)
	setting.UpdatedAt = parseTime(updatedAt)

	return &setting, nil
}

// GetSettingByName retrieves a setting by name
func (s *Server) GetSettingByName(name string) (*Setting, error) {

	query := `
	SELECT id, name, type, value, description, created_at, updated_at
	FROM settings
	WHERE name = ?
	`

	var setting Setting
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, name).Scan(&setting.ID, &setting.Name, &setting.Type, &setting.Value, &setting.Description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	setting.CreatedAt = parseTime(createdAt)
	setting.UpdatedAt = parseTime(updatedAt)

	return &setting, nil
}

// UpdateSetting updates an existing setting
func (s *Server) UpdateSetting(id int64, value string) error {

	now := time.Now()
	query := `
	UPDATE settings
	SET value = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, value, now, id)
	return err
}

// DeleteSetting deletes a setting by ID
func (s *Server) DeleteSetting(id int64) error {

	query := `DELETE FROM settings WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// SettingsListView renders the settings list page
func (s *Server) SettingsListView(w http.ResponseWriter, r *http.Request) {
	settings, err := s.GetAllSettings()
	if err != nil {
		log.Printf("Error fetching settings: %v", err)
		s.Error(w, err)
		return
	}

	s.Render(w, "settings/index", H{
		"settings": settings,
	})
}

// SettingsNewHandler handles creating a new setting
func (s *Server) SettingsNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "settings/new", H{})
		return
	}

	// POST: Create new setting
	name := r.FormValue("name")
	settingType := r.FormValue("type")
	value := r.FormValue("value")
	description := r.FormValue("description")

	if name == "" || settingType == "" || value == "" {
		s.Render(w, "settings/new", H{
			"error":       "Name, type, and value are required",
			"name":        name,
			"type":        settingType,
			"value":       value,
			"description": description,
		})
		return
	}

	_, err := s.CreateSetting(name, settingType, value, description)
	if err != nil {
		s.Render(w, "settings/new", H{
			"error":       "Failed to create setting: " + err.Error(),
			"name":        name,
			"type":        settingType,
			"value":       value,
			"description": description,
		})
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// SettingsEditHandler handles editing a setting
func (s *Server) SettingsEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}
	value := r.FormValue("value")
	err = s.UpdateSetting(id, value)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func init() {
	RegisterInitSchemaHook((*Server).initSettingsSchema)
}
