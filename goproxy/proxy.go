package goproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	proxyURL   = "https://proxy.golang.org"
	goDevURL   = "https://go.dev/dl/?mode=json"
	maxWorkers = 10 // Limit concurrent requests to avoid rate limiting
)

// Client handles requests to the Go module proxy
type Client struct {
	httpClient *http.Client
	cache      map[string]string
	cacheMu    sync.RWMutex
}

// NewClient creates a new Go proxy client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		cache: make(map[string]string),
	}
}

// VersionInfo represents the response from proxy.golang.org/@latest
type VersionInfo struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
}

// GoRelease represents a Go release from go.dev
type GoRelease struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

// GetLatestVersion fetches the latest version of a module from the proxy
func (c *Client) GetLatestVersion(modulePath string) (string, error) {
	// Check cache first
	c.cacheMu.RLock()
	if version, ok := c.cache[modulePath]; ok {
		c.cacheMu.RUnlock()
		return version, nil
	}
	c.cacheMu.RUnlock()

	// Escape the module path for URL
	escapedPath := escapeModulePath(modulePath)
	url := fmt.Sprintf("%s/%s/@latest", proxyURL, escapedPath)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Module not found on proxy (might be private)
		return "", fmt.Errorf("module not found on proxy")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("proxy request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var info VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}

	// Cache the result
	c.cacheMu.Lock()
	c.cache[modulePath] = info.Version
	c.cacheMu.Unlock()

	return info.Version, nil
}

// GetLatestGoVersion fetches the latest stable Go version from go.dev
func (c *Client) GetLatestGoVersion() (string, error) {
	// Check cache
	c.cacheMu.RLock()
	if version, ok := c.cache["__go_version__"]; ok {
		c.cacheMu.RUnlock()
		return version, nil
	}
	c.cacheMu.RUnlock()

	resp, err := c.httpClient.Get(goDevURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch Go versions: status %d", resp.StatusCode)
	}

	var releases []GoRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", err
	}

	for _, release := range releases {
		if release.Stable {
			version := strings.TrimPrefix(release.Version, "go")
			// Cache the result
			c.cacheMu.Lock()
			c.cache["__go_version__"] = version
			c.cacheMu.Unlock()
			return version, nil
		}
	}

	return "", fmt.Errorf("no stable Go version found")
}

// VersionResult holds the result of a version check
type VersionResult struct {
	ModulePath    string
	LatestVersion string
	Error         error
}

// GetLatestVersions fetches latest versions for multiple modules concurrently
func (c *Client) GetLatestVersions(modules []string) map[string]VersionResult {
	results := make(map[string]VersionResult)
	var mu sync.Mutex

	// Create a semaphore to limit concurrent requests
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, mod := range modules {
		wg.Add(1)
		go func(modulePath string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			version, err := c.GetLatestVersion(modulePath)
			mu.Lock()
			results[modulePath] = VersionResult{
				ModulePath:    modulePath,
				LatestVersion: version,
				Error:         err,
			}
			mu.Unlock()
		}(mod)
	}

	wg.Wait()
	return results
}

// escapeModulePath escapes a module path for use in a URL
// See: https://go.dev/ref/mod#goproxy-protocol
func escapeModulePath(path string) string {
	var result strings.Builder
	for _, r := range path {
		if r >= 'A' && r <= 'Z' {
			result.WriteByte('!')
			result.WriteRune(r + ('a' - 'A'))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
