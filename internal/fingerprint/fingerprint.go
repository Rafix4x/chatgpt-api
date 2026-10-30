package fingerprint

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	http_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const DefaultProfile = "chrome_146"

var profilesByName = map[string]profiles.ClientProfile{
	"chrome_120":      profiles.Chrome_120,
	"chrome_124":      profiles.Chrome_124,
	"chrome_131":      profiles.Chrome_131,
	"chrome_133":      profiles.Chrome_133,
	"chrome_144":      profiles.Chrome_144,
	"chrome_146":      profiles.Chrome_146,
	"firefox_120":     profiles.Firefox_120,
	"firefox_123":     profiles.Firefox_123,
	"firefox_147":     profiles.Firefox_147,
	"safari_16_0":     profiles.Safari_16_0,
	"safari_ios_17_0": profiles.Safari_IOS_17_0,
}

func ResolveProfile(name string) profiles.ClientProfile {
	if p, ok := profilesByName[strings.ToLower(strings.TrimSpace(name))]; ok {
		return p
	}
	return profilesByName[DefaultProfile]
}

func KnownProfile(name string) bool {
	_, ok := profilesByName[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func Names() []string {
	out := make([]string, 0, len(profilesByName))
	for k := range profilesByName {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

type Options struct {
	Profile   string
	Timeout   time.Duration
	ProxyURL  string
	CookieJar http.CookieJar
}

type adapter struct {
	client http_client.HttpClient
}

var (
	cacheMu sync.Mutex
	cache   = make(map[string]*adapter)
)

func New(opts Options) (*http.Client, error) {
	return NewWithTimeout(opts, 0)
}

func NewWithTimeout(opts Options, timeout time.Duration) (*http.Client, error) {
	if opts.Profile == "" {
		opts.Profile = DefaultProfile
	}
	if !KnownProfile(opts.Profile) {
		return nil, fmt.Errorf("unknown impersonate profile %q (known: %s)", opts.Profile, strings.Join(Names(), ", "))
	}

	if timeout > 0 {
		opts.Timeout = timeout
	}
	timeoutSec := int(opts.Timeout.Seconds())
	if timeoutSec <= 0 {
		timeoutSec = 300
	}

	key := fmt.Sprintf("%s|%s|%d", strings.ToLower(opts.Profile), opts.ProxyURL, timeoutSec)
	cacheMu.Lock()
	a, ok := cache[key]
	cacheMu.Unlock()

	if !ok {
		options := []http_client.HttpClientOption{
			http_client.WithClientProfile(ResolveProfile(opts.Profile)),
			http_client.WithTimeoutSeconds(timeoutSec),
			http_client.WithNotFollowRedirects(),
			http_client.WithRandomTLSExtensionOrder(),
			http_client.WithTransportOptions(&http_client.TransportOptions{DisableCompression: true}),
		}
		if opts.ProxyURL != "" {
			options = append(options, http_client.WithProxyUrl(opts.ProxyURL))
		}

		client, err := http_client.NewHttpClient(http_client.NewNoopLogger(), options...)
		if err != nil {
			return nil, err
		}
		a = &adapter{client: client}

		cacheMu.Lock()
		cache[key] = a
		cacheMu.Unlock()
	}

	return &http.Client{
		Transport: &RoundTripper{a: a},
		Timeout:   time.Duration(timeoutSec) * time.Second,
		Jar:       opts.CookieJar,
	}, nil
}

type RoundTripper struct {
	a *adapter
}

func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
	}

	freq, err := fhttp.NewRequest(req.Method, req.URL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, vv := range req.Header {
		for _, v := range vv {
			freq.Header.Add(k, v)
		}
	}

	fresp, err := rt.a.client.Do(freq)
	if err != nil {
		return nil, err
	}

	resp := &http.Response{
		Status:        fresp.Status,
		StatusCode:    fresp.StatusCode,
		Proto:         fresp.Proto,
		ProtoMajor:    fresp.ProtoMajor,
		ProtoMinor:    fresp.ProtoMinor,
		Header:        make(http.Header),
		Body:          fresp.Body,
		ContentLength: fresp.ContentLength,
		Request:       req,
	}
	for k, vv := range fresp.Header {
		for _, v := range vv {
			resp.Header.Add(k, v)
		}
	}
	return resp, nil
}
