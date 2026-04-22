output "artifact_registry_repo_id" {
  description = "The Artifact Registry repository ID (short name)."
  value       = google_artifact_registry_repository.app.repository_id
}

output "artifact_registry_url" {
  description = "Full Docker registry URL for pushing images."
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.app.repository_id}"
}

output "new_arrival_topic_id" {
  description = "Fully-qualified topic id for safety-incident.new-arrival."
  value       = google_pubsub_topic.new_arrival.id
}

output "new_arrival_topic_name" {
  description = "Short topic name for safety-incident.new-arrival."
  value       = google_pubsub_topic.new_arrival.name
}

output "new_arrival_dlq_id" {
  description = "Fully-qualified id for the DLQ topic."
  value       = google_pubsub_topic.new_arrival_dlq.id
}

output "ingestion_claude_secret_id" {
  value = google_secret_manager_secret.ingestion_claude_api_key.id
}

output "ingestion_claude_secret_name" {
  value = google_secret_manager_secret.ingestion_claude_api_key.secret_id
}

output "ingestion_mapbox_secret_id" {
  value = google_secret_manager_secret.ingestion_mapbox_api_key.id
}

output "ingestion_mapbox_secret_name" {
  value = google_secret_manager_secret.ingestion_mapbox_api_key.secret_id
}

output "cms_integration_token_secret_id" {
  value = google_secret_manager_secret.cms_integration_token.id
}

output "cms_integration_token_secret_name" {
  value = google_secret_manager_secret.cms_integration_token.secret_id
}

output "workload_identity_provider" {
  description = "Fully-qualified WIF provider resource name for GitHub Actions."
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "ci_deployer_email" {
  value = google_service_account.ci_deployer.email
}
