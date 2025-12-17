package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/hubfly/hubfly-reverse-proxy/internal/api"
	"github.com/hubfly/hubfly-reverse-proxy/internal/certbot"
	"github.com/hubfly/hubfly-reverse-proxy/internal/nginx"
	"github.com/hubfly/hubfly-reverse-proxy/internal/store"
)

func main() {
	// Setup structured logging
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(logger)

	configDir := flag.String("config-dir", "/etc/hubfly", "Directory for config and data")
	port := flag.String("port", "81", "API listening port")
	flag.Parse()

	slog.Info("Initializing Hubfly...", "config_dir", *configDir, "port", *port)

	// Ensure config dir exists
	if err := os.MkdirAll(*configDir, 0755); err != nil {
		slog.Error("Failed to create config dir", "error", err)
		os.Exit(1)
	}

	// Initialize Store
	st, err := store.NewJSONStore(*configDir)
	if err != nil {
		slog.Error("Failed to initialize store", "error", err)
		os.Exit(1)
	}

	// Initialize Nginx Manager
	nm := nginx.NewManager(*configDir)
	if err := nm.EnsureDirs(); err != nil {
		slog.Error("Failed to create nginx dirs", "error", err)
		os.Exit(1)
	}

	// Initialize Certbot Manager
	// We assume webroot at /var/www/hubfly as per design
	cm := certbot.NewManager("/var/www/hubfly", "cert-support@hubfly.app")

	// Initialize API Server
	srv := api.NewServer(st, nm, cm)

	slog.Info("Hubfly API starting", "address", ":"+*port)

	if err := http.ListenAndServe(":"+*port, srv.Routes()); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
