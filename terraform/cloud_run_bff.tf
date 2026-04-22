resource "google_cloud_run_v2_service" "bff" {
  name     = "bff"
  location = var.region

  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.bff_runtime.email

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    containers {
      image = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.app.repository_id}/bff:${var.bff_image_tag}"

      ports {
        container_port = 8080
      }

      env {
        name  = "PLATFORM_SERVICE_NAME"
        value = "bff"
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
        name  = "BFF_PORT"
        value = "8080"
      }
      env {
        name  = "BFF_CMS_BASE_URL"
        value = var.cms_base_url
      }
      env {
        name  = "BFF_CMS_WORKSPACE_ID"
        value = var.cms_workspace_id
      }
      env {
        name = "BFF_CMS_INTEGRATION_TOKEN"
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

      startup_probe {
        http_get {
          path = "/healthz"
        }
        initial_delay_seconds = 3
        period_seconds        = 5
        failure_threshold     = 3
        timeout_seconds       = 2
      }

      liveness_probe {
        http_get {
          path = "/healthz"
        }
        period_seconds    = 30
        failure_threshold = 3
        timeout_seconds   = 2
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  depends_on = [
    google_project_service.enabled,
    google_secret_manager_secret_iam_member.bff_cms,
  ]
}

# Allow unauthenticated access (Flutter app authenticates via Firebase ID Token
# in the Authorization header, validated by the AuthInterceptor).
resource "google_cloud_run_v2_service_iam_member" "bff_invoker" {
  location = google_cloud_run_v2_service.bff.location
  name     = google_cloud_run_v2_service.bff.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
