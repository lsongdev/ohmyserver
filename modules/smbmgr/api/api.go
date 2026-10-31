// Package api is the Samba manager's JSON API: shares and server settings
// edited as parameters, smb.conf edited as text, Samba users, and who is
// connected. Every change to smb.conf passes testparm before it replaces
// the file, and smbd reloads after it.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/lsongdev/ohmyserver/modules/smbmgr/system"
)

// Options configures the API.
type Options struct {
	Logger *slog.Logger
	// Runner runs commands; nil runs them on this host.
	Runner system.Runner
	// Config is smb.conf; empty asks smbd.
	Config string
	// Passwd lists the system's users; empty is /etc/passwd.
	Passwd string
	// Installed overrides looking for smbd and testparm, for tests.
	Installed *bool
}

// Service serves the API.
type Service struct {
	samba     *system.Samba
	installed bool
	passwd    string
	logger    *slog.Logger
}

// New finds Samba and its configuration file.
func New(opts Options) *Service {
	run := opts.Runner
	if run == nil {
		run = system.Exec{}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	installed := system.Installed()
	if opts.Installed != nil {
		installed = *opts.Installed
	}
	config := opts.Config
	if config == "" && installed {
		config = system.DefaultConfig(context.Background(), run)
	}
	passwd := opts.Passwd
	if passwd == "" {
		passwd = "/etc/passwd"
	}
	return &Service{samba: &system.Samba{Run: run, Config: config}, installed: installed, passwd: passwd, logger: logger}
}

// Supported reports whether Samba is installed. Without it every endpoint
// but /info answers 503 "unsupported", so a shell may leave the module out.
func (s *Service) Supported() bool { return s.installed }

// Info describes the server.
type Info struct {
	Installed bool            `json:"installed"`
	Version   string          `json:"version,omitempty"`
	Config    string          `json:"config,omitempty"`
	Service   *system.Service `json:"service,omitempty"`
}

// Share summarizes a share for the list, with Samba's defaults applied.
type Share struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Comment     string `json:"comment,omitempty"`
	ReadOnly    bool   `json:"read_only"`
	Guest       bool   `json:"guest"`
	Browseable  bool   `json:"browseable"`
	Available   bool   `json:"available"`
	ValidUsers  string `json:"valid_users,omitempty"`
	WriteList   string `json:"write_list,omitempty"`
	Special     bool   `json:"special,omitempty"` // [homes] or [printers]
	Printable   bool   `json:"printable,omitempty"`
	PathMissing bool   `json:"path_missing,omitempty"`
}

func summarize(sec *system.Section) Share {
	sh := Share{Name: sec.Name, ReadOnly: true, Browseable: true, Available: true}
	sh.Special = strings.EqualFold(sec.Name, "homes") || strings.EqualFold(sec.Name, "printers")
	// In file order, so the last of a parameter and its synonyms wins.
	for _, p := range sec.Params() {
		b, isBool := system.Bool(p.Value)
		switch system.Canon(p.Key) {
		case "path", "directory":
			sh.Path = p.Value
		case "comment":
			sh.Comment = p.Value
		case "readonly":
			if isBool {
				sh.ReadOnly = b
			}
		case "writeable", "writable", "writeok":
			if isBool {
				sh.ReadOnly = !b
			}
		case "guestok", "public":
			if isBool {
				sh.Guest = b
			}
		case "browseable", "browsable":
			if isBool {
				sh.Browseable = b
			}
		case "available":
			if isBool {
				sh.Available = b
			}
		case "printable", "printok":
			if isBool {
				sh.Printable = b
			}
		case "validusers":
			sh.ValidUsers = p.Value
		case "writelist":
			sh.WriteList = p.Value
		}
	}
	// Paths with substitutions (%S, %U, …) depend on who connects.
	if sh.Path != "" && !strings.Contains(sh.Path, "%") {
		if _, err := os.Stat(sh.Path); err != nil {
			sh.PathMissing = true
		}
	}
	return sh
}

// Shares is the list with the version of smb.conf it came from.
type Shares struct {
	Version string  `json:"version"`
	Shares  []Share `json:"shares"`
}

