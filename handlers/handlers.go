package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/maersk/engineering-dashboard/config"
	"github.com/maersk/engineering-dashboard/github"
	"github.com/maersk/engineering-dashboard/gomod"
	"github.com/maersk/engineering-dashboard/goproxy"
	"github.com/maersk/engineering-dashboard/jira"
	"github.com/maersk/engineering-dashboard/models"
	"github.com/maersk/engineering-dashboard/sonarqube"
)

type Handler struct {
	ghClient    *github.Client
	proxyClient *goproxy.Client
	gomodParser *gomod.Parser
	sqClient    *sonarqube.Client
	jiraClient  *jira.Client
	config      *config.Config
	templates   *template.Template
}

func NewHandler(ghClient *github.Client, sqClient *sonarqube.Client, jiraClient *jira.Client, cfg *config.Config, templatesDir string) (*Handler, error) {
	tmpl, err := template.ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return nil, err
	}

	proxyClient := goproxy.NewClient()
	gomodParser := gomod.NewParser(proxyClient)

	return &Handler{
		ghClient:    ghClient,
		proxyClient: proxyClient,
		gomodParser: gomodParser,
		sqClient:    sqClient,
		jiraClient:  jiraClient,
		config:      cfg,
		templates:   tmpl,
	}, nil
}

// Dashboard serves the main dashboard page with both security and dependency info
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "security"
	}

	data := struct {
		ActiveTab        string
		Security         models.DashboardMetrics
		Dependencies     models.DependencyMetrics
		CodeQuality      models.CodeQualityMetrics
		SonarQubeEnabled bool
		JiraEnabled      bool
		JiraBaseURL      string
	}{
		ActiveTab:        tab,
		SonarQubeEnabled: h.sqClient != nil && h.sqClient.IsConfigured(),
		JiraEnabled:      h.jiraClient != nil && h.jiraClient.IsConfigured(),
	}

	if data.JiraEnabled {
		data.JiraBaseURL = h.jiraClient.GetBaseURL()
	}

	// Always fetch security metrics for the overview
	data.Security = h.ghClient.GetDashboardMetrics(h.config.Repositories)

	// Fetch dependencies if on that tab
	if tab == "dependencies" {
		data.Dependencies = h.GetDependencyMetrics()
	}

	// Fetch code quality if on that tab and SonarQube is configured
	if tab == "codequality" && data.SonarQubeEnabled {
		data.CodeQuality = h.GetCodeQualityMetrics()
	}

	if err := h.templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// GetDependencyMetrics fetches and analyzes go.mod for all repositories
func (h *Handler) GetDependencyMetrics() models.DependencyMetrics {
	metrics := models.DependencyMetrics{
		TotalRepos:   len(h.config.Repositories),
		Repositories: make([]models.RepoDependencySummary, len(h.config.Repositories)),
	}

	// Get latest Go version once
	latestGo, _ := h.proxyClient.GetLatestGoVersion()
	metrics.LatestGoVersion = latestGo

	var wg sync.WaitGroup
	for i, repo := range h.config.Repositories {
		wg.Add(1)
		go func(idx int, r config.Repository) {
			defer wg.Done()
			metrics.Repositories[idx] = h.getRepoDependencySummary(r.Owner, r.Repo, latestGo)
		}(i, repo)
	}
	wg.Wait()

	// Aggregate metrics
	for _, repoSummary := range metrics.Repositories {
		if repoSummary.IsGoRepo {
			metrics.GoRepos++
		}
		if len(repoSummary.GoMods) > 0 {
			metrics.ReposWithGoMod++
			metrics.TotalGoModFiles += len(repoSummary.GoMods)

			for _, gm := range repoSummary.GoMods {
				metrics.TotalDependencies += gm.TotalDeps
				metrics.OutdatedDependencies += gm.OutdatedDeps
				if gm.GoVersionOutdated {
					metrics.OutdatedGoVersions++
				}
			}
		}
	}

	return metrics
}

