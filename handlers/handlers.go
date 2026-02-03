package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/maersk/security-dashboard/config"
	"github.com/maersk/security-dashboard/github"
	"github.com/maersk/security-dashboard/gomod"
	"github.com/maersk/security-dashboard/goproxy"
	"github.com/maersk/security-dashboard/models"
)

type Handler struct {
	ghClient    *github.Client
	proxyClient *goproxy.Client
	gomodParser *gomod.Parser
	config      *config.Config
	templates   *template.Template
}

func NewHandler(ghClient *github.Client, cfg *config.Config, templatesDir string) (*Handler, error) {
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
		ActiveTab    string
		Security     models.DashboardMetrics
		Dependencies models.DependencyMetrics
	}{
		ActiveTab: tab,
	}

	// Always fetch security metrics for the overview
	data.Security = h.ghClient.GetDashboardMetrics(h.config.Repositories)

	// Fetch dependencies if on that tab
	if tab == "dependencies" {
		data.Dependencies = h.GetDependencyMetrics()
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
