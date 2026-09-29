package host

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	frontendassets "github.com/lsongdev/ohmyserver/frontend"
	acmeweb "github.com/lsongdev/ohmyserver/modules/acme/web"
	monitorweb "github.com/lsongdev/ohmyserver/modules/monitor/web"
	"github.com/lsongdev/ohmyserver/pkg/moduleweb"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacking not supported")
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
			"ua", r.UserAgent(),
		)
	})
}

func serveHTTP(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{Handler: handler}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			_ = server.Close()
		}
		err := <-serveErr
		if shutdownErr != nil {
			return shutdownErr
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func NewHTTPRouter(s *Server) (http.Handler, error) {
	return NewHTTPRouterAtBase(s, "")
}

var basePathPattern = regexp.MustCompile(`^/(?:[A-Za-z0-9._~-]+(?:/[A-Za-z0-9._~-]+)*)?/?$`)

func normalizeHTTPBasePath(value string) (string, error) {
	if value == "" || value == "/" {
		return "", nil
	}
	if !basePathPattern.MatchString(value) {
		return "", fmt.Errorf("invalid HTTP base path %q", value)
	}
	clean := path.Clean(value)
	if clean != strings.TrimRight(value, "/") || clean == "." || clean == "/" {
		return "", fmt.Errorf("invalid HTTP base path %q", value)
	}
	return clean, nil
}

func NewHTTPRouterAtBase(s *Server, configuredBasePath string) (http.Handler, error) {
	basePath, err := normalizeHTTPBasePath(configuredBasePath)
	if err != nil {
		return nil, err
	}
	router := http.NewServeMux()

	// Auth and protocol endpoints not owned by a feature module stay at the host.
	router.HandleFunc("/logout", s.LogoutHandler)
	router.Handle("GET /.well-known/acme-challenge/{token}", acmeweb.HTTP01Handler(s.ACMEEngine()))

	apiModules, err := NewAPIModules(s)
	if err != nil {
		return nil, err
	}
	router.Handle("/v1/modules", apiModules.IndexHandler(basePath))
	router.Handle("/v1/", http.StripPrefix("/v1", apiModules.Handler()))

	frontendFS, err := fs.Sub(frontendassets.Files(), ".")
	if err != nil {
		return nil, err
	}
	router.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.LoginView(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.Authenticated(r) {
			http.Redirect(w, r, basePath+"/", http.StatusSeeOther)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, frontendFS, "login.html")
	})
	router.Handle("/frontend/runtime/", http.StripPrefix("/frontend/runtime/", moduleweb.BrowserHandler()))
	staticFrontend := http.StripPrefix("/frontend/", http.FileServer(http.FS(frontendFS)))
	router.Handle("/frontend/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		staticFrontend.ServeHTTP(w, r)
	}))
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		if basePath == "" {
			http.ServeFileFS(w, r, frontendFS, "index.html")
			return
		}
		index, err := fs.ReadFile(frontendFS, "index.html")
		if err != nil {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
			return
		}
		baseTag := []byte(`<base href="/">`)
		if bytes.Count(index, baseTag) != 1 {
			http.Error(w, "frontend base path marker missing", http.StatusInternalServerError)
			return
		}
		index = bytes.Replace(index, baseTag, []byte(`<base href="`+basePath+`/">`), 1)
		apiConfig, err := json.Marshal(map[string]string{"apiBase": basePath + "/v1"})
		if err != nil {
			http.Error(w, "frontend configuration unavailable", http.StatusInternalServerError)
			return
		}
		configScript := append([]byte(`<script>window.OhMyServerConfig=Object.assign({},window.OhMyServerConfig,`), apiConfig...)
		configScript = append(configScript, []byte(`);</script>`)...)
		importMapTag := []byte(`<script type="importmap">`)
		if bytes.Count(index, importMapTag) != 1 {
			http.Error(w, "frontend import map marker missing", http.StatusInternalServerError)
			return
		}
		index = bytes.Replace(index, importMapTag, append(append([]byte(nil), configScript...), importMapTag...), 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(index)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(index)
		}
	})

	// Preserve non-UI integration endpoints; management pages are hash-routed
	// and module operations are mounted exclusively under /v1/<module>.
	router.Handle("/monitor/telegraf", monitorweb.TelegrafHandler(s.MonitorRepository()))
	router.HandleFunc("/web/metrics", s.WebServerMetricsHandler)
	if basePath == "" {
		return s.AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			router.ServeHTTP(w, r)
		}), nil
	}
	protectedRouter := s.AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		router.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ACME HTTP-01 validation is defined at the origin root, even when the
		// management application itself is served from a configured base path.
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			protectedRouter.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == basePath {
			http.Redirect(w, r, basePath+"/", http.StatusPermanentRedirect)
			return
		}
		if !strings.HasPrefix(r.URL.Path, basePath+"/") {
			http.NotFound(w, r)
			return
		}
		http.StripPrefix(basePath, protectedRouter).ServeHTTP(w, r)
	}), nil
}