func (h *Handler) getRepoDependencySummary(owner, repo, latestGo string) models.RepoDependencySummary {
	summary := models.RepoDependencySummary{
		Owner:    owner,
		Repo:     repo,
		FullName: fmt.Sprintf("%s/%s", owner, repo),
		GoMods:   []models.GoModInfo{},
	}

	// Check if this is a Go repository (using GitHub's language detection)
	isGo, err := h.ghClient.IsGoRepo(owner, repo)
	if err != nil {
		summary.Error = fmt.Sprintf("Failed to check repo languages: %v", err)
		return summary
	}
	summary.IsGoRepo = isGo

	// Only search for go.mod files if this is a Go repo
	if !isGo {
		return summary
	}

	// Find and fetch all go.mod files in the repository
	goModFiles, err := h.ghClient.GetAllGoModFiles(owner, repo)
	if err != nil {
		summary.Error = fmt.Sprintf("Failed to find go.mod files: %v", err)
		return summary
	}

	if len(goModFiles) == 0 {
		// Go repo but no go.mod found (might be using GOPATH or vendoring)
		return summary
	}

	// Parse each go.mod file
	for _, gmFile := range goModFiles {
		goModInfo := h.gomodParser.Parse(gmFile.Content)
		goModInfo.FilePath = gmFile.Path
		goModInfo.LatestGoVersion = latestGo

		// Check Go version
		if goModInfo.GoVersion != "" && latestGo != "" {
			goModInfo.GoVersionOutdated = isGoVersionOutdated(goModInfo.GoVersion, latestGo)
		}

		// Check for dependency updates
		h.gomodParser.CheckForUpdates(&goModInfo)

		summary.GoMods = append(summary.GoMods, goModInfo)
	}

	return summary
}

// isGoVersionOutdated checks if the current Go version is outdated
func isGoVersionOutdated(current, latest string) bool {
	// Simple check: compare major.minor versions
	// current might be "1.21" and latest might be "1.22.1"
	currentParts := parseGoVersion(current)
	latestParts := parseGoVersion(latest)

	if currentParts[0] < latestParts[0] {
		return true
	}
	if currentParts[0] == latestParts[0] && currentParts[1] < latestParts[1] {
		return true
	}
	return false
}

func parseGoVersion(v string) [2]int {
	var parts [2]int
	fmt.Sscanf(v, "%d.%d", &parts[0], &parts[1])
	return parts
}

// API Handlers