// Section is a share's or [global]'s parameters, with the version they came
// from; edits send that version back.
type Section struct {
	Version string         `json:"version"`
	Name    string         `json:"name"`
	Params  []system.Param `json:"params"`
	Share   *Share         `json:"share,omitempty"`
}

type sectionBody struct {
	Version string         `json:"version"`
	Name    string         `json:"name"`
	Params  []system.Param `json:"params"`
}

type textBody struct {
	Text    string `json:"text"`
	Version string `json:"version"`
}

type userBody struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(pattern string, fn func(*http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !s.installed {
				respond(w, nil, system.ErrUnsupported)
				return
			}
			data, err := fn(r)
			respond(w, data, err)
		})
	}
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		info := Info{Installed: s.installed}
		if s.installed {
			info.Version = s.samba.Version(r.Context())
			info.Config = s.samba.Config
			info.Service = s.samba.Service(r.Context())
		}
		respond(w, info, nil)
	})

	// smb.conf as text.
	handle("GET /config", func(r *http.Request) (any, error) { return s.samba.Read() })
	handle("POST /config/check", func(r *http.Request) (any, error) {
		var body textBody
		if err := decode(r, &body); err != nil {
			return nil, err
		}
		return s.samba.Check(r.Context(), body.Text)
	})
	handle("PUT /config", func(r *http.Request) (any, error) {
		var body textBody
		if err := decode(r, &body); err != nil {
			return nil, err
		}
		if body.Version == "" {
			return nil, fmt.Errorf("%w: version is required", system.ErrInvalid)
		}
		saved, err := s.samba.Write(r.Context(), body.Text, body.Version)
		s.logged(err, "saved smb.conf")
		return saved, err
	})
	handle("POST /reload", func(r *http.Request) (any, error) { return nil, s.samba.Reload(r.Context()) })

	// [global].
	handle("GET /global", func(r *http.Request) (any, error) {
		f, err := s.samba.Read()
		if err != nil {
			return nil, err
		}
		global := system.Parse(f.Text).Global()
		return Section{Version: f.Version, Name: global.Name, Params: global.Params()}, nil
	})
	handle("PUT /global", func(r *http.Request) (any, error) {
		var body sectionBody
		if err := decodeSection(r, &body); err != nil {
			return nil, err
		}
		saved, err := s.samba.Edit(r.Context(), body.Version, func(c *system.Conf) error {
			return c.Global().SetParams(body.Params)
		})
		s.logged(err, "saved the global settings")
		return saved, err
	})

	// Shares.
	handle("GET /shares", func(r *http.Request) (any, error) {
		f, err := s.samba.Read()
		if err != nil {
			return nil, err
		}
		list := Shares{Version: f.Version, Shares: []Share{}}
		for _, sec := range system.Parse(f.Text).Shares() {
			list.Shares = append(list.Shares, summarize(sec))
		}
		return list, nil
	})
	handle("GET /shares/{name}", func(r *http.Request) (any, error) {
		f, err := s.samba.Read()
		if err != nil {
			return nil, err
		}
		sec := system.Parse(f.Text).Section(r.PathValue("name"))
		if sec == nil || strings.EqualFold(sec.Name, "global") {
			return nil, fmt.Errorf("%w: no share named %q", system.ErrNotFound, r.PathValue("name"))
		}
		sh := summarize(sec)
		return Section{Version: f.Version, Name: sec.Name, Params: sec.Params(), Share: &sh}, nil
	})
	handle("POST /shares", func(r *http.Request) (any, error) {
		var body sectionBody
		if err := decodeSection(r, &body); err != nil {
			return nil, err
		}
		saved, err := s.samba.Edit(r.Context(), body.Version, func(c *system.Conf) error {
			_, err := c.AddShare(body.Name, body.Params)
			return err
		})
		s.logged(err, "added a share", "share", body.Name)
		return saved, err
	})
	handle("PUT /shares/{name}", func(r *http.Request) (any, error) {
		var body sectionBody
		if err := decodeSection(r, &body); err != nil {
			return nil, err
		}
		name := r.PathValue("name")
		saved, err := s.samba.Edit(r.Context(), body.Version, func(c *system.Conf) error {
			sec := c.Section(name)
			if sec == nil || strings.EqualFold(sec.Name, "global") {
				return fmt.Errorf("%w: no share named %q", system.ErrNotFound, name)
			}
			if body.Name != "" {
				if err := c.Rename(sec, body.Name); err != nil {
					return err
				}
			}
			return sec.SetParams(body.Params)
		})
		s.logged(err, "saved a share", "share", name)
		return saved, err
	})
	handle("DELETE /shares/{name}", func(r *http.Request) (any, error) {
		name := r.PathValue("name")
		saved, err := s.samba.Edit(r.Context(), r.URL.Query().Get("version"), func(c *system.Conf) error {
			if !c.RemoveShare(name) {
				return fmt.Errorf("%w: no share named %q", system.ErrNotFound, name)
			}
			return nil
		})
		s.logged(err, "removed a share", "share", name)
		return saved, err
	})

	// Users.
	handle("GET /users", func(r *http.Request) (any, error) { return s.samba.Users(r.Context()) })
	handle("GET /system-users", func(r *http.Request) (any, error) { return system.SystemUsers(s.passwd) })
	handle("POST /users", func(r *http.Request) (any, error) {
		var body userBody
		if err := decode(r, &body); err != nil {
			return nil, err
		}
		if err := s.systemUser(body.Name); err != nil {
			return nil, err
		}
		err := s.samba.AddUser(r.Context(), body.Name, body.Password)
		s.logged(err, "added a user", "user", body.Name)
		return nil, err
	})
	handle("PUT /users/{name}/password", func(r *http.Request) (any, error) {
		var body userBody
		if err := decode(r, &body); err != nil {
			return nil, err
		}
		name := r.PathValue("name")
		err := s.samba.SetPassword(r.Context(), name, body.Password)
		s.logged(err, "changed a user's password", "user", name)
		return nil, err
	})
	handle("POST /users/{name}/{action}", func(r *http.Request) (any, error) {
		name, action := r.PathValue("name"), r.PathValue("action")
		if action != "enable" && action != "disable" {
			return nil, fmt.Errorf("%w action %q", system.ErrInvalid, action)
		}
		err := s.samba.SetEnabled(r.Context(), name, action == "enable")
		s.logged(err, action+"d a user", "user", name)
		return nil, err
	})
	handle("DELETE /users/{name}", func(r *http.Request) (any, error) {
		name := r.PathValue("name")
		err := s.samba.DeleteUser(r.Context(), name)
		s.logged(err, "removed a user", "user", name)
		return nil, err
	})

	handle("GET /status", func(r *http.Request) (any, error) { return s.samba.Status(r.Context()) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint", nil)
	})
	return mux
}

