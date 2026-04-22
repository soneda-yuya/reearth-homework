# ---- CI deployer permissions -----------------------------------------------
# CI needs to: push images to Artifact Registry, deploy Cloud Run services/jobs,
# act as (impersonate) runtime SAs, manage terraform state.

resource "google_project_iam_member" "ci_run_admin" {
  project = var.project_id
  role    = "roles/run.admin"
  member  = "serviceAccount:${google_service_account.ci_deployer.email}"
}

resource "google_project_iam_member" "ci_ar_writer" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.ci_deployer.email}"
}

resource "google_project_iam_member" "ci_sa_user" {
  project = var.project_id
  role    = "roles/iam.serviceAccountUser"
  member  = "serviceAccount:${google_service_account.ci_deployer.email}"
}

# Terraform administers project-scoped resources; give CI enough to plan/apply.
resource "google_project_iam_member" "ci_secret_admin" {
  project = var.project_id
  role    = "roles/secretmanager.admin"
  member  = "serviceAccount:${google_service_account.ci_deployer.email}"
}

resource "google_project_iam_member" "ci_pubsub_editor" {
  project = var.project_id
  role    = "roles/pubsub.editor"
  member  = "serviceAccount:${google_service_account.ci_deployer.email}"
}

# ---- Runtime permissions (project-scoped) ----------------------------------

# BFF reads Firestore and verifies ID tokens.
resource "google_project_iam_member" "bff_datastore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.bff_runtime.email}"
}

resource "google_project_iam_member" "bff_firebase_auth" {
  project = var.project_id
  role    = "roles/firebaseauth.admin"
  member  = "serviceAccount:${google_service_account.bff_runtime.email}"
}

# Notifier reads Firestore (subscribers) and sends FCM.
resource "google_project_iam_member" "notifier_datastore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.notifier_runtime.email}"
}

resource "google_project_iam_member" "notifier_fcm" {
  project = var.project_id
  role    = "roles/cloudmessaging.messagesSender"
  member  = "serviceAccount:${google_service_account.notifier_runtime.email}"
}
