package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

func (a *App) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		if len(a.APIKeys) > 0 && r.URL.Path != "/" {
			if !a.authorized(r) {
				writeJSON(sw, http.StatusUnauthorized, map[string]any{
					"error": map[string]any{"message": "invalid api key"},
				})
				a.Logf("%s %s %s -> 401 (%s)", clientIP(r), r.Method, r.URL.Path, time.Since(start))
				return
			}
		}

		next.ServeHTTP(sw, r)
		a.Logf("%s %s %s -> %d (%s)", clientIP(r), r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

func (a *App) authorized(r *http.Request) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		token = r.Header.Get("X-Api-Key")
	}
	if token == "" {
		token = r.URL.Query().Get("key")
	}
	for _, k := range a.APIKeys {
		if constantTimeEqual(k, token) {
			return true
		}
	}
	return false
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return hex.EncodeToString(ha[:]) == hex.EncodeToString(hb[:])
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if i := strings.IndexByte(ip, ','); i > 0 {
			return strings.TrimSpace(ip[:i])
		}
		return strings.TrimSpace(ip)
	}
	if i := strings.LastIndexByte(r.RemoteAddr, ':'); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
