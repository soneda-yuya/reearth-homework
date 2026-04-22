# Secret *definitions* only. Values are populated manually with
#   gcloud secrets versions add <name> --data-file=-
# to keep secret material out of terraform state history.
# Per-service IAM accessor bindings live in each application module.

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
