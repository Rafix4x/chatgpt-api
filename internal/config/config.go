package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Port              int      `json:"port"`
	Host              string   `json:"host"`
	DefaultModel      string   `json:"default_model"`
	RequestTimeoutSec int      `json:"request_timeout_sec"`
	RetryAttempts     int      `json:"retry_attempts"`
	RetryDelaySec     int      `json:"retry_delay_sec"`
	LogRequests       bool     `json:"log_requests"`
	APIKeys           []string `json:"api_keys"`

	ChatGPTBase         string   `json:"chatgpt_base"`
	ChatGPTUserAgent    string   `json:"chatgpt_user_agent"`
	ChatGPTAcceptLang   string   `json:"chatgpt_accept_language"`
	ChatGPTProxies      []string `json:"chatgpt_proxies"`
	ChatGPTDefaultModel string   `json:"chatgpt_default_model"`
	Impersonate         string   `json:"impersonate"`
	SessionFile         string   `json:"session_file"`
	Sessions            []string `json:"sessions"`
}

func Default() Config {
	return Config{
		Port:                8082,
		Host:                "0.0.0.0",
		DefaultModel:        "gpt-6",
		RequestTimeoutSec:   300,
		RetryAttempts:       4,
		RetryDelaySec:       2,
		LogRequests:         true,
		APIKeys:             []string{},
		ChatGPTBase:         "https://chatgpt.com",
		ChatGPTUserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36",
		ChatGPTAcceptLang:   "en-US,en;q=0.9",
		ChatGPTProxies:      []string{},
		ChatGPTDefaultModel: "gpt-6",
		Impersonate:         "chrome_146",
		SessionFile:         "",
		Sessions:            []string{},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	var aux struct {
		Port                *int      `json:"port"`
		Host                *string   `json:"host"`
		DefaultModel        *string   `json:"default_model"`
		RequestTimeoutSec   *int      `json:"request_timeout_sec"`
		RetryAttempts       *int      `json:"retry_attempts"`
		RetryDelaySec       *int      `json:"retry_delay_sec"`
		LogRequests         *bool     `json:"log_requests"`
		APIKeys             *[]string `json:"api_keys"`
		ChatGPTBase         *string   `json:"chatgpt_base"`
		ChatGPTUserAgent    *string   `json:"chatgpt_user_agent"`
		ChatGPTAcceptLang   *string   `json:"chatgpt_accept_language"`
		ChatGPTProxies      *[]string `json:"chatgpt_proxies"`
		ChatGPTDefaultModel *string   `json:"chatgpt_default_model"`
		Impersonate         *string   `json:"impersonate"`
		SessionFile         *string   `json:"session_file"`
		Sessions            *[]string `json:"sessions"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return cfg, err
	}

	if aux.Port != nil {
		cfg.Port = *aux.Port
	}
	if aux.Host != nil {
		cfg.Host = *aux.Host
	}
	if aux.DefaultModel != nil {
		cfg.DefaultModel = *aux.DefaultModel
	}
	if aux.RequestTimeoutSec != nil {
		cfg.RequestTimeoutSec = *aux.RequestTimeoutSec
	}
	if aux.RetryAttempts != nil {
		cfg.RetryAttempts = *aux.RetryAttempts
	}
	if aux.RetryDelaySec != nil {
		cfg.RetryDelaySec = *aux.RetryDelaySec
	}
	if aux.LogRequests != nil {
		cfg.LogRequests = *aux.LogRequests
	}
	if aux.APIKeys != nil {
		cfg.APIKeys = *aux.APIKeys
	}
	if aux.ChatGPTBase != nil {
		cfg.ChatGPTBase = *aux.ChatGPTBase
	}
	if aux.ChatGPTUserAgent != nil {
		cfg.ChatGPTUserAgent = *aux.ChatGPTUserAgent
	}
	if aux.ChatGPTAcceptLang != nil {
		cfg.ChatGPTAcceptLang = *aux.ChatGPTAcceptLang
	}
	if aux.ChatGPTProxies != nil {
		cfg.ChatGPTProxies = *aux.ChatGPTProxies
	}
	if aux.ChatGPTDefaultModel != nil {
		cfg.ChatGPTDefaultModel = *aux.ChatGPTDefaultModel
	}
	if aux.Impersonate != nil {
		cfg.Impersonate = *aux.Impersonate
	}
	if aux.SessionFile != nil {
		cfg.SessionFile = *aux.SessionFile
	}
	if aux.Sessions != nil {
		cfg.Sessions = *aux.Sessions
	}

	return cfg, nil
}
