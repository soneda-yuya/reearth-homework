resource "google_secret_manager_secret_iam_member" "cms" {
  secret_id = var.cms_integration_token_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime.email}"
}
