variable "project_id" {
  type = string
}

variable "environment" {
  type = string
}

variable "app_name" {
  type = string
}

variable "cluster_name" {
  type = string
}

output "autoscaling_status" {
  value = "Autoscaling managed natively via GKE cluster node pool autoscaler and HorizontalPodAutoscaler (HPA)"
}
