output "environment" {
  value       = var.environment
  description = "Deployment environment name"
}

output "app_default_hostname" {
  value       = azurerm_linux_web_app.app.default_hostname
  description = "Default hostname of the App Service (e.g. <name>.azurewebsites.net)"
}

output "app_url" {
  value       = "https://${azurerm_linux_web_app.app.default_hostname}"
  description = "Full HTTPS URL of the deployed application"
}

output "app_service_name" {
  value       = azurerm_linux_web_app.app.name
  description = "Name of the App Service resource"
}

# ── Commented-out outputs from previous AKS / Container App setup ────────────

# output "app_fqdn" {
#   value       = azurerm_container_app.ca.ingress[0].fqdn
#   description = "FQDN of the deployed container app"
# }

# output "public_ip_address" {
#   value       = azurerm_public_ip.aks_ingress.ip_address
#   description = "Public IP address of the AKS ingress"
# }