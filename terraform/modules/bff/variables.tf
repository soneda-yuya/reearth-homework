variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "env" {
  description = "Deployment environment (dev / prod). Passed to the container as PLATFORM_ENV."
  type        = string
}

variable "image_tag" {
  description = "Image tag to deploy (typically the git SHA). CI overrides this per push."
  type        = string
}

variable "artifact_registry_url" {
  description = "Registry URL from the shared module, e.g. asia-northeast1-docker.pkg.dev/<project>/app"
  type        = string
}

variable "cms_base_url" {
  type = string
}

variable "cms_workspace_id" {
  type = string
}

variable "cms_integration_token_secret_name" {
  description = "Short name of the Secret Manager secret holding the CMS integration token."
  type        = string
}
