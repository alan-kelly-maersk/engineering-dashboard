package github

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/maersk/engineering-dashboard/config"
	"github.com/maersk/engineering-dashboard/models"
)

const (
	baseURL = "https://api.github.com"
)

type Client struct {
	httpClient *http.Client
	token      string
}

func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token: token,
	}
}

func (c *Client) doRequest(url string, result interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Println("Failed to close response body", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) GetDependabotAlerts(owner, repo string) ([]models.DependabotAlert, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/dependabot/alerts?state=open&per_page=100", baseURL, owner, repo)
	var alerts []models.DependabotAlert
	if err := c.doRequest(url, &alerts); err != nil {
		return nil, err
	}
	return alerts, nil
}

func (c *Client) GetCodeScanningAlerts(owner, repo string) ([]models.CodeScanningAlert, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/code-scanning/alerts?state=open&per_page=100", baseURL, owner, repo)
	var alerts []models.CodeScanningAlert
	if err := c.doRequest(url, &alerts); err != nil {
		return nil, err
	}
	return alerts, nil
}

func (c *Client) GetSecretScanningAlerts(owner, repo string) ([]models.SecretScanningAlert, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/secret-scanning/alerts?state=open&per_page=100", baseURL, owner, repo)
	var alerts []models.SecretScanningAlert
	if err := c.doRequest(url, &alerts); err != nil {
		return nil, err
	}
	return alerts, nil
}

// FileContent represents the GitHub API response for file contents
type FileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
	SHA      string `json:"sha"`
}

// GetFileContent fetches a file's content from a repository
func (c *Client) GetFileContent(owner, repo, path string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s", baseURL, owner, repo, path)

	var fileContent FileContent
	if err := c.doRequest(url, &fileContent); err != nil {
		return "", err
	}

	if fileContent.Encoding != "base64" {
		return "", fmt.Errorf("unexpected encoding: %s", fileContent.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(fileContent.Content)
	if err != nil {
		return "", fmt.Errorf("failed to decode content: %w", err)
	}

	return string(decoded), nil
}

// RepoInfo represents basic repository information
type RepoInfo struct {
	DefaultBranch string `json:"default_branch"`
}

// TreeResponse represents the Git Trees API response
type TreeResponse struct {
	SHA       string     `json:"sha"`
	Tree      []TreeItem `json:"tree"`
	Truncated bool       `json:"truncated"`
}

// TreeItem represents a single item in the tree
type TreeItem struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"` // "blob" for files, "tree" for directories
	SHA  string `json:"sha"`
	Size int    `json:"size,omitempty"`
}

// GetRepoLanguages returns the languages used in a repository
// Returns a map of language name to bytes of code
func (c *Client) GetRepoLanguages(owner, repo string) (map[string]int, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/languages", baseURL, owner, repo)
	var languages map[string]int
	if err := c.doRequest(url, &languages); err != nil {
		return nil, err
	}
	return languages, nil
}

// IsGoRepo checks if a repository contains Go code
func (c *Client) IsGoRepo(owner, repo string) (bool, error) {
	languages, err := c.GetRepoLanguages(owner, repo)
	if err != nil {
		return false, err
	}
	_, hasGo := languages["Go"]
	return hasGo, nil
}

// GetRepoInfo fetches basic repository information
func (c *Client) GetRepoInfo(owner, repo string) (*RepoInfo, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", baseURL, owner, repo)
	var info RepoInfo
	if err := c.doRequest(url, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// FindGoModFiles searches for all go.mod files in a repository
func (c *Client) FindGoModFiles(owner, repo string) ([]string, error) {
	// First get the default branch
	repoInfo, err := c.GetRepoInfo(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repo info: %w", err)
	}

	// Get the full tree recursively
	url := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", baseURL, owner, repo, repoInfo.DefaultBranch)
	var tree TreeResponse
	if err := c.doRequest(url, &tree); err != nil {
		return nil, fmt.Errorf("failed to get repo tree: %w", err)
	}

	// Find all go.mod files
	var goModPaths []string
	for _, item := range tree.Tree {
		if item.Type == "blob" && (item.Path == "go.mod" || strings.HasSuffix(item.Path, "/go.mod")) {
			goModPaths = append(goModPaths, item.Path)
		}
	}

	return goModPaths, nil
}

// GoModFile represents a go.mod file with its path and content
type GoModFile struct {
	Path    string
	Content string
}

// GetAllGoModFiles finds and fetches all go.mod files in a repository
// Only searches if the repository is detected as a Go repository
func (c *Client) GetAllGoModFiles(owner, repo string) ([]GoModFile, error) {
	// First check if this is a Go repository
	isGo, err := c.IsGoRepo(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to check repo languages: %w", err)
	}
	if !isGo {
		return nil, nil // Not a Go repo, return empty
	}

	// Find all go.mod files
	paths, err := c.FindGoModFiles(owner, repo)
	if err != nil {
		return nil, err
	}

	if len(paths) == 0 {
		return nil, nil
	}

	// Fetch content for each go.mod file concurrently
	results := make([]GoModFile, len(paths))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var fetchErrors []string

	for i, path := range paths {
		wg.Add(1)
		go func(idx int, p string) {
			defer wg.Done()
			content, err := c.GetFileContent(owner, repo, p)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fetchErrors = append(fetchErrors, fmt.Sprintf("%s: %v", p, err))
			} else {
				results[idx] = GoModFile{Path: p, Content: content}
			}
		}(i, path)
	}
	wg.Wait()

	// Filter out empty results (failed fetches)
	var validResults []GoModFile
	for _, r := range results {
		if r.Path != "" {
			validResults = append(validResults, r)
		}
	}

	return validResults, nil
}

func (c *Client) GetRepoSecuritySummary(owner, repo string) models.RepoSecuritySummary {
	summary := models.RepoSecuritySummary{
		Owner:    owner,
		Repo:     repo,
		FullName: fmt.Sprintf("%s/%s", owner, repo),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []string

	wg.Add(3)

	go func() {
		defer wg.Done()
		alerts, err := c.GetDependabotAlerts(owner, repo)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errors = append(errors, fmt.Sprintf("dependabot: %v", err))
		} else {
			summary.DependabotAlerts = alerts
		}
	}()

	go func() {
		defer wg.Done()
		alerts, err := c.GetCodeScanningAlerts(owner, repo)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errors = append(errors, fmt.Sprintf("code-scanning: %v", err))
		} else {
			summary.CodeScanningAlerts = alerts
		}
	}()

	go func() {
		defer wg.Done()
		alerts, err := c.GetSecretScanningAlerts(owner, repo)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errors = append(errors, fmt.Sprintf("secret-scanning: %v", err))
		} else {
			summary.SecretScanningAlerts = alerts
		}
	}()

	wg.Wait()

	if len(errors) > 0 {
		summary.Error = fmt.Sprintf("%v", errors)
	}

	return summary
}

