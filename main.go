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

	// Create handler
	handler, err := handlers.NewHandler(client, sqClient, jiraClient, cfg, *templatesDir)
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
	http.HandleFunc("/api/jira/enabled", handler.APIJiraEnabled)
	http.HandleFunc("/api/jira/epics", handler.APIJiraEpics)
	http.HandleFunc("/api/jira/ticket", handler.APIJiraCreateTicket)
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
