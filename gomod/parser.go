package gomod

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/maersk/engineering-dashboard/goproxy"
	"github.com/maersk/engineering-dashboard/models"
)

var (
	moduleRegex  = regexp.MustCompile(`(?m)^module\s+(.+)$`)
	goVersionRe  = regexp.MustCompile(`(?m)^go\s+(\d+\.\d+(?:\.\d+)?)$`)
	requireRegex = regexp.MustCompile(`(?m)^\s*([^\s]+)\s+(v[^\s]+)(\s*//\s*indirect)?`)
)

// Parser parses go.mod files
type Parser struct {
	proxyClient *goproxy.Client
}

// NewParser creates a new go.mod parser
func NewParser(proxyClient *goproxy.Client) *Parser {
	return &Parser{
		proxyClient: proxyClient,
	}
}

// Parse parses a go.mod file content and returns GoModInfo
func (p *Parser) Parse(content string) models.GoModInfo {
	info := models.GoModInfo{
		Dependencies: []models.Dependency{},
	}

	// Extract module path
	if matches := moduleRegex.FindStringSubmatch(content); len(matches) > 1 {
		info.ModulePath = strings.TrimSpace(matches[1])
	}

	// Extract Go version
	if matches := goVersionRe.FindStringSubmatch(content); len(matches) > 1 {
		info.GoVersion = matches[1]
	}

	// Extract dependencies from require blocks
	deps := p.extractDependencies(content)
	info.Dependencies = deps

	// Count stats
	for _, dep := range deps {
		info.TotalDeps++
		if dep.IsIndirect {
			info.IndirectDeps++
		} else {
			info.DirectDeps++
		}
	}

	return info
}

// extractDependencies extracts all dependencies from go.mod content
func (p *Parser) extractDependencies(content string) []models.Dependency {
	var deps []models.Dependency
	seen := make(map[string]bool)

	// Handle both single-line requires and require blocks
	lines := strings.Split(content, "\n")
	inRequireBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for require block start/end
		if strings.HasPrefix(line, "require (") || strings.HasPrefix(line, "require(") {
			inRequireBlock = true
			continue
		}
		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}

		// Skip replace, exclude, retract directives
		if strings.HasPrefix(line, "replace ") ||
			strings.HasPrefix(line, "exclude ") ||
			strings.HasPrefix(line, "retract ") {
			continue
		}

		// Handle single-line require
		if strings.HasPrefix(line, "require ") {
			line = strings.TrimPrefix(line, "require ")
		} else if !inRequireBlock {
			continue
		}

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Parse the dependency line
		matches := requireRegex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			path := matches[1]
			version := matches[2]
			isIndirect := len(matches) > 3 && matches[3] != ""

			// Skip if already seen
			if seen[path] {
				continue
			}
			seen[path] = true

			deps = append(deps, models.Dependency{
				Path:           path,
				CurrentVersion: version,
				IsIndirect:     isIndirect,
			})
		}
	}

	return deps
}

// CheckForUpdates checks all dependencies for available updates
func (p *Parser) CheckForUpdates(info *models.GoModInfo) {
	if len(info.Dependencies) == 0 {
		return
	}

	// Get latest Go version
	if info.GoVersion != "" {
		latestGo, err := p.proxyClient.GetLatestGoVersion()
		if err == nil {
			info.LatestGoVersion = latestGo
			info.GoVersionOutdated = isGoVersionOutdated(info.GoVersion, latestGo)
		}
	}

	// Collect all module paths
	modules := make([]string, len(info.Dependencies))
	for i, dep := range info.Dependencies {
		modules[i] = dep.Path
	}

	// Fetch latest versions concurrently
	results := p.proxyClient.GetLatestVersions(modules)

	// Update dependencies with latest versions
	for i := range info.Dependencies {
		dep := &info.Dependencies[i]
		if result, ok := results[dep.Path]; ok {
			if result.Error != nil {
				dep.Error = result.Error.Error()
			} else {
				dep.LatestVersion = result.LatestVersion
				dep.IsOutdated = compareVersions(dep.CurrentVersion, dep.LatestVersion) < 0
				if dep.IsOutdated {
					info.OutdatedDeps++
				}
			}
		}
	}
}

// isGoVersionOutdated checks if the current Go version is outdated
// Only compares major.minor since go.mod specifies major.minor and patch versions are backwards compatible
// e.g., "1.25" is NOT outdated if latest is "1.25.6", but IS outdated if latest is "1.26"
func isGoVersionOutdated(current, latest string) bool {
	currentMajor, currentMinor := parseGoMajorMinor(current)
	latestMajor, latestMinor := parseGoMajorMinor(latest)

	if currentMajor < latestMajor {
		return true
	}
	if currentMajor == latestMajor && currentMinor < latestMinor {
		return true
	}
	return false
}

// parseGoMajorMinor extracts major and minor version numbers from a Go version string
func parseGoMajorMinor(v string) (major, minor int) {
	_, err := fmt.Sscanf(v, "%d.%d", &major, &minor)
	if err != nil {
		return 0, 0
	}
	return
}

// compareVersions compares two semantic versions
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Strip 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// Handle +incompatible suffix
	v1 = strings.Split(v1, "+")[0]
	v2 = strings.Split(v2, "+")[0]

	// Handle pre-release versions (e.g., v1.2.3-beta)
	v1Parts := strings.Split(v1, "-")
	v2Parts := strings.Split(v2, "-")

	// Compare main version parts
	parts1 := strings.Split(v1Parts[0], ".")
	parts2 := strings.Split(v2Parts[0], ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1 = parseVersionPart(parts1[i])
		}
		if i < len(parts2) {
			n2 = parseVersionPart(parts2[i])
		}

		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	// If main versions are equal, check pre-release
	// A version without pre-release is greater than one with pre-release
	if len(v1Parts) > 1 && len(v2Parts) == 1 {
		return -1
	}
	if len(v1Parts) == 1 && len(v2Parts) > 1 {
		return 1
	}

	return 0
}

func parseVersionPart(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}