func (c *Client) GetDashboardMetrics(repos []config.Repository) models.DashboardMetrics {
	metrics := models.DashboardMetrics{
		TotalRepos:             len(repos),
		DependabotBySeverity:   make(map[string]int),
		CodeScanningBySeverity: make(map[string]int),
		Repositories:           make([]models.RepoSecuritySummary, len(repos)),
	}

	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r config.Repository) {
			defer wg.Done()
			metrics.Repositories[idx] = c.GetRepoSecuritySummary(r.Owner, r.Repo)
		}(i, repo)
	}
	wg.Wait()

	// Aggregate metrics
	for _, repoSummary := range metrics.Repositories {
		metrics.TotalDependabotAlerts += len(repoSummary.DependabotAlerts)
		metrics.TotalCodeScanningAlerts += len(repoSummary.CodeScanningAlerts)
		metrics.TotalSecretAlerts += len(repoSummary.SecretScanningAlerts)

		for _, alert := range repoSummary.DependabotAlerts {
			severity := alert.SecurityAdvisory.Severity
			if severity == "" {
				severity = alert.SecurityVuln.Severity
			}
			if severity == "" {
				severity = "unknown"
			}
			metrics.DependabotBySeverity[severity]++
		}

		for _, alert := range repoSummary.CodeScanningAlerts {
			severity := alert.Rule.SecuritySeverityLevel
			if severity == "" {
				severity = alert.Rule.Severity
			}
			if severity == "" {
				severity = "unknown"
			}
			metrics.CodeScanningBySeverity[severity]++
		}
	}

	return metrics
}
