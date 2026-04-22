resource "google_cloud_run_v2_service" "notifier" {
  name     = "notifier"
  location = var.region

  ingress = "INGRESS_TRAFFIC_INTERNAL_ONLY"

  template {
    service_account = google_service_account.runtime.email

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = "${var.artifact_registry_url}/notifier:${var.image_tag}"

      ports {
        container_port = 8080
      }

      env {
        name  = "PLATFORM_SERVICE_NAME"
        value = "notifier"
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
        name  = "NOTIFIER_PORT"
        value = "8080"
      }
      env {
        name  = "NOTIFIER_PUBSUB_SUBSCRIPTION"
        value = "notifier-safety-incident-new-arrival"
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
}
