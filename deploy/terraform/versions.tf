terraform {
  required_version = ">= 1.17.1"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 8.9.0"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "~> 8.9.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6.3"
    }
  }
}
