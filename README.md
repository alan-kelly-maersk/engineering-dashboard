# Engineering Dashboard

A Go-based web dashboard that aggregates GitHub Security information, Go dependency status, and SonarQube code quality metrics across multiple repositories.

## Features

### Security Tab
- **Dependabot Alerts**: View vulnerability alerts from Dependabot
- **Code Scanning Alerts**: View alerts from GitHub Advanced Security code scanning
- **Secret Scanning Alerts**: View exposed secrets detected in repositories
- **Aggregated Metrics**: Dashboard overview with severity breakdowns

### Dependencies Tab
- **Go Version Tracking**: See which repos are on outdated Go versions
- **Dependency Analysis**: Check all go.mod dependencies against the Go module proxy
- **Update Detection**: Identify which dependencies have newer versions available
- **Direct vs Indirect**: Distinguish between direct and indirect dependencies

### Code Quality Tab (SonarQube)
- **Quality Gate Status**: See which projects pass or fail the quality gate
- **Bugs & Vulnerabilities**: Track code bugs and security vulnerabilities
- **Code Smells**: Monitor maintainability issues
- **Security Hotspots**: Review security-sensitive code that needs manual review
- **Test Coverage**: Track code coverage percentages
- **Code Duplication**: Monitor duplicated code percentages
- **Ratings**: View reliability, security, and maintainability ratings (A-E)

## Requirements

- Go 1.21 or later
- GitHub Personal Access Token (PAT) with the following scopes:
  - `repo` (for private repositories)
  - `security_events` (for code scanning alerts)

## Setup

1. **Clone and configure repositories**

   Edit `repos.yaml` to add your repositories:

   ```yaml
   repositories:
     - owner: your-org
       repo: repo-1
     - owner: your-org
       repo: repo-2
       sonarqube_project: custom-project-key  # Optional: defaults to repo name
   ```

   The `sonarqube_project` field is optional. If not specified, the repo name is used as the SonarQube project key.

2. **Set your GitHub token**

   ```bash
   export GITHUB_TOKEN=ghp_your_token_here
   ```

3. **Set SonarQube credentials (optional)**

   To enable the Code Quality tab, set your SonarQube URL and API token:

   ```bash
   export SONARQUBE_URL=https://sonarqube.example.com
   export SONARQUBE_TOKEN=squ_your_token_here
   ```

   The Code Quality tab will only appear if both environment variables are set.

3. **Run the server**

   ```bash
   go run main.go
   ```

   Or build and run:

   ```bash
   go build -o engineering-dashboard
   ./engineering-dashboard
   ```

4. **Open the dashboard**

   Navigate to http://localhost:8080

## Configuration Options

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | (see below) | Server port (overrides `PORT` env var) |
| `-config` | `repos.yaml` | Path to repositories config file |
| `-templates` | `templates` | Path to templates directory |

Port precedence: `-port` flag > `PORT` env var > `8080` (default)

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Dashboard HTML view (use `?tab=security`, `?tab=dependencies`, or `?tab=codequality`) |
| `GET /api/metrics` | JSON security metrics for all configured repositories |
| `GET /api/dependencies` | JSON dependency metrics for all configured repositories |
| `GET /api/codequality` | JSON code quality metrics from SonarQube |
| `GET /api/repo?owner=X&repo=Y` | JSON security data for a specific repository |
| `GET /api/repo/dependencies?owner=X&repo=Y` | JSON dependency data for a specific repository |
| `GET /api/repo/codequality?project=X` | JSON code quality data for a SonarQube project |
| `GET /health` | Health check endpoint |

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_TOKEN` | Yes | GitHub Personal Access Token |
| `PORT` | No | Server port (default: `8080`) |
| `SONARQUBE_URL` | No | SonarQube server URL (e.g., `https://sonarqube.example.com`) |
| `SONARQUBE_TOKEN` | No | SonarQube API token (generate from User > My Account > Security) |

## Project Structure

```
engineering-dashboard/
├── main.go              # Application entry point
├── go.mod               # Go module definition
├── repos.yaml           # Repository configuration
├── config/
│   └── config.go        # Configuration loading
├── github/
│   └── client.go        # GitHub API client
├── gomod/
│   └── parser.go        # go.mod file parser
├── goproxy/
│   └── proxy.go         # Go module proxy client
├── sonarqube/
│   └── client.go        # SonarQube API client
├── handlers/
│   └── handlers.go      # HTTP handlers
├── models/
│   └── models.go        # Data models
├── templates/
│   └── dashboard.html   # Dashboard template
└── static/              # Static assets (if needed)
```

## How It Works

### Security Analysis
The dashboard uses GitHub's REST API to fetch:
- Dependabot alerts (`/repos/{owner}/{repo}/dependabot/alerts`)
- Code scanning alerts (`/repos/{owner}/{repo}/code-scanning/alerts`)
- Secret scanning alerts (`/repos/{owner}/{repo}/secret-scanning/alerts`)

### Dependency Analysis
For each repository:
1. Fetches the `go.mod` file via GitHub's Contents API
2. Parses it to extract the Go version and all dependencies
3. Queries `proxy.golang.org` to find the latest version of each dependency
4. Fetches the latest stable Go version from `go.dev/dl/?mode=json`
5. Compares versions to identify outdated packages

### Code Quality Analysis (SonarQube)
For each repository with a configured SonarQube project:
1. Fetches project measures via `/api/measures/component` (bugs, vulnerabilities, code smells, coverage, etc.)
2. Fetches quality gate status via `/api/qualitygates/project_status`
3. Fetches last analysis date via `/api/project_analyses/search`

The SonarQube API uses token-based authentication (Basic Auth with token as username).

## Notes

- The dashboard fetches data in real-time from GitHub's API and the Go module proxy
- Rate limiting may apply for large numbers of repositories or dependencies
- Only open security alerts are shown by default
- Some security features require GitHub Advanced Security to be enabled on repositories
- Private Go modules won't have version info from the public proxy (shown as "N/A")
- Dependency checking uses concurrent requests with rate limiting (10 concurrent) to avoid overwhelming the proxy
- The Code Quality tab only appears when `SONARQUBE_URL` and `SONARQUBE_TOKEN` are configured
- If a repository's SonarQube project key differs from the repo name, specify it with `sonarqube_project` in repos.yaml
