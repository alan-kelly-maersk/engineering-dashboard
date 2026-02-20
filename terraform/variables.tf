variable "environment" {
  type        = string
  description = "Deployment environment (int, prod)"
}

variable "container_image" {
  type        = string
  description = "Container image to deploy (e.g. ghcr.io/org/repo:sha)"
}

variable "cpu" {
  type        = number
  description = "CPU allocation for the container"
  default     = 0.25
}

variable "memory" {
  type        = string
  description = "Memory allocation for the container"
  default     = "0.5Gi"
}

# Application secrets

variable "github_token" {
  type        = string
  description = "GitHub personal access token"
  sensitive   = true
}

variable "sonarqube_token" {
  type        = string
  description = "SonarQube authentication token"
  sensitive   = true
}

variable "sonarqube_url" {
  type        = string
  description = "SonarQube server URL"
}

variable "atlassian_url" {
  type        = string
  description = "Atlassian instance URL"
}

variable "atlassian_cloud_id" {
  type        = string
  description = "Atlassian Cloud ID"
}

variable "atlassian_email" {
  type        = string
  description = "Atlassian account email"
}

variable "atlassian_api_token" {
  type        = string
  description = "Atlassian API token"
  sensitive   = true
}

variable "jira_project_key" {
  type        = string
  description = "Jira project key"
}

variable "jira_epic_link_field" {
  type        = string
  description = "Jira epic link custom field ID"
}

# Azure AD Authentication (optional)

variable "azure_client_id" {
  type        = string
  description = "Azure AD App Registration client ID"
  default     = ""
}

variable "azure_client_secret" {
  type        = string
  description = "Azure AD App Registration client secret"
  sensitive   = true
  default     = ""
}

variable "azure_tenant_id" {
  type        = string
  description = "Azure AD tenant ID"
  default     = ""
}

variable "azure_redirect_url" {
  type        = string
  description = "OAuth2 callback URL (e.g. https://your-app.azurecontainerapps.io/auth/callback)"
  default     = ""
}

variable "session_secret" {
  type        = string
  description = "Secret key for signing session cookies (base64 encoded, min 32 bytes)"
  sensitive   = true
  default     = ""
}
