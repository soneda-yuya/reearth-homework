resource "google_artifact_registry_repository" "app" {
  provider      = google
  location      = var.region
  repository_id = "app"
  format        = "DOCKER"
  description   = "Container images for ingestion / bff / notifier / setup"

  depends_on = [google_project_service.enabled]
}
