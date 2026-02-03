package models

import "time"

// DependabotAlert represents a Dependabot security alert
type DependabotAlert struct {
	Number           int       `json:"number"`
	State            string    `json:"state"`
	Severity         string    `json:"severity"`
	SecurityAdvisory Advisory  `json:"security_advisory"`
	SecurityVuln     Vuln      `json:"security_vulnerability"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	DismissedAt      time.Time `json:"dismissed_at,omitempty"`
	DismissedReason  string    `json:"dismissed_reason,omitempty"`
	HTMLURL          string    `json:"html_url"`
}

type Advisory struct {
	GHSAID      string `json:"ghsa_id"`
	CVEID       string `json:"cve_id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

type Vuln struct {
	Package         Package `json:"package"`
	Severity        string  `json:"severity"`
	VulnVersions    string  `json:"vulnerable_version_range"`
	FirstPatchedVer *struct {
		Identifier string `json:"identifier"`
	} `json:"first_patched_version"`
}

type Package struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
}

// CodeScanningAlert represents a code scanning alert
type CodeScanningAlert struct {
	Number             int       `json:"number"`
	State              string    `json:"state"`
	Rule               Rule      `json:"rule"`
	Tool               Tool      `json:"tool"`
	MostRecentInstance Instance  `json:"most_recent_instance"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	HTMLURL            string    `json:"html_url"`
}

type Rule struct {
	ID                    string   `json:"id"`
	Severity              string   `json:"severity"`
	SecuritySeverityLevel string   `json:"security_severity_level"`
	Description           string   `json:"description"`
	Tags                  []string `json:"tags"`
}

type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Instance struct {
	Ref      string   `json:"ref"`
	State    string   `json:"state"`
	Location Location `json:"location"`
}

type Location struct {
	Path        string `json:"path"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	StartColumn int    `json:"start_column"`
	EndColumn   int    `json:"end_column"`
}

// SecretScanningAlert represents a secret scanning alert
type SecretScanningAlert struct {
	Number         int       `json:"number"`
	State          string    `json:"state"`
	SecretType     string    `json:"secret_type"`
	SecretTypeDesc string    `json:"secret_type_display_name"`
	Secret         string    `json:"secret"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	HTMLURL        string    `json:"html_url"`
	Resolution     string    `json:"resolution,omitempty"`
}

// RepoSecuritySummary holds all security data for a repository
type RepoSecuritySummary struct {
	Owner                string
	Repo                 string
	FullName             string
	DependabotAlerts     []DependabotAlert
	CodeScanningAlerts   []CodeScanningAlert
	SecretScanningAlerts []SecretScanningAlert
	Error                string
}

// DashboardMetrics holds aggregated metrics for the dashboard
type DashboardMetrics struct {
	TotalRepos              int
	TotalDependabotAlerts   int
	TotalCodeScanningAlerts int
	TotalSecretAlerts       int

	// Severity breakdowns
	DependabotBySeverity   map[string]int
	CodeScanningBySeverity map[string]int

	// Per-repo summaries
	Repositories []RepoSecuritySummary
}

// Dependency represents a Go module dependency
type Dependency struct {
	Path           string `json:"path"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	IsOutdated     bool   `json:"is_outdated"`
	IsIndirect     bool   `json:"is_indirect"`
	Error          string `json:"error,omitempty"`
}

// GoModInfo holds parsed go.mod information
type GoModInfo struct {
	FilePath          string       `json:"file_path"` // Path to go.mod file in repo
	GoVersion         string       `json:"go_version"`
	LatestGoVersion   string       `json:"latest_go_version"`
	GoVersionOutdated bool         `json:"go_version_outdated"`
	ModulePath        string       `json:"module_path"`
	Dependencies      []Dependency `json:"dependencies"`
	TotalDeps         int          `json:"total_deps"`
	OutdatedDeps      int          `json:"outdated_deps"`
	DirectDeps        int          `json:"direct_deps"`
	IndirectDeps      int          `json:"indirect_deps"`
	Error             string       `json:"error,omitempty"`
}

// RepoDependencySummary holds dependency info for a repository
type RepoDependencySummary struct {
	Owner    string      `json:"owner"`
	Repo     string      `json:"repo"`
	FullName string      `json:"full_name"`
	IsGoRepo bool        `json:"is_go_repo"` // Whether GitHub detects Go in this repo
	GoMods   []GoModInfo `json:"go_mods"`    // All go.mod files found (supports monorepos)
	Error    string      `json:"error,omitempty"`
}

// Helper methods for RepoDependencySummary
func (r *RepoDependencySummary) TotalDeps() int {
	total := 0
	for _, gm := range r.GoMods {
		total += gm.TotalDeps
	}
	return total
}

