resource "google_project_iam_member" "datastore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_project_iam_member" "fcm" {
  project = var.project_id
  # roles/cloudmessaging.messagesSender is a legacy role and is not supported
  # at the project scope. For FCM v1 HTTP API sends via Firebase Admin SDK,
  # the project-level role is roles/firebasecloudmessaging.admin.
  role   = "roles/firebasecloudmessaging.admin"
  member = "serviceAccount:${google_service_account.runtime.email}"
}

# Pub/Sub push delivers with an OIDC token signed by the runtime SA
# (see subscription.tf push_config.oidc_token). Cloud Run checks that the
# token subject — not the Pub/Sub service agent — holds roles/run.invoker.
# Granting run.invoker to the runtime SA is therefore the correct way to
# authorise Pub/Sub push.
resource "google_cloud_run_v2_service_iam_member" "runtime_invoker" {
  location = google_cloud_run_v2_service.notifier.location
  name     = google_cloud_run_v2_service.notifier.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.runtime.email}"
}

# Pub/Sub push with oidc_token requires the Pub/Sub service agent to be able
# to mint identity tokens as the runtime SA. Without this, deliveries fail
# and messages eventually go to the DLQ.
resource "google_service_account_iam_member" "pubsub_token_creator" {
  service_account_id = google_service_account.runtime.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

# Dead-lettering requires the Pub/Sub service agent to be allowed to publish
# to the DLQ topic. Without this, the subscription creation succeeds but
# messages exceeding max_delivery_attempts silently fail to route to the DLQ.
resource "google_pubsub_topic_iam_member" "dlq_publisher" {
  topic  = var.new_arrival_dlq_id
  role   = "roles/pubsub.publisher"
  member = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
