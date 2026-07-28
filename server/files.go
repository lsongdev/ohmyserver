package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileInfo represents file information
type FileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

// FileView handles file manager UI
func (s *Server) FileView(w http.ResponseWriter, r *http.Request) {
	// Handle method override for DELETE
	if r.Method == http.MethodPost {
		methodOverride := r.FormValue("_method")
		if methodOverride == "DELETE" {
			s.FileDeleteHandler(w, r)
			return
		}
		s.FilePost(w, r)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		// Default to user's home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "/"
		}
		http.Redirect(w, r, "/files?path="+homeDir, http.StatusSeeOther)
		return
	}

	showHidden := r.URL.Query().Get("show_hidden") == "true"

	// Check if path is a file or directory
	info, err := os.Stat(path)
	if err != nil {
		s.Render(w, "files/index", H{
			"currentPath":  "/",
			"errorMessage": err.Error(),
		})
		return
	}

	// If it's a file, show download option
	if !info.IsDir() {
		parent := ""
		if path != "/" {
			parent = filepath.Dir(path)
			if parent == "." {
				parent = "/"
			}
		}
		s.Render(w, "files/index", H{
			"currentPath": path,
			"parent":      parent,
			"fileInfo":    info,
			"view":        "file",
		})
		return
	}

	// List directory
	entries, err := os.ReadDir(path)
	if err != nil {
		s.Render(w, "files/index", H{
			"currentPath":  "/",
			"errorMessage": err.Error(),
		})
		return
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		// Skip hidden files if not showing them
		if !showHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		fileInfo, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    fileInfo.Name(),
			Path:    filepath.Join(path, fileInfo.Name()),
			Size:    fileInfo.Size(),
			Mode:    fileInfo.Mode().String(),
			ModTime: fileInfo.ModTime(),
			IsDir:   fileInfo.IsDir(),
		})
	}

	// Sort: directories first, then files, alphabetically
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	// Get parent path
	parent := ""
	if path != "/" {
		parent = filepath.Dir(path)
		if parent == "." {
			parent = "/"
		}
	}

	s.Render(w, "files/index", H{
		"currentPath": path,
		"parent":      parent,
		"files":       files,
		"filesLength": len(files),
		"view":        "list",
		"showHidden":  showHidden,
	})
}

// FileDeleteHandler handles DELETE /files
func (s *Server) FileDeleteHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	returnPath := r.URL.Query().Get("return")
	if returnPath == "" {
		returnPath = filepath.Dir(path)
	}

	if err := os.RemoveAll(path); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true}`))
}

// FileNew handles new file/folder/upload form
func (s *Server) FileNew(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		homeDir, _ := os.UserHomeDir()
		path = homeDir
	}

	showHidden := r.URL.Query().Get("show_hidden") == "true"
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "file"
	}

	s.Render(w, "files/new", H{
		"currentPath": path,
		"mode":        mode,
		"showHidden":  showHidden,
	})
}

// FileDownload handles file download
func (s *Server) FileDownload(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	returnPath := r.URL.Query().Get("return")
	if returnPath == "" {
		returnPath = filepath.Dir(path)
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		s.Render(w, "files/index", H{
			"currentPath":  returnPath,
			"errorMessage": "file not found",
		})
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\""+info.Name()+"\"")
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path)
}

// FilePost handles file/folder creation and upload POST requests
func (s *Server) FilePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	action := r.FormValue("action")
	path := r.FormValue("path")
	showHidden := r.FormValue("show_hidden")

	if path == "" {
		homeDir, _ := os.UserHomeDir()
		path = homeDir
	}

	redirectPath := "/files?path=" + path
	if showHidden == "true" {
		redirectPath += "&show_hidden=true"
	}

	switch action {
	case "create_file":
		filename := r.FormValue("filename")
		content := r.FormValue("content")
		if filename == "" {
			http.Redirect(w, r, "/files/new?path="+path+"&mode=file", http.StatusSeeOther)
			return
		}

		filePath := filepath.Join(path, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			http.Redirect(w, r, "/files/new?path="+path+"&mode=file", http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		return

	case "create_folder":
		foldername := r.FormValue("foldername")
		if foldername == "" {
			http.Redirect(w, r, "/files/new?path="+path+"&mode=folder", http.StatusSeeOther)
			return
		}

		folderPath := filepath.Join(path, foldername)
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			http.Redirect(w, r, "/files/new?path="+path+"&mode=folder", http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		return

	case "upload":
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Redirect(w, r, "/files/new?path="+path+"&mode=upload", http.StatusSeeOther)
			return
		}
		defer file.Close()

		uploadPath := filepath.Join(path, header.Filename)
		outFile, err := os.Create(uploadPath)
		if err != nil {
			http.Redirect(w, r, "/files/new?path="+path+"&mode=upload", http.StatusSeeOther)
			return
		}
		defer outFile.Close()

		if _, err := io.Copy(outFile, file); err != nil {
			http.Redirect(w, r, "/files/new?path="+path+"&mode=upload", http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}

// getFileIcon returns file icon based on extension
func getFileIcon(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp", ".ico":
		return "🖼️"
	case ".js", ".ts", ".go", ".py", ".java", ".c", ".cpp", ".h", ".html", ".css", ".json", ".yaml", ".yml", ".md", ".sh", ".sql":
		return "📄"
	case ".zip", ".tar", ".gz", ".rar", ".7z":
		return "📦"
	default:
		return "📄"
	}
}
