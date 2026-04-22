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

# No Firebase Auth role is granted here: the BFF only verifies Firebase
# ID Tokens (which Firebase Admin SDK does against cached Google public
# certificates — no IAM required). If a future use case needs to call admin
# APIs (e.g. createUser, disableUser), add the narrowest role that covers
# only those operations at that time.
