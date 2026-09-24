package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	docker "github.com/lsongdev/ohmyserver/modules/docker"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

type AppTemplate = docker.AppTemplate
type AppCatalogRecord = docker.AppCatalogRecord

func (r *Repository) InitializeAppCatalog(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS app_templates (
		id TEXT PRIMARY KEY, name TEXT, available INTEGER, exposable INTEGER, dynamic_config INTEGER,
		port INTEGER, platform_version INTEGER, version TEXT, categories TEXT, description TEXT,
		short_desc TEXT, author TEXT, source TEXT, form_fields TEXT, supported_architectures TEXT,
		created_at INTEGER, updated_at INTEGER, force_pull INTEGER, min_version TEXT, icon TEXT,
		compose_file TEXT, content_hash TEXT NOT NULL DEFAULT ''
	); CREATE INDEX IF NOT EXISTS idx_apps_name ON app_templates(name); CREATE INDEX IF NOT EXISTS idx_apps_available ON app_templates(available);`)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `ALTER TABLE app_templates ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	return nil
}

func (r *Repository) ReconcileAppCatalog(ctx context.Context, records []AppCatalogRecord) error {
	if len(records) == 0 {
		return fmt.Errorf("no valid app templates to reconcile")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO app_templates (
		id,name,available,exposable,dynamic_config,port,platform_version,version,categories,description,short_desc,author,source,form_fields,supported_architectures,created_at,updated_at,force_pull,min_version,icon,compose_file,content_hash
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET
		name=excluded.name,available=excluded.available,exposable=excluded.exposable,dynamic_config=excluded.dynamic_config,
		port=excluded.port,platform_version=excluded.platform_version,version=excluded.version,categories=excluded.categories,
		description=excluded.description,short_desc=excluded.short_desc,author=excluded.author,source=excluded.source,
		form_fields=excluded.form_fields,supported_architectures=excluded.supported_architectures,created_at=excluded.created_at,
		updated_at=excluded.updated_at,force_pull=excluded.force_pull,min_version=excluded.min_version,icon=excluded.icon,
		compose_file=excluded.compose_file,content_hash=excluded.content_hash WHERE app_templates.content_hash <> excluded.content_hash`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		a := record.App
		if _, err := stmt.ExecContext(ctx, a.ID, a.Name, boolInt(a.Available), boolInt(a.Exposable), boolInt(a.DynamicConfig), a.Port, a.PlatformVersion, a.Version, record.CategoriesJSON, a.Description, a.ShortDesc, a.Author, a.Source, record.FormFieldsJSON, record.ArchsJSON, a.CreatedAt, a.UpdatedAt, boolInt(a.ForcePull), a.MinVersion, a.Icon, a.ComposeFile, record.ContentHash); err != nil {
			return fmt.Errorf("upsert app template %s: %w", a.ID, err)
		}
		seen[a.ID] = struct{}{}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM app_templates`)
	if err != nil {
		return err
	}
	var removed []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if _, ok := seen[id]; !ok {
			removed = append(removed, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range removed {
		if _, err := tx.ExecContext(ctx, `DELETE FROM app_templates WHERE id=?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const appTemplateColumns = `id,name,available,exposable,dynamic_config,port,platform_version,version,categories,description,short_desc,author,source,form_fields,supported_architectures,created_at,updated_at,force_pull,min_version,icon,compose_file`

func scanAppTemplate(row interface{ Scan(...any) error }) (*AppTemplate, error) {
	var a AppTemplate
	var available, exposable, dynamic, force int
	var categories, fields, archs string
	err := row.Scan(&a.ID, &a.Name, &available, &exposable, &dynamic, &a.Port, &a.PlatformVersion, &a.Version, &categories, &a.Description, &a.ShortDesc, &a.Author, &a.Source, &fields, &archs, &a.CreatedAt, &a.UpdatedAt, &force, &a.MinVersion, &a.Icon, &a.ComposeFile)
	if err != nil {
		return nil, err
	}
	a.Available, a.Exposable, a.DynamicConfig, a.ForcePull = available == 1, exposable == 1, dynamic == 1, force == 1
	_ = json.Unmarshal([]byte(categories), &a.Categories)
	_ = json.Unmarshal([]byte(fields), &a.FormFields)
	_ = json.Unmarshal([]byte(archs), &a.SupportedArchitectures)
	return &a, nil
}

func (r *Repository) GetAppTemplate(ctx context.Context, id string) (*AppTemplate, error) {
	return scanAppTemplate(r.db.QueryRowContext(ctx, `SELECT `+appTemplateColumns+` FROM app_templates WHERE id=?`, id))
}

func (r *Repository) ListAppTemplates(ctx context.Context, category string) ([]*AppTemplate, error) {
	query := `SELECT ` + appTemplateColumns + ` FROM app_templates WHERE available=1`
	args := []any{}
	if category != "" && category != "all" {
		query += ` AND categories LIKE ?`
		args = append(args, `%"`+category+`"%`)
	}
	query += ` ORDER BY name COLLATE NOCASE`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*AppTemplate, 0)
	for rows.Next() {
		item, err := scanAppTemplate(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) AppCategories(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT categories FROM app_templates WHERE available=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := make(map[string]struct{})
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var categories []string
		if err := json.Unmarshal([]byte(raw), &categories); err == nil {
			for _, category := range categories {
				set[category] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(set))
	for category := range set {
		result = append(result, category)
	}
	sort.Strings(result)
	return result, rows.Err()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
