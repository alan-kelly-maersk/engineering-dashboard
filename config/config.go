package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Repository struct {
	Owner            string `yaml:"owner"`
	Repo             string `yaml:"repo"`
	SonarQubeProject string `yaml:"sonarqube_project,omitempty"` // Optional: defaults to repo name if not specified
}

type Config struct {
	Repositories []Repository `yaml:"repositories"`
}

// SonarQubeProjectKey returns the SonarQube project key, defaulting to repo name if not specified
func (r Repository) SonarQubeProjectKey() string {
	if r.SonarQubeProject != "" {
		return r.SonarQubeProject
	}
	return r.Repo
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}
