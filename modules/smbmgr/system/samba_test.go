package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fake answers commands by name; testparm fails when the file it checks
// contains "maybe", as the real one does for a bad boolean.
type fake struct {
	calls []string
	stdin []string
	fail  map[string]error
}

func (f *fake) Run(ctx context.Context, stdin, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.stdin = append(f.stdin, stdin)
	if err := f.fail[name]; err != nil {
		return nil, nil, err
	}
	switch name {
	case "testparm":
		data, _ := os.ReadFile(args[len(args)-1])
		stderr := "Load smb config files from x\n"
		if strings.Contains(string(data), "bogus") {
			stderr += "Unknown parameter encountered: \"bogus\"\nIgnoring unknown parameter \"bogus\"\n"
		}
		if strings.Contains(string(data), "maybe") {
			return nil, []byte(stderr + "set_variable_helper(maybe): value is not boolean!\nError loading services.\n"), &ExitError{Command: "testparm", Code: 1}
		}
		return nil, []byte(stderr + "Loaded services file OK.\nWARNING: The 'netbios name' is too long\n\nServer role: ROLE_STANDALONE\n"), nil
	case "pdbedit":
		return []byte("Unix username:        lsong\nNT username:\nAccount Flags:        [U          ]\nFull Name:            Lsong\n---------------\n" +
			"Unix username:        guest\nAccount Flags:        [DX         ]\n"), nil, nil
	}
	return nil, nil, nil
}

func newSamba(t *testing.T, text string) (*Samba, *fake) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "smb.conf")
	if err := os.WriteFile(path, []byte(text), 0o640); err != nil {
		t.Fatal(err)
	}
	f := &fake{fail: map[string]error{}}
	return &Samba{Run: f, Config: path}, f
}

func TestCheck(t *testing.T) {
	s, _ := newSamba(t, "")
	c, err := s.Check(context.Background(), "[global]\n")
	if err != nil || !c.OK || len(c.Errors) != 0 || len(c.Warnings) != 1 {
		t.Errorf("good: %+v %v", c, err)
	}
	c, _ = s.Check(context.Background(), "[a]\n bogus = 1\n")
	if c.OK || len(c.Errors) != 1 || !strings.Contains(c.Errors[0], "bogus") {
		t.Errorf("unknown parameter: %+v", c)
	}
	c, _ = s.Check(context.Background(), "[a]\n read only = maybe\n")
	if c.OK || len(c.Errors) != 2 {
		t.Errorf("bad boolean: %+v", c)
	}
}

func TestWrite(t *testing.T) {
	ctx := context.Background()
	s, f := newSamba(t, nas)
	file, _ := s.Read()

	// testparm refuses: the file stays.
	_, err := s.Write(ctx, "[a]\n read only = maybe\n", file.Version)
	var check *CheckError
	if !errors.As(err, &check) {
		t.Fatalf("bad config: %v", err)
	}
	if after, _ := s.Read(); after.Text != nas || after.Backup != "" {
		t.Fatal("a rejected config changed the file")
	}

	saved, err := s.Write(ctx, "[global]\n workgroup = HOME", file.Version)
	if err != nil || !saved.Reloaded {
		t.Fatalf("write: %+v %v", saved, err)
	}
	after, _ := s.Read()
	if after.Text != "[global]\n workgroup = HOME\n" || after.Version != saved.Version {
		t.Errorf("written %q", after.Text)
	}
	if backup, _ := os.ReadFile(after.Backup); string(backup) != nas {
		t.Error("no backup of the previous file")
	}
	if info, _ := os.Stat(s.Config); info.Mode().Perm() != 0o640 {
		t.Errorf("mode %v", info.Mode())
	}
	if last := f.calls[len(f.calls)-1]; last != "smbcontrol smbd reload-config" {
		t.Errorf("last command %q", last)
	}

	// From the old version: someone else saved meanwhile.
	if _, err := s.Write(ctx, "[global]\n", file.Version); !errors.Is(err, ErrConflict) {
		t.Errorf("stale version: %v", err)
	}

	// smbd not running: saved anyway, and said so.
	f.fail["smbcontrol"] = &ExitError{Command: "smbcontrol", Code: 1, Output: "Can't find pid for destination 'smbd'"}
	saved, err = s.Write(ctx, "[global]\n workgroup = X\n", after.Version)
	if err != nil || saved.Reloaded || !strings.Contains(saved.ReloadError, "pid") {
		t.Errorf("without smbd: %+v %v", saved, err)
	}
}

