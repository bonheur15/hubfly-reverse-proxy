package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/hubfly/hubfly-reverse-proxy/internal/api"
	"github.com/hubfly/hubfly-reverse-proxy/internal/certbot"
	"github.com/hubfly/hubfly-reverse-proxy/internal/nginx"
	"github.com/hubfly/hubfly-reverse-proxy/internal/store"
)

func main() {
	configDir := flag.String("config-dir", "/etc/hubfly", "Directory for config and data")
	port := flag.String("port", "8080", "API listening port")
	flag.Parse()

	// Ensure config dir exists
	if err := os.MkdirAll(*configDir, 0755); err != nil {
		log.Fatalf("Failed to create config dir: %v", err)
	}

	// Initialize Store
	st, err := store.NewJSONStore(*configDir)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}

	// Initialize Nginx Manager
	nm := nginx.NewManager(*configDir)
	if err := nm.EnsureDirs(); err != nil {
		log.Fatalf("Failed to create nginx dirs: %v", err)
	}

	// Initialize Certbot Manager
	// We assume webroot at /var/www/hubfly as per design
	cm := certbot.NewManager("/var/www/hubfly", "admin@example.com")
	// In real app, email should be configurable per site or global config

	// Initialize API Server
	srv := api.NewServer(st, nm, cm)

	log.Printf("Hubfly API starting on :%s...", *port)
	log.Printf("Config Dir: %s", *configDir)

	if err := http.ListenAndServe(":"+*port, srv.Routes()); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
