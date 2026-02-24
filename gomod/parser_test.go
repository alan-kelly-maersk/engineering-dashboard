package gomod

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maersk/engineering-dashboard/goproxy"
	"github.com/maersk/engineering-dashboard/models"
)

func TestNewParser(t *testing.T) {
	client := goproxy.NewClient()
	p := NewParser(client)
	if p == nil {
		t.Fatal("NewParser returned nil")
	}
	if p.proxyClient != client {
		t.Error("proxyClient not set correctly")
	}
}

func TestParse_BasicGoMod(t *testing.T) {
	content := `module github.com/example/project

go 1.25

require (
	github.com/pkg/errors v0.9.1
	golang.org/x/text v0.14.0 // indirect
)
`
	p := NewParser(goproxy.NewClient())
	info := p.Parse(content)

	if info.ModulePath != "github.com/example/project" {
		t.Errorf("ModulePath = %q, want %q", info.ModulePath, "github.com/example/project")
	}
	if info.GoVersion != "1.25" {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, "1.25")
	}
	if info.TotalDeps != 2 {
		t.Errorf("TotalDeps = %d, want 2", info.TotalDeps)
	}
	if info.DirectDeps != 1 {
		t.Errorf("DirectDeps = %d, want 1", info.DirectDeps)
	}
	if info.IndirectDeps != 1 {
		t.Errorf("IndirectDeps = %d, want 1", info.IndirectDeps)
	}
}

func TestParse_EmptyContent(t *testing.T) {
	p := NewParser(goproxy.NewClient())
	info := p.Parse("")

	if info.ModulePath != "" {
		t.Errorf("ModulePath = %q, want empty", info.ModulePath)
	}
	if info.TotalDeps != 0 {
		t.Errorf("TotalDeps = %d, want 0", info.TotalDeps)
	}
	if len(info.Dependencies) != 0 {
		t.Errorf("Dependencies should be empty, got %d", len(info.Dependencies))
	}
}

func TestParse_SingleLineRequire(t *testing.T) {
	content := `module example.com/test

go 1.24

require github.com/stretchr/testify v1.8.4
`
	p := NewParser(goproxy.NewClient())
	info := p.Parse(content)

	if info.TotalDeps != 1 {
		t.Fatalf("TotalDeps = %d, want 1", info.TotalDeps)
	}
	if info.Dependencies[0].Path != "github.com/stretchr/testify" {
		t.Errorf("dependency path = %q", info.Dependencies[0].Path)
	}
	if info.Dependencies[0].CurrentVersion != "v1.8.4" {
		t.Errorf("dependency version = %q", info.Dependencies[0].CurrentVersion)
	}
	if info.Dependencies[0].IsIndirect {
		t.Error("expected direct dependency")
	}
}

func TestParse_MultipleRequireBlocks(t *testing.T) {
	content := `module example.com/test

go 1.25

require (
	github.com/pkg/errors v0.9.1
)

require (
	golang.org/x/text v0.14.0 // indirect
)
`
	p := NewParser(goproxy.NewClient())
	info := p.Parse(content)

	if info.TotalDeps != 2 {
		t.Errorf("TotalDeps = %d, want 2", info.TotalDeps)
	}
}

func TestParse_SkipsReplaceExcludeRetract(t *testing.T) {
	content := `module example.com/test

go 1.25

require github.com/pkg/errors v0.9.1

replace github.com/pkg/errors => github.com/pkg/errors v0.9.2
exclude github.com/old/pkg v1.0.0
retract v1.0.0
`
	p := NewParser(goproxy.NewClient())
	info := p.Parse(content)

	if info.TotalDeps != 1 {
		t.Errorf("TotalDeps = %d, want 1", info.TotalDeps)
	}
}

func TestParse_DuplicateDependencies(t *testing.T) {
	content := `module example.com/test

go 1.25

require (
	github.com/pkg/errors v0.9.1
	github.com/pkg/errors v0.9.1
)
`
	p := NewParser(goproxy.NewClient())
	info := p.Parse(content)

	if info.TotalDeps != 1 {
		t.Errorf("TotalDeps = %d, want 1 (should deduplicate)", info.TotalDeps)
	}
}

