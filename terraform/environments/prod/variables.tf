variable "project_id" {
  description = "GCP project ID for the prod environment."
  type        = string
  default     = "overseas-safety-map"
}

variable "project_number" {
  description = "GCP project number (numeric). Provide via CI or tfvars."
  type        = string
}

variable "region" {
  type    = string
  default = "asia-northeast1"
}

variable "github_repository" {
  type    = string
  default = "soneda-yuya/reearth-homework"
}

variable "tfstate_bucket" {
  description = "GCS bucket holding terraform state. Created manually during bootstrap."
  type        = string
  default     = "overseas-safety-map-tfstate"
}

# ---- External configuration ------------------------------------------------
variable "mofa_base_url" {
  type    = string
  default = "https://www.ezairyu.mofa.go.jp/html/opendata"
}

variable "cms_base_url" {
  type = string
}

variable "cms_workspace_id" {
  type = string
}

# ---- Image tags (CI overrides these per deploy) ----------------------------
variable "bff_image_tag" {
  type    = string
  default = "latest"
}

variable "ingestion_image_tag" {
  type    = string
  default = "latest"
}

variable "notifier_image_tag" {
  type    = string
  default = "latest"
}

variable "setup_image_tag" {
  type    = string
  default = "latest"
}
