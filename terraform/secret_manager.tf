# Secret definitions only. Values are populated manually with
#   gcloud secrets versions add <name> --data-file=-
# to avoid committing secrets into terraform state history.

resource "google_secret_manager_secret" "ingestion_claude_api_key" {
  secret_id = "ingestion-claude-api-key"
  replication {
    auto {}
  }

  depends_on = [google_project_service.enabled]
}

resource "google_secret_manager_secret" "ingestion_mapbox_api_key" {
  secret_id = "ingestion-mapbox-api-key"
  replication {
    auto {}
  }

  depends_on = [google_project_service.enabled]
}

resource "google_secret_manager_secret" "cms_integration_token" {
  secret_id = "cms-integration-token"
  replication {
    auto {}
  }

  depends_on = [google_project_service.enabled]
}

# ---- IAM bindings: runtime SAs can read the secrets they need --------------
resource "google_secret_manager_secret_iam_member" "ingestion_claude" {
  secret_id = google_secret_manager_secret.ingestion_claude_api_key.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.ingestion_runtime.email}"
}

resource "google_secret_manager_secret_iam_member" "ingestion_mapbox" {
  secret_id = google_secret_manager_secret.ingestion_mapbox_api_key.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.ingestion_runtime.email}"
}

resource "google_secret_manager_secret_iam_member" "ingestion_cms" {
  secret_id = google_secret_manager_secret.cms_integration_token.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.ingestion_runtime.email}"
}

resource "google_secret_manager_secret_iam_member" "bff_cms" {
  secret_id = google_secret_manager_secret.cms_integration_token.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.bff_runtime.email}"
}

resource "google_secret_manager_secret_iam_member" "setup_cms" {
  secret_id = google_secret_manager_secret.cms_integration_token.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.setup_runtime.email}"
}
