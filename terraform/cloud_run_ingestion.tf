resource "google_cloud_run_v2_job" "ingestion" {
  name     = "ingestion"
  location = var.region

  template {
    template {
      service_account = google_service_account.ingestion_runtime.email
      timeout         = "300s"

      containers {
        image = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.app.repository_id}/ingestion:${var.ingestion_image_tag}"

        env {
          name  = "PLATFORM_SERVICE_NAME"
          value = "ingestion"
        }
        env {
          name  = "PLATFORM_ENV"
          value = "prod"
        }
        env {
          name  = "PLATFORM_GCP_PROJECT_ID"
          value = var.project_id
        }
        env {
          name  = "PLATFORM_OTEL_EXPORTER"
          value = "gcp"
        }
        env {
          name  = "INGESTION_MOFA_BASE_URL"
          value = var.mofa_base_url
        }
        env {
          name  = "INGESTION_PUBSUB_TOPIC"
          value = google_pubsub_topic.new_arrival.name
        }
        env {
          name  = "INGESTION_CMS_BASE_URL"
          value = var.cms_base_url
        }
        env {
          name  = "INGESTION_CMS_WORKSPACE_ID"
          value = var.cms_workspace_id
        }
        env {
          name = "INGESTION_CLAUDE_API_KEY"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.ingestion_claude_api_key.secret_id
              version = "latest"
            }
          }
        }
        env {
          name = "INGESTION_MAPBOX_API_KEY"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.ingestion_mapbox_api_key.secret_id
              version = "latest"
            }
          }
        }
        env {
          name = "INGESTION_CMS_INTEGRATION_TOKEN"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.cms_integration_token.secret_id
              version = "latest"
            }
          }
        }

        resources {
          limits = {
            cpu    = "1"
            memory = "512Mi"
          }
        }
      }
    }
  }

  depends_on = [
    google_project_service.enabled,
    google_secret_manager_secret_iam_member.ingestion_claude,
    google_secret_manager_secret_iam_member.ingestion_mapbox,
    google_secret_manager_secret_iam_member.ingestion_cms,
  ]
}
