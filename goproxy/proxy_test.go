package goproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.proxyURL != "https://proxy.golang.org" {
		t.Errorf("proxyURL = %q", c.proxyURL)
	}
	if c.goDevURL != "https://go.dev/dl/?mode=json" {
		t.Errorf("goDevURL = %q", c.goDevURL)
	}
	if c.cache == nil {
		t.Error("cache not initialized")
	}
}

func TestNewClientWithURLs(t *testing.T) {
	c := NewClientWithURLs("http://proxy.test", "http://godev.test")
	if c.proxyURL != "http://proxy.test" {
		t.Errorf("proxyURL = %q", c.proxyURL)
	}
	if c.goDevURL != "http://godev.test" {
		t.Errorf("goDevURL = %q", c.goDevURL)
	}
}

func TestEscapeModulePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/pkg/errors", "github.com/pkg/errors"},
		{"github.com/Azure/azure-sdk", "github.com/!azure/azure-sdk"},
		{"github.com/BurntSushi/toml", "github.com/!burnt!sushi/toml"},
		{"golang.org/x/text", "golang.org/x/text"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := escapeModulePath(tt.input); got != tt.want {
				t.Errorf("escapeModulePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetLatestVersion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VersionInfo{Version: "v1.2.3"})
	}))
	defer server.Close()

	c := NewClientWithURLs(server.URL, "")
	version, err := c.GetLatestVersion("github.com/pkg/errors")
	if err != nil {
		t.Fatalf("GetLatestVersion() error = %v", err)
	}
	if version != "v1.2.3" {
		t.Errorf("version = %q, want %q", version, "v1.2.3")
	}
}

func TestGetLatestVersion_Cached(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VersionInfo{Version: "v1.0.0"})
	}))
	defer server.Close()

	c := NewClientWithURLs(server.URL, "")

	// First call
	v1, _ := c.GetLatestVersion("github.com/test/mod")
	// Second call should be cached
	v2, _ := c.GetLatestVersion("github.com/test/mod")

	if v1 != v2 {
		t.Errorf("cached response mismatch: %q vs %q", v1, v2)
	}
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", callCount)
	}
}

func TestGetLatestVersion_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClientWithURLs(server.URL, "")
	_, err := c.GetLatestVersion("github.com/nonexistent/mod")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestGetLatestVersion_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	c := NewClientWithURLs(server.URL, "")
	_, err := c.GetLatestVersion("github.com/test/mod")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestGetLatestVersion_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClientWithURLs(server.URL, "")
	_, err := c.GetLatestVersion("github.com/test/mod")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetLatestGoVersion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		releases := []GoRelease{
			{Version: "go1.25.6", Stable: true},
			{Version: "go1.26rc1", Stable: false},
		}
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	c := NewClientWithURLs("", server.URL)
	version, err := c.GetLatestGoVersion()
	if err != nil {
		t.Fatalf("GetLatestGoVersion() error = %v", err)
	}
	if version != "1.25.6" {
		t.Errorf("version = %q, want %q", version, "1.25.6")
	}
}

func TestGetLatestGoVersion_Cached(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GoRelease{{Version: "go1.25", Stable: true}})
	}))
	defer server.Close()

	c := NewClientWithURLs("", server.URL)
	c.GetLatestGoVersion()
	c.GetLatestGoVersion()

	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (cached), got %d", callCount)
	}
}

func TestGetLatestGoVersion_NoStableRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GoRelease{{Version: "go1.26rc1", Stable: false}})
	}))
	defer server.Close()

	c := NewClientWithURLs("", server.URL)
	_, err := c.GetLatestGoVersion()
	if err == nil {
		t.Fatal("expected error when no stable release")
	}
}

func TestGetLatestGoVersion_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClientWithURLs("", server.URL)
	_, err := c.GetLatestGoVersion()
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestGetLatestGoVersion_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClientWithURLs("", server.URL)
	_, err := c.GetLatestGoVersion()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetLatestVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VersionInfo{Version: "v1.0.0"})
	}))
	defer server.Close()

	c := NewClientWithURLs(server.URL, "")
	modules := []string{"github.com/a/b", "github.com/c/d", "github.com/e/f"}
	results := c.GetLatestVersions(modules)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	for _, mod := range modules {
		result, ok := results[mod]
		if !ok {
			t.Errorf("missing result for %s", mod)
			continue
		}
		if result.Error != nil {
			t.Errorf("unexpected error for %s: %v", mod, result.Error)
		}
		if result.LatestVersion != "v1.0.0" {
			t.Errorf("version for %s = %q, want v1.0.0", mod, result.LatestVersion)
		}
	}
}

func TestGetLatestVersions_Empty(t *testing.T) {
	c := NewClient()
	results := c.GetLatestVersions(nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(results))
	}
}

func TestGetLatestVersions_WithErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClientWithURLs(server.URL, "")
	results := c.GetLatestVersions([]string{"github.com/nonexistent"})

	if result, ok := results["github.com/nonexistent"]; ok {
		if result.Error == nil {
			t.Error("expected error for nonexistent module")
		}
	} else {
		t.Error("missing result")
	}
}
