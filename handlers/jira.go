package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/maersk/engineering-dashboard/jira"
	"github.com/maersk/engineering-dashboard/models"
)

// APIJiraEnabled returns whether Jira integration is configured
func (h *Handler) APIJiraEnabled(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	enabled := h.jiraClient != nil && h.jiraClient.IsConfigured()
	response := map[string]interface{}{
		"enabled": enabled,
	}

	if enabled {
		response["project_key"] = h.jiraClient.GetProjectKey()
		response["base_url"] = h.jiraClient.GetBaseURL()
	}

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// APIJiraEpics returns the list of open epics in the configured project
func (h *Handler) APIJiraEpics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.jiraClient == nil || !h.jiraClient.IsConfigured() {
		http.Error(w, "Jira not configured", http.StatusServiceUnavailable)
		return
	}

	epics, err := h.jiraClient.GetEpics()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch epics: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to models.JiraEpic for API response
	response := make([]models.JiraEpic, len(epics))
	for i, e := range epics {
		response[i] = models.JiraEpic{
			Key:     e.Key,
			Summary: e.Summary,
		}
	}

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// APIJiraCreateTicket creates a Jira ticket for a concern
func (h *Handler) APIJiraCreateTicket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.jiraClient == nil || !h.jiraClient.IsConfigured() {
		http.Error(w, "Jira not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.JiraTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Generate ticket summary and description based on concern type
	summary, description := h.generateTicketContent(req)

	// Create the issue
	createReq := jira.CreateIssueRequest{
		Summary:     summary,
		Description: description,
		EpicKey:     req.EpicKey,
	}

	result, err := h.jiraClient.CreateStory(createReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create ticket: %v", err), http.StatusInternalServerError)
		return
	}

	response := models.JiraTicketResponse{
		Key: result.Key,
		URL: result.URL,
	}

	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// generateTicketContent creates the summary and description for a Jira ticket
func (h *Handler) generateTicketContent(req models.JiraTicketRequest) (summary, description string) {
	repo := req.Repo
	details := req.Details

	switch req.ConcernType {
	case models.ConcernMissingSonarQube:
		summary = fmt.Sprintf("Add %s to SonarQube", repo)
		description = fmt.Sprintf(`## Summary
The repository %s is not configured in SonarQube and needs to be onboarded for code quality analysis.

## Tasks
- Configure SonarQube project for this repository
- Set up CI/CD pipeline integration
- Configure quality gates
- Verify initial scan completes successfully

## Links
- Repository: https://github.com/%s`, repo, repo)

	case models.ConcernOutdatedGo:
		currentVersion := getStringFromDetails(details, "current_version")
		latestVersion := getStringFromDetails(details, "latest_version")
		filePath := getStringFromDetails(details, "file_path")

		summary = fmt.Sprintf("Update Go from %s to %s in %s", currentVersion, latestVersion, repo)
		description = fmt.Sprintf(`## Summary
The Go version in this repository is outdated and should be updated to the latest version.

## Details
- Repository: %s
- File: %s
- Current Go version: %s
- Latest Go version: %s

## Tasks
- Update go.mod to use Go %s
- Run tests to ensure compatibility
- Update any CI/CD configurations if needed

## Links
- Repository: https://github.com/%s`, repo, filePath, currentVersion, latestVersion, latestVersion, repo)

	case models.ConcernOutdatedDependencies:
		count := getIntFromDetails(details, "count")
		deps := getStringFromDetails(details, "dependencies")

		summary = fmt.Sprintf("Update %d outdated dependencies in %s", count, repo)
		description = fmt.Sprintf(`## Summary
This repository has %d outdated Go dependencies that should be updated.

## Outdated Dependencies
%s

## Tasks
- Review each dependency update for breaking changes
- Update dependencies using go get -u
- Run tests to verify compatibility
- Update go.sum

## Links
- Repository: https://github.com/%s`, count, deps, repo)

	case models.ConcernDependabotAlerts:
		count := getIntFromDetails(details, "count")
		alerts := getStringFromDetails(details, "alerts")
		criticalCount := getIntFromDetails(details, "critical")
		highCount := getIntFromDetails(details, "high")

		severity := ""
		if criticalCount > 0 {
			severity = fmt.Sprintf(" (%d critical)", criticalCount)
		} else if highCount > 0 {
			severity = fmt.Sprintf(" (%d high)", highCount)
		}

		summary = fmt.Sprintf("Address %d Dependabot alerts%s in %s", count, severity, repo)
		description = fmt.Sprintf(`## Summary
This repository has %d open Dependabot security alerts that need to be addressed.

## Severity Breakdown
%s

## Tasks
- Review each alert and assess impact
- Update affected dependencies
- Test for regressions
- Dismiss any false positives with justification

## Links
- Repository: https://github.com/%s
- Security Alerts: https://github.com/%s/security/dependabot`, count, alerts, repo, repo)

	case models.ConcernCodeScanningAlerts:
		count := getIntFromDetails(details, "count")
		alerts := getStringFromDetails(details, "alerts")

		summary = fmt.Sprintf("Fix %d code scanning issues in %s", count, repo)
		description = fmt.Sprintf(`## Summary
This repository has %d open code scanning alerts that need to be fixed.

## Alerts
%s

## Tasks
- Review each alert and understand the vulnerability
- Implement fixes for each issue
- Run code scanning to verify fixes
- Dismiss any false positives with justification

## Links
- Repository: https://github.com/%s
- Code Scanning: https://github.com/%s/security/code-scanning`, count, alerts, repo, repo)

	case models.ConcernSecretScanningAlerts:
		count := getIntFromDetails(details, "count")
		alerts := getStringFromDetails(details, "alerts")

		summary = fmt.Sprintf("Remediate %d exposed secrets in %s", count, repo)
		description = fmt.Sprintf(`## Summary
This repository has %d secret scanning alerts indicating potentially exposed credentials.

## Exposed Secrets
%s

## Tasks
- Immediately rotate all exposed credentials
- Remove secrets from repository history if possible
- Implement proper secret management (environment variables, vault, etc.)
- Add pre-commit hooks to prevent future leaks

## Links
- Repository: https://github.com/%s
- Secret Scanning: https://github.com/%s/security/secret-scanning`, count, alerts, repo, repo)

	case models.ConcernSonarQubeVulnerabilities:
		vulnCount := getIntFromDetails(details, "vulnerabilities")
		hotspotCount := getIntFromDetails(details, "hotspots")
		projectKey := getStringFromDetails(details, "project_key")

		summary = fmt.Sprintf("Fix %d vulnerabilities and %d security hotspots in %s", vulnCount, hotspotCount, repo)
		description = fmt.Sprintf(`## Summary
SonarQube has identified security issues in this repository that need attention.

## Details
- Vulnerabilities: %d
- Security Hotspots: %d
- SonarQube Project: %s

## Tasks
- Review each vulnerability in SonarQube
- Implement fixes following SonarQube recommendations
- Review and resolve security hotspots
- Verify fixes with a new SonarQube scan

## Links
- Repository: https://github.com/%s`, vulnCount, hotspotCount, projectKey, repo)

	case models.ConcernQualityGateFailed:
		projectKey := getStringFromDetails(details, "project_key")
		conditions := getStringFromDetails(details, "conditions")

		summary = fmt.Sprintf("Fix quality gate failure in %s", repo)
		description = fmt.Sprintf(`## Summary
The SonarQube quality gate is failing for this repository.

## Failed Conditions
%s

## Details
- SonarQube Project: %s

## Tasks
- Review failing quality gate conditions
- Address each failing metric
- Re-run analysis to verify fixes
- Ensure quality gate passes before merging new code

## Links
- Repository: https://github.com/%s`, conditions, projectKey, repo)

	default:
		summary = fmt.Sprintf("Address issue in %s", repo)
		description = fmt.Sprintf("An issue was identified in repository %s that needs attention.", repo)
	}

	return summary, description
}

// Helper functions to extract values from details map
func getStringFromDetails(details map[string]interface{}, key string) string {
	if details == nil {
		return ""
	}
	if val, ok := details[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getIntFromDetails(details map[string]interface{}, key string) int {
	if details == nil {
		return 0
	}
	if val, ok := details[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case int64:
			return int(v)
		}
	}
	return 0
}

// FormatDependencyList formats a list of outdated dependencies for the ticket
func FormatDependencyList(deps []string, maxItems int) string {
	if len(deps) == 0 {
		return "No dependencies listed"
	}

	var sb strings.Builder
	shown := len(deps)
	if maxItems > 0 && shown > maxItems {
		shown = maxItems
	}

	for i := 0; i < shown; i++ {
		sb.WriteString(fmt.Sprintf("- %s\n", deps[i]))
	}

	if len(deps) > shown {
		sb.WriteString(fmt.Sprintf("\n... and %d more\n", len(deps)-shown))
	}

	return sb.String()
}

// FormatAlertList formats a list of security alerts for the ticket
func FormatAlertList(alerts []string, maxItems int) string {
	return FormatDependencyList(alerts, maxItems) // Same format
}
