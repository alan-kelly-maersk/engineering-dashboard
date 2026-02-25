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
  prefix               = "l7se-do-${var.environment}"
  container_image_name = replace(var.container_image, "ghcr.io/", "")
}

data "azurerm_resource_group" "rg" {
  name = "l7se-do"
}

# ── App Service ──────────────────────────────────────────────────────────────

resource "azurerm_service_plan" "asp" {
  name                = "${local.prefix}-asp"
  resource_group_name = data.azurerm_resource_group.rg.name
  location            = data.azurerm_resource_group.rg.location
  os_type             = "Linux"
  sku_name            = var.app_service_sku

  tags = {
    app_id      = "A22087"
    environment = var.environment
  }
}

resource "azurerm_linux_web_app" "app" {
  name                = "${local.prefix}-app"
  resource_group_name = data.azurerm_resource_group.rg.name
  location            = data.azurerm_resource_group.rg.location
  service_plan_id     = azurerm_service_plan.asp.id
  https_only          = true

  site_config {
    application_stack {
      docker_image_name        = local.container_image_name
      docker_registry_url      = "https://ghcr.io"
      docker_registry_username = "ghcr-token"
      docker_registry_password = var.github_token
    }
  }

  app_settings = {
    WEBSITES_PORT        = "8080"
    GITHUB_TOKEN         = var.github_token
    SONARQUBE_TOKEN      = var.sonarqube_token
    SONARQUBE_URL        = var.sonarqube_url
    ATLASSIAN_URL        = var.atlassian_url
    ATLASSIAN_CLOUD_ID   = var.atlassian_cloud_id
    ATLASSIAN_EMAIL      = var.atlassian_email
    ATLASSIAN_API_TOKEN  = var.atlassian_api_token
    JIRA_PROJECT_KEY     = var.jira_project_key
    JIRA_EPIC_LINK_FIELD = var.jira_epic_link_field
    AZURE_CLIENT_ID      = var.azure_client_id
    AZURE_CLIENT_SECRET  = var.azure_client_secret
    AZURE_TENANT_ID      = var.azure_tenant_id
    AZURE_REDIRECT_URL   = var.azure_redirect_url
    SESSION_SECRET       = var.session_secret
  }

  tags = {
    app_id      = "A22087"
    environment = var.environment
  }
}

# ── AKS (commented out – kept for potential future migration) ────────────────

# resource "azurerm_container_app_environment" "cae" {
#   name                = "${local.prefix}-cae"
#   resource_group_name = data.azurerm_resource_group.rg.name
#   location            = data.azurerm_resource_group.rg.location
# }

# resource "azurerm_container_app" "ca" {
#   name                         = "${local.prefix}-ca"
#   container_app_environment_id = azurerm_container_app_environment.cae.id
#   resource_group_name          = data.azurerm_resource_group.rg.name
#   revision_mode                = "Single"
#
#   registry {
#     server               = "ghcr.io"
#     username             = "ghcr-token"
#     password_secret_name = "github-token"
#   }
#
#   secret {
#     name  = "github-token"
#     value = var.github_token
#   }
#
#   template {
#     container {
#       name   = "${local.prefix}-ca"
#       image  = var.container_image
#       cpu    = var.cpu
#       memory = var.memory
#     }
#   }
#
#   ingress {
#     external_enabled = true
#     target_port      = 8080
#
#     traffic_weight {
#       percentage      = 100
#       latest_revision = true
#     }
#   }
# }

# resource "azurerm_public_ip" "aks_ingress" {
#   name                = "aks-ingress-publicip"
#   location            = data.azurerm_resource_group.rg.location
#   resource_group_name = data.azurerm_resource_group.rg.name
#   allocation_method   = "Static"
#   sku                 = "Standard"
#   domain_name_label   = "l7sedoaks"
#
#   tags = {
#     app_id      = "A22087"
#     environment = "production"
#   }
# }

# resource "azurerm_kubernetes_cluster" "l7sedoaks" {
#   name                = "l7sedoaks"
#   location            = data.azurerm_resource_group.rg.location
#   resource_group_name = data.azurerm_resource_group.rg.name
#   dns_prefix          = "l7sedoaks"
#
#   default_node_pool {
#     name       = "default"
#     node_count = 2
#     vm_size    = "Standard_D2_v2"
#
#     upgrade_settings {
#       drain_timeout_in_minutes      = 0
#       max_surge                     = "10%"
#       node_soak_duration_in_minutes = 0
#     }
#   }
#
#   network_profile {
#     network_plugin = "azure"
#     load_balancer_profile {
#       outbound_ip_address_ids = [azurerm_public_ip.aks_ingress.id]
#     }
#   }
#
#   identity {
#     type = "SystemAssigned"
#   }
#
#   tags = {
#     app_id = "A22087"
#   }
# }