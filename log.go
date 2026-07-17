package main

import (
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

type logger struct {
	mu  sync.Mutex
	out io.Writer
}

func newLogger(w io.Writer) *logger { return &logger{out: w} }

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