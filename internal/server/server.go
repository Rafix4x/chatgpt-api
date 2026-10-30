package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/Rafix4x/chatgpt-api/internal/chatgpt"
)

type App struct {
	Chat         *chatgpt.Client
	DefaultModel string
	Version      string
	APIKeys      []string
	Logf         func(format string, args ...any)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func startSSE(w http.ResponseWriter) bool {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return true
}

func writeSSEData(w http.ResponseWriter, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("data: " + string(b) + "\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return err
}

func writeSSEDone(w http.ResponseWriter) error {
	_, err := w.Write([]byte("data: [DONE]\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return err
}

func (a *App) HandleHealth(w http.ResponseWriter, r *http.Request) {
	names := make([]string, 0, len(chatgpt.MODELS))
	for _, m := range chatgpt.MODELS {
		names = append(names, m.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"version":     a.Version,
		"models":      names,
		"egress_slot": a.Chat.Egress(),
	})
}

func (a *App) HandleModels(w http.ResponseWriter, r *http.Request) {
	data := make([]map[string]any, 0, len(chatgpt.MODELS))
	for _, m := range chatgpt.MODELS {
		data = append(data, map[string]any{
			"id":          m.ID,
			"object":      "model",
			"created":     1700000000,
			"owned_by":    "openai",
			"description": m.Description,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}
