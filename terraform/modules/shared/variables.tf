variable "project_id" {
  description = "GCP project ID."
  type        = string
}

variable "project_number" {
  description = "GCP project number (numeric)."
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
