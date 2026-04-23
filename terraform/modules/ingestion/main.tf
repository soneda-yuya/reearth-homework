resource "google_cloud_run_v2_job" "ingestion" {
  name     = "ingestion"
  location = var.region

  template {
    template {
      service_account = google_service_account.runtime.email
      timeout         = "300s"
      max_retries     = 0

      containers {
        image = "${var.artifact_registry_url}/ingestion:${var.image_tag}"

        env {
          name  = "INGESTION_MODE"
          value = "incremental"
        }
        env {
          name  = "INGESTION_PUBSUB_TOPIC_ID"
          value = var.new_arrival_topic_id
        }

        env {
          name  = "PLATFORM_SERVICE_NAME"
          value = "ingestion"
        }
        env {
          name  = "PLATFORM_ENV"
          value = var.env
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
              secret  = var.claude_api_key_secret_name
              version = "latest"
            }
          }
        }
        env {
          name = "INGESTION_MAPBOX_API_KEY"
          value_source {
            secret_key_ref {
              secret  = var.mapbox_api_key_secret_name
              version = "latest"
            }
          }
        }
        env {
          name = "INGESTION_CMS_INTEGRATION_TOKEN"
          value_source {
            secret_key_ref {
              secret  = var.cms_integration_token_secret_name
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
    google_secret_manager_secret_iam_member.claude,
    google_secret_manager_secret_iam_member.mapbox,
    google_secret_manager_secret_iam_member.cms,
  ]
}
