#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# GCP Bootstrap & Terraform Automation Script for Internal Developer Platform
# ==============================================================================

PROJECT_ID="${1:-}"
REGION="${2:-us-central1}"
BILLING_ACCOUNT_ID="${3:-}"

if [[ -z "${PROJECT_ID}" ]]; then
  echo "Usage: $0 <gcp-project-id> [region] [billing-account-id]" >&2
  exit 1
fi

echo "==> 1. Setting active GCP project to: ${PROJECT_ID}"
if ! gcloud projects describe "${PROJECT_ID}" >/dev/null 2>&1; then
  echo "Project ${PROJECT_ID} does not exist. Creating..."
  gcloud projects create "${PROJECT_ID}" --name="IDP Platform (${PROJECT_ID})"
  
  if [[ -n "${BILLING_ACCOUNT_ID}" ]]; then
    echo "Linking billing account ${BILLING_ACCOUNT_ID}..."
    gcloud billing projects link "${PROJECT_ID}" --billing-account="${BILLING_ACCOUNT_ID}"
  else
    echo "WARNING: No billing account provided. Ensure billing is linked or deployment will fail." >&2
  fi
fi

gcloud config set project "${PROJECT_ID}"

echo "==> 2. Enabling all required Google Cloud APIs..."
APIS=(
  "container.googleapis.com"
  "compute.googleapis.com"
  "sqladmin.googleapis.com"
  "redis.googleapis.com"
  "servicenetworking.googleapis.com"
  "secretmanager.googleapis.com"
  "cloudkms.googleapis.com"
  "storage.googleapis.com"
  "artifactregistry.googleapis.com"
  "iam.googleapis.com"
  "cloudresourcemanager.googleapis.com"
  "logging.googleapis.com"
  "monitoring.googleapis.com"
)

gcloud services enable "${APIS[@]}"

echo "==> 3. Creating Terraform Remote State GCS Bucket..."
TF_STATE_BUCKET="${PROJECT_ID}-tf-state"
if ! gcloud storage buckets describe "gs://${TF_STATE_BUCKET}" >/dev/null 2>&1; then
  echo "Creating state bucket gs://${TF_STATE_BUCKET}..."
  gcloud storage buckets create "gs://${TF_STATE_BUCKET}" \
    --project="${PROJECT_ID}" \
    --location="${REGION}" \
    --uniform-bucket-level-access
  
  gcloud storage buckets update "gs://${TF_STATE_BUCKET}" --versioning
else
  echo "State bucket gs://${TF_STATE_BUCKET} already exists."
fi

echo "==> 4. Updating Terraform backend config with state bucket..."
sed -i "s/bucket = .*/bucket = \"${TF_STATE_BUCKET}\"/" deploy/terraform/backend.tf

echo "==> 5. Initializing Terraform..."
cd deploy/terraform
terraform init

echo "==> 6. Running Terraform Plan..."
terraform plan -var="project_id=${PROJECT_ID}" -var="region=${REGION}" -var-file="environments/dev.tfvars"

echo "==> Bootstrap and validation complete!"
echo "To apply, run: cd deploy/terraform && terraform apply -var=\"project_id=${PROJECT_ID}\" -var-file=\"environments/dev.tfvars\""
