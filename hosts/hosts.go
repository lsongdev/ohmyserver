// Package hosts describes saved SSH hosts and connects to them: password,
// private key or the panel's own key authentication, and host keys pinned
// on first use.
package hosts

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Authentication methods.
const (
	AuthPassword = "password"
	AuthKey      = "key"   // a private key stored with the host
	AuthPanel    = "panel" // the panel's own key (see Keys)
)

// Host is a saved SSH host. Secrets never leave the server: JSON carries
// only whether they are set.
type Host struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Address    string `json:"address"` // host name or IP
	Port       int    `json:"port"`
	User       string `json:"user"`
	Auth       string `json:"auth"`
	Password   string `json:"-"`
	PrivateKey string `json:"-"` // PEM
	Passphrase string `json:"-"` // of PrivateKey, if encrypted
	// HostKey is the server's key pinned on first connection, in
	// authorized_keys format; empty until then.
	HostKey         string    `json:"-"`
	Description     string    `json:"description"`
	LastConnectedAt time.Time `json:"last_connected_at,omitzero"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// View is how a host is shown: no secrets, the pinned key's fingerprint.
type View struct {
	Host
	HasPassword   bool   `json:"has_password"`
	HasKey        bool   `json:"has_key"`
	HostKeyType   string `json:"host_key_type,omitempty"`
	HostKeySHA256 string `json:"host_key_sha256,omitempty"`
}

// View returns h for display.
func (h Host) View() View {
	v := View{Host: h, HasPassword: h.Password != "", HasKey: h.PrivateKey != ""}
	if key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(h.HostKey)); err == nil {
		v.HostKeyType, v.HostKeySHA256 = key.Type(), ssh.FingerprintSHA256(key)
	}
	return v
}

// ErrInvalid marks a host that fails validation.
var ErrInvalid = errors.New("invalid host")

// Normalize trims and validates h, filling defaults.
func (h *Host) Normalize() error {
	h.Name = strings.TrimSpace(h.Name)
	h.Address = strings.TrimSpace(h.Address)
	h.User = strings.TrimSpace(h.User)
	h.Description = strings.TrimSpace(h.Description)
	if h.Port == 0 {
		h.Port = 22
	}
	if h.Auth == "" {
		h.Auth = AuthPassword
	}
	switch {
	case h.Name == "":
		return fmt.Errorf("%w: name required", ErrInvalid)
	case h.Address == "" || strings.ContainsAny(h.Address, " /"):
		return fmt.Errorf("%w: address required", ErrInvalid)
	case h.Port < 1 || h.Port > 65535:
		return fmt.Errorf("%w: port must be 1-65535", ErrInvalid)
	case h.User == "":
		return fmt.Errorf("%w: user required", ErrInvalid)
	}
	switch h.Auth {
	case AuthPassword:
		if h.Password == "" {
			return fmt.Errorf("%w: password required", ErrInvalid)
		}
	case AuthKey:
		if _, err := h.signer(); err != nil {
			return fmt.Errorf("%w: private key: %v", ErrInvalid, err)
		}
	case AuthPanel:
	default:
		return fmt.Errorf("%w: auth must be password, key or panel", ErrInvalid)
	}
	return nil
}

func (h *Host) signer() (ssh.Signer, error) {
	if strings.TrimSpace(h.PrivateKey) == "" {
		return nil, errors.New("required")
	}
	if h.Passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase([]byte(h.PrivateKey), []byte(h.Passphrase))
	}
	s, err := ssh.ParsePrivateKey([]byte(h.PrivateKey))
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		return nil, errors.New("the key is encrypted; enter its passphrase")
	}
	return s, err
}

// HostKeyError reports a server whose key differs from the pinned one:
// either the server was reinstalled or someone is in between. The operator
// must forget the pinned key to connect again.
type HostKeyError struct {
	Pinned, Offered string // SHA256 fingerprints
}

func (e *HostKeyError) Error() string {
	return fmt.Sprintf("host key changed: pinned %s, server offered %s", e.Pinned, e.Offered)
}

// Conn is an authenticated connection.
type Conn struct {
	*ssh.Client
	// HostKey is the server's key in authorized_keys format; the caller
	// pins it when the host had none.
	HostKey string
}

// Dial connects and authenticates to h. panelKey is the panel's signer,
// used when h.Auth is AuthPanel.
func Dial(ctx context.Context, h Host, panelKey ssh.Signer) (*Conn, error) {
	var auth ssh.AuthMethod
	switch h.Auth {
	case AuthPassword:
		auth = ssh.Password(h.Password)
	case AuthKey:
		s, err := h.signer()
		if err != nil {
			return nil, fmt.Errorf("private key: %w", err)
		}
		auth = ssh.PublicKeys(s)
	case AuthPanel:
		if panelKey == nil {
			return nil, errors.New("the panel has no key")
		}
		auth = ssh.PublicKeys(panelKey)
	default:
		return nil, fmt.Errorf("unknown auth %q", h.Auth)
	}
	methods := []ssh.AuthMethod{auth}
	if h.Auth == AuthPassword {
		// Many servers only offer keyboard-interactive for passwords.
		methods = append(methods, ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = h.Password
			}
			return answers, nil
		}))
	}
	var offered string
	config := &ssh.ClientConfig{
		User: h.User,
		Auth: methods,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			offered = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
			if h.HostKey == "" {
				return nil // first use: the caller pins it
			}
			pinned, _, _, _, err := ssh.ParseAuthorizedKey([]byte(h.HostKey))
			if err != nil {
				return fmt.Errorf("pinned host key unreadable: %w", err)
			}
			if string(pinned.Marshal()) != string(key.Marshal()) {
				return &HostKeyError{Pinned: ssh.FingerprintSHA256(pinned), Offered: ssh.FingerprintSHA256(key)}
			}
			return nil
		},
		Timeout: 15 * time.Second,
	}
	addr := net.JoinHostPort(h.Address, strconv.Itoa(h.Port))
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	// The handshake has no context of its own: closing the socket ends it.
	stop := context.AfterFunc(ctx, func() { raw.Close() })
	c, chans, reqs, err := ssh.NewClientConn(raw, addr, config)
	if !stop() && err == nil {
		err = ctx.Err()
	}
	if err != nil {
		raw.Close()
		var hk *HostKeyError
		if errors.As(err, &hk) {
			return nil, hk
		}
		return nil, err
	}
	return &Conn{Client: ssh.NewClient(c, chans, reqs), HostKey: offered}, nil
}

// KeepAlive sends keepalive requests until the connection closes, and
// closes it when the server stops answering, so a dead peer does not leave
// a terminal hanging.
func (c *Conn) KeepAlive(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		done := make(chan error, 1)
		go func() {
			_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				return
			}
		case <-time.After(interval):
			c.Close()
			return
		}
	}
}

// NewKey generates a panel key: an ed25519 private key in OpenSSH PEM form.
func NewKey(comment string) (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(block)), nil
}

// PublicKey returns the authorized_keys line of a PEM private key.
func PublicKey(privatePEM, comment string) (string, ssh.Signer, error) {
	s, err := ssh.ParsePrivateKey([]byte(privatePEM))
	if err != nil {
		return "", nil, err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.PublicKey())))
	if comment != "" {
		line += " " + comment
	}
	return line, s, nil
}
