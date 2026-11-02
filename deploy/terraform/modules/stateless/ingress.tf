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

resource "google_compute_global_address" "ingress_ip" {
  project = var.project_id
  name    = "${var.app_name}-${var.environment}-ingress-ip"
}

output "ingress_ip_address" {
  value = google_compute_global_address.ingress_ip.address
}
