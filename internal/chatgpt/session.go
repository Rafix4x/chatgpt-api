package chatgpt

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

type SessionFile struct {
	mu     sync.Mutex
	path   string
	mtime  time.Time
	header string
	ua     string
	loaded bool
}

func NewSessionFile(path string) *SessionFile {
	return &SessionFile{path: path}
}

func (s *SessionFile) Current() (header, ua string) {
	if s == nil || s.path == "" {
		return "", ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := os.Stat(s.path)
	if err != nil {
		if s.loaded {
			return s.header, s.ua
		}
		return "", ""
	}
	if s.loaded && st.ModTime().Equal(s.mtime) {
		return s.header, s.ua
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		return s.header, s.ua
	}
	var sess struct {
		Cookies []struct {
			Name   string `json:"name"`
			Value  string `json:"value"`
			Domain string `json:"domain"`
		} `json:"cookies"`
		UserAgent string `json:"user_agent"`
	}
	if err := json.Unmarshal(data, &sess); err != nil {
		return s.header, s.ua
	}

	parts := make([]string, 0, len(sess.Cookies))
	for _, c := range sess.Cookies {
		if c.Name == "" || c.Value == "" {
			continue
		}
		parts = append(parts, c.Name+"="+c.Value)
	}
	s.header = strings.Join(parts, "; ")
	s.ua = sess.UserAgent
	s.mtime = st.ModTime()
	s.loaded = true
	return s.header, s.ua
}
