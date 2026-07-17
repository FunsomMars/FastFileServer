// Package main provides a high-performance single-root file sharing HTTP server.
//
// Features:
//   - Recursive directory browsing via web UI
//   - Concurrent downloads (one goroutine per connection)
//   - Zero-copy transfers via sendfile / TransmitFile (via http.ServeFile)
//   - Arbitrary file size with constant memory footprint
//   - HTTP Range / 206 Partial Content (resumable downloads)
//   - Optional upload (PUT and multipart POST)
//   - Optional Basic Auth
//   - Modern responsive UI with dark mode
package main

import (
	"context"
	"errors"
	"flag"
	ifs "io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type config struct {
	root       string
	addr       string
	enableUp   bool
	maxUpload  int64
	overwrite  bool
	noListing  bool
	followSym  bool
	authUser   string
	authPass   string
	logFile    string
	prettySize bool
}

func parseFlags() config {
	c := config{}
	flag.StringVar(&c.root, "root", "", "Root directory to serve (required, recursive)")
	flag.StringVar(&c.addr, "addr", ":8080", "Listen address (e.g. :8080, 0.0.0.0:9000)")
	flag.BoolVar(&c.enableUp, "upload", false, "Enable file upload (PUT/POST)")
	flag.Int64Var(&c.maxUpload, "max-upload", 0, "Max upload size in bytes (0 = unlimited)")
	flag.BoolVar(&c.overwrite, "overwrite", false, "Allow upload to overwrite existing files")
	flag.BoolVar(&c.noListing, "no-listing", false, "Disable directory listing")
	flag.BoolVar(&c.followSym, "follow-symlinks", false, "Follow symbolic links (off by default for safety)")
	flag.StringVar(&c.authUser, "auth", "", "Basic Auth in user:pass form (optional)")
	flag.StringVar(&c.logFile, "log", "", "Access log file (empty = stdout)")
	flag.BoolVar(&c.prettySize, "pretty", true, "Pretty-print sizes")
	flag.Parse()

	if c.authUser != "" {
		idx := strings.IndexByte(c.authUser, ':')
		if idx < 0 {
			log.Fatal("-auth must be in user:pass form")
		}
		c.authPass = c.authUser[idx+1:]
		c.authUser = c.authUser[:idx]
	}

	if c.root == "" {
		log.Fatal("-root is required")
	}
	abs, err := filepath.Abs(c.root)
	if err != nil {
		log.Fatalf("invalid -root: %v", err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		log.Fatalf("-root not accessible: %v", err)
	}
	if !fi.IsDir() {
		log.Fatalf("-root must be a directory: %s", abs)
	}
	c.root = abs
	return c
}

func main() {
	cfg := parseFlags()

	// Build shared dependencies.
	fs := &fileServer{
		root:      cfg.root,
		enableUp:  cfg.enableUp,
		maxUpload: cfg.maxUpload,
		overwrite: cfg.overwrite,
		noListing: cfg.noListing,
		followSym: cfg.followSym,
	}

	// Set up access logger.
	var accessLog *logger
	if cfg.logFile != "" {
		f, err := os.OpenFile(cfg.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("open log file: %v", err)
		}
		accessLog = newLogger(f)
	} else {
		accessLog = newLogger(os.Stdout)
	}

	mux := http.NewServeMux()

	// UI + listing - browse any directory path
	mux.HandleFunc("GET /{$}", fs.handleBrowse)
	mux.HandleFunc("GET /{path...}", fs.handleBrowse)
	mux.HandleFunc("GET /api/list", fs.handleAPIList)
	mux.HandleFunc("GET /api/info", fs.handleAPIInfo)

	// Downloads (serve raw files)
	mux.HandleFunc("GET /files/", fs.handleDownload)
	mux.HandleFunc("HEAD /files/", fs.handleDownload)

	// Uploads
	if cfg.enableUp {
		mux.HandleFunc("PUT /files/", fs.handleUploadPUT)
		mux.HandleFunc("POST /upload", fs.handleUploadMulti)
		mux.HandleFunc("DELETE /files/", fs.handleDelete)
	}

	// Static assets (CSS/JS/favicon) - serve from embedded web/ subdir.
	webSub, err := ifs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(webSub))))
	mux.HandleFunc("GET /favicon.ico", handleFavicon)

	// Build middleware chain.
	var h http.Handler = mux
	h = accessLog.middleware(h)
	if cfg.authUser != "" {
		h = basicAuth(cfg.authUser, cfg.authPass)(h)
	}
	h = securityHeaders(h)

	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       0, // allow long uploads
		WriteTimeout:      0, // allow large downloads
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}

	// Graceful shutdown.
	idleDone := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
		close(idleDone)
	}()

	log.Printf("fileserver starting")
	log.Printf("  root      : %s", cfg.root)
	log.Printf("  addr      : %s", cfg.addr)
	log.Printf("  upload    : %v (max %s, overwrite=%v)", cfg.enableUp, sizeStr(cfg.maxUpload), cfg.overwrite)
	log.Printf("  listing   : %v", !cfg.noListing)
	log.Printf("  auth      : %v", cfg.authUser != "")
	log.Printf("  open      : http://localhost%s", cfg.addr)

	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
	<-idleDone
	log.Printf("bye")
}