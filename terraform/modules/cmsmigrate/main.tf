resource "google_cloud_run_v2_job" "cmsmigrate" {
  name     = "cms-migrate"
  location = var.region

  template {
    template {
      service_account = google_service_account.runtime.email
      timeout         = "120s"

      containers {
        image = "${var.artifact_registry_url}/cmsmigrate:${var.image_tag}"

        env {
          name  = "PLATFORM_SERVICE_NAME"
          value = "cmsmigrate"
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
          name  = "CMSMIGRATE_CMS_BASE_URL"
          value = var.cms_base_url
        }
        env {
          name  = "CMSMIGRATE_CMS_WORKSPACE_ID"
          value = var.cms_workspace_id
        }
        env {
          name = "CMSMIGRATE_CMS_INTEGRATION_TOKEN"
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