func TestParse_CommentsAndEmptyLines(t *testing.T) {
	content := `module example.com/test

go 1.25

require (
	// This is a comment
	github.com/pkg/errors v0.9.1

	golang.org/x/text v0.14.0
)
`
	p := NewParser(goproxy.NewClient())
	info := p.Parse(content)

	if info.TotalDeps != 2 {
		t.Errorf("TotalDeps = %d, want 2", info.TotalDeps)
	}
}

func TestParse_GoVersionWithPatch(t *testing.T) {
	content := `module example.com/test

go 1.25.3
`
	p := NewParser(goproxy.NewClient())
	info := p.Parse(content)

	if info.GoVersion != "1.25.3" {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, "1.25.3")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v1.0.1", -1},
		{"v1.0.1", "v1.0.0", 1},
		{"v1.0.0", "v2.0.0", -1},
		{"v2.0.0", "v1.0.0", 1},
		{"v0.9.1", "v0.10.0", -1},
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.0", "v1.2", 0},
		// Pre-release versions
		{"v1.0.0-beta", "v1.0.0", -1},
		{"v1.0.0", "v1.0.0-beta", 1},
		{"v1.0.0-alpha", "v1.0.0-beta", 0}, // same main version
		// Incompatible suffix
		{"v2.0.0+incompatible", "v2.0.0", 0},
		{"v1.0.0+incompatible", "v2.0.0", -1},
		// Without v prefix
		{"1.0.0", "2.0.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			if got := compareVersions(tt.v1, tt.v2); got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
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
		{"1.25", "2.0", true},
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

func TestParseGoMajorMinor(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
	}{
		{"1.25", 1, 25},
		{"1.25.6", 1, 25},
		{"2.0", 2, 0},
		{"invalid", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			major, minor := parseGoMajorMinor(tt.input)
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Errorf("parseGoMajorMinor(%q) = (%d, %d), want (%d, %d)", tt.input, major, minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestParseVersionPart(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"10beta", 10},
		{"", 0},
		{"abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseVersionPart(tt.input); got != tt.want {
				t.Errorf("parseVersionPart(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestCheckForUpdates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/github.com/pkg/errors/@latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Version":"v0.10.0","Time":"2024-01-01T00:00:00Z"}`))
	})
	mux.HandleFunc("/golang.org/x/text/@latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Version":"v0.15.0","Time":"2024-01-01T00:00:00Z"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Go version endpoint
		if r.URL.Path == "/" && r.URL.RawQuery != "" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"version":"go1.25.6","stable":true}]`))
			return
		}
		http.NotFound(w, r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	proxyClient := goproxy.NewClientWithURLs(server.URL, server.URL+"/?mode=json")
	p := NewParser(proxyClient)

	info := &models.GoModInfo{
		GoVersion: "1.24",
		Dependencies: []models.Dependency{
			{Path: "github.com/pkg/errors", CurrentVersion: "v0.9.1"},
			{Path: "golang.org/x/text", CurrentVersion: "v0.14.0"},
		},
	}

	p.CheckForUpdates(info)

	if info.LatestGoVersion != "1.25.6" {
		t.Errorf("LatestGoVersion = %q, want %q", info.LatestGoVersion, "1.25.6")
	}
	if !info.GoVersionOutdated {
		t.Error("expected GoVersionOutdated = true")
	}

	for _, dep := range info.Dependencies {
		if dep.LatestVersion == "" {
			t.Errorf("dependency %s has no latest version", dep.Path)
		}
		if !dep.IsOutdated {
			t.Errorf("dependency %s should be outdated", dep.Path)
		}
	}

	if info.OutdatedDeps != 2 {
		t.Errorf("OutdatedDeps = %d, want 2", info.OutdatedDeps)
	}
}

func TestCheckForUpdates_EmptyDeps(t *testing.T) {
	p := NewParser(goproxy.NewClient())
	info := &models.GoModInfo{}
	p.CheckForUpdates(info)
	// Should not panic or error
}

func TestCheckForUpdates_ProxyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	proxyClient := goproxy.NewClientWithURLs(server.URL, server.URL)
	p := NewParser(proxyClient)

	info := &models.GoModInfo{
		GoVersion: "1.25",
		Dependencies: []models.Dependency{
			{Path: "github.com/nonexistent/pkg", CurrentVersion: "v1.0.0"},
		},
	}

	p.CheckForUpdates(info)

	if info.Dependencies[0].Error == "" {
		t.Error("expected error for nonexistent module")
	}
}
