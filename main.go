package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/maersk/engineering-dashboard/config"
	"github.com/maersk/engineering-dashboard/github"
	"github.com/maersk/engineering-dashboard/handlers"
	"github.com/maersk/engineering-dashboard/sonarqube"
)

func main() {
	port := flag.String("port", "", "Server port (overrides PORT env var)")
	configPath := flag.String("config", "repos.yaml", "Path to repositories config file")
	templatesDir := flag.String("templates", "templates", "Path to templates directory")
	flag.Parse()

	// Determine port: CLI flag > PORT env var > default
	serverPort := *port
	if serverPort == "" {
		serverPort = os.Getenv("PORT")
	}
	if serverPort == "" {
		serverPort = "8080"
	}

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

	// Create SonarQube client (optional)
	var sqClient *sonarqube.Client
	sonarToken := os.Getenv("SONARQUBE_TOKEN")
	sonarURL := os.Getenv("SONARQUBE_URL")
	if sonarToken != "" && sonarURL != "" {
		sqClient = sonarqube.NewClient(sonarURL, sonarToken)
		log.Printf("SonarQube integration enabled (URL: %s)", sonarURL)
	} else {
		log.Printf("SonarQube integration disabled (set SONARQUBE_URL and SONARQUBE_TOKEN to enable)")
	}

	// Create handler
	handler, err := handlers.NewHandler(client, sqClient, cfg, *templatesDir)
	if err != nil {
		log.Fatalf("Failed to create handler: %v", err)
	}

	// Setup routes
	http.HandleFunc("/", handler.Dashboard)
	http.HandleFunc("/api/metrics", handler.APIMetrics)
	http.HandleFunc("/api/dependencies", handler.APIDependencies)
	http.HandleFunc("/api/repo", handler.APIRepo)
	http.HandleFunc("/api/repo/dependencies", handler.APIRepoDependencies)
	http.HandleFunc("/api/codequality", handler.APICodeQuality)
	http.HandleFunc("/api/repo/codequality", handler.APIRepoCodeQuality)
	http.HandleFunc("/health", handler.Health)

	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	addr := fmt.Sprintf(":%s", serverPort)
	log.Printf("Starting server on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
