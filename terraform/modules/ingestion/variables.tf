variable "project_id" {
  type = string
}

variable "project_number" {
  description = "GCP project number (numeric). Required to reference the Cloud Scheduler service agent."
  type        = string
}

variable "region" {
  type = string
}

variable "env" {
  type = string
}

variable "image_tag" {
  type = string
}

variable "artifact_registry_url" {
  type = string
}

variable "mofa_base_url" {
  type = string
}

variable "cms_base_url" {
  type = string
}

variable "cms_workspace_id" {
  type = string
}

variable "new_arrival_topic_id" {
  description = "Fully-qualified Pub/Sub topic id the job publishes NewArrivalEvent to (e.g. projects/.../topics/safety-incident.new-arrival)."
  type        = string
}

variable "cms_integration_token_secret_id" {
  type = string
}

variable "cms_integration_token_secret_name" {
  type = string
}

variable "claude_api_key_secret_id" {
  type = string
}

variable "claude_api_key_secret_name" {
  type = string
}

variable "mapbox_api_key_secret_id" {
  type = string
}

variable "mapbox_api_key_secret_name" {
  type = string
}

variable "schedule" {
  description = "Cron expression for the Cloud Scheduler job."
  type        = string
  default     = "*/5 * * * *"
}

variable "schedule_time_zone" {
  type    = string
  default = "Asia/Tokyo"
}
