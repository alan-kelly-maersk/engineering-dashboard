package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSonarQubeProjectKey_WithCustomKey(t *testing.T) {
	r := Repository{Owner: "org", Repo: "myrepo", SonarQubeProject: "custom-key"}
	if got := r.SonarQubeProjectKey(); got != "custom-key" {
		t.Errorf("SonarQubeProjectKey() = %q, want %q", got, "custom-key")
	}
}

func TestSonarQubeProjectKey_DefaultsToRepo(t *testing.T) {
	r := Repository{Owner: "org", Repo: "myrepo"}
	if got := r.SonarQubeProjectKey(); got != "myrepo" {
		t.Errorf("SonarQubeProjectKey() = %q, want %q", got, "myrepo")
	}
}

func TestLoad_Success(t *testing.T) {
	content := `repositories:
  - owner: testorg
    repo: testrepo
    sonarqube_project: sq-project
  - owner: testorg
    repo: another-repo
`
	dir := t.TempDir()
	path := filepath.Join(dir, "repos.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Repositories) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(cfg.Repositories))
	}

	if cfg.Repositories[0].Owner != "testorg" {
		t.Errorf("Repositories[0].Owner = %q, want %q", cfg.Repositories[0].Owner, "testorg")
	}
	if cfg.Repositories[0].Repo != "testrepo" {
		t.Errorf("Repositories[0].Repo = %q, want %q", cfg.Repositories[0].Repo, "testrepo")
	}
	if cfg.Repositories[0].SonarQubeProject != "sq-project" {
		t.Errorf("Repositories[0].SonarQubeProject = %q, want %q", cfg.Repositories[0].SonarQubeProject, "sq-project")
	}
	if cfg.Repositories[1].Repo != "another-repo" {
		t.Errorf("Repositories[1].Repo = %q, want %q", cfg.Repositories[1].Repo, "another-repo")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/repos.yaml")
	if err == nil {
		t.Fatal("Load() expected error for nonexistent file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{{{invalid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid YAML, got nil")
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Repositories) != 0 {
		t.Errorf("expected 0 repositories, got %d", len(cfg.Repositories))
	}
}
