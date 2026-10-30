package chatgpt

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Rafix4x/chatgpt-api/internal/fingerprint"
)

type Model struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

var MODELS = []Model{
	{ID: "gpt-6", Slug: "gpt-6", Description: "GPT-6 reasoning model (anonymous)"},
}

const ModelID = "gpt-6"

var modelIndex = func() map[string]string {
	m := make(map[string]string, len(MODELS))
	for _, v := range MODELS {
		m[v.ID] = v.Slug
	}
	return m
}()

func ResolveModel(name string) (id, slug string) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "chatgpt:")
	if slug, ok := modelIndex[name]; ok {
		return name, slug
	}
	return ModelID, MODELS[0].Slug
}

type Message struct {
	Role    string
	Content string
}

type ChatRequest struct {
	Model    string
	Messages []Message
}

type UpstreamError struct {
	Status int
	Kind   string
	Msg    string
}

func (e *UpstreamError) Error() string {
	return e.Msg
}

type Client struct {
	Base        string
	Impersonate string
	UA          string
	Lang        string
	Timeout     time.Duration
	Delay       time.Duration
	Attempts    int
	Logf        func(string, ...any)
	Session     *SessionFile

	mu      sync.Mutex
	jar     http.CookieJar
	proxies []string
	egress  int
	buildID string
	device  string
}

func NewClient(base, impersonate, ua, lang string, timeout time.Duration, proxies []string, attempts int, sessionPath string, logf func(string, ...any)) *Client {
	jar, _ := cookiejar.New(nil)
	if base == "" {
		base = "https://chatgpt.com"
	}
	if attempts <= 0 {
		attempts = 4
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Client{
		Base:        strings.TrimRight(base, "/"),
		Impersonate: impersonate,
		UA:          ua,
		Lang:        lang,
		Timeout:     timeout,
		Delay:       time.Second,
		Attempts:    attempts,
		Logf:        logf,
		Session:     NewSessionFile(sessionPath),
		jar:         jar,
		proxies:     proxies,
	}
}

func (c *Client) httpClient() (*http.Client, error) {
	c.mu.Lock()
	idx := c.egress
	proxies := c.proxies
	c.mu.Unlock()

	proxyURL := ""
	if idx > 0 && idx <= len(proxies) {
		proxyURL = proxies[idx-1]
	}

	if c.Impersonate != "" {
		hc, err := fingerprint.New(fingerprint.Options{
			Profile:   c.Impersonate,
			Timeout:   c.Timeout,
			ProxyURL:  proxyURL,
			CookieJar: c.jar,
		})
		if err == nil {
			return hc, nil
		}
		c.Logf("fingerprint profile %q unavailable (%v); falling back to stdlib TLS", c.Impersonate, err)
	}

	transport := &http.Transport{
		ForceAttemptHTTP2:   false,
		DisableCompression:  true,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}
	if proxyURL != "" {
		pu, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid chatgpt_proxies[%d]: %w", idx-1, err)
		}
		transport.Proxy = http.ProxyURL(pu)
	}

	return &http.Client{Transport: transport, Jar: c.jar, Timeout: c.Timeout}, nil
}

func (c *Client) rotate() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.egress >= len(c.proxies) {
		return false
	}
	c.egress++
	return true
}

func (c *Client) Egress() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.egress
}
