package sonarqube

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client handles communication with SonarQube API
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new SonarQube API client
// baseURL should be the SonarQube server URL (e.g., "https://sonarqube.example.com")
// token is the SonarQube API token
func NewClient(baseURL, token string) *Client {
	// Remove trailing slash from baseURL
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured returns true if the client has valid configuration
func (c *Client) IsConfigured() bool {
	return c.baseURL != "" && c.token != ""
}

// ProjectMetrics represents the quality metrics for a SonarQube project
type ProjectMetrics struct {
	ProjectKey       string  `json:"project_key"`
	ProjectName      string  `json:"project_name"`
	Bugs             int     `json:"bugs"`
	Vulnerabilities  int     `json:"vulnerabilities"`
	CodeSmells       int     `json:"code_smells"`
	SecurityHotspots int     `json:"security_hotspots"`
	Coverage         float64 `json:"coverage"`
	DuplicatedLines  float64 `json:"duplicated_lines_density"`
	LinesOfCode      int     `json:"lines_of_code"`
	// Ratings: A=1, B=2, C=3, D=4, E=5
	ReliabilityRating     string `json:"reliability_rating"`
	SecurityRating        string `json:"security_rating"`
	MaintainabilityRating string `json:"maintainability_rating"`
	// Quality gate status
	QualityGateStatus string `json:"quality_gate_status"`
	// Last analysis date
	LastAnalysis string `json:"last_analysis"`
	// Error if project not found or API error
	Error string `json:"error,omitempty"`
}

// API response structures
type measuresResponse struct {
	Component struct {
		Key      string    `json:"key"`
		Name     string    `json:"name"`
		Measures []measure `json:"measures"`
	} `json:"component"`
}

type measure struct {
	Metric string `json:"metric"`
	Value  string `json:"value"`
}

type projectStatusResponse struct {
	ProjectStatus struct {
		Status string `json:"status"`
	} `json:"projectStatus"`
}

type analysisResponse struct {
	Analyses []struct {
		Key  string `json:"key"`
		Date string `json:"date"`
	} `json:"analyses"`
}

// GetProjectMetrics fetches quality metrics for a SonarQube project
func (c *Client) GetProjectMetrics(projectKey string) ProjectMetrics {
	metrics := ProjectMetrics{
		ProjectKey: projectKey,
	}

	if !c.IsConfigured() {
		metrics.Error = "SonarQube not configured"
		return metrics
	}

	// Metrics to fetch
	metricKeys := []string{
		"bugs",
		"vulnerabilities",
		"code_smells",
		"security_hotspots",
		"coverage",
		"duplicated_lines_density",
		"ncloc",
		"reliability_rating",
		"security_rating",
		"sqale_rating", // maintainability rating
	}

	// Fetch measures
	measuresURL := fmt.Sprintf("%s/api/measures/component?component=%s&metricKeys=%s",
		c.baseURL,
		url.QueryEscape(projectKey),
		strings.Join(metricKeys, ","),
	)

	resp, err := c.doRequest(measuresURL)
	if err != nil {
		metrics.Error = fmt.Sprintf("Failed to fetch measures: %v", err)
		return metrics
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		metrics.Error = "Project not found in SonarQube"
		return metrics
	}

	if resp.StatusCode != http.StatusOK {
		metrics.Error = fmt.Sprintf("SonarQube API returned status %d", resp.StatusCode)
		return metrics
	}

	var measuresResp measuresResponse
	if err := json.NewDecoder(resp.Body).Decode(&measuresResp); err != nil {
		metrics.Error = fmt.Sprintf("Failed to parse measures response: %v", err)
		return metrics
	}

	metrics.ProjectName = measuresResp.Component.Name
	if metrics.ProjectName == "" {
		metrics.ProjectName = projectKey
	}

	// Parse measures
	for _, m := range measuresResp.Component.Measures {
		switch m.Metric {
		case "bugs":
			fmt.Sscanf(m.Value, "%d", &metrics.Bugs)
		case "vulnerabilities":
			fmt.Sscanf(m.Value, "%d", &metrics.Vulnerabilities)
		case "code_smells":
			fmt.Sscanf(m.Value, "%d", &metrics.CodeSmells)
		case "security_hotspots":
			fmt.Sscanf(m.Value, "%d", &metrics.SecurityHotspots)
		case "coverage":
			fmt.Sscanf(m.Value, "%f", &metrics.Coverage)
		case "duplicated_lines_density":
			fmt.Sscanf(m.Value, "%f", &metrics.DuplicatedLines)
		case "ncloc":
			fmt.Sscanf(m.Value, "%d", &metrics.LinesOfCode)
		case "reliability_rating":
			metrics.ReliabilityRating = ratingToLetter(m.Value)
		case "security_rating":
			metrics.SecurityRating = ratingToLetter(m.Value)
		case "sqale_rating":
			metrics.MaintainabilityRating = ratingToLetter(m.Value)
		}
	}

	// Fetch quality gate status
	c.fetchQualityGateStatus(&metrics, projectKey)

	// Fetch last analysis date
	c.fetchLastAnalysis(&metrics, projectKey)

	return metrics
}

func (c *Client) fetchQualityGateStatus(metrics *ProjectMetrics, projectKey string) {
	statusURL := fmt.Sprintf("%s/api/qualitygates/project_status?projectKey=%s",
		c.baseURL,
		url.QueryEscape(projectKey),
	)

	resp, err := c.doRequest(statusURL)
	if err != nil {
		return // Non-critical, don't set error
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var statusResp projectStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return
	}

	metrics.QualityGateStatus = statusResp.ProjectStatus.Status
}

func (c *Client) fetchLastAnalysis(metrics *ProjectMetrics, projectKey string) {
	analysisURL := fmt.Sprintf("%s/api/project_analyses/search?project=%s&ps=1",
		c.baseURL,
		url.QueryEscape(projectKey),
	)

	resp, err := c.doRequest(analysisURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var analysisResp analysisResponse
	if err := json.NewDecoder(resp.Body).Decode(&analysisResp); err != nil {
		return
	}

	if len(analysisResp.Analyses) > 0 {
		metrics.LastAnalysis = analysisResp.Analyses[0].Date
	}
}

func (c *Client) doRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// SonarQube uses basic auth with token as username and empty password
	req.SetBasicAuth(c.token, "")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// ratingToLetter converts SonarQube numeric rating to letter grade
func ratingToLetter(value string) string {
	switch value {
	case "1.0", "1":
		return "A"
	case "2.0", "2":
		return "B"
	case "3.0", "3":
		return "C"
	case "4.0", "4":
		return "D"
	case "5.0", "5":
		return "E"
	default:
		return value
	}
}
