variable "project_id" {
  description = "GCP project ID (MVP uses a single prod project)."
  type        = string
  default     = "overseas-safety-map"
}

variable "project_number" {
  description = "GCP project number (numeric). Used in WIF principal set."
  type        = string
}

variable "region" {
  description = "Default region for all regional resources."
  type        = string
  default     = "asia-northeast1"
}

variable "github_repository" {
  description = "GitHub repo allowed to impersonate ci-deployer via WIF (owner/name)."
  type        = string
  default     = "soneda-yuya/reearth-homework"
}

# ---- Cloud Run image tags ---------------------------------------------------
# CI overrides these with -var='..._image_tag=<git-sha>' on each deploy.
variable "bff_image_tag" {
  description = "Image tag deployed to cloud_run.bff"
  type        = string
  default     = "latest"
}

variable "ingestion_image_tag" {
  description = "Image tag deployed to cloud_run.ingestion Job"
  type        = string
  default     = "latest"
}

variable "notifier_image_tag" {
  description = "Image tag deployed to cloud_run.notifier"
  type        = string
  default     = "latest"
}

variable "setup_image_tag" {
  description = "Image tag deployed to cloud_run.setup Job"
  type        = string
  default     = "latest"
}

# ---- External configuration (non-secret) -----------------------------------
variable "mofa_base_url" {
  description = "Base URL for MOFA open-data XML."
  type        = string
  default     = "https://www.ezairyu.mofa.go.jp/html/opendata"
}

variable "cms_base_url" {
  description = "reearth-cms instance base URL."
  type        = string
}

variable "cms_workspace_id" {
  description = "reearth-cms workspace id (manually created)."
  type        = string
}
