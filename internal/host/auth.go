package host

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

const authCookieName = "token"
const authCookieMaxAge = 7 * 24 * 3600 // 7 days

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// AuthMiddleware wraps a handler with access-key authentication.
// If accessKey is empty, auth is skipped.
func (s *Server) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config == nil || s.config.AccessKey == "" {
			next(w, r)
			return
		}

		path := r.URL.Path
		if path == "/login" || path == "/logout" || strings.HasPrefix(path, "/.well-known/acme-challenge/") {
			next(w, r)
			return
		}

		if !s.Authenticated(r) {
			http.Redirect(w, r, s.hostPath("/login"), http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}

func (s *Server) Authenticated(r *http.Request) bool {
	if s.config == nil || s.config.AccessKey == "" {
		return true
	}
	cookie, err := r.Cookie(authCookieName)
	return err == nil && cookie.Value == hashKey(s.config.AccessKey)
}

func (s *Server) hostPath(path string) string {
	if s.config == nil {
		return path
	}
	return strings.TrimRight(s.config.BasePath, "/") + path
}

// LoginView handles access-key form submissions.
func (s *Server) LoginView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providedKey := r.FormValue("access_key")
	if providedKey == "" || providedKey != s.config.AccessKey {
		http.Redirect(w, r, s.hostPath("/login?error=invalid"), http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    hashKey(s.config.AccessKey),
		Path:     "/",
		MaxAge:   authCookieMaxAge,
		HttpOnly: true,
	})

	http.Redirect(w, r, s.hostPath("/"), http.StatusSeeOther)
}

// LogoutHandler clears the auth cookie
func (s *Server) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   authCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, s.hostPath("/login"), http.StatusSeeOther)
}
