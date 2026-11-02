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

variable "kms_key_id" {
  type = string
}

resource "random_id" "bucket_suffix" {
  byte_length = 4
}

resource "google_storage_bucket" "artifacts" {
  project                     = var.project_id
  name                        = "${var.app_name}-${var.environment}-artifacts-${random_id.bucket_suffix.hex}"
  location                    = var.region
  force_destroy               = var.environment == "prod" ? false : true
  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }

  encryption {
    default_kms_key_name = var.kms_key_id
  }

  lifecycle_rule {
    condition {
      age = 90
    }
    action {
      type = "Delete"
    }
  }

  labels = {
    environment = var.environment
    managed_by  = "terraform"
  }
}

output "bucket_name" {
  value = google_storage_bucket.artifacts.name
}

output "bucket_url" {
  value = google_storage_bucket.artifacts.url
}
