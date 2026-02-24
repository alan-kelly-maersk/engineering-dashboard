package github

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maersk/engineering-dashboard/config"
	"github.com/maersk/engineering-dashboard/models"
)

func newTestServer(handler http.Handler) (*httptest.Server, *Client) {
	server := httptest.NewServer(handler)
	client := NewClientWithBaseURL("test-token", server.URL)
	return server, client
}

func TestNewClient(t *testing.T) {
	c := NewClient("my-token")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.token != "my-token" {
		t.Errorf("token = %q", c.token)
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, defaultBaseURL)
	}
}

func TestNewClientWithBaseURL(t *testing.T) {
	c := NewClientWithBaseURL("token", "http://custom.api")
	if c.baseURL != "http://custom.api" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
}

func TestDoRequest_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or wrong Authorization header")
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Error("missing Accept header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": "value"})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	var result map[string]string
	err := client.doRequest(server.URL+"/test", &result)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("result = %v", result)
	}
}

func TestDoRequest_ServerError(t *testing.T) {
	server, client := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	var result map[string]string
	err := client.doRequest(server.URL+"/test", &result)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestGetDependabotAlerts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/dependabot/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		alerts := []models.DependabotAlert{
			{Number: 1, State: "open", Severity: "high"},
			{Number: 2, State: "open", Severity: "critical"},
		}
		json.NewEncoder(w).Encode(alerts)
	})

	server, client := newTestServer(mux)
	defer server.Close()

	alerts, err := client.GetDependabotAlerts("org", "repo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(alerts) != 2 {
		t.Errorf("got %d alerts, want 2", len(alerts))
	}
}

func TestGetDependabotAlerts_Error(t *testing.T) {
	server, client := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	_, err := client.GetDependabotAlerts("org", "repo")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetCodeScanningAlerts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/code-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.CodeScanningAlert{{Number: 1, State: "open"}})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	alerts, err := client.GetCodeScanningAlerts("org", "repo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(alerts) != 1 {
		t.Errorf("got %d alerts", len(alerts))
	}
}

func TestGetCodeScanningAlerts_Error(t *testing.T) {
	server, client := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer server.Close()

	_, err := client.GetCodeScanningAlerts("org", "repo")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetSecretScanningAlerts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/secret-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.SecretScanningAlert{{Number: 1, State: "open"}})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	alerts, err := client.GetSecretScanningAlerts("org", "repo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(alerts) != 1 {
		t.Errorf("got %d alerts", len(alerts))
	}
}

func TestGetSecretScanningAlerts_Error(t *testing.T) {
	server, client := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	_, err := client.GetSecretScanningAlerts("org", "repo")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetFileContent(t *testing.T) {
	content := "module github.com/example/test\n\ngo 1.25\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/contents/go.mod", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FileContent{
			Content:  encoded,
			Encoding: "base64",
			SHA:      "abc123",
		})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	got, err := client.GetFileContent("org", "repo", "go.mod")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestGetFileContent_UnexpectedEncoding(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/contents/file.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FileContent{Content: "data", Encoding: "utf-8"})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	_, err := client.GetFileContent("org", "repo", "file.txt")
	if err == nil {
		t.Fatal("expected error for non-base64 encoding")
	}
}

func TestGetFileContent_Error(t *testing.T) {
	server, client := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	_, err := client.GetFileContent("org", "repo", "missing.txt")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetRepoLanguages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"Go": 50000, "Shell": 1000})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	langs, err := client.GetRepoLanguages("org", "repo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if langs["Go"] != 50000 {
		t.Errorf("Go = %d", langs["Go"])
	}
}

func TestIsGoRepo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/gorepo/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"Go": 50000})
	})
	mux.HandleFunc("/repos/org/jsrepo/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"JavaScript": 30000})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	isGo, err := client.IsGoRepo("org", "gorepo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !isGo {
		t.Error("expected true for Go repo")
	}

	isGo, err = client.IsGoRepo("org", "jsrepo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if isGo {
		t.Error("expected false for JS repo")
	}
}

func TestGetRepoInfo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RepoInfo{DefaultBranch: "main"})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	info, err := client.GetRepoInfo("org", "repo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if info.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q", info.DefaultBranch)
	}
}

func TestFindGoModFiles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo", func(w http.ResponseWriter, r *http.Request) {
		// Distinguish between repo info and tree requests
		if r.URL.Path == "/repos/org/repo" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(RepoInfo{DefaultBranch: "main"})
		}
	})
	mux.HandleFunc("/repos/org/repo/git/trees/main", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TreeResponse{
			SHA: "abc",
			Tree: []TreeItem{
				{Path: "go.mod", Type: "blob"},
				{Path: "cmd/tool/go.mod", Type: "blob"},
				{Path: "README.md", Type: "blob"},
				{Path: "pkg", Type: "tree"},
			},
		})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	paths, err := client.FindGoModFiles("org", "repo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2", len(paths))
	}
	if paths[0] != "go.mod" || paths[1] != "cmd/tool/go.mod" {
		t.Errorf("paths = %v", paths)
	}
}

func TestGetAllGoModFiles(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("module example.com\n\ngo 1.25\n"))

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"Go": 50000})
	})
	mux.HandleFunc("/repos/org/repo", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/org/repo" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(RepoInfo{DefaultBranch: "main"})
		}
	})
	mux.HandleFunc("/repos/org/repo/git/trees/main", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TreeResponse{
			Tree: []TreeItem{{Path: "go.mod", Type: "blob"}},
		})
	})
	mux.HandleFunc("/repos/org/repo/contents/go.mod", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FileContent{Content: content, Encoding: "base64"})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	files, err := client.GetAllGoModFiles("org", "repo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Path != "go.mod" {
		t.Errorf("path = %q", files[0].Path)
	}
}

