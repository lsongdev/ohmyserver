// Package system reads and changes a Samba server: its smb.conf, checked
// with testparm before it replaces the file, its users (smbpasswd and
// pdbedit) and who is connected (smbstatus).
package system

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var (
	// ErrUnsupported means Samba is not installed.
	ErrUnsupported = errors.New("samba is not installed")
	// ErrConflict means the file changed since the version an edit started from.
	ErrConflict = errors.New("smb.conf changed since it was loaded; reload to see the changes")
	// ErrNotFound is a missing share or user.
	ErrNotFound = errors.New("not found")
)

// CheckError is a configuration testparm rejects.
type CheckError struct{ Check Check }

func (e *CheckError) Error() string {
	return "testparm rejects the configuration: " + strings.Join(e.Check.Errors, "; ")
}

// Samba manages one server's configuration and users.
type Samba struct {
	Run    Runner
	Config string // smb.conf
	mu     sync.Mutex
}

// DefaultConfig asks smbd where its configuration is, falling back to the
// usual place.
func DefaultConfig(ctx context.Context, run Runner) string {
	if out, _, err := run.Run(ctx, "", "smbd", "-b"); err == nil {
		for line := range strings.Lines(string(out)) {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && k == "CONFIGFILE" {
				return strings.TrimSpace(v)
			}
		}
	}
	return "/etc/samba/smb.conf"
}

// Installed reports whether smbd and testparm are available.
func Installed() bool {
	for _, name := range []string{"smbd", "testparm"} {
		if _, err := exec.LookPath(name); err != nil {
			return false
		}
	}
	return true
}

// Version returns smbd's version, such as "4.25.1".
func (s *Samba) Version(ctx context.Context) string {
	out, _, err := s.Run.Run(ctx, "", "smbd", "-V")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "Version ")
}

// File is smb.conf's text and its version, a hash that edits send back so
// they cannot overwrite changes made meanwhile.
type File struct {
	Path    string `json:"path"`
	Text    string `json:"text"`
	Version string `json:"version"`
	Backup  string `json:"backup,omitempty"` // the previous version, if saved here before
}

func version(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

func (s *Samba) backupPath() string { return s.Config + ".bak" }

// Read reads smb.conf; a missing file reads as empty.
func (s *Samba) Read() (File, error) {
	data, err := os.ReadFile(s.Config)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return File{}, err
	}
	f := File{Path: s.Config, Text: string(data), Version: version(string(data))}
	if _, err := os.Stat(s.backupPath()); err == nil {
		f.Backup = s.backupPath()
	}
	return f, nil
}

