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

# Cloud Scheduler mints an OAuth token as scheduler_invoker on every fire,
# which requires the Cloud Scheduler service agent to be allowed to
# impersonate the invoker SA. Without this binding, the first tick fails
# with a token-generation error.
resource "google_service_account_iam_member" "scheduler_agent_token_creator" {
  service_account_id = google_service_account.scheduler_invoker.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:service-${var.project_number}@gcp-sa-cloudscheduler.iam.gserviceaccount.com"
}

resource "google_cloud_scheduler_job" "ingestion" {
  name        = "ingestion-new-arrival-5min"
  description = "Run the ingestion Cloud Run Job on a fixed cadence."
  schedule    = var.schedule
  time_zone   = var.schedule_time_zone
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

  # The IAM bindings below must land before the first scheduled tick; they are
  # not in the attribute graph so Terraform would otherwise create resources
  # in parallel and the first tick(s) could error with permission / token
  # generation failures.
  depends_on = [
    google_cloud_run_v2_job_iam_member.scheduler_invokes_ingestion,
    google_service_account_iam_member.scheduler_agent_token_creator,
  ]
}