// systemUser checks that a Samba user would have a system account to run as.
func (s *Service) systemUser(name string) error {
	users, err := system.SystemUsers(s.passwd)
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.Name == name {
			return nil
		}
	}
	return fmt.Errorf("%w: %q is not a system user (uid 1000 or above); create the account first", system.ErrInvalid, name)
}

func (s *Service) logged(err error, msg string, args ...any) {
	if err == nil {
		s.logger.Info("samba: "+msg, args...)
	}
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v); err != nil {
		return fmt.Errorf("%w body: %v", system.ErrInvalid, err)
	}
	return nil
}

func decodeSection(r *http.Request, body *sectionBody) error {
	if err := decode(r, body); err != nil {
		return err
	}
	if body.Version == "" {
		return fmt.Errorf("%w: version is required", system.ErrInvalid)
	}
	if body.Params == nil {
		body.Params = []system.Param{}
	}
	return nil
}

func respond(w http.ResponseWriter, data any, err error) {
	var check *system.CheckError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	case errors.As(err, &check):
		writeError(w, http.StatusUnprocessableEntity, "invalid_config", strings.Join(check.Check.Errors, "\n"), check.Check)
	case errors.Is(err, system.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
	case errors.Is(err, system.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, system.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, system.ErrUnsupported):
		writeError(w, http.StatusServiceUnavailable, "unsupported", err.Error(), nil)
	default:
		var exit *system.ExitError
		if errors.As(err, &exit) {
			writeError(w, http.StatusBadGateway, "command_failed", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string, check any) {
	e := map[string]any{"code": code, "message": message}
	if check != nil {
		e["check"] = check
	}
	writeJSON(w, status, map[string]any{"error": e})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