func (h *Handler) APIMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := h.ghClient.GetDashboardMetrics(h.config.Repositories)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (h *Handler) APIDependencies(w http.ResponseWriter, r *http.Request) {
	metrics := h.GetDependencyMetrics()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (h *Handler) APIRepo(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")

	if owner == "" || repo == "" {
		http.Error(w, "owner and repo query parameters are required", http.StatusBadRequest)
		return
	}

	summary := h.ghClient.GetRepoSecuritySummary(owner, repo)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (h *Handler) APIRepoDependencies(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")

	if owner == "" || repo == "" {
		http.Error(w, "owner and repo query parameters are required", http.StatusBadRequest)
		return
	}

	latestGo, _ := h.proxyClient.GetLatestGoVersion()
	summary := h.getRepoDependencySummary(owner, repo, latestGo)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// GetCodeQualityMetrics fetches SonarQube metrics for all repositories
func (h *Handler) GetCodeQualityMetrics() models.CodeQualityMetrics {
	metrics := models.CodeQualityMetrics{
		TotalRepos:   len(h.config.Repositories),
		Repositories: make([]models.RepoCodeQualitySummary, len(h.config.Repositories)),
	}

	if h.sqClient == nil || !h.sqClient.IsConfigured() {
		return metrics
	}

	var wg sync.WaitGroup
	var coverageSum float64
	var coverageCount int
	var mu sync.Mutex

	for i, repo := range h.config.Repositories {
		wg.Add(1)
		go func(idx int, r config.Repository) {
			defer wg.Done()

			summary := models.RepoCodeQualitySummary{
				Owner:            r.Owner,
				Repo:             r.Repo,
				FullName:         fmt.Sprintf("%s/%s", r.Owner, r.Repo),
				SonarQubeProject: r.SonarQubeProjectKey(),
			}

			// Fetch metrics from SonarQube
			sqMetrics := h.sqClient.GetProjectMetrics(summary.SonarQubeProject)

			summary.Bugs = sqMetrics.Bugs
			summary.Vulnerabilities = sqMetrics.Vulnerabilities
			summary.CodeSmells = sqMetrics.CodeSmells
			summary.SecurityHotspots = sqMetrics.SecurityHotspots
			summary.Coverage = sqMetrics.Coverage
			summary.DuplicatedLines = sqMetrics.DuplicatedLines
			summary.LinesOfCode = sqMetrics.LinesOfCode
			summary.ReliabilityRating = sqMetrics.ReliabilityRating
			summary.SecurityRating = sqMetrics.SecurityRating
			summary.MaintainabilityRating = sqMetrics.MaintainabilityRating
			summary.QualityGateStatus = sqMetrics.QualityGateStatus
			summary.LastAnalysis = sqMetrics.LastAnalysis
			summary.Error = sqMetrics.Error

			metrics.Repositories[idx] = summary

			// Aggregate metrics (thread-safe)
			mu.Lock()
			if sqMetrics.Error == "" {
				metrics.AnalyzedRepos++
				metrics.TotalBugs += sqMetrics.Bugs
				metrics.TotalVulnerabilities += sqMetrics.Vulnerabilities
				metrics.TotalCodeSmells += sqMetrics.CodeSmells
				metrics.TotalSecurityHotspots += sqMetrics.SecurityHotspots

				if sqMetrics.QualityGateStatus == "OK" {
					metrics.QualityGatePassed++
				} else if sqMetrics.QualityGateStatus != "" {
					metrics.QualityGateFailed++
				}

				if sqMetrics.Coverage > 0 {
					coverageSum += sqMetrics.Coverage
					coverageCount++
				}
			}
			mu.Unlock()
		}(i, repo)
	}
	wg.Wait()

	// Calculate average coverage
	if coverageCount > 0 {
		metrics.AverageCoverage = coverageSum / float64(coverageCount)
	}

	metrics.ConfiguredRepos = metrics.TotalRepos

	return metrics
}

// APICodeQuality returns code quality metrics as JSON
func (h *Handler) APICodeQuality(w http.ResponseWriter, r *http.Request) {
	if h.sqClient == nil || !h.sqClient.IsConfigured() {
		http.Error(w, "SonarQube not configured", http.StatusServiceUnavailable)
		return
	}

	metrics := h.GetCodeQualityMetrics()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// APIRepoCodeQuality returns code quality metrics for a specific repository
func (h *Handler) APIRepoCodeQuality(w http.ResponseWriter, r *http.Request) {
	if h.sqClient == nil || !h.sqClient.IsConfigured() {
		http.Error(w, "SonarQube not configured", http.StatusServiceUnavailable)
		return
	}

	projectKey := r.URL.Query().Get("project")
	if projectKey == "" {
		// Try to get from owner/repo params
		owner := r.URL.Query().Get("owner")
		repo := r.URL.Query().Get("repo")
		if repo == "" {
			http.Error(w, "project or repo query parameter is required", http.StatusBadRequest)
			return
		}
		// Find the repo config to get the correct project key
		for _, repoConfig := range h.config.Repositories {
			if repoConfig.Owner == owner && repoConfig.Repo == repo {
				projectKey = repoConfig.SonarQubeProjectKey()
				break
			}
		}
		if projectKey == "" {
			projectKey = repo
		}
	}

	metrics := h.sqClient.GetProjectMetrics(projectKey)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}
