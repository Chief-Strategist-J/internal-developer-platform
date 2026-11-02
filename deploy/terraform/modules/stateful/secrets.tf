variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "environment" {
  type = string
}

variable "app_name" {
  type = string
}

variable "db_password" {
  type      = string
  sensitive = true
}

resource "google_secret_manager_secret" "db_password_secret" {
  project   = var.project_id
  secret_id = "${var.app_name}-${var.environment}-db-password"

  replication {
    auto {}
  }

  labels = {
    environment = var.environment
    managed_by  = "terraform"
  }
}

resource "google_secret_manager_secret_version" "db_password_version" {
  secret      = google_secret_manager_secret.db_password_secret.id
  secret_data = var.db_password
}

output "secret_id" {
  value = google_secret_manager_secret.db_password_secret.secret_id
}

output "secret_name" {
  value = google_secret_manager_secret.db_password_secret.name
}
