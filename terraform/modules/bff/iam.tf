variable "cms_integration_token_secret_id" {
  description = "Fully-qualified Secret Manager resource id for cms-integration-token."
  type        = string
}

resource "google_secret_manager_secret_iam_member" "bff_cms" {
  secret_id = var.cms_integration_token_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_project_iam_member" "bff_datastore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_project_iam_member" "bff_firebase_auth" {
  project = var.project_id
  role    = "roles/firebaseauth.admin"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}
