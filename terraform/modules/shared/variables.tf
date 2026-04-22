variable "project_id" {
  description = "GCP project ID."
  type        = string
}

variable "project_number" {
  description = "GCP project number (numeric). Required to build the WIF audience URL that matches the one google-github-actions/auth@v2 mints by default."
  type        = string
}

variable "region" {
  description = "Default region for all regional resources."
  type        = string
}

variable "github_repository" {
  description = "GitHub repository allowed to impersonate ci-deployer (owner/name)."
  type        = string
}

variable "tfstate_bucket" {
  description = "Pre-existing GCS bucket that stores terraform state. Granted to ci-deployer via storage.objectAdmin."
  type        = string
}
