terraform {
  backend "gcs" {
    bucket = "REPLACE_WITH_YOUR_TERRAFORM_STATE_BUCKET"
    prefix = "internal-developer-platform/state"
  }
}
