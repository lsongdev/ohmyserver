// Package updater checks GitHub Releases and atomically installs verified
// OhMyServer binaries. It has no dependency on the panel or its database.
package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	DefaultRepository     = "lsongdev/ohmyserver"
	DefaultChecksumsAsset = "checksums.txt"
	DefaultTarget         = "/usr/local/bin/ohmyserver"
	DefaultService        = "ohmyserver.service"
	DefaultTrigger        = "/run/ohmyserver/update"
	defaultAPIBaseURL     = "https://api.github.com"
	maxBinarySize         = 256 << 20
	maxChecksumsSize      = 1 << 20
)

type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

func (r Release) Asset(name string) (Asset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return Asset{}, false
}

type Client struct {
	Repository string
	APIBaseURL string
	HTTPClient *http.Client
}

func NewClient() *Client {
	return &Client{
		Repository: DefaultRepository,
		APIBaseURL: defaultAPIBaseURL,
		HTTPClient: &http.Client{Timeout: 2 * time.Minute},
	}
}

func (c *Client) Latest(ctx context.Context) (Release, error) {
	repository := strings.TrimSpace(c.Repository)
	if repository == "" || strings.Count(repository, "/") != 1 {
		return Release{}, errors.New("updater: repository must be owner/name")
	}
	baseURL := strings.TrimRight(c.APIBaseURL, "/")
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/repos/"+repository+"/releases/latest", nil)
	if err != nil {
		return Release{}, fmt.Errorf("updater: create release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ohmyserver-updater")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("updater: get latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("updater: GitHub returned %s", resp.Status)
	}
	var release Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&release); err != nil {
		return Release{}, fmt.Errorf("updater: decode latest release: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return Release{}, errors.New("updater: latest release has no tag")
	}
	return release, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func AssetName(goos, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("updater: unsupported operating system %q", goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("updater: unsupported architecture %q", goarch)
	}
	name := "ohmyserver-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name, nil
}

type Installer struct {
	Client         *Client
	Target         string
	AssetName      string
	ChecksumsAsset string
}

type Result struct {
	Release    Release
	Asset      Asset
	Target     string
	Backup     string
	SHA256     string
	Downloaded int64
}

func (i *Installer) Apply(ctx context.Context) (Result, error) {
	if i.Client == nil {
		return Result{}, errors.New("updater: client is required")
	}
	target := strings.TrimSpace(i.Target)
	if target == "" || !filepath.IsAbs(target) {
		return Result{}, errors.New("updater: target must be an absolute path")
	}
	assetName := i.AssetName
	if assetName == "" {
		var err error
		assetName, err = AssetName(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return Result{}, err
		}
	}
	checksumsName := i.ChecksumsAsset
	if checksumsName == "" {
		checksumsName = DefaultChecksumsAsset
	}

	release, err := i.Client.Latest(ctx)
	if err != nil {
		return Result{}, err
	}
	binaryAsset, ok := release.Asset(assetName)
	if !ok {
		return Result{}, fmt.Errorf("updater: release %s does not contain %s", release.TagName, assetName)
	}
	checksumsAsset, ok := release.Asset(checksumsName)
	if !ok {
		return Result{}, fmt.Errorf("updater: release %s does not contain %s", release.TagName, checksumsName)
	}
	expectedHash, err := i.fetchExpectedHash(ctx, checksumsAsset, assetName)
	if err != nil {
		return Result{}, err
	}

	info, err := os.Stat(target)
	if err != nil {
		return Result{}, fmt.Errorf("updater: inspect target: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".ohmyserver-update-*")
	if err != nil {
		return Result{}, fmt.Errorf("updater: create staged binary: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	downloaded, actualHash, err := i.downloadBinary(ctx, temp, binaryAsset)
	if err != nil {
		temp.Close()
		return Result{}, err
	}
	if actualHash != expectedHash {
		temp.Close()
		return Result{}, fmt.Errorf("updater: checksum mismatch for %s: expected %s, got %s",
			assetName, expectedHash, actualHash)
	}
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		temp.Close()
		return Result{}, fmt.Errorf("updater: set staged binary mode: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return Result{}, fmt.Errorf("updater: sync staged binary: %w", err)
	}
	if err := temp.Close(); err != nil {
		return Result{}, fmt.Errorf("updater: close staged binary: %w", err)
	}

	backup := target + ".previous"
	if err := os.Rename(target, backup); err != nil {
		return Result{}, fmt.Errorf("updater: back up current binary: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		restoreErr := os.Rename(backup, target)
		if restoreErr != nil {
			return Result{}, fmt.Errorf("updater: install staged binary: %w (restore also failed: %v)", err, restoreErr)
		}
		return Result{}, fmt.Errorf("updater: install staged binary: %w", err)
	}
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		return Result{}, err
	}
	return Result{
		Release: release, Asset: binaryAsset, Target: target, Backup: backup,
		SHA256: actualHash, Downloaded: downloaded,
	}, nil
}

func (i *Installer) fetchExpectedHash(ctx context.Context, asset Asset, binaryName string) (string, error) {
	body, err := i.getAsset(ctx, asset, maxChecksumsSize)
	if err != nil {
		return "", fmt.Errorf("updater: download checksums: %w", err)
	}
	defer body.Close()
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name != binaryName {
			continue
		}
		sum := strings.ToLower(fields[0])
		decoded, err := hex.DecodeString(sum)
		if err != nil || len(decoded) != sha256.Size {
			return "", fmt.Errorf("updater: invalid SHA256 for %s", binaryName)
		}
		return sum, nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("updater: read checksums: %w", err)
	}
	return "", fmt.Errorf("updater: %s has no checksum for %s", asset.Name, binaryName)
}

func (i *Installer) downloadBinary(ctx context.Context, destination *os.File, asset Asset) (int64, string, error) {
	body, err := i.getAsset(ctx, asset, maxBinarySize)
	if err != nil {
		return 0, "", fmt.Errorf("updater: download binary: %w", err)
	}
	defer body.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), body)
	if err != nil {
		return written, "", fmt.Errorf("updater: write staged binary: %w", err)
	}
	if written == 0 {
		return 0, "", errors.New("updater: downloaded binary is empty")
	}
	if written > maxBinarySize {
		return written, "", fmt.Errorf("updater: downloaded binary exceeds the %d byte limit", maxBinarySize)
	}
	return written, hex.EncodeToString(hash.Sum(nil)), nil
}

func (i *Installer) getAsset(ctx context.Context, asset Asset, limit int64) (io.ReadCloser, error) {
	if strings.TrimSpace(asset.BrowserDownloadURL) == "" {
		return nil, fmt.Errorf("asset %s has no download URL", asset.Name)
	}
	if asset.Size > limit {
		return nil, fmt.Errorf("asset %s exceeds the %d byte limit", asset.Name, limit)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ohmyserver-updater")
	resp, err := i.Client.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s returned %s", asset.Name, resp.Status)
	}
	return struct {
		io.Reader
		io.Closer
	}{Reader: io.LimitReader(resp.Body, limit+1), Closer: resp.Body}, nil
}

func syncDirectory(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("updater: open target directory: %w", err)
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("updater: sync target directory: %w", err)
	}
	return nil
}
