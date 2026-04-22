output "artifact_registry_repo" {
  description = "Docker registry URL to push deployable images into."
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.app.repository_id}"
}

output "bff_service_url" {
  description = "Public URL of the BFF Cloud Run service."
  value       = google_cloud_run_v2_service.bff.uri
}

output "notifier_service_url" {
  description = "Public URL of the notifier Cloud Run service (Pub/Sub push target)."
  value       = google_cloud_run_v2_service.notifier.uri
}

output "workload_identity_provider" {
  description = "Fully-qualified WIF provider name for GitHub Actions auth."
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "ci_deployer_sa_email" {
  description = "Service account email CI impersonates."
  value       = google_service_account.ci_deployer.email
}
