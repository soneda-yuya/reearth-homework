resource "google_cloud_run_v2_job" "setup" {
  name     = "setup"
  location = var.region

  template {
    template {
      service_account = google_service_account.runtime.email
      timeout         = "120s"

      containers {
        image = "${var.artifact_registry_url}/setup:${var.image_tag}"

        env {
          name  = "PLATFORM_SERVICE_NAME"
          value = "setup"
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
          name  = "SETUP_CMS_BASE_URL"
          value = var.cms_base_url
        }
        env {
          name  = "SETUP_CMS_WORKSPACE_ID"
          value = var.cms_workspace_id
        }
        env {
          name = "SETUP_CMS_INTEGRATION_TOKEN"
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
            memory = "256Mi"
          }
        }
      }
    }
  }

  depends_on = [google_secret_manager_secret_iam_member.cms]
}
