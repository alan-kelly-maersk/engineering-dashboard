package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client handles communication with Jira API
type Client struct {
	baseURL       string
	cloudID       string // Atlassian Cloud ID for API calls
	email         string
	token         string
	projectKey    string
	epicLinkField string
	httpClient    *http.Client
}

// NewClient creates a new Jira API client
func NewClient(baseURL, cloudID, email, token, projectKey, epicLinkField string) *Client {
	baseURL = strings.TrimSuffix(baseURL, "/")

	if epicLinkField == "" {
		epicLinkField = "customfield_10002" // Default for classic Jira projects
	}

	return &Client{
		baseURL:       baseURL,
		cloudID:       cloudID,
		email:         email,
		token:         token,
		projectKey:    projectKey,
		epicLinkField: epicLinkField,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured returns true if the client has valid configuration
func (c *Client) IsConfigured() bool {
	return c.baseURL != "" && c.cloudID != "" && c.email != "" && c.token != "" && c.projectKey != ""
}

// apiURL returns the correct API base URL using the cloud ID
func (c *Client) apiURL() string {
	return fmt.Sprintf("https://api.atlassian.com/ex/jira/%s", c.cloudID)
}

// GetProjectKey returns the configured project key
func (c *Client) GetProjectKey() string {
	return c.projectKey
}

// GetBaseURL returns the configured base URL
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// Epic represents a Jira epic
type Epic struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
}

// CreateIssueRequest contains the data needed to create a Jira issue
type CreateIssueRequest struct {
	Summary     string `json:"summary"`
	Description string `json:"description"`
	EpicKey     string `json:"epic_key,omitempty"`
}

// CreateIssueResponse contains the response from creating a Jira issue
type CreateIssueResponse struct {
	Key  string `json:"key"`
	ID   string `json:"id"`
	Self string `json:"self"`
	URL  string `json:"url"` // Constructed browse URL
}

// jiraSearchResponse represents the JQL search response
type jiraSearchResponse struct {
	Issues []struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
		} `json:"fields"`
	} `json:"issues"`
	Total int `json:"total"`
}

// jiraCreateResponse represents the create issue response
type jiraCreateResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

// GetEpics fetches all open epics in the configured project
func (c *Client) GetEpics() ([]Epic, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("jira client not configured")
	}

	// JQL to find open epics in the project
	jql := fmt.Sprintf("project = %s AND type = Epic AND resolution = Unresolved ORDER BY created DESC", c.projectKey)

	// Use the /rest/api/3/search/jql endpoint with POST
	searchURL := fmt.Sprintf("%s/rest/api/3/search/jql", c.baseURL)

	requestBody := map[string]interface{}{
		"jql":        jql,
		"fields":     []string{"summary"},
		"maxResults": 100,
	}
	jsonBody, _ := json.Marshal(requestBody)

	resp, err := c.doRequest("POST", searchURL, jsonBody)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch epics: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Println("Failed to close response body", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jira API returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp jiraSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse epics response: %w", err)
	}

	epics := make([]Epic, len(searchResp.Issues))
	for i, issue := range searchResp.Issues {
		epics[i] = Epic{
			Key:     issue.Key,
			Summary: issue.Fields.Summary,
		}
	}

	return epics, nil
}

// CreateStory creates a new Story issue linked to an epic
func (c *Client) CreateStory(req CreateIssueRequest) (*CreateIssueResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("jira client not configured")
	}

	// Build the request body using Atlassian Document Format for description
	fields := map[string]interface{}{
		"project": map[string]string{
			"key": c.projectKey,
		},
		"issuetype": map[string]string{
			"name": "Story",
		},
		"summary": req.Summary,
	}

	// Add description in Atlassian Document Format (ADF)
	if req.Description != "" {
		fields["description"] = map[string]interface{}{
			"type":    "doc",
			"version": 1,
			"content": c.markdownToADF(req.Description),
		}
	}

	// Add epic link if provided
	if req.EpicKey != "" {
		fields[c.epicLinkField] = req.EpicKey
	}

	body := map[string]interface{}{
		"fields": fields,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	createURL := fmt.Sprintf("%s/rest/api/3/issue", c.apiURL())

	resp, err := c.doRequest("POST", createURL, jsonBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Println("Failed to close response body", err)
		}
	}()
	if resp.StatusCode != http.StatusCreated {
		var errBody map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&errBody)
		if err != nil {
			return nil, fmt.Errorf("failed to parse error body: %w", err)
		}
		return nil, fmt.Errorf("jira API returned status %d: %v", resp.StatusCode, errBody)
	}

	var createResp jiraCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return nil, fmt.Errorf("failed to parse create response: %w", err)
	}

	return &CreateIssueResponse{
		Key:  createResp.Key,
		ID:   createResp.ID,
		Self: createResp.Self,
		URL:  fmt.Sprintf("%s/browse/%s", c.baseURL, createResp.Key),
	}, nil
}

// markdownToADF converts simple markdown to Atlassian Document Format
func (c *Client) markdownToADF(markdown string) []interface{} {
	var content []interface{}

	lines := strings.Split(markdown, "\n")
	var currentParagraph []interface{}

	flushParagraph := func() {
		if len(currentParagraph) > 0 {
			content = append(content, map[string]interface{}{
				"type":    "paragraph",
				"content": currentParagraph,
			})
			currentParagraph = nil
		}
	}

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		// Handle headers
		if strings.HasPrefix(line, "### ") {
			flushParagraph()
			content = append(content, map[string]interface{}{
				"type": "heading",
				"attrs": map[string]int{
					"level": 3,
				},
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": strings.TrimPrefix(line, "### "),
					},
				},
			})
			continue
		}
		if strings.HasPrefix(line, "## ") {
			flushParagraph()
			content = append(content, map[string]interface{}{
				"type": "heading",
				"attrs": map[string]int{
					"level": 2,
				},
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": strings.TrimPrefix(line, "## "),
					},
				},
			})
			continue
		}

		// Handle bullet points
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			flushParagraph()
			content = append(content, map[string]interface{}{
				"type": "bulletList",
				"content": []interface{}{
					map[string]interface{}{
						"type": "listItem",
						"content": []interface{}{
							map[string]interface{}{
								"type": "paragraph",
								"content": []interface{}{
									map[string]interface{}{
										"type": "text",
										"text": strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* "),
									},
								},
							},
						},
					},
				},
			})
			continue
		}

		// Handle empty lines
		if line == "" {
			flushParagraph()
			continue
		}

		// Regular text - add to current paragraph
		if len(currentParagraph) > 0 {
			currentParagraph = append(currentParagraph, map[string]interface{}{
				"type": "text",
				"text": " ",
			})
		}
		currentParagraph = append(currentParagraph, map[string]interface{}{
			"type": "text",
			"text": line,
		})
	}

	flushParagraph()

	// Ensure we have at least one content block
	if len(content) == 0 {
		content = append(content, map[string]interface{}{
			"type": "paragraph",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": markdown,
				},
			},
		})
	}

	return content
}

func (c *Client) doRequest(method, url string, body []byte) (*http.Response, error) {
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}

	// Jira Cloud uses basic auth with email and API token
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}
