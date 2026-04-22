resource "google_pubsub_subscription" "new_arrival" {
  name  = "notifier-safety-incident-new-arrival"
  topic = var.new_arrival_topic_id

  ack_deadline_seconds = 60

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.notifier.uri}/pubsub/push"

    oidc_token {
      service_account_email = google_service_account.runtime.email
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  dead_letter_policy {
    dead_letter_topic     = var.new_arrival_dlq_id
    max_delivery_attempts = 5
  }
}
