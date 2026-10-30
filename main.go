package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Rafix4x/chatgpt-api/internal/chatgpt"
	"github.com/Rafix4x/chatgpt-api/internal/config"
	"github.com/Rafix4x/chatgpt-api/internal/fingerprint"
	"github.com/Rafix4x/chatgpt-api/internal/server"
)

var Version = "1.0.0"

func main() {
	portFlag := flag.Int("port", 0, "Server port")
	hostFlag := flag.String("host", "", "Bind address")
	configFlag := flag.String("config", "", "Config file path")
	impersonateFlag := flag.String("impersonate", "", "TLS profile: "+strings.Join(fingerprint.Names(), ", "))
	sessionFlag := flag.String("session-file", "", "Cookie file captured by -capture-session")
	versionFlag := flag.Bool("version", false, "Print version")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("chatgpt-api %s\n", Version)
		os.Exit(0)
	}

	configPath := *configFlag
	if configPath == "" {
		if _, err := os.Stat("./config.json"); err == nil {
			configPath = "./config.json"
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil && configPath != "" {
		log.Printf("config %s: %v", configPath, err)
	}

	applyEnv(&cfg)

	if *portFlag != 0 {
		cfg.Port = *portFlag
	}
	if *hostFlag != "" {
		cfg.Host = *hostFlag
	}
	if *impersonateFlag != "" {
		if !fingerprint.KnownProfile(*impersonateFlag) {
			log.Fatalf("unknown impersonate profile %q (known: %s)", *impersonateFlag, strings.Join(fingerprint.Names(), ", "))
		}
		cfg.Impersonate = *impersonateFlag
	}
	if *sessionFlag != "" {
		cfg.SessionFile = *sessionFlag
	}
	if cfg.ChatGPTDefaultModel != "" {
		cfg.DefaultModel = cfg.ChatGPTDefaultModel
	}

	logFn := func(format string, args ...any) {
		if cfg.LogRequests {
			log.Printf(format, args...)
		}
	}

	client := chatgpt.NewClient(
		cfg.ChatGPTBase,
		cfg.Impersonate,
		cfg.ChatGPTUserAgent,
		cfg.ChatGPTAcceptLang,
		time.Duration(cfg.RequestTimeoutSec)*time.Second,
		cfg.ChatGPTProxies,
		cfg.RetryAttempts,
		cfg.SessionFile,
		logFn,
	)

	app := &server.App{
		Chat:         client,
		DefaultModel: cfg.DefaultModel,
		Version:      Version,
		APIKeys:      cfg.APIKeys,
		Logf:         logFn,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", app.HandleHealth)
	mux.HandleFunc("GET /health", app.HandleHealth)
	mux.HandleFunc("GET /v1/models", app.HandleModels)
	mux.HandleFunc("POST /v1/chat/completions", app.HandleChat)

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:           app.Handler(mux),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("chatgpt-api %s listening on %s", Version, srv.Addr)
		log.Printf("upstream=%s model=%s tls=%s", cfg.ChatGPTBase, cfg.DefaultModel, cfg.Impersonate)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("shutdown complete")
}

func applyEnv(cfg *config.Config) {
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("API_KEYS"); v != "" {
		cfg.APIKeys = splitList(v)
	}
	if v := os.Getenv("CHATGPT_BASE"); v != "" {
		cfg.ChatGPTBase = v
	}
	if v := os.Getenv("IMPERSONATE"); v != "" {
		cfg.Impersonate = v
	}
	if v := os.Getenv("SESSION_FILE"); v != "" {
		cfg.SessionFile = v
	}
	if v := os.Getenv("SESSIONS"); v != "" {
		cfg.Sessions = splitList(v)
	}
	if v := os.Getenv("CHATGPT_PROXIES"); v != "" {
		cfg.ChatGPTProxies = splitList(v)
	}
	if v := os.Getenv("DEFAULT_MODEL"); v != "" {
		cfg.ChatGPTDefaultModel = v
		cfg.DefaultModel = v
	}
	if v := os.Getenv("LOG_REQUESTS"); v != "" {
		cfg.LogRequests = v == "1" || strings.EqualFold(v, "true")
	}
}

func splitList(v string) []string {
	out := make([]string, 0, 4)
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
