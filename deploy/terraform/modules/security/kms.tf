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

resource "google_kms_key_ring" "keyring" {
  project  = var.project_id
  name     = "${var.app_name}-${var.environment}-keyring"
  location = var.region
}

resource "google_kms_crypto_key" "app_key" {
  name            = "${var.app_name}-${var.environment}-key"
  key_ring        = google_kms_key_ring.keyring.id
  rotation_period = "7776000s" # 90 days

  lifecycle {
    prevent_destroy = true
  }
}

output "key_ring_id" {
  value = google_kms_key_ring.keyring.id
}

output "crypto_key_id" {
  value = google_kms_crypto_key.app_key.id
}
