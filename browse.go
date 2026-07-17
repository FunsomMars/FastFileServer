package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // URL path, leading slash, for /files/
	Rel     string `json:"rel"`  // POSIX-style relative path
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	SizeStr string `json:"sizeStr"`
	ModTime int64  `json:"modTime"` // unix ms
	ModStr  string `json:"modStr"`
	Kind    string `json:"kind"` // folder, image, video, audio, archive, doc, code, exe, iso, font
}

// browseData is passed to the HTML template.
type browseData struct {
	Root       string
	Breadcrumb []crumb
	Parent     string // path of parent or ""
	Entries    []entry
	TotalSize  int64
	Upload     bool
	NoListing  bool
}

type crumb struct {
	Name string
	Path string
	IsLast bool
}

var browseTmpl = func() *template.Template {
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		panic(err)
	}
	t, err := template.New("browse").Funcs(funcMap).Parse(string(data))
	if err != nil {
		panic(err)
	}
	return t
}()

var funcMap = template.FuncMap{
	"fmtSize": sizeStr,
	"lower":   strings.ToLower,
	"icon":    iconFor,
}

func (fs *fileServer) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if fs.noListing {
		http.Error(w, "listing disabled", http.StatusForbidden)
		return
	}
	subPath := r.URL.Path
	if subPath == "/" || subPath == "" {
		subPath = "/"
	}
	dir, err := fs.resolveSafe(subPath)
	if err != nil {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	// If user pointed at a file via /, redirect to /files/.
	fi, err := os.Stat(dir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !fi.IsDir() {
		http.Redirect(w, r, "/files/"+strings.TrimPrefix(subPath, "/"), http.StatusFound)
		return
	}

	entries, total, err := scanDir(fs.root, dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := browseData{
		Root:       filepath.Base(fs.root),
		Breadcrumb: buildBreadcrumb(fs.root, dir),
		Parent:     parentRel(fs.root, dir),
		Entries:    entries,
		TotalSize:  total,
		Upload:     fs.enableUp,
		NoListing:  fs.noListing,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if err := browseTmpl.Execute(w, data); err != nil {
		log.Printf("template: %v", err)
	}
}

func (fs *fileServer) handleAPIList(w http.ResponseWriter, r *http.Request) {
	if fs.noListing {
		http.Error(w, "listing disabled", http.StatusForbidden)
		return
	}
	sub := r.URL.Query().Get("path")
	if sub == "" {
		sub = "/"
	}
	dir, err := fs.resolveSafe(sub)
	if err != nil {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	fi, err := os.Stat(dir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !fi.IsDir() {
		http.Error(w, "not a directory", http.StatusBadRequest)
		return
	}
	entries, total, err := scanDir(fs.root, dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]any{
		"path":      sub,
		"entries":   entries,
		"totalSize": total,
		"totalStr":  sizeStr(total),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

func (fs *fileServer) handleAPIInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"root":         fs.root,
		"upload":       fs.enableUp,
		"maxUpload":    fs.maxUpload,
		"listing":      !fs.noListing,
		"overwrite":    fs.overwrite,
		"version":      version,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(info)
}

const version = "1.0.0"

func scanDir(root, dir string) ([]entry, int64, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	names, err := f.Readdirnames(0)
	if err != nil {
		return nil, 0, err
	}
	out := make([]entry, 0, len(names))
	var total int64
	for _, n := range names {
		full := filepath.Join(dir, n)
		fi, err := os.Lstat(full)
		if err != nil {
			continue
		}
		// Skip symlinks unless enabled.
		if fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		rel, _ := filepath.Rel(root, full)
		relPosix := filepath.ToSlash(rel)
		e := entry{
			Name:    n,
			Rel:     relPosix,
			Path:    "/files/" + relPosix,
			IsDir:   fi.IsDir(),
			Size:    fi.Size(),
			SizeStr: sizeStr(fi.Size()),
			ModTime: fi.ModTime().UnixMilli(),
			ModStr:  fi.ModTime().Format("2006-01-02 15:04"),
			Kind:    classify(fi.Name(), fi.IsDir()),
		}
		out = append(out, e)
		if !fi.IsDir() {
			total += fi.Size()
		}
	}
	// Sort: directories first, then case-insensitive name.
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, total, nil
}

func classify(name string, isDir bool) string {
	if isDir {
		return "folder"
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".ico", ".heic":
		return "image"
	case ".mp4", ".mkv", ".avi", ".mov", ".webm", ".flv", ".wmv":
		return "video"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a":
		return "audio"
	case ".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".tgz":
		return "archive"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".txt", ".md", ".rtf":
		return "doc"
	case ".go", ".py", ".js", ".ts", ".c", ".cpp", ".h", ".java", ".rs", ".rb", ".php", ".sh", ".json", ".yaml", ".yml", ".xml", ".html", ".css", ".sql":
		return "code"
	case ".exe", ".msi", ".bat", ".cmd", ".com", ".app", ".dmg":
		return "exe"
	case ".iso", ".img":
		return "iso"
	case ".ttf", ".otf", ".woff", ".woff2":
		return "font"
	}
	return "file"
}

func buildBreadcrumb(root, dir string) []crumb {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return nil
	}
	if rel == "." {
		return []crumb{{Name: filepath.Base(root), Path: "/", IsLast: true}}
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	out := []crumb{{Name: filepath.Base(root), Path: "/"}}
	acc := ""
	for i, p := range parts {
		if p == "" {
			continue
		}
		acc += "/" + p
		out = append(out, crumb{Name: p, Path: acc, IsLast: i == len(parts)-1})
	}
	return out
}

func parentRel(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return ""
	}
	parent := filepath.Dir(rel)
	if parent == "." {
		return ""
	}
	return "/" + filepath.ToSlash(parent)
}

func iconFor(kind string) string {
	switch kind {
	case "folder":
		return "📁"
	case "image":
		return "🖼"
	case "video":
		return "🎬"
	case "audio":
		return "🎵"
	case "archive":
		return "📦"
	case "doc":
		return "📄"
	case "code":
		return "⟨⟩"
	case "exe":
		return "⚙"
	case "iso":
		return "💿"
	case "font":
		return "𝐀"
	}
	return "📄"
}

func sizeStr(n int64) string {
	if n <= 0 {
		if n == 0 {
			return "0 B"
		}
		return "-"
	}
	const k = 1024
	suffixes := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0
	f := float64(n)
	for f >= k && i < len(suffixes)-1 {
		f /= k
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, suffixes[0])
	}
	if f >= 100 {
		return fmt.Sprintf("%.0f %s", f, suffixes[i])
	}
	if f >= 10 {
		return fmt.Sprintf("%.1f %s", f, suffixes[i])
	}
	return fmt.Sprintf("%.2f %s", f, suffixes[i])
}

// silence unused
var _ = os.Stat