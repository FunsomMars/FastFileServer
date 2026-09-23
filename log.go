package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type logger struct {
	mu     sync.Mutex
	out    io.Writer
	path   string // optional: when set we can reopen on SIGHUP
}

func newLogger(w io.Writer) *logger { return &logger{out: w} }

// reopen is only useful when the logger was constructed from a real file and
// the path is known. It is a no-op for stdout/err. Callers (e.g. SIGHUP handler)
// can use this to pick up an externally rotated log file.
func (l *logger) reopen() error {
	if l == nil || l.path == "" {
		return nil
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	l.mu.Lock()
	old := l.out
	l.out = f
	l.mu.Unlock()
	if c, ok := old.(io.Closer); ok {
		_ = c.Close()
	}
	return nil
}

func (l *logger) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		dur := time.Since(start)

		l.mu.Lock()
		defer l.mu.Unlock()
		log.Printf("%s - %s \"%s %s %s\" %d %dB %s",
			r.RemoteAddr,
			r.Header.Get("X-Forwarded-For"),
			r.Method, r.URL.RequestURI(), r.Proto,
			sw.code, sw.bytes, dur)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code  int
	bytes int64
}

func (w *statusWriter) WriteHeader(c int) {
	w.code = c
	w.ResponseWriter.WriteHeader(c)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// http.Flusher passthrough
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}