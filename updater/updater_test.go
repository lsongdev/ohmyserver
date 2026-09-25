package updater

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "ohmyserver-linux-amd64"},
		{"darwin", "arm64", "ohmyserver-darwin-arm64"},
		{"windows", "amd64", "ohmyserver-windows-amd64.exe"},
	}
	for _, test := range tests {
		got, err := AssetName(test.goos, test.goarch)
		if err != nil || got != test.want {
			t.Fatalf("AssetName(%q, %q) = %q, %v; want %q", test.goos, test.goarch, got, err, test.want)
		}
	}
}

func TestInstallerApply(t *testing.T) {
	const (
		assetName = "ohmyserver-linux-amd64"
		oldBinary = "old binary"
		newBinary = "new verified binary"
	)
	sum := sha256.Sum256([]byte(newBinary))
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/lsongdev/ohmyserver/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v2","assets":[`+
			`{"name":"%s","size":%d,"browser_download_url":"%s/binary"},`+
			`{"name":"checksums.txt","size":100,"browser_download_url":"%s/checksums"}]}`,
			assetName, len(newBinary), baseURL, baseURL)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(newBinary))
	})
	mux.HandleFunc("/checksums", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%x  %s\n", sum, assetName)
	})
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	baseURL = httpServer.URL

	dir := t.TempDir()
	target := filepath.Join(dir, "ohmyserver")
	if err := os.WriteFile(target, []byte(oldBinary), 0751); err != nil {
		t.Fatal(err)
	}
	client := NewClient()
	client.APIBaseURL = baseURL
	client.HTTPClient = httpServer.Client()
	result, err := (&Installer{
		Client: client, Target: target, AssetName: assetName,
	}).Apply(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != newBinary {
		t.Fatalf("target = %q, want %q", got, newBinary)
	}
	backup, err := os.ReadFile(result.Backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != oldBinary {
		t.Fatalf("backup = %q, want %q", backup, oldBinary)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0751); got != want {
		t.Fatalf("mode = %o, want %o", got, want)
	}
}

func TestInstallerRejectsChecksumMismatch(t *testing.T) {
	const assetName = "ohmyserver-linux-amd64"
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/lsongdev/ohmyserver/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v2","assets":[`+
			`{"name":"%s","size":3,"browser_download_url":"%s/binary"},`+
			`{"name":"checksums.txt","size":100,"browser_download_url":"%s/checksums"}]}`,
			assetName, baseURL, baseURL)
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("new")) })
	mux.HandleFunc("/checksums", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%064d  %s\n", 0, assetName)
	})
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	baseURL = httpServer.URL

	target := filepath.Join(t.TempDir(), "ohmyserver")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	client := NewClient()
	client.APIBaseURL = baseURL
	client.HTTPClient = httpServer.Client()
	_, err := (&Installer{Client: client, Target: target, AssetName: assetName}).Apply(context.Background())
	if err == nil {
		t.Fatal("Apply succeeded with a mismatched checksum")
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "old" {
		t.Fatalf("target changed after failed verification: %q", got)
	}
}
