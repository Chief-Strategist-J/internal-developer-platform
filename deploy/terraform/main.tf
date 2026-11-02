provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

provider "google-beta" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

module "security" {
  source      = "./modules/security"
  project_id  = var.project_id
  region      = var.region
  environment = var.environment
  app_name    = var.app_name
}

module "kms" {
  source      = "./modules/security"
  project_id  = var.project_id
  region      = var.region
  environment = var.environment
  app_name    = var.app_name
}

module "stateful_database" {
  source      = "./modules/stateful"
  project_id  = var.project_id
  region      = var.region
  environment = var.environment
  app_name    = var.app_name
  db_tier     = var.db_tier
  network_id  = module.security.network_id
  kms_key_id  = module.kms.crypto_key_id

  depends_on = [module.security.peering_connection]
}

module "stateful_cache" {
  source      = "./modules/stateful"
  project_id  = var.project_id
  region      = var.region
  environment = var.environment
  app_name    = var.app_name
  network_id  = module.security.network_id

  depends_on = [module.security.peering_connection]
}

module "stateful_storage" {
  source      = "./modules/stateful"
  project_id  = var.project_id
  region      = var.region
  environment = var.environment
  app_name    = var.app_name
  kms_key_id  = module.kms.crypto_key_id
}

module "stateful_secrets" {
  source      = "./modules/stateful"
  project_id  = var.project_id
  region      = var.region
  environment = var.environment
  app_name    = var.app_name
  db_password = module.stateful_database.db_password
}

module "stateless_compute" {
  source                = "./modules/stateless"
  project_id            = var.project_id
  region                = var.region
  zone                  = var.zone
  environment           = var.environment
  app_name              = var.app_name
  network_name          = module.security.network_name
  subnet_name           = module.security.subnet_name
  gke_num_nodes         = var.gke_num_nodes
  service_account_email = module.security.gke_service_account_email
}

module "stateless_ingress" {
  source      = "./modules/stateless"
  project_id  = var.project_id
  region      = var.region
  environment = var.environment
  app_name    = var.app_name
}
