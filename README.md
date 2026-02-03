# Engineering Dashboard

A Go-based web dashboard that aggregates GitHub Security information and Go dependency status across multiple repositories.

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
   ```

2. **Set your GitHub token**

   ```bash
   export GITHUB_TOKEN=ghp_your_token_here
   ```

3. **Run the server**

   ```bash
   go run main.go
   ```

   Or build and run:

   ```bash
   go build -o security-dashboard
   ./security-dashboard
   ```

4. **Open the dashboard**

   Navigate to http://localhost:8080

## Configuration Options

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | Server port |
| `-config` | `repos.yaml` | Path to repositories config file |
| `-templates` | `templates` | Path to templates directory |

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Dashboard HTML view (use `?tab=security` or `?tab=dependencies`) |
| `GET /api/metrics` | JSON security metrics for all configured repositories |
| `GET /api/dependencies` | JSON dependency metrics for all configured repositories |
| `GET /api/repo?owner=X&repo=Y` | JSON security data for a specific repository |
| `GET /api/repo/dependencies?owner=X&repo=Y` | JSON dependency data for a specific repository |
| `GET /health` | Health check endpoint |

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_TOKEN` | Yes | GitHub Personal Access Token |

## Project Structure

```
security-dashboard/
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

## Notes

- The dashboard fetches data in real-time from GitHub's API and the Go module proxy
- Rate limiting may apply for large numbers of repositories or dependencies
- Only open security alerts are shown by default
- Some security features require GitHub Advanced Security to be enabled on repositories
- Private Go modules won't have version info from the public proxy (shown as "N/A")
- Dependency checking uses concurrent requests with rate limiting (10 concurrent) to avoid overwhelming the proxy
