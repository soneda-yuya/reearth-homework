# Dedicated SA so Cloud Scheduler can invoke the ingestion Cloud Run Job.
resource "google_service_account" "scheduler_invoker" {
  account_id   = "scheduler-invoker"
  display_name = "Cloud Scheduler invoker"
}

resource "google_cloud_run_v2_job_iam_member" "scheduler_invokes_ingestion" {
  location = google_cloud_run_v2_job.ingestion.location
  name     = google_cloud_run_v2_job.ingestion.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.scheduler_invoker.email}"
}

resource "google_cloud_scheduler_job" "ingestion_new_arrival_5min" {
  name        = "ingestion-new-arrival-5min"
  description = "Run ingestion Cloud Run Job every 5 minutes"
  schedule    = "*/5 * * * *"
  time_zone   = "Asia/Tokyo"
  region      = var.region

  retry_config {
    retry_count = 0
  }

  http_target {
    http_method = "POST"
    uri         = "https://${var.region}-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/${var.project_id}/jobs/${google_cloud_run_v2_job.ingestion.name}:run"

    oauth_token {
      service_account_email = google_service_account.scheduler_invoker.email
      scope                 = "https://www.googleapis.com/auth/cloud-platform"
    }
  }

  depends_on = [google_project_service.enabled]
}
