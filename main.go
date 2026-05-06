package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/maersk/engineering-dashboard/auth"
	"github.com/maersk/engineering-dashboard/config"
	"github.com/maersk/engineering-dashboard/github"
	"github.com/maersk/engineering-dashboard/handlers"
	"github.com/maersk/engineering-dashboard/jira"
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
	// override with default
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

	// Create Jira client (optional)
	var jiraClient *jira.Client
	jiraURL := os.Getenv("ATLASSIAN_URL")
	jiraCloudID := os.Getenv("ATLASSIAN_CLOUD_ID")
	jiraEmail := os.Getenv("ATLASSIAN_EMAIL")
	jiraToken := os.Getenv("ATLASSIAN_API_TOKEN")
	jiraProject := os.Getenv("JIRA_PROJECT_KEY")
	jiraEpicField := os.Getenv("JIRA_EPIC_LINK_FIELD")
	if jiraEpicField == "" {
		jiraEpicField = "customfield_10002" // Default for classic Jira projects
	}

	if jiraURL != "" && jiraCloudID != "" && jiraEmail != "" && jiraToken != "" && jiraProject != "" {
		jiraClient = jira.NewClient(jiraURL, jiraCloudID, jiraEmail, jiraToken, jiraProject, jiraEpicField)
		log.Printf("Jira integration enabled (URL: %s, Cloud ID: %s, Project: %s)", jiraURL, jiraCloudID, jiraProject)
	} else {
		log.Printf("Jira integration disabled (set ATLASSIAN_URL, ATLASSIAN_CLOUD_ID, ATLASSIAN_EMAIL, ATLASSIAN_API_TOKEN, and JIRA_PROJECT_KEY to enable)")
	}

	// Setup Azure AD authentication (optional)
	var sessionKey []byte
	if secret := os.Getenv("SESSION_SECRET"); secret != "" {
		sessionKey, err = base64.StdEncoding.DecodeString(secret)
		if err != nil {
			// If it's not base64, use the raw string as the key
			sessionKey = []byte(secret)
		}
	}

	authHandler := auth.NewHandler(auth.Config{
		ClientID:     os.Getenv("AZURE_CLIENT_ID"),
		ClientSecret: os.Getenv("AZURE_CLIENT_SECRET"),
		TenantID:     os.Getenv("AZURE_TENANT_ID"),
		RedirectURL:  os.Getenv("AZURE_REDIRECT_URL"),
		SessionKey:   sessionKey,
	})

	// Create handler
	handler, err := handlers.NewHandler(client, sqClient, jiraClient, cfg, *templatesDir, authHandler.IsEnabled())
	if err != nil {
		log.Fatalf("Failed to create handler: %v", err)
	}

	// Auth routes (always registered, handlers check if auth is enabled)
	http.HandleFunc("/auth/login", authHandler.Login)
	http.HandleFunc("/auth/callback", authHandler.Callback)
	http.HandleFunc("/auth/logout", authHandler.Logout)

	// Protected routes (middleware redirects to login if auth is enabled)
	http.HandleFunc("/", authHandler.RequireAuth(handler.Dashboard))
	http.HandleFunc("/api/metrics", authHandler.RequireAuth(handler.APIMetrics))
	http.HandleFunc("/api/dependencies", authHandler.RequireAuth(handler.APIDependencies))
	http.HandleFunc("/api/repo", authHandler.RequireAuth(handler.APIRepo))
	http.HandleFunc("/api/repo/dependencies", authHandler.RequireAuth(handler.APIRepoDependencies))
	http.HandleFunc("/api/codequality", authHandler.RequireAuth(handler.APICodeQuality))
	http.HandleFunc("/api/repo/codequality", authHandler.RequireAuth(handler.APIRepoCodeQuality))
	http.HandleFunc("/api/jira/enabled", authHandler.RequireAuth(handler.APIJiraEnabled))
	http.HandleFunc("/api/jira/epics", authHandler.RequireAuth(handler.APIJiraEpics))
	http.HandleFunc("/api/jira/ticket", authHandler.RequireAuth(handler.APIJiraCreateTicket))

	// Public routes (no auth required)
	http.HandleFunc("/health", handler.Health)

	// Serve static files (public, needed for login page styling if any)
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	addr := fmt.Sprintf(":%s", serverPort)
	log.Printf("Starting server on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
