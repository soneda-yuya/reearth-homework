variable "project_id" {
  type = string
}

variable "project_number" {
  description = "Required to reference the default gcp-sa-pubsub account."
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

variable "new_arrival_topic_id" {
  description = "Topic id the subscription consumes from."
  type        = string
}

variable "new_arrival_dlq_id" {
  description = "Topic id messages are routed to after exceeding delivery attempts."
  type        = string
}
