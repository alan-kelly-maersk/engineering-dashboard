# output "app_fqdn" {
#   value       = azurerm_container_app.ca.ingress[0].fqdn
#   description = "FQDN of the deployed container app"
# }

# output "app_url" {
#   value       = "https://${azurerm_container_app.ca.ingress[0].fqdn}"
#   description = "Full URL of the deployed application"
# }

output "environment" {
  value       = var.environment
  description = "Deployment environment name"
}

# output "container_app_name" {
#   value       = azurerm_container_app.ca.name
#   description = "Name of the container app resource"
# }

output "public_ip_address" {
  value       = azurerm_public_ip.aks_ingress.ip_address
  description = "Public IP address of the AKS ingress"
}
