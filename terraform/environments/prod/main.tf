provider "google" {
  project = var.project_id
  region  = var.region
}

provider "google-beta" {
  project = var.project_id
  region  = var.region
}

locals {
  env = "prod"
  # Must match the bucket declared in versions.tf backend block exactly.
  # Backend config cannot reference variables so we duplicate the literal
  # here deliberately; a mismatch would leave ci-deployer without access
  # to the real state bucket.
  tfstate_bucket = "overseas-safety-map-tfstate"
}

module "shared" {
  source = "../../modules/shared"

  project_id        = var.project_id
  project_number    = var.project_number
  region            = var.region
  github_repository = var.github_repository
  tfstate_bucket    = local.tfstate_bucket
}

module "bff" {
  source = "../../modules/bff"

  project_id                        = var.project_id
  region                            = var.region
  env                               = local.env
  image_tag                         = var.bff_image_tag
  artifact_registry_url             = module.shared.artifact_registry_url
  cms_base_url                      = var.cms_base_url
  cms_workspace_id                  = var.cms_workspace_id
  cms_integration_token_secret_id   = module.shared.cms_integration_token_secret_id
  cms_integration_token_secret_name = module.shared.cms_integration_token_secret_name
}

module "ingestion" {
  source = "../../modules/ingestion"

  project_id                        = var.project_id
  project_number                    = var.project_number
  region                            = var.region
  env                               = local.env
  image_tag                         = var.ingestion_image_tag
  artifact_registry_url             = module.shared.artifact_registry_url
  mofa_base_url                     = var.mofa_base_url
  cms_base_url                      = var.cms_base_url
  cms_workspace_id                  = var.cms_workspace_id
  new_arrival_topic_id              = module.shared.new_arrival_topic_id
  cms_integration_token_secret_id   = module.shared.cms_integration_token_secret_id
  cms_integration_token_secret_name = module.shared.cms_integration_token_secret_name
  claude_api_key_secret_id          = module.shared.ingestion_claude_secret_id
  claude_api_key_secret_name        = module.shared.ingestion_claude_secret_name
  mapbox_api_key_secret_id          = module.shared.ingestion_mapbox_secret_id
  mapbox_api_key_secret_name        = module.shared.ingestion_mapbox_secret_name
}

module "notifier" {
  source = "../../modules/notifier"

  project_id            = var.project_id
  project_number        = var.project_number
  region                = var.region
  env                   = local.env
  image_tag             = var.notifier_image_tag
  artifact_registry_url = module.shared.artifact_registry_url
  new_arrival_topic_id  = module.shared.new_arrival_topic_id
  new_arrival_dlq_id    = module.shared.new_arrival_dlq_id
}

module "cmsmigrate" {
  source = "../../modules/cmsmigrate"

  project_id                        = var.project_id
  region                            = var.region
  env                               = local.env
  image_tag                         = var.cmsmigrate_image_tag
  artifact_registry_url             = module.shared.artifact_registry_url
  cms_base_url                      = var.cms_base_url
  cms_workspace_id                  = var.cms_workspace_id
  cms_integration_token_secret_id   = module.shared.cms_integration_token_secret_id
  cms_integration_token_secret_name = module.shared.cms_integration_token_secret_name
}
