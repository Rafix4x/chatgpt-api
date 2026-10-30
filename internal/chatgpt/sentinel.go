package chatgpt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Rafix4x/chatgpt-api/internal/pow"
)

func (c *Client) fetchBuildID() string {
	c.mu.Lock()
	if c.buildID != "" {
		defer c.mu.Unlock()
		return c.buildID
	}
	c.mu.Unlock()

	hc, err := c.httpClient()
	if err != nil {
		return ""
	}
	req, err := http.NewRequest(http.MethodGet, c.Base+"/", nil)
	if err != nil {
		return ""
	}
	req.Header = c.buildHeaders(false)
	req.Header.Set("Accept", "text/html")

	resp, err := hc.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))

	id := extractAttr(string(body), "data-build=")
	c.mu.Lock()
	c.buildID = id
	c.mu.Unlock()
	return id
}

func extractAttr(html, key string) string {
	i := strings.Index(html, key)
	if i < 0 {
		return ""
	}
	rest := html[i+len(key):]
	if len(rest) == 0 {
		return ""
	}
	var quote byte
	if rest[0] == '"' || rest[0] == '\'' {
		quote = rest[0]
		rest = rest[1:]
	} else {
		return ""
	}
	end := strings.IndexByte(rest, quote)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func (c *Client) deviceID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.device == "" {
		c.device = pow.RandomUUID()
	}
	return c.device
}

type sentinelResp struct {
	Token       string `json:"token"`
	ProofOfWork *struct {
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
	Turnstile *struct {
		DX string `json:"dx"`
	} `json:"turnstile"`
}

func (c *Client) getTokens() (string, string, error) {
	cfg := pow.BuildConfig(c.fetchBuildID(), c.deviceID(), c.effectiveUA())
	pValue, err := cfg.Encode()
	if err != nil {
		return "", "", err
	}

	payload, _ := json.Marshal(map[string]string{"p": pValue})
	hc, err := c.httpClient()
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequest(http.MethodPost, c.Base+"/backend-anon/sentinel/chat-requirements", strings.NewReader(string(payload)))
	if err != nil {
		return "", "", err
	}
	req.Header = c.buildHeaders(false)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OAI-Device-Id", c.deviceID())
	if bid := c.fetchBuildID(); bid != "" {
		req.Header.Set("OAI-Client-Version", bid)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200<<10))
	c.Logf("sentinel: HTTP %d cf-mitigated=%q body=%q", resp.StatusCode, resp.Header.Get("cf-mitigated"), truncate(string(body), 120))

	if resp.StatusCode != http.StatusOK {
		return "", "", &UpstreamError{
			Status: resp.StatusCode,
			Kind:   "sentinel",
			Msg:    fmt.Sprintf("sentinel requirements HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}

	var sr sentinelResp
	if err := json.Unmarshal(body, &sr); err != nil {
		return "", "", fmt.Errorf("sentinel requirements: %w", err)
	}
	if sr.Token == "" {
		return "", "", fmt.Errorf("sentinel requirements: empty token")
	}
	if sr.ProofOfWork == nil || sr.ProofOfWork.Seed == "" {
		return sr.Token, "", nil
	}

	cfg[3] = float64(0)
	cfg[9] = float64(1400)
	proof, err := cfg.Solve(sr.ProofOfWork.Seed, sr.ProofOfWork.Difficulty, 500000)
	if err != nil {
		return sr.Token, "", err
	}
	return sr.Token, proof, nil
}

func (c *Client) getConduit(next bool, parentID string) (string, error) {
	body := map[string]any{
		"action":                        "next",
		"fork_from_shared_post":         false,
		"parent_message_id":             "client-created-root",
		"model":                         "auto",
		"timezone_offset_min":           0,
		"history_and_training_disabled": true,
		"conversation_mode":             map[string]any{"kind": "primary_assistant"},
		"system_hints":                  []string{},
		"supports_buffering":            true,
		"supported_encodings":           []string{"v1"},
	}
	if next && parentID != "" {
		body["parent_message_id"] = parentID
	}
	raw, _ := json.Marshal(body)

	hc, err := c.httpClient()
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, c.Base+"/backend-anon/f/conversation/prepare", strings.NewReader(string(raw)))
	if err != nil {
		return "", err
	}
	req.Header = c.buildHeaders(false)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OAI-Device-Id", c.deviceID())

	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	var out struct {
		Status       string `json:"status"`
		ConduitToken string `json:"conduit_token"`
	}
	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		return "", fmt.Errorf("conduit: %w", err)
	}
	if out.ConduitToken == "" {
		return "", &UpstreamError{
			Status: resp.StatusCode,
			Kind:   "conduit",
			Msg:    fmt.Sprintf("conduit handshake failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes))),
		}
	}
	return out.ConduitToken, nil
}

func (c *Client) effectiveUA() string {
	if c.Session != nil {
		if cookie, ua := c.Session.Current(); cookie != "" && ua != "" {
			return ua
		}
	}
	return c.UA
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