func TestEdit(t *testing.T) {
	s, _ := newSamba(t, nas)
	file, _ := s.Read()
	_, err := s.Edit(context.Background(), file.Version, func(c *Conf) error {
		_, err := c.AddShare("media", []Param{{Key: "path", Value: "/data/media"}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := s.Read()
	if !strings.HasPrefix(after.Text, nas) || !strings.HasSuffix(after.Text, "[media]\n   path = /data/media\n") {
		t.Errorf("edited:\n%s", after.Text)
	}
}

func TestUsers(t *testing.T) {
	ctx := context.Background()
	s, f := newSamba(t, "")
	users, err := s.Users(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("%+v %v", users, err)
	}
	if u := users[0]; u.Name != "lsong" || u.FullName != "Lsong" || u.Disabled || u.Flags != "U" {
		t.Errorf("lsong: %+v", u)
	}
	if u := users[1]; !u.Disabled || !u.NoExpiry {
		t.Errorf("guest: %+v", u)
	}
	if err := s.AddUser(ctx, "lsong", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if call, in := f.calls[len(f.calls)-1], f.stdin[len(f.stdin)-1]; call != "smbpasswd -s -a lsong" || in != "s3cret\ns3cret\n" {
		t.Errorf("add: %q %q", call, in)
	}
	for _, bad := range [][2]string{{"-x", "pw"}, {"a b", "pw"}, {"lsong", ""}, {"lsong", "a\nb"}} {
		if err := s.AddUser(ctx, bad[0], bad[1]); !errors.Is(err, ErrInvalid) {
			t.Errorf("AddUser(%q, %q): %v", bad[0], bad[1], err)
		}
	}
}

func TestSystemUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passwd")
	os.WriteFile(path, []byte("root:x:0:0::/root:/bin/bash\nlsong:x:1000:1000::/home/lsong:/bin/bash\nnobody:x:65534:65534::/:/usr/bin/nologin\n"), 0o644)
	users, err := SystemUsers(path)
	if err != nil || len(users) != 1 || users[0].Name != "lsong" {
		t.Errorf("%+v %v", users, err)
	}
}

func TestParseStatus(t *testing.T) {
	st, err := parseStatus([]byte(`{"sessions": {"1": {"session_id": "1", "username": "lsong", "groupname": "lsong", "remote_machine": "192.168.8.111",
		"session_dialect": "SMB3_11", "creation_time": "2026-10-29T11:34:53+08:00", "encryption": {"cipher": "-"}, "signing": {"cipher": "AES-128-GMAC"}}},
		"tcons": {"5": {"service": "data", "session_id": "1", "machine": "192.168.8.111", "connected_at": "2026-10-29T11:34:56+08:00"}},
		"open_files": {"/data/a.pdf": {"service_path": "/data", "filename": "a.pdf", "opens": {"x": {"access_mask": {"text": "R"}, "opened_at": "2026-10-29T11:38:02+08:00"}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Sessions) != 1 || st.Sessions[0].Signing != "AES-128-GMAC" || len(st.Connections) != 1 || st.Connections[0].Share != "data" ||
		len(st.OpenFiles) != 1 || st.OpenFiles[0].Access != "R" || st.OpenFiles[0].Opens != 1 {
		t.Errorf("%+v", st)
	}
}
