package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/maersk/security-dashboard/config"
	"github.com/maersk/security-dashboard/github"
	"github.com/maersk/security-dashboard/handlers"
)

func main() {
	port := flag.String("port", "8080", "Server port")
	configPath := flag.String("config", "repos.yaml", "Path to repositories config file")
	templatesDir := flag.String("templates", "templates", "Path to templates directory")
	flag.Parse()

	// Get GitHub token from environment
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded %d repositories from config", len(cfg.Repositories))

	// Create GitHub client
	client := github.NewClient(token)

	// Create handler
	handler, err := handlers.NewHandler(client, cfg, *templatesDir)
	if err != nil {
		log.Fatalf("Failed to create handler: %v", err)
	}

	// Setup routes
	http.HandleFunc("/", handler.Dashboard)
	http.HandleFunc("/api/metrics", handler.APIMetrics)
	http.HandleFunc("/api/dependencies", handler.APIDependencies)
	http.HandleFunc("/api/repo", handler.APIRepo)
	http.HandleFunc("/api/repo/dependencies", handler.APIRepoDependencies)
	http.HandleFunc("/health", handler.Health)

	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	addr := fmt.Sprintf(":%s", *port)
	log.Printf("Starting server on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
