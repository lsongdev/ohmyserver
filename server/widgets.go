package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Widget struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Config    string    `json:"config"`
	SortOrder int       `json:"sort_order"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Server) initWidgetSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS widgets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config TEXT DEFAULT '',
		sort_order INTEGER DEFAULT 0,
		enabled INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_widgets_enabled ON widgets(enabled);
	CREATE INDEX IF NOT EXISTS idx_widgets_sort_order ON widgets(sort_order);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *Server) CreateWidget(name, widgetType, config string, sortOrder int, enabled bool) (*Widget, error) {
	now := time.Now()
	query := `
	INSERT INTO widgets (name, type, config, sort_order, enabled, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, widgetType, config, sortOrder, enabled, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Widget{
		ID:        id,
		Name:      name,
		Type:      widgetType,
		Config:    config,
		SortOrder: sortOrder,
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *Server) GetAllWidgets() ([]Widget, error) {
	query := `
	SELECT id, name, type, config, sort_order, enabled, created_at, updated_at
	FROM widgets
	ORDER BY sort_order ASC, name ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var widgets []Widget
	for rows.Next() {
		var w Widget
		var createdAt, updatedAt string
		if err := rows.Scan(&w.ID, &w.Name, &w.Type, &w.Config, &w.SortOrder, &w.Enabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		w.CreatedAt = parseTime(createdAt)
		w.UpdatedAt = parseTime(updatedAt)
		widgets = append(widgets, w)
	}

	return widgets, rows.Err()
}

func (s *Server) GetEnabledWidgets() ([]Widget, error) {
	query := `
	SELECT id, name, type, config, sort_order, enabled, created_at, updated_at
	FROM widgets
	WHERE enabled = 1
	ORDER BY sort_order ASC, name ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var widgets []Widget
	for rows.Next() {
		var w Widget
		var createdAt, updatedAt string
		if err := rows.Scan(&w.ID, &w.Name, &w.Type, &w.Config, &w.SortOrder, &w.Enabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		w.CreatedAt = parseTime(createdAt)
		w.UpdatedAt = parseTime(updatedAt)
		widgets = append(widgets, w)
	}

	return widgets, rows.Err()
}

func (s *Server) GetWidgetByID(id int64) (*Widget, error) {
	query := `
	SELECT id, name, type, config, sort_order, enabled, created_at, updated_at
	FROM widgets
	WHERE id = ?
	`

	var w Widget
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&w.ID, &w.Name, &w.Type, &w.Config, &w.SortOrder, &w.Enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	w.CreatedAt = parseTime(createdAt)
	w.UpdatedAt = parseTime(updatedAt)

	return &w, nil
}

func (s *Server) UpdateWidget(id int64, name, widgetType, config string, sortOrder int, enabled bool) error {
	now := time.Now()
	query := `
	UPDATE widgets
	SET name = ?, type = ?, config = ?, sort_order = ?, enabled = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, widgetType, config, sortOrder, enabled, now, id)
	return err
}

func (s *Server) DeleteWidget(id int64) error {
	query := `DELETE FROM widgets WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

func (s *Server) WidgetListView(w http.ResponseWriter, r *http.Request) {
	items, err := s.GetAllWidgets()
	if err != nil {
		log.Printf("Error fetching widgets: %v", err)
		s.Error(w, err)
		return
	}
	s.Render(w, "widgets/index", H{"items": items})
}

func (s *Server) WidgetNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "widgets/new", H{})
		return
	}

	name := r.FormValue("name")
	widgetType := r.FormValue("type")
	config := r.FormValue("config")
	sortOrderStr := r.FormValue("sort_order")
	enabled := r.FormValue("enabled") == "on"

	sortOrder, _ := strconv.Atoi(sortOrderStr)

	if name == "" || widgetType == "" {
		s.Render(w, "widgets/new", H{
			"error": "Name and type are required",
			"name":  name,
			"type":  widgetType,
			"config": config,
		})
		return
	}

	_, err := s.CreateWidget(name, widgetType, config, sortOrder, enabled)
	if err != nil {
		s.Render(w, "widgets/new", H{
			"error":  "Failed to create widget: " + err.Error(),
			"name":   name,
			"type":   widgetType,
			"config": config,
		})
		return
	}

	http.Redirect(w, r, "/widgets", http.StatusSeeOther)
}

func (s *Server) WidgetDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	item, err := s.GetWidgetByID(id)
	if err != nil {
		log.Printf("Error fetching widget %d: %v", id, err)
		s.Error(w, err)
		return
	}

	method := r.Method
	if r.Method == http.MethodPost {
		if m := r.FormValue("_method"); m != "" {
			method = m
		}
	}

	switch method {
	case http.MethodGet:
		s.Render(w, "widgets/detail", H{"item": item})

	case http.MethodPut:
		name := r.FormValue("name")
		widgetType := r.FormValue("type")
		config := r.FormValue("config")
		sortOrderStr := r.FormValue("sort_order")
		enabled := r.FormValue("enabled") == "on"
		sortOrder, _ := strconv.Atoi(sortOrderStr)

		err = s.UpdateWidget(id, name, widgetType, config, sortOrder, enabled)
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/widgets", http.StatusSeeOther)

	case http.MethodDelete:
		err = s.DeleteWidget(id)
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/widgets", http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) WidgetEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	item, err := s.GetWidgetByID(id)
	if err != nil {
		log.Printf("Error fetching widget %d: %v", id, err)
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		configParsed := make(map[string]any)
		if item.Config != "" {
			json.Unmarshal([]byte(item.Config), &configParsed)
		}
		s.Render(w, "widgets/new", H{
			"server": item,
			"config": configParsed,
		})
		return
	}

	// POST: update
	name := r.FormValue("name")
	widgetType := r.FormValue("type")
	config := r.FormValue("config")
	sortOrderStr := r.FormValue("sort_order")
	enabled := r.FormValue("enabled") == "on"
	sortOrder, _ := strconv.Atoi(sortOrderStr)

	err = s.UpdateWidget(id, name, widgetType, config, sortOrder, enabled)
	if err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/widgets", http.StatusSeeOther)
}

func init() {
	RegisterInitSchemaHook((*Server).initWidgetSchema)
}
