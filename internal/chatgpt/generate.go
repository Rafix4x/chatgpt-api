package chatgpt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Rafix4x/chatgpt-api/internal/pow"
)

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
func (c *Client) buildHeaders(stream bool) http.Header {
	h := make(http.Header)
	accept := "*/*"
	if stream {
		accept = "text/event-stream"
	}
	h.Set("Accept", accept)
	h.Set("Accept-Language", c.Lang)
	h.Set("Content-Type", "application/json")
	h.Set("Origin", "https://chatgpt.com")
	h.Set("Referer", "https://chatgpt.com/")
	ua := c.UA
	cookieHeader := ""
	if c.Session != nil {
		if ch, sessionUA := c.Session.Current(); ch != "" {
			cookieHeader = ch
			if sessionUA != "" {
				ua = sessionUA
			}
		}
	}
	h.Set("User-Agent", ua)
	if cookieHeader != "" {
		h.Set("Cookie", cookieHeader)
	}
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Site", "same-origin")
	return h
}

type anonPart struct {
	ID      string         `json:"id"`
	Author  map[string]any `json:"author"`
	Content map[string]any `json:"content"`
}

type anonBody struct {
	Action             string         `json:"action"`
	Messages           []anonPart     `json:"messages"`
	ParentMessageID    string         `json:"parent_message_id"`
	Model              string         `json:"model"`
	TimezoneOffsetMin  int            `json:"timezone_offset_min"`
	HistoryAndTraining bool           `json:"history_and_training_disabled"`
	Metadata           map[string]any `json:"metadata"`
}

func buildBody(model string, msgs []Message) anonBody {
	var sb strings.Builder
	lastUser := ""
	for _, m := range msgs {
		role := m.Role
		if role == "" {
			role = "user"
		}
		switch role {
		case "system", "developer":
			sb.WriteString("system: ")
		case "assistant":
			sb.WriteString("assistant: ")
		default:
			sb.WriteString("user: ")
			lastUser = m.Content
		}
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}

	prompt := strings.TrimSpace(sb.String())
	if len(msgs) == 1 {
		prompt = lastUser
	}

	return anonBody{
		Action: "next",
		Messages: []anonPart{{
			ID:     newUUID(),
			Author: map[string]any{"role": "user"},
			Content: map[string]any{
				"content_type": "text",
				"parts":        []string{prompt},
			},
		}},
		ParentMessageID:   "client-created-root",
		Model:             model,
		TimezoneOffsetMin: 0,
		Metadata:          map[string]any{},
	}
}

func (c *Client) conversation(ctx context.Context, body anonBody) (io.ReadCloser, error) {
	sentinelToken, proofToken, err := c.getTokens()
	if err != nil {
		return nil, err
	}
	conduitToken, err := c.getConduit(false, "")
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < c.Attempts; attempt++ {
		hc, err := c.httpClient()
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/backend-anon/f/conversation", bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header = c.buildHeaders(true)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("OAI-Device-Id", c.deviceID())
		if bid := c.fetchBuildID(); bid != "" {
			req.Header.Set("OAI-Client-Version", bid)
		}
		req.Header.Set("OAI-Echo-Logs", pow.EchoLogs())
		req.Header.Set("OpenAI-Sentinel-Chat-Requirements-Token", sentinelToken)
		if proofToken != "" {
			req.Header.Set("OpenAI-Sentinel-Proof-Token", proofToken)
		}
		req.Header.Set("X-Conduit-Token", conduitToken)

		resp, err := hc.Do(req)
		if err != nil {
			lastErr = &UpstreamError{Kind: "transport", Msg: err.Error()}
			if c.rotate() {
				continue
			}
			time.Sleep(c.Delay)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return resp.Body, nil
		}

		text, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		snippet := strings.TrimSpace(string(text))

		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable:
			kind := "ip_rejected"
			if resp.StatusCode == http.StatusForbidden {
				kind = "cloudflare_challenge"
				if strings.Contains(snippet, "Unusual activity") {
					kind = "anti_abuse"
				}
			} else if resp.StatusCode == http.StatusTooManyRequests {
				kind = "rate_limited"
			}
			lastErr = &UpstreamError{
				Status: resp.StatusCode,
				Kind:   kind,
				Msg:    fmt.Sprintf("upstream HTTP %d: %s", resp.StatusCode, snippet),
			}
			if c.rotate() {
				c.Logf("chatgpt: %s on egress slot %d, rotating to %d", kind, c.Egress()-1, c.Egress())
				continue
			}
		default:
			lastErr = &UpstreamError{
				Status: resp.StatusCode,
				Kind:   "http",
				Msg:    fmt.Sprintf("upstream HTTP %d: %s", resp.StatusCode, snippet),
			}
		}
		time.Sleep(c.Delay)
	}
	if lastErr == nil {
		lastErr = &UpstreamError{Kind: "http", Msg: "no attempts made"}
	}
	return nil, lastErr
}