// Check is what testparm says about a configuration.
type Check struct {
	OK       bool     `json:"ok"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// testparm's chatter, which says nothing about the configuration.
var noise = regexp.MustCompile(`^(Load smb config files from|Loaded services file OK|Weak crypto is allowed|Server role:|Press enter to see|Ignoring unknown parameter)`)

// Check runs testparm on text. Unknown parameters count as errors: Samba
// ignores them, so they are almost always a typo.
func (s *Samba) Check(ctx context.Context, text string) (Check, error) {
	tmp, err := os.CreateTemp("", "smbmgr-*.conf")
	if err != nil {
		return Check{}, err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(text)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Check{}, err
	}
	_, stderr, err := s.Run.Run(ctx, "", "testparm", "-s", tmp.Name())
	var exit *ExitError
	if err != nil && !errors.As(err, &exit) {
		return Check{}, err
	}
	// Unknown parameters are always errors; the rest are errors when
	// testparm fails, and advice when it does not.
	c := Check{Errors: []string{}, Warnings: []string{}}
	for line := range strings.Lines(string(stderr)) {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || noise.MatchString(line):
		case strings.HasPrefix(line, "Unknown parameter encountered"), exit != nil:
			c.Errors = append(c.Errors, line)
		default:
			c.Warnings = append(c.Warnings, line)
		}
	}
	if exit != nil && len(c.Errors) == 0 {
		c.Errors = append(c.Errors, exit.Error())
	}
	c.OK = len(c.Errors) == 0
	return c, nil
}

// Saved is the outcome of replacing smb.conf.
type Saved struct {
	Version     string   `json:"version"`
	Warnings    []string `json:"warnings"`
	Reloaded    bool     `json:"reloaded"`
	ReloadError string   `json:"reload_error,omitempty"`
}

// Write replaces smb.conf with text if testparm accepts it and the file is
// still at version base (any version when base is empty), keeps the
// previous file as smb.conf.bak, and asks smbd to reload.
func (s *Samba) Write(ctx context.Context, text, base string) (Saved, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(ctx, text, base)
}

// Edit changes the parsed configuration with fn and writes it like Write.
func (s *Samba) Edit(ctx context.Context, base string, fn func(*Conf) error) (Saved, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.Read()
	if err != nil {
		return Saved{}, err
	}
	if base != "" && base != f.Version {
		return Saved{}, ErrConflict
	}
	conf := Parse(f.Text)
	if err := fn(conf); err != nil {
		return Saved{}, err
	}
	return s.write(ctx, conf.String(), f.Version)
}

func (s *Samba) write(ctx context.Context, text, base string) (Saved, error) {
	if !strings.HasSuffix(text, "\n") && text != "" {
		text += "\n"
	}
	current, err := s.Read()
	if err != nil {
		return Saved{}, err
	}
	if base != "" && base != current.Version {
		return Saved{}, ErrConflict
	}
	check, err := s.Check(ctx, text)
	if err != nil {
		return Saved{}, err
	}
	if !check.OK {
		return Saved{}, &CheckError{Check: check}
	}
	saved := Saved{Version: version(text), Warnings: check.Warnings}
	if text == current.Text {
		return saved, nil
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(s.Config); err == nil {
		mode = info.Mode().Perm()
		if err := writeFile(s.backupPath(), []byte(current.Text), mode); err != nil {
			return Saved{}, fmt.Errorf("keeping the previous smb.conf: %w", err)
		}
	}
	if err := writeFile(s.Config, []byte(text), mode); err != nil {
		return Saved{}, err
	}
	if err := s.Reload(ctx); err != nil {
		saved.ReloadError = err.Error()
	} else {
		saved.Reloaded = true
	}
	return saved, nil
}

// writeFile replaces a file atomically: a reader sees the old or the new
// contents, never a half-written file.
func writeFile(name string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // after a successful rename, there is nothing to remove
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}

// Reload asks the running smbd processes to read smb.conf again. Existing
// connections keep some settings until they reconnect.
func (s *Samba) Reload(ctx context.Context) error {
	_, _, err := s.Run.Run(ctx, "", "smbcontrol", "smbd", "reload-config")
	return err
}

// Service is the systemd unit running smbd: smb.service on Arch and
// Fedora, smbd.service on Debian and Ubuntu.
type Service struct {
	Unit   string `json:"unit"`
	Active string `json:"active"` // active, inactive, failed, …
	Sub    string `json:"sub"`
}

// Service finds smbd's unit; nil without systemd or a unit.
func (s *Samba) Service(ctx context.Context) *Service {
	out, _, err := s.Run.Run(ctx, "", "systemctl", "show", "-p", "Id,LoadState,ActiveState,SubState", "smb.service", "smbd.service")
	if err != nil {
		return nil
	}
	// One block per unit, separated by a blank line.
	for block := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n\n") {
		props := map[string]string{}
		for line := range strings.Lines(block) {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
				props[k] = v
			}
		}
		if props["LoadState"] == "loaded" {
			return &Service{Unit: props["Id"], Active: props["ActiveState"], Sub: props["SubState"]}
		}
	}
	return nil
}

// User is a Samba account. Passwords and hashes are never read.
type User struct {
	Name     string `json:"name"`
	FullName string `json:"full_name,omitempty"`
	Flags    string `json:"flags"` // pdbedit's account flags, e.g. "U" or "UD"
	Disabled bool   `json:"disabled"`
	Locked   bool   `json:"locked"`
	NoExpiry bool   `json:"no_expiry"`
}

// Users lists the accounts in Samba's password database.
func (s *Samba) Users(ctx context.Context) ([]User, error) {
	out, _, err := s.Run.Run(ctx, "", "pdbedit", "-L", "-v")
	if err != nil {
		return nil, err
	}
	return parseUsers(string(out)), nil
}

// parseUsers reads pdbedit -L -v: "Key: value" lines, accounts separated
// by a line of dashes.
func parseUsers(out string) []User {
	users := []User{}
	var u *User
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "---") {
			u = nil
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if k == "Unix username" {
			users = append(users, User{Name: v})
			u = &users[len(users)-1]
			continue
		}
		if u == nil {
			continue
		}
		switch k {
		case "Full Name":
			u.FullName = v
		case "Account Flags":
			u.Flags = strings.Trim(strings.ReplaceAll(v, " ", ""), "[]")
			u.Disabled = strings.Contains(u.Flags, "D")
			u.Locked = strings.Contains(u.Flags, "L")
			u.NoExpiry = strings.Contains(u.Flags, "X")
		}
	}
	return users
}

var userName = regexp.MustCompile(`^[a-z_][a-z0-9_.-]{0,31}\$?$`)

func checkUser(name string) error {
	if !userName.MatchString(name) {
		return fmt.Errorf("%w user name %q", ErrInvalid, name)
	}
	return nil
}

func checkPassword(password string) error {
	if password == "" || strings.ContainsAny(password, "\r\n\x00") {
		return fmt.Errorf("%w password: it must not be empty or span lines", ErrInvalid)
	}
	return nil
}

// AddUser gives an existing system user a Samba password.
func (s *Samba) AddUser(ctx context.Context, name, password string) error {
	if err := checkUser(name); err != nil {
		return err
	}
	if err := checkPassword(password); err != nil {
		return err
	}
	_, _, err := s.Run.Run(ctx, password+"\n"+password+"\n", "smbpasswd", "-s", "-a", name)
	return err
}

// SetPassword changes a Samba user's password.
func (s *Samba) SetPassword(ctx context.Context, name, password string) error {
	if err := checkUser(name); err != nil {
		return err
	}
	if err := checkPassword(password); err != nil {
		return err
	}
	_, _, err := s.Run.Run(ctx, password+"\n"+password+"\n", "smbpasswd", "-s", name)
	return err
}

// SetEnabled enables or disables a Samba user.
func (s *Samba) SetEnabled(ctx context.Context, name string, enabled bool) error {
	if err := checkUser(name); err != nil {
		return err
	}
	flag := "-d"
	if enabled {
		flag = "-e"
	}
	_, _, err := s.Run.Run(ctx, "", "smbpasswd", flag, name)
	return err
}

// DeleteUser removes a user from Samba's database; the system account stays.
func (s *Samba) DeleteUser(ctx context.Context, name string) error {
	if err := checkUser(name); err != nil {
		return err
	}
	_, _, err := s.Run.Run(ctx, "", "smbpasswd", "-x", name)
	return err
}

// SystemUser is an account in /etc/passwd that could get a Samba password.
type SystemUser struct {
	Name string `json:"name"`
	UID  int    `json:"uid"`
	Home string `json:"home,omitempty"`
}

// SystemUsers lists the people's accounts in passwd: uid 1000 and up,
// without nobody.
func SystemUsers(passwd string) ([]SystemUser, error) {
	f, err := os.Open(passwd)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	users := []SystemUser{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 7 {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 || uid >= 65534 {
			continue
		}
		users = append(users, SystemUser{Name: fields[0], UID: uid, Home: fields[5]})
	}
	return users, scanner.Err()
}
