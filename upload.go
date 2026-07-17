package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// handleDownload serves a file from /files/... using http.ServeFile which
// automatically uses sendfile/TransmitFile (zero-copy) and supports Range/206/ETag.
func (fs *fileServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/files/")
	full, err := fs.resolveSafe("/" + urlPath)
	if err != nil {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	fi, err := os.Stat(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if fi.IsDir() {
		// Redirect to browse view for directories.
		http.Redirect(w, r, "/"+strings.TrimPrefix(urlPath, "/"), http.StatusFound)
		return
	}
	// Add Content-Disposition for nicer downloads (browsers honor filename).
	disposition := fmt.Sprintf(`attachment; filename*=UTF-8''%s`,
		url.PathEscape(filepath.Base(full)))
	w.Header().Set("Content-Disposition", disposition)

	// http.ServeFile handles Range/If-Modified-Since/ETag/HEAD automatically.
	http.ServeFile(w, r, full)
}

func (fs *fileServer) handleUploadPUT(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/files/")
	full, err := fs.resolveSafe("/" + urlPath)
	if err != nil {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	// Don't allow uploading into a directory.
	if strings.HasSuffix(urlPath, "/") || urlPath == "" {
		http.Error(w, "PUT target must be a file", http.StatusBadRequest)
		return
	}
	if !fs.overwrite {
		if _, err := os.Stat(full); err == nil {
			http.Error(w, "file exists (use -overwrite to allow)", http.StatusConflict)
			return
		}
	}
	// Create parent dir if needed.
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	var written int64
	if fs.maxUpload > 0 {
		written, err = io.CopyN(f, r.Body, fs.maxUpload+1)
		if err != nil && err != io.EOF {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if written > fs.maxUpload {
			os.Remove(full)
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}
	} else {
		written, err = io.Copy(f, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"name": filepath.Base(full),
		"size": written,
		"path": "/files/" + filepath.ToSlash(urlPath),
	})
}

// handleUploadMulti accepts multipart form upload to current directory.
func (fs *fileServer) handleUploadMulti(w http.ResponseWriter, r *http.Request) {
	// 32 MB in memory, rest goes to temp files.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	targetDir := r.URL.Query().Get("path")
	if targetDir == "" {
		targetDir = "/"
	}
	full, err := fs.resolveSafe(targetDir)
	if err != nil {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	if fi, err := os.Stat(full); err != nil || !fi.IsDir() {
		http.Error(w, "target is not a directory", http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		http.Error(w, "no files", http.StatusBadRequest)
		return
	}
	type result struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		OK   bool   `json:"ok"`
		Err  string `json:"err,omitempty"`
	}
	results := make([]result, 0, len(files))
	for _, fh := range files {
		dst := filepath.Join(full, filepath.Base(fh.Filename))
		if !fs.overwrite {
			if _, err := os.Stat(dst); err == nil {
				results = append(results, result{Name: fh.Filename, OK: false, Err: "exists"})
				continue
			}
		}
		f, err := fh.Open()
		if err != nil {
			results = append(results, result{Name: fh.Filename, OK: false, Err: err.Error()})
			continue
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			f.Close()
			results = append(results, result{Name: fh.Filename, OK: false, Err: err.Error()})
			continue
		}
		var n int64
		if fs.maxUpload > 0 {
			n, err = io.CopyN(out, f, fs.maxUpload+1)
		} else {
			n, err = io.Copy(out, f)
		}
		out.Close()
		f.Close()
		if err != nil && err != io.EOF {
			os.Remove(dst)
			results = append(results, result{Name: fh.Filename, OK: false, Err: err.Error()})
			continue
		}
		if n > fs.maxUpload && fs.maxUpload > 0 {
			os.Remove(dst)
			results = append(results, result{Name: fh.Filename, OK: false, Err: "too large"})
			continue
		}
		results = append(results, result{Name: fh.Filename, Size: n, OK: true})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "results": results})
}

func (fs *fileServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	urlPath := strings.TrimPrefix(r.URL.Path, "/files/")
	full, err := fs.resolveSafe("/" + urlPath)
	if err != nil {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	fi, err := os.Lstat(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if fi.IsDir() {
		http.Error(w, "DELETE on dir not allowed", http.StatusBadRequest)
		return
	}
	if err := os.Remove(full); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}