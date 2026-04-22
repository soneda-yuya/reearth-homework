# Shared Pub/Sub topic and DLQ. Per-application subscriptions live in their
# own module (see modules/notifier).

resource "google_pubsub_topic" "new_arrival" {
  name = "safety-incident.new-arrival"

  depends_on = [google_project_service.enabled]
}

resource "google_pubsub_topic" "new_arrival_dlq" {
  name = "safety-incident.new-arrival.dlq"

  depends_on = [google_project_service.enabled]
}
