package shell

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sessionCookie = "ohmyserver_session"

// Auth guards a host with a single shared secret (a password or token).
//
// Browsers sign in once and receive an HttpOnly session cookie, which also
// covers EventSource and WebSocket requests (they cannot send headers).
// Scripts send "Authorization: Bearer <secret>". Sessions are signed with a
// key derived from the secret, so they survive restarts and changing the
// secret signs everyone out.
type Auth struct {
	Secret string
	// SessionTTL is how long a sign-in lasts (7 days by default).
	SessionTTL time.Duration

	failures sync.Mutex // serialises failed attempts to slow guessing
}

func (a *Auth) ttl() time.Duration {
	if a.SessionTTL > 0 {
		return a.SessionTTL
	}
	return 7 * 24 * time.Hour
}

func (a *Auth) key() []byte {
	sum := sha256.Sum256([]byte("ohmyserver-session\x00" + a.Secret))
	return sum[:]
}

func (a *Auth) sign(expires int64) string {
	payload := strconv.FormatInt(expires, 10)
	mac := hmac.New(sha256.New, a.key())
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *Auth) validSession(value string) bool {
	payload, _, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	expires, err := strconv.ParseInt(payload, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	return hmac.Equal([]byte(value), []byte(a.sign(expires)))
}

func (a *Auth) matches(secret string) bool {
	return a.Secret != "" && subtle.ConstantTimeCompare([]byte(secret), []byte(a.Secret)) == 1
}

// Authenticated reports whether r carries a valid session or bearer secret.
func (a *Auth) Authenticated(r *http.Request) bool {
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && a.matches(token) {
		return true
	}
	if c, err := r.Cookie(sessionCookie); err == nil && a.validSession(c.Value) {
		return true
	}
	return false
}

func (a *Auth) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Authenticated(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="panel"`)
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "unauthorized", "message": "sign in required"}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Auth) register(mux *http.ServeMux) {
	// The session probe always succeeds, so the shell can check before
	// loading anything that would fail with 401.
	mux.HandleFunc("GET /_auth/session", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"authenticated": a.Authenticated(r), "required": true})
	})
	mux.HandleFunc("POST /_auth/login", a.login)
	mux.HandleFunc("POST /_auth/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		w.WriteHeader(http.StatusNoContent)
	})
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_request", "message": "invalid JSON body"}})
		return
	}
	if !a.matches(body.Secret) {
		// One failed attempt at a time, each taking a second.
		a.failures.Lock()
		time.Sleep(time.Second)
		a.failures.Unlock()
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "unauthorized", "message": "wrong password"}})
		return
	}
	expires := time.Now().Add(a.ttl())
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: a.sign(expires.Unix()), Path: "/", Expires: expires,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	w.WriteHeader(http.StatusNoContent)
}
