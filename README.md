# Engineering Dashboard

A Go-based web dashboard that aggregates GitHub Security information, Go dependency status, and SonarQube code quality metrics across multiple repositories. Includes Jira integration to create tickets for identified concerns.

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

### Jira Integration
- **Create Tickets**: Click the "Jira" button next to any concern to create a ticket
- **Epic Selection**: Choose which epic to link the new story to
- **Pre-populated Content**: Tickets are automatically filled with relevant details
- **Direct Links**: Get a clickable link to the created ticket

Supported concern types:
- Missing SonarQube project
- Outdated Go version
- Outdated dependencies
- Dependabot security alerts
- Code scanning alerts
- Secret scanning alerts
- SonarQube vulnerabilities/security hotspots
- Quality gate failures

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

4. **Set Jira/Atlassian credentials (optional)**

   To enable the Jira integration for creating tickets:

   ```bash
   export ATLASSIAN_URL=https://your-company.atlassian.net
   export ATLASSIAN_CLOUD_ID=your-cloud-id-uuid
   export ATLASSIAN_EMAIL=your.email@company.com
   export ATLASSIAN_API_TOKEN=your_api_token_here
   export JIRA_PROJECT_KEY=PROJ
   export JIRA_EPIC_LINK_FIELD=customfield_10002  # Optional, defaults to customfield_10002
   ```

   To find your Cloud ID, you can use the Atlassian API or check your Atlassian admin settings.
   
   To create an API token, visit: https://id.atlassian.com/manage-profile/security/api-tokens

   The Jira buttons will only appear if all required environment variables are set.

5. **Run the server**

   ```bash
   go run main.go
   ```

   Or build and run:

   ```bash
   go build -o engineering-dashboard
   ./engineering-dashboard
   ```

6. **Open the dashboard**

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
| `GET /api/jira/enabled` | Check if Jira integration is configured |
| `GET /api/jira/epics` | List open epics in the configured Jira project |
| `POST /api/jira/ticket` | Create a Jira ticket for a concern |
| `GET /health` | Health check endpoint |

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_TOKEN` | Yes | GitHub Personal Access Token |
| `PORT` | No | Server port (default: `8080`) |
| `SONARQUBE_URL` | No | SonarQube server URL (e.g., `https://sonarqube.example.com`) |
| `SONARQUBE_TOKEN` | No | SonarQube API token (generate from User > My Account > Security) |
| `ATLASSIAN_URL` | No | Atlassian instance URL (e.g., `https://company.atlassian.net`) |
| `ATLASSIAN_CLOUD_ID` | No | Atlassian Cloud ID (UUID) |
| `ATLASSIAN_EMAIL` | No | Email address for Atlassian authentication |
| `ATLASSIAN_API_TOKEN` | No | Atlassian API token |
| `JIRA_PROJECT_KEY` | No | Jira project key for ticket creation |
| `JIRA_EPIC_LINK_FIELD` | No | Custom field for epic link (default: `customfield_10002`) |

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
├── jira/
│   └── client.go        # Jira/Atlassian API client
├── handlers/
│   ├── handlers.go      # HTTP handlers
│   └── jira.go          # Jira API handlers
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

### Jira Integration
When creating a ticket:
1. User clicks the "Jira" button next to a concern
2. Dashboard fetches open epics from the configured Jira project via `/rest/api/3/search/jql`
3. User selects an epic from the modal
4. Dashboard creates a Story via `/rest/api/3/issue` with:
   - Pre-populated summary based on the concern type
   - Detailed description with relevant information and links
   - Epic link to the selected epic
5. User receives a link to the created ticket

The Jira API uses Basic Auth with email and API token.

## Notes

- The dashboard fetches data in real-time from GitHub's API and the Go module proxy
- Rate limiting may apply for large numbers of repositories or dependencies
- Only open security alerts are shown by default
- Some security features require GitHub Advanced Security to be enabled on repositories
- Private Go modules won't have version info from the public proxy (shown as "N/A")
- Dependency checking uses concurrent requests with rate limiting (10 concurrent) to avoid overwhelming the proxy
- The Code Quality tab only appears when `SONARQUBE_URL` and `SONARQUBE_TOKEN` are configured
- If a repository's SonarQube project key differs from the repo name, specify it with `sonarqube_project` in repos.yaml
- Jira buttons only appear when all Atlassian environment variables are configured
- The Jira API token must have access to the configured project - regenerate the token if you recently gained project access
- Tickets are created as Stories linked to the selected Epic using `customfield_10002` (configurable via `JIRA_EPIC_LINK_FIELD`)
