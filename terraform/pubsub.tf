resource "google_pubsub_topic" "new_arrival" {
  name = "safety-incident.new-arrival"

  depends_on = [google_project_service.enabled]
}

resource "google_pubsub_topic" "new_arrival_dlq" {
  name = "safety-incident.new-arrival.dlq"

  depends_on = [google_project_service.enabled]
}

resource "google_pubsub_subscription" "notifier_new_arrival" {
  name  = "notifier-safety-incident-new-arrival"
  topic = google_pubsub_topic.new_arrival.id

  ack_deadline_seconds = 60

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.notifier.uri}/pubsub/push"

    oidc_token {
      service_account_email = google_service_account.notifier_runtime.email
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.new_arrival_dlq.id
    max_delivery_attempts = 5
  }

  depends_on = [google_cloud_run_v2_service.notifier]
}

# Allow ingestion runtime to publish.
resource "google_pubsub_topic_iam_member" "ingestion_publisher" {
  topic  = google_pubsub_topic.new_arrival.id
  role   = "roles/pubsub.publisher"
  member = "serviceAccount:${google_service_account.ingestion_runtime.email}"
}

# Pub/Sub needs permission to invoke the notifier Cloud Run service when it
# pushes messages. The default pubsub service account is
# service-${project_number}@gcp-sa-pubsub.iam.gserviceaccount.com
resource "google_cloud_run_v2_service_iam_member" "pubsub_invoker" {
  location = google_cloud_run_v2_service.notifier.location
  name     = google_cloud_run_v2_service.notifier.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
