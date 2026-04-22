resource "google_project_iam_member" "datastore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_project_iam_member" "fcm" {
  project = var.project_id
  role    = "roles/cloudmessaging.messagesSender"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

# Pub/Sub needs to invoke the notifier service when pushing messages.
resource "google_cloud_run_v2_service_iam_member" "pubsub_invoker" {
  location = google_cloud_run_v2_service.notifier.location
  name     = google_cloud_run_v2_service.notifier.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

# Pub/Sub push with oidc_token requires the Pub/Sub service agent to be able
# to mint identity tokens as the runtime SA. Without this, deliveries fail
# and messages eventually go to the DLQ.
resource "google_service_account_iam_member" "pubsub_token_creator" {
  service_account_id = google_service_account.runtime.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
