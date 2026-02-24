terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }

  backend "azurerm" {
    resource_group_name  = "l7se-do"
    storage_account_name = "l7sedotfstate"
    container_name       = "tfstate"
    key                  = "engineering-dashboard.tfstate"
    use_azuread_auth     = true
  }
}

provider "azurerm" {
  features {}
}

locals {
  prefix = "l7se-do-${var.environment}"
}

data "azurerm_resource_group" "rg" {
  name = "l7se-do"
}

resource "azurerm_container_app_environment" "cae" {
  name                = "${local.prefix}-cae"
  resource_group_name = data.azurerm_resource_group.rg.name
  location            = data.azurerm_resource_group.rg.location
}

resource "azurerm_container_app" "ca" {
  name                         = "${local.prefix}-ca"
  container_app_environment_id = azurerm_container_app_environment.cae.id
  resource_group_name          = data.azurerm_resource_group.rg.name
  revision_mode                = "Single"

  registry {
    server               = "ghcr.io"
    username             = "ghcr-token"
    password_secret_name = "github-token"
  }

  secret {
    name  = "github-token"
    value = var.github_token
  }

  template {
    container {
      name   = "${local.prefix}-ca"
      image  = var.container_image
      cpu    = var.cpu
      memory = var.memory

      env {
        name  = "GITHUB_TOKEN"
        value = var.github_token
      }
      env {
        name  = "SONARQUBE_TOKEN"
        value = var.sonarqube_token
      }
      env {
        name  = "SONARQUBE_URL"
        value = var.sonarqube_url
      }
      env {
        name  = "ATLASSIAN_URL"
        value = var.atlassian_url
      }
      env {
        name  = "ATLASSIAN_CLOUD_ID"
        value = var.atlassian_cloud_id
      }
      env {
        name  = "ATLASSIAN_EMAIL"
        value = var.atlassian_email
      }
      env {
        name  = "ATLASSIAN_API_TOKEN"
        value = var.atlassian_api_token
      }
      env {
        name  = "JIRA_PROJECT_KEY"
        value = var.jira_project_key
      }
      env {
        name  = "JIRA_EPIC_LINK_FIELD"
        value = var.jira_epic_link_field
      }
      env {
        name  = "AZURE_CLIENT_ID"
        value = var.azure_client_id
      }
      env {
        name  = "AZURE_CLIENT_SECRET"
        value = var.azure_client_secret
      }
      env {
        name  = "AZURE_TENANT_ID"
        value = var.azure_tenant_id
      }
      env {
        name  = "AZURE_REDIRECT_URL"
        value = var.azure_redirect_url
      }
      env {
        name  = "SESSION_SECRET"
        value = var.session_secret
      }
    }
  }

  ingress {
    external_enabled = true
    target_port      = 8080

    traffic_weight {
      percentage      = 100
      latest_revision = true
    }
  }
}
