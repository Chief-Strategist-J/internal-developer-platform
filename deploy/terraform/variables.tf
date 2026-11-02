variable "project_id" {
  description = "GCP Project ID"
  type        = string
}

variable "region" {
  description = "Primary GCP Region"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "Primary GCP Zone"
  type        = string
  default     = "us-central1-a"
}

variable "environment" {
  description = "Deployment environment tier (dev, staging, prod)"
  type        = string
}

variable "app_name" {
  description = "Application/Platform name"
  type        = string
  default     = "internal-developer-platform"
}

variable "gke_num_nodes" {
  description = "Number of GKE nodes per zone"
  type        = number
  default     = 2
}

variable "db_tier" {
  description = "Cloud SQL machine tier"
  type        = string
  default     = "db-custom-2-7680"
}
