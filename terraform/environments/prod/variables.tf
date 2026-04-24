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
  default = "soneda-yuya/overseas-safety-map"
}

# NOTE: the tfstate bucket name is deliberately NOT a variable. Terraform
# backend config is static (it cannot reference variables), so the bucket
# name lives in versions.tf as a literal. The same literal is reused as a
# local in main.tf so the ci-deployer IAM grant always targets the exact
# bucket used for backend state; making it variable would let an override
# silently misalign IAM and backend.

# ---- External configuration ------------------------------------------------
variable "mofa_base_url" {
  type = string
  # MOFA OpenData's *data* API lives at /opendata/area (NOT /html/opendata/).
  # The /html/opendata/ path serves a browsable sample page with only 3
  # placeholder items; the real production feed with hundreds of live
  # advisories is at the no-html path. Confirmed against MOFA's own manual:
  #   https://www.ezairyu.mofa.go.jp/html/opendata/support/usemanual.pdf
  #   (§2.4.1, table 2-5 — "新着情報のアクセス方法")
  # The code appends /newarrivalA.xml (incremental, 48h window) or
  # /00A.xml (initial, 1-year window) under this base.
  default = "https://www.ezairyu.mofa.go.jp/opendata/area"
}

variable "cms_base_url" {
  description = "Base URL of the external reearth-cms instance (e.g. https://cms.example.com). The CMS itself is managed outside this project."
  type        = string
}

variable "cms_workspace_id" {
  description = "Workspace ID in the external reearth-cms where the SafetyIncident schema is applied."
  type        = string
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

variable "cmsmigrate_image_tag" {
  type    = string
  default = "latest"
}