func TestGetAllGoModFiles_NotGoRepo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/languages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"Python": 50000})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	files, err := client.GetAllGoModFiles("org", "repo")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if files != nil {
		t.Errorf("expected nil for non-Go repo, got %v", files)
	}
}

func TestGetRepoSecuritySummary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/dependabot/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.DependabotAlert{{Number: 1, State: "open"}})
	})
	mux.HandleFunc("/repos/org/repo/code-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.CodeScanningAlert{{Number: 1, State: "open"}})
	})
	mux.HandleFunc("/repos/org/repo/secret-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.SecretScanningAlert{})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	summary := client.GetRepoSecuritySummary("org", "repo")

	if summary.Owner != "org" {
		t.Errorf("Owner = %q", summary.Owner)
	}
	if summary.Repo != "repo" {
		t.Errorf("Repo = %q", summary.Repo)
	}
	if summary.FullName != "org/repo" {
		t.Errorf("FullName = %q", summary.FullName)
	}
	if len(summary.DependabotAlerts) != 1 {
		t.Errorf("DependabotAlerts = %d", len(summary.DependabotAlerts))
	}
	if len(summary.CodeScanningAlerts) != 1 {
		t.Errorf("CodeScanningAlerts = %d", len(summary.CodeScanningAlerts))
	}
	if len(summary.SecretScanningAlerts) != 0 {
		t.Errorf("SecretScanningAlerts = %d", len(summary.SecretScanningAlerts))
	}
}

func TestGetRepoSecuritySummary_WithErrors(t *testing.T) {
	server, client := newTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	summary := client.GetRepoSecuritySummary("org", "repo")
	if summary.Error == "" {
		t.Error("expected error in summary")
	}
}

func TestGetDashboardMetrics(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo1/dependabot/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.DependabotAlert{
			{Number: 1, SecurityAdvisory: models.Advisory{Severity: "high"}},
			{Number: 2, SecurityAdvisory: models.Advisory{Severity: "critical"}},
		})
	})
	mux.HandleFunc("/repos/org/repo1/code-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.CodeScanningAlert{
			{Number: 1, Rule: models.Rule{SecuritySeverityLevel: "high"}},
		})
	})
	mux.HandleFunc("/repos/org/repo1/secret-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.SecretScanningAlert{})
	})
	mux.HandleFunc("/repos/org/repo2/dependabot/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.DependabotAlert{})
	})
	mux.HandleFunc("/repos/org/repo2/code-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.CodeScanningAlert{})
	})
	mux.HandleFunc("/repos/org/repo2/secret-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.SecretScanningAlert{{Number: 1, State: "open"}})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	repos := []config.Repository{
		{Owner: "org", Repo: "repo1"},
		{Owner: "org", Repo: "repo2"},
	}

	metrics := client.GetDashboardMetrics(repos)

	if metrics.TotalRepos != 2 {
		t.Errorf("TotalRepos = %d, want 2", metrics.TotalRepos)
	}
	if metrics.TotalDependabotAlerts != 2 {
		t.Errorf("TotalDependabotAlerts = %d, want 2", metrics.TotalDependabotAlerts)
	}
	if metrics.TotalCodeScanningAlerts != 1 {
		t.Errorf("TotalCodeScanningAlerts = %d, want 1", metrics.TotalCodeScanningAlerts)
	}
	if metrics.TotalSecretAlerts != 1 {
		t.Errorf("TotalSecretAlerts = %d, want 1", metrics.TotalSecretAlerts)
	}
	if metrics.DependabotBySeverity["high"] != 1 {
		t.Errorf("DependabotBySeverity[high] = %d", metrics.DependabotBySeverity["high"])
	}
	if metrics.DependabotBySeverity["critical"] != 1 {
		t.Errorf("DependabotBySeverity[critical] = %d", metrics.DependabotBySeverity["critical"])
	}
	if metrics.CodeScanningBySeverity["high"] != 1 {
		t.Errorf("CodeScanningBySeverity[high] = %d", metrics.CodeScanningBySeverity["high"])
	}
}

func TestGetDashboardMetrics_FallbackSeverity(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo/dependabot/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.DependabotAlert{
			{Number: 1, SecurityVuln: models.Vuln{Severity: "medium"}},
			{Number: 2}, // no severity at all
		})
	})
	mux.HandleFunc("/repos/org/repo/code-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.CodeScanningAlert{
			{Number: 1, Rule: models.Rule{Severity: "warning"}},
			{Number: 2}, // no severity
		})
	})
	mux.HandleFunc("/repos/org/repo/secret-scanning/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.SecretScanningAlert{})
	})

	server, client := newTestServer(mux)
	defer server.Close()

	repos := []config.Repository{{Owner: "org", Repo: "repo"}}
	metrics := client.GetDashboardMetrics(repos)

	if metrics.DependabotBySeverity["medium"] != 1 {
		t.Errorf("expected medium severity fallback, got %v", metrics.DependabotBySeverity)
	}
	if metrics.DependabotBySeverity["unknown"] != 1 {
		t.Errorf("expected unknown severity fallback, got %v", metrics.DependabotBySeverity)
	}
	if metrics.CodeScanningBySeverity["warning"] != 1 {
		t.Errorf("expected warning severity fallback, got %v", metrics.CodeScanningBySeverity)
	}
	if metrics.CodeScanningBySeverity["unknown"] != 1 {
		t.Errorf("expected unknown code scanning severity, got %v", metrics.CodeScanningBySeverity)
	}
}