func (r *RepoDependencySummary) OutdatedDeps() int {
	total := 0
	for _, gm := range r.GoMods {
		total += gm.OutdatedDeps
	}
	return total
}

func (r *RepoDependencySummary) HasOutdatedGoVersion() bool {
	for _, gm := range r.GoMods {
		if gm.GoVersionOutdated {
			return true
		}
	}
	return false
}

// DependencyMetrics holds aggregated dependency metrics
type DependencyMetrics struct {
	TotalRepos           int                     `json:"total_repos"`
	GoRepos              int                     `json:"go_repos"`           // Repos detected as Go by GitHub
	ReposWithGoMod       int                     `json:"repos_with_go_mod"`  // Repos with at least one go.mod
	TotalGoModFiles      int                     `json:"total_go_mod_files"` // Total go.mod files across all repos
	TotalDependencies    int                     `json:"total_dependencies"`
	OutdatedDependencies int                     `json:"outdated_dependencies"`
	OutdatedGoVersions   int                     `json:"outdated_go_versions"`
	LatestGoVersion      string                  `json:"latest_go_version"`
	Repositories         []RepoDependencySummary `json:"repositories"`
}

// FullDashboard combines security and dependency metrics
type FullDashboard struct {
	Security     DashboardMetrics  `json:"security"`
	Dependencies DependencyMetrics `json:"dependencies"`
}

// Jira Integration Models

// ConcernType represents the type of concern that can generate a Jira ticket
type ConcernType string

const (
	ConcernMissingSonarQube         ConcernType = "missing_sonarqube"
	ConcernOutdatedGo               ConcernType = "outdated_go"
	ConcernOutdatedDependencies     ConcernType = "outdated_dependencies"
	ConcernDependabotAlerts         ConcernType = "dependabot_alerts"
	ConcernCodeScanningAlerts       ConcernType = "code_scanning_alerts"
	ConcernSecretScanningAlerts     ConcernType = "secret_scanning_alerts"
	ConcernSonarQubeVulnerabilities ConcernType = "sonarqube_vulnerabilities"
	ConcernQualityGateFailed        ConcernType = "quality_gate_failed"
)

// JiraTicketRequest represents a request to create a Jira ticket
type JiraTicketRequest struct {
	ConcernType ConcernType            `json:"concern_type"`
	Repo        string                 `json:"repo"`
	EpicKey     string                 `json:"epic_key"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// JiraTicketResponse represents the response after creating a Jira ticket
type JiraTicketResponse struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

// JiraEpic represents a Jira epic for selection
type JiraEpic struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
}

// SonarQube Code Quality Models

// RepoCodeQualitySummary holds SonarQube metrics for a repository
type RepoCodeQualitySummary struct {
	Owner            string `json:"owner"`
	Repo             string `json:"repo"`
	FullName         string `json:"full_name"`
	SonarQubeProject string `json:"sonarqube_project"`
	// Metrics from SonarQube
	Bugs                  int     `json:"bugs"`
	Vulnerabilities       int     `json:"vulnerabilities"`
	CodeSmells            int     `json:"code_smells"`
	SecurityHotspots      int     `json:"security_hotspots"`
	Coverage              float64 `json:"coverage"`
	DuplicatedLines       float64 `json:"duplicated_lines_density"`
	LinesOfCode           int     `json:"lines_of_code"`
	ReliabilityRating     string  `json:"reliability_rating"`
	SecurityRating        string  `json:"security_rating"`
	MaintainabilityRating string  `json:"maintainability_rating"`
	QualityGateStatus     string  `json:"quality_gate_status"`
	LastAnalysis          string  `json:"last_analysis"`
	Error                 string  `json:"error,omitempty"`
}

// CodeQualityMetrics holds aggregated code quality metrics
type CodeQualityMetrics struct {
	TotalRepos            int                      `json:"total_repos"`
	ConfiguredRepos       int                      `json:"configured_repos"` // Repos with SonarQube configured
	AnalyzedRepos         int                      `json:"analyzed_repos"`   // Repos successfully analyzed
	TotalBugs             int                      `json:"total_bugs"`
	TotalVulnerabilities  int                      `json:"total_vulnerabilities"`
	TotalCodeSmells       int                      `json:"total_code_smells"`
	TotalSecurityHotspots int                      `json:"total_security_hotspots"`
	QualityGatePassed     int                      `json:"quality_gate_passed"`
	QualityGateFailed     int                      `json:"quality_gate_failed"`
	AverageCoverage       float64                  `json:"average_coverage"`
	Repositories          []RepoCodeQualitySummary `json:"repositories"`
}
