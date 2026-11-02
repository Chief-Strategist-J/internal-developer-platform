variable "project_id" {
  type = string
}

variable "environment" {
  type = string
}

variable "app_name" {
  type = string
}

resource "google_service_account" "gke_sa" {
  project      = var.project_id
  account_id   = "${var.app_name}-${var.environment}-gke-sa"
  display_name = "GKE Nodes Service Account for ${var.app_name}"
}

resource "google_project_iam_member" "gke_log_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.gke_sa.email}"
}

resource "google_project_iam_member" "gke_metric_writer" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.gke_sa.email}"
}

resource "google_project_iam_member" "gke_monitoring_viewer" {
  project = var.project_id
  role    = "roles/monitoring.viewer"
  member  = "serviceAccount:${google_service_account.gke_sa.email}"
}

resource "google_project_iam_member" "gke_artifact_reader" {
  project = var.project_id
  role    = "roles/artifactregistry.reader"
  member  = "serviceAccount:${google_service_account.gke_sa.email}"
}

resource "google_service_account" "workload_identity_sa" {
  project      = var.project_id
  account_id   = "${var.app_name}-${var.environment}-workload"
  display_name = "Workload Identity Service Account for IDP applications"
}

resource "google_service_account_iam_member" "workload_identity_binding" {
  service_account_id = google_service_account.workload_identity_sa.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[default/platform-api]"
}

output "gke_service_account_email" {
  value = google_service_account.gke_sa.email
}

output "workload_service_account_email" {
  value = google_service_account.workload_identity_sa.email
}
