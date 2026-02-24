package handlers

import (
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maersk/engineering-dashboard/config"
	"github.com/maersk/engineering-dashboard/github"
	"github.com/maersk/engineering-dashboard/gomod"
	"github.com/maersk/engineering-dashboard/goproxy"
	"github.com/maersk/engineering-dashboard/models"
	"github.com/maersk/engineering-dashboard/sonarqube"
)

func setupMockGitHubServer() *httptest.Server {
	mux := http.NewServeMux()

	// Default responses for all alert endpoints
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case contains(path, "/dependabot/alerts"):
			_ = json.NewEncoder(w).Encode([]models.DependabotAlert{})
		case contains(path, "/code-scanning/alerts"):
			_ = json.NewEncoder(w).Encode([]models.CodeScanningAlert{})
		case contains(path, "/secret-scanning/alerts"):
			_ = json.NewEncoder(w).Encode([]models.SecretScanningAlert{})
		case contains(path, "/languages"):
			_ = json.NewEncoder(w).Encode(map[string]int{"Go": 50000})
		case contains(path, "/git/trees/"):
			_ = json.NewEncoder(w).Encode(github.TreeResponse{
				Tree: []github.TreeItem{{Path: "go.mod", Type: "blob"}},
			})
		case contains(path, "/contents/go.mod"):
			content := base64.StdEncoding.EncodeToString([]byte("module example.com\n\ngo 1.25\n\nrequire github.com/pkg/errors v0.9.1\n"))
			_ = json.NewEncoder(w).Encode(github.FileContent{Content: content, Encoding: "base64"})
		default:
			// Repo info
			_ = json.NewEncoder(w).Encode(github.RepoInfo{DefaultBranch: "main"})
		}
	})

	return httptest.NewServer(mux)
}

func setupMockSonarQubeServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/measures/component", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"component": map[string]interface{}{
				"key":  "test-project",
				"name": "Test Project",
				"measures": []map[string]string{
					{"metric": "bugs", "value": "2"},
					{"metric": "vulnerabilities", "value": "1"},
					{"metric": "coverage", "value": "80.5"},
				},
			},
		})
	})
	mux.HandleFunc("/api/qualitygates/project_status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"projectStatus": map[string]string{"status": "OK"},
		})
	})
	mux.HandleFunc("/api/project_analyses/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"analyses": []map[string]string{{"key": "a1", "date": "2024-01-01"}},
		})
	})

	return httptest.NewServer(mux)
}

func setupMockProxyServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.RawQuery != "" {
			// Go version endpoint
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"version": "go1.25.6", "stable": true},
			})
			return
		}
		// Module version endpoint
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"Version": "v0.9.1",
		})
	})
	return httptest.NewServer(mux)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func newTestHandler(t *testing.T, ghServer, sqServer *httptest.Server, proxyServer *httptest.Server) *Handler {
	t.Helper()

	ghClient := github.NewClientWithBaseURL("test-token", ghServer.URL)

	var sqClient *sonarqube.Client
	if sqServer != nil {
		sqClient = sonarqube.NewClient(sqServer.URL, "sq-token")
	}

	proxyClient := goproxy.NewClientWithURLs(proxyServer.URL, proxyServer.URL+"/?mode=json")
	gomodParser := gomod.NewParser(proxyClient)

	tmpl := template.Must(template.New("dashboard.html").Parse(
		`ActiveTab:{{.ActiveTab}} SQ:{{.SonarQubeEnabled}} Auth:{{.AuthEnabled}} User:{{.UserName}}`))

	return &Handler{
		ghClient:    ghClient,
		sqClient:    sqClient,
		proxyClient: proxyClient,
		gomodParser: gomodParser,
		config: &config.Config{
			Repositories: []config.Repository{
				{Owner: "org", Repo: "repo1"},
			},
		},
		templates:   tmpl,
		authEnabled: false,
	}
}

func TestHealth(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get(ContentTypeHeader) != ApplicationJSON {
		t.Errorf("Content-Type = %q", w.Header().Get(ContentTypeHeader))
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %q", result["status"])
	}
}

