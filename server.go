package main

import (
	"net/http"
	"path/filepath"
	"strings"
)

type fileServer struct {
	root      string
	enableUp  bool
	maxUpload int64
	overwrite  bool
	noListing bool
	followSym bool
}

// resolveSafe joins root + urlPath and verifies the result is inside root.
// Symlinks are rejected by default.
func (fs *fileServer) resolveSafe(urlPath string) (string, error) {
	if urlPath == "" || urlPath == "/" {
		return fs.root, nil
	}
	// strip leading slash
	rel := strings.TrimPrefix(urlPath, "/")
	rel = filepath.FromSlash(rel)
	full := filepath.Join(fs.root, rel)
	// Clean prevents ".." traversal at the textual level.
	cleaned := filepath.Clean(full)
	// Verify still under root (handles Windows case-insensitivity too).
	relCheck, err := filepath.Rel(fs.root, cleaned)
	if err != nil || strings.HasPrefix(relCheck, "..") || strings.Contains(relCheck, ".."+string(filepath.Separator)) && relCheck != ".." {
		return "", errBadPath
	}
	if relCheck == ".." {
		return "", errBadPath
	}
	if !fs.followSym {
		// If the parent contains a symlink that escapes, reject.
		// Evaluate parent (must exist).
		parent := filepath.Dir(cleaned)
		realParent, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return "", err
		}
		if !strings.HasPrefix(realParent, fs.root) && realParent != fs.root {
			return "", errBadPath
		}
	}
	return cleaned, nil
}

var errBadPath = &httpError{code: http.StatusBadRequest, msg: "bad path"}

type httpError struct {
	code int
	msg  string
}

func (e *httpError) Error() string { return e.msg }

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	// Inline SVG favicon - folder icon.
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(faviconSVG))
}

const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#3b82f6" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>`