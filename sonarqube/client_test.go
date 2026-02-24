package sonarqube

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("https://sonar.example.com/", "token")
	if c.baseURL != "https://sonar.example.com" {
		t.Errorf("baseURL = %q, trailing slash should be removed", c.baseURL)
	}
	if c.token != "token" {
		t.Errorf("token = %q", c.token)
	}
}

func TestIsConfigured(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		token   string
		want    bool
	}{
		{"both set", "http://sonar", "token", true},
		{"no url", "", "token", false},
		{"no token", "http://sonar", "", false},
		{"both empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.baseURL, tt.token)
			if got := c.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBaseURL(t *testing.T) {
	c := NewClient("https://sonar.example.com", "token")
	if c.GetBaseURL() != "https://sonar.example.com" {
		t.Errorf("GetBaseURL() = %q", c.GetBaseURL())
	}
}

func TestRatingToLetter(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0", "A"},
		{"1", "A"},
		{"2.0", "B"},
		{"2", "B"},
		{"3.0", "C"},
		{"3", "C"},
		{"4.0", "D"},
		{"4", "D"},
		{"5.0", "E"},
		{"5", "E"},
		{"unknown", "unknown"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ratingToLetter(tt.input); got != tt.want {
				t.Errorf("ratingToLetter(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetProjectMetrics_NotConfigured(t *testing.T) {
	c := NewClient("", "")
	metrics := c.GetProjectMetrics("project")
	if metrics.Error == "" {
		t.Error("expected error when not configured")
	}
}

func TestGetProjectMetrics_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/measures/component", func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := r.BasicAuth()
		if !ok || user != "test-token" {
			t.Error("expected basic auth with token")
		}

		resp := measuresResponse{}
		resp.Component.Key = "my-project"
		resp.Component.Name = "My Project"
		resp.Component.Measures = []measure{
			{Metric: "bugs", Value: "5"},
			{Metric: "vulnerabilities", Value: "3"},
			{Metric: "code_smells", Value: "42"},
			{Metric: "security_hotspots", Value: "2"},
			{Metric: "coverage", Value: "78.5"},
			{Metric: "duplicated_lines_density", Value: "3.2"},
			{Metric: "ncloc", Value: "10000"},
			{Metric: "reliability_rating", Value: "2.0"},
			{Metric: "security_rating", Value: "1.0"},
			{Metric: "sqale_rating", Value: "3.0"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/qualitygates/project_status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projectStatusResponse{
			ProjectStatus: struct {
				Status string `json:"status"`
			}{Status: "OK"},
		})
	})
	mux.HandleFunc("/api/project_analyses/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(analysisResponse{
			Analyses: []struct {
				Key  string `json:"key"`
				Date string `json:"date"`
			}{{Key: "a1", Date: "2024-06-01T10:00:00+0000"}},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	c := NewClient(server.URL, "test-token")
	metrics := c.GetProjectMetrics("my-project")

	if metrics.Error != "" {
		t.Fatalf("unexpected error: %s", metrics.Error)
	}
	if metrics.ProjectName != "My Project" {
		t.Errorf("ProjectName = %q", metrics.ProjectName)
	}
	if metrics.Bugs != 5 {
		t.Errorf("Bugs = %d", metrics.Bugs)
	}
	if metrics.Vulnerabilities != 3 {
		t.Errorf("Vulnerabilities = %d", metrics.Vulnerabilities)
	}
	if metrics.CodeSmells != 42 {
		t.Errorf("CodeSmells = %d", metrics.CodeSmells)
	}
	if metrics.SecurityHotspots != 2 {
		t.Errorf("SecurityHotspots = %d", metrics.SecurityHotspots)
	}
	if metrics.Coverage != 78.5 {
		t.Errorf("Coverage = %f", metrics.Coverage)
	}
	if metrics.DuplicatedLines != 3.2 {
		t.Errorf("DuplicatedLines = %f", metrics.DuplicatedLines)
	}
	if metrics.LinesOfCode != 10000 {
		t.Errorf("LinesOfCode = %d", metrics.LinesOfCode)
	}
	if metrics.ReliabilityRating != "B" {
		t.Errorf("ReliabilityRating = %q", metrics.ReliabilityRating)
	}
	if metrics.SecurityRating != "A" {
		t.Errorf("SecurityRating = %q", metrics.SecurityRating)
	}
	if metrics.MaintainabilityRating != "C" {
		t.Errorf("MaintainabilityRating = %q", metrics.MaintainabilityRating)
	}
	if metrics.QualityGateStatus != "OK" {
		t.Errorf("QualityGateStatus = %q", metrics.QualityGateStatus)
	}
	if metrics.LastAnalysis == "" {
		t.Error("LastAnalysis should be set")
	}
}

func TestGetProjectMetrics_ProjectNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClient(server.URL, "token")
	metrics := c.GetProjectMetrics("nonexistent")
	if metrics.Error == "" {
		t.Error("expected error for 404")
	}
}

func TestGetProjectMetrics_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(server.URL, "token")
	metrics := c.GetProjectMetrics("project")
	if metrics.Error == "" {
		t.Error("expected error for 500")
	}
}

func TestGetProjectMetrics_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClient(server.URL, "token")
	metrics := c.GetProjectMetrics("project")
	if metrics.Error == "" {
		t.Error("expected error for invalid JSON")
	}
}

func TestGetProjectMetrics_EmptyProjectName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/measures/component", func(w http.ResponseWriter, r *http.Request) {
		resp := measuresResponse{}
		resp.Component.Key = "my-project"
		resp.Component.Name = "" // empty name
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/qualitygates/project_status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/api/project_analyses/search", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	c := NewClient(server.URL, "token")
	metrics := c.GetProjectMetrics("my-project")
	if metrics.ProjectName != "my-project" {
		t.Errorf("ProjectName should default to key, got %q", metrics.ProjectName)
	}
}

func TestFetchQualityGateStatus_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/measures/component" {
			resp := measuresResponse{}
			resp.Component.Name = "test"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(server.URL, "token")
	metrics := c.GetProjectMetrics("project")
	if metrics.QualityGateStatus != "" {
		t.Errorf("QualityGateStatus should be empty on error, got %q", metrics.QualityGateStatus)
	}
}

func TestFetchLastAnalysis_NoAnalyses(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/measures/component", func(w http.ResponseWriter, r *http.Request) {
		resp := measuresResponse{}
		resp.Component.Name = "test"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/api/qualitygates/project_status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projectStatusResponse{})
	})
	mux.HandleFunc("/api/project_analyses/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(analysisResponse{Analyses: nil})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	c := NewClient(server.URL, "token")
	metrics := c.GetProjectMetrics("project")
	if metrics.LastAnalysis != "" {
		t.Errorf("LastAnalysis should be empty, got %q", metrics.LastAnalysis)
	}
}

func TestDoRequest_SetsBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Error("expected basic auth")
		}
		if user != "my-token" || pass != "" {
			t.Errorf("auth = (%q, %q), want (my-token, empty)", user, pass)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("missing Accept header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.URL, "my-token")
	resp, err := c.doRequest(server.URL + "/test")
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}
