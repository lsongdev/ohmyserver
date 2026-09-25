// Command proxy packages a module directory as a minimal local Go module
// proxy artifact for release-boundary integration tests.
package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 5 {
		fatalf("usage: proxy <module-dir> <module-path> <version> <proxy-dir>")
	}
	if err := packageModule(os.Args[1], os.Args[2], os.Args[3], os.Args[4]); err != nil {
		fatalf("package module: %v", err)
	}
}

func packageModule(moduleDir, modulePath, version, proxyDir string) error {
	mod, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(mod), "module "+modulePath+"\n") {
		return fmt.Errorf("go.mod does not declare module %q", modulePath)
	}
	if !strings.HasPrefix(version, "v") || strings.ContainsAny(version, "/\\") {
		return fmt.Errorf("invalid module version %q", version)
	}

	versionDir := filepath.Join(proxyDir, filepath.FromSlash(modulePath), "@v")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(versionDir, version+".mod"), mod, 0o644); err != nil {
		return err
	}
	info, err := json.Marshal(struct {
		Version string    `json:"Version"`
		Time    time.Time `json:"Time"`
	}{Version: version, Time: time.Now().UTC()})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(versionDir, version+".info"), info, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(versionDir, "list"), []byte(version+"\n"), 0o644); err != nil {
		return err
	}
	return writeModuleZip(moduleDir, modulePath, version, filepath.Join(versionDir, version+".zip"))
}

func writeModuleZip(moduleDir, modulePath, version, destination string) error {
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	root := modulePath + "@" + version + "/"
	walkErr := filepath.WalkDir(moduleDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != moduleDir && (entry.Name() == ".git" || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("module contains symlink %q", path)
		}
		relative, err := filepath.Rel(moduleDir, path)
		if err != nil {
			return err
		}
		name := root + filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
		header.Method = zip.Deflate
		out, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	zipErr := writer.Close()
	fileErr := file.Close()
	if walkErr != nil {
		return walkErr
	}
	if zipErr != nil {
		return zipErr
	}
	return fileErr
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
