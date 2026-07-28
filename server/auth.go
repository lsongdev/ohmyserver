package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
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
		if s.config.AccessKey == "" {
			next(w, r)
			return
		}

		path := r.URL.Path
		if path == "/login" || path == "/logout" {
			next(w, r)
			return
		}

		expected := hashKey(s.config.AccessKey)
		cookie, err := r.Cookie(authCookieName)
		if err != nil || cookie.Value != expected {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}

// LoginView handles GET/POST for the login page
func (s *Server) LoginView(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		expected := hashKey(s.config.AccessKey)
		cookie, _ := r.Cookie(authCookieName)
		if cookie != nil && cookie.Value == expected {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		s.Render(w, "login", H{})
		return
	}

	providedKey := r.FormValue("access_key")
	if providedKey == "" || providedKey != s.config.AccessKey {
		s.Render(w, "login", H{"errorMessage": "Invalid access key"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    hashKey(s.config.AccessKey),
		Path:     "/",
		MaxAge:   authCookieMaxAge,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// LogoutHandler clears the auth cookie
func (s *Server) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   authCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