func TestAPIRepo_MissingParams(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name  string
		query string
	}{
		{"no params", ""},
		{"only owner", "?owner=org"},
		{"only repo", "?repo=myrepo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/repo"+tt.query, nil)
			w := httptest.NewRecorder()
			h.APIRepo(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestAPIRepoDependencies_MissingParams(t *testing.T) {
	h := &Handler{proxyClient: goproxy.NewClient()}

	tests := []struct {
		name  string
		query string
	}{
		{"no params", ""},
		{"only owner", "?owner=org"},
		{"only repo", "?repo=myrepo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/repo/dependencies"+tt.query, nil)
			w := httptest.NewRecorder()
			h.APIRepoDependencies(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestAPICodeQuality_NotConfigured(t *testing.T) {
	h := &Handler{}

	req := httptest.NewRequest("GET", "/api/codequality", nil)
	w := httptest.NewRecorder()
	h.APICodeQuality(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestAPIRepoCodeQuality_NotConfigured(t *testing.T) {
	h := &Handler{}

	req := httptest.NewRequest("GET", "/api/repo/codequality", nil)
	w := httptest.NewRecorder()
	h.APIRepoCodeQuality(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestAPIRepoCodeQuality_MissingParams(t *testing.T) {
	sqServer := setupMockSonarQubeServer()
	defer sqServer.Close()

	h := &Handler{
		sqClient: sonarqube.NewClient(sqServer.URL, "token"),
		config: &config.Config{
			Repositories: []config.Repository{},
		},
	}

	req := httptest.NewRequest("GET", "/api/repo/codequality", nil)
	w := httptest.NewRecorder()
	h.APIRepoCodeQuality(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPIRepoCodeQuality_WithProjectParam(t *testing.T) {
	sqServer := setupMockSonarQubeServer()
	defer sqServer.Close()

	h := &Handler{
		sqClient: sonarqube.NewClient(sqServer.URL, "token"),
		config:   &config.Config{},
	}

	req := httptest.NewRequest("GET", "/api/repo/codequality?project=my-project", nil)
	w := httptest.NewRecorder()
	h.APIRepoCodeQuality(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAPIRepoCodeQuality_WithOwnerRepo(t *testing.T) {
	sqServer := setupMockSonarQubeServer()
	defer sqServer.Close()

	h := &Handler{
		sqClient: sonarqube.NewClient(sqServer.URL, "token"),
		config: &config.Config{
			Repositories: []config.Repository{
				{Owner: "org", Repo: "myrepo", SonarQubeProject: "custom-key"},
			},
		},
	}

	req := httptest.NewRequest("GET", "/api/repo/codequality?owner=org&repo=myrepo", nil)
	w := httptest.NewRecorder()
	h.APIRepoCodeQuality(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAPIRepoCodeQuality_FallbackToRepoName(t *testing.T) {
	sqServer := setupMockSonarQubeServer()
	defer sqServer.Close()

	h := &Handler{
		sqClient: sonarqube.NewClient(sqServer.URL, "token"),
		config: &config.Config{
			Repositories: []config.Repository{}, // No matching repo config
		},
	}

	req := httptest.NewRequest("GET", "/api/repo/codequality?owner=org&repo=unknown-repo", nil)
	w := httptest.NewRecorder()
	h.APIRepoCodeQuality(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestIsGoVersionOutdated(t *testing.T) {
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"1.25", "1.25", false},
		{"1.25", "1.25.6", false},
		{"1.24", "1.25", true},
		{"1.25", "1.26", true},
		{"2.0", "1.25", false},
	}

	for _, tt := range tests {
		t.Run(tt.current+"_vs_"+tt.latest, func(t *testing.T) {
			if got := isGoVersionOutdated(tt.current, tt.latest); got != tt.want {
				t.Errorf("isGoVersionOutdated(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestParseGoVersion(t *testing.T) {
	tests := []struct {
		input string
		want  [2]int
	}{
		{"1.25", [2]int{1, 25}},
		{"1.25.6", [2]int{1, 25}},
		{"2.0", [2]int{2, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseGoVersion(tt.input)
			if got != tt.want {
				t.Errorf("parseGoVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDashboard(t *testing.T) {
	ghServer := setupMockGitHubServer()
	defer ghServer.Close()
	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	h := newTestHandler(t, ghServer, nil, proxyServer)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.Dashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestDashboard_DependenciesTab(t *testing.T) {
	ghServer := setupMockGitHubServer()
	defer ghServer.Close()
	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	h := newTestHandler(t, ghServer, nil, proxyServer)

	req := httptest.NewRequest("GET", "/?tab=dependencies", nil)
	w := httptest.NewRecorder()
	h.Dashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestDashboard_CodeQualityTab(t *testing.T) {
	ghServer := setupMockGitHubServer()
	defer ghServer.Close()
	sqServer := setupMockSonarQubeServer()
	defer sqServer.Close()
	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	h := newTestHandler(t, ghServer, sqServer, proxyServer)

	req := httptest.NewRequest("GET", "/?tab=codequality", nil)
	w := httptest.NewRecorder()
	h.Dashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAPIMetrics(t *testing.T) {
	ghServer := setupMockGitHubServer()
	defer ghServer.Close()
	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	h := newTestHandler(t, ghServer, nil, proxyServer)

	req := httptest.NewRequest("GET", "/api/metrics", nil)
	w := httptest.NewRecorder()
	h.APIMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get(ContentTypeHeader) != ApplicationJSON {
		t.Errorf("Content-Type = %q", w.Header().Get(ContentTypeHeader))
	}
}

func TestAPIDependencies(t *testing.T) {
	ghServer := setupMockGitHubServer()
	defer ghServer.Close()
	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	h := newTestHandler(t, ghServer, nil, proxyServer)

	req := httptest.NewRequest("GET", "/api/dependencies", nil)
	w := httptest.NewRecorder()
	h.APIDependencies(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAPIRepo_Success(t *testing.T) {
	ghServer := setupMockGitHubServer()
	defer ghServer.Close()
	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	h := newTestHandler(t, ghServer, nil, proxyServer)

	req := httptest.NewRequest("GET", "/api/repo?owner=org&repo=repo1", nil)
	w := httptest.NewRecorder()
	h.APIRepo(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAPIRepoDependencies_Success(t *testing.T) {
	ghServer := setupMockGitHubServer()
	defer ghServer.Close()
	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	h := newTestHandler(t, ghServer, nil, proxyServer)

	req := httptest.NewRequest("GET", "/api/repo/dependencies?owner=org&repo=repo1", nil)
	w := httptest.NewRecorder()
	h.APIRepoDependencies(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAPICodeQuality_Success(t *testing.T) {
	ghServer := setupMockGitHubServer()
	defer ghServer.Close()
	sqServer := setupMockSonarQubeServer()
	defer sqServer.Close()
	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	h := newTestHandler(t, ghServer, sqServer, proxyServer)

	req := httptest.NewRequest("GET", "/api/codequality", nil)
	w := httptest.NewRecorder()
	h.APICodeQuality(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGetDependencyMetrics(t *testing.T) {
	ghServer := setupMockGitHubServer()
	defer ghServer.Close()
	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	h := newTestHandler(t, ghServer, nil, proxyServer)
	metrics := h.GetDependencyMetrics()

	if metrics.TotalRepos != 1 {
		t.Errorf("TotalRepos = %d", metrics.TotalRepos)
	}
}

func TestGetCodeQualityMetrics_NilClient(t *testing.T) {
	h := &Handler{
		config: &config.Config{Repositories: []config.Repository{{Owner: "org", Repo: "repo"}}},
	}

	metrics := h.GetCodeQualityMetrics()
	if metrics.TotalRepos != 1 {
		t.Errorf("TotalRepos = %d", metrics.TotalRepos)
	}
	if metrics.AnalyzedRepos != 0 {
		t.Errorf("AnalyzedRepos = %d, want 0", metrics.AnalyzedRepos)
	}
}

func TestGetCodeQualityMetrics_WithClient(t *testing.T) {
	sqServer := setupMockSonarQubeServer()
	defer sqServer.Close()

	h := &Handler{
		sqClient: sonarqube.NewClient(sqServer.URL, "token"),
		config: &config.Config{
			Repositories: []config.Repository{
				{Owner: "org", Repo: "repo1"},
			},
		},
	}

	metrics := h.GetCodeQualityMetrics()
	if metrics.ConfiguredRepos != 1 {
		t.Errorf("ConfiguredRepos = %d", metrics.ConfiguredRepos)
	}
	if metrics.AnalyzedRepos != 1 {
		t.Errorf("AnalyzedRepos = %d, want 1", metrics.AnalyzedRepos)
	}
	if metrics.QualityGatePassed != 1 {
		t.Errorf("QualityGatePassed = %d, want 1", metrics.QualityGatePassed)
	}
}

func TestGetRepoDependencySummary_NotGoRepo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/jsrepo/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"JavaScript": 50000})
	})
	ghServer := httptest.NewServer(mux)
	defer ghServer.Close()

	proxyServer := setupMockProxyServer()
	defer proxyServer.Close()

	ghClient := github.NewClientWithBaseURL("token", ghServer.URL)
	proxyClient := goproxy.NewClientWithURLs(proxyServer.URL, proxyServer.URL+"/?mode=json")

	h := &Handler{
		ghClient:    ghClient,
		proxyClient: proxyClient,
		gomodParser: gomod.NewParser(proxyClient),
		config:      &config.Config{},
	}

	summary := h.getRepoDependencySummary("org", "jsrepo", "1.25")
	if summary.IsGoRepo {
		t.Error("should not be Go repo")
	}
	if len(summary.GoMods) != 0 {
		t.Error("should have no go mods")
	}
}
