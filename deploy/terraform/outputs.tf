output "gke_cluster_name" {
  description = "GKE Cluster Name"
  value       = module.stateless_compute.cluster_name
}

output "gke_cluster_endpoint" {
  description = "GKE Cluster API Endpoint"
  value       = module.stateless_compute.cluster_endpoint
}

output "database_connection_name" {
  description = "Cloud SQL Connection Name"
  value       = module.stateful_database.instance_connection_name
}

output "database_private_ip" {
  description = "Cloud SQL Private IP Address"
  value       = module.stateful_database.private_ip_address
}

output "redis_host" {
  description = "Memorystore Redis Host"
  value       = module.stateful_cache.host
}

output "redis_port" {
  description = "Memorystore Redis Port"
  value       = module.stateful_cache.port
}

output "artifacts_bucket" {
  description = "Storage Bucket for Artifacts"
  value       = module.stateful_storage.bucket_name
}

output "ingress_static_ip" {
  description = "Global Static IP for Ingress"
  value       = module.stateless_ingress.ingress_ip_address
}