func (c *Client) do(ctx context.Context, path string, payload any) (io.ReadCloser, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < c.Attempts; attempt++ {
		hc, err := c.httpClient()
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header = c.buildHeaders(true)

		resp, err := hc.Do(req)
		if err != nil {
			msg := strings.TrimPrefix(err.Error(), "Post "+strconv.Quote(c.Base+path)+": ")
			lastErr = &UpstreamError{Kind: "transport", Msg: msg}
			if c.rotate() {
				c.Logf("chatgpt: transport failure (%v), rotating egress to slot %d", msg, c.Egress())
				continue
			}
			time.Sleep(c.Delay)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return resp.Body, nil
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		snippet := strings.TrimSpace(string(body))

		switch resp.StatusCode {
		case http.StatusForbidden, http.StatusServiceUnavailable:
			kind := "cloudflare_challenge"
			msg := "cloudflare challenge (cf-mitigated) on this egress IP; add a browser-cleared IP/proxy to chatgpt_proxies"
			if !strings.Contains(strings.ToLower(snippet), "cloudflare") && snippet != "" {
				kind = "forbidden"
				msg = fmt.Sprintf("upstream HTTP %d: %s", resp.StatusCode, snippet)
			}
			lastErr = &UpstreamError{Status: resp.StatusCode, Kind: kind, Msg: msg}
			if c.rotate() {
				c.Logf("chatgpt: rotating egress to slot %d after HTTP %d", c.Egress(), resp.StatusCode)
				continue
			}
		case http.StatusUnauthorized:
			lastErr = &UpstreamError{
				Status: resp.StatusCode,
				Kind:   "ip_rejected",
				Msg:    "anonymous backend refused this egress IP (HTTP 401); add an egress that has already loaded chatgpt.com to chatgpt_proxies",
			}
			if c.rotate() {
				c.Logf("chatgpt: egress IP rejected, rotating to slot %d", c.Egress())
				continue
			}
		case http.StatusTooManyRequests:
			lastErr = &UpstreamError{
				Status: resp.StatusCode,
				Kind:   "rate_limited",
				Msg:    "anonymous quota exhausted for this IP; add more chatgpt_proxies entries or retry later",
			}
			if c.rotate() {
				c.Logf("chatgpt: rate limited, rotating egress to slot %d", c.Egress())
				continue
			}
		default:
			lastErr = &UpstreamError{
				Status: resp.StatusCode,
				Kind:   "http",
				Msg:    fmt.Sprintf("upstream HTTP %d: %s", resp.StatusCode, snippet),
			}
		}
		time.Sleep(c.Delay)
	}
	if lastErr == nil {
		lastErr = &UpstreamError{Kind: "http", Msg: "no attempts made"}
	}
	if ue, ok := lastErr.(*UpstreamError); ok && len(c.cfgProxies()) > 0 {
		ue.Msg = fmt.Sprintf("%s (all %d egress slots exhausted)", ue.Msg, 1+len(c.cfgProxies()))
	}
	return nil, lastErr
}

func (c *Client) cfgProxies() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.proxies...)
}

func (c *Client) Generate(req ChatRequest) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	rc, err := c.conversation(ctx, buildBody(req.Model, req.Messages))
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var sb strings.Builder
	err = ParseSSE(rc, func(ev Event) error {
		if ev.Text != "" {
			sb.WriteString(ev.Text)
		}
		return nil
	})
	if err != nil && sb.Len() == 0 {
		return "", err
	}
	return sb.String(), nil
}

func (c *Client) GenerateStream(req ChatRequest, emit func(string) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	rc, err := c.conversation(ctx, buildBody(req.Model, req.Messages))
	if err != nil {
		return err
	}
	defer rc.Close()

	return ParseSSE(rc, func(ev Event) error {
		if ev.Text == "" {
			return nil
		}
		return emit(ev.Text)
	})
}
