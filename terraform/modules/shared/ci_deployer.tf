resource "google_service_account" "ci_deployer" {
  account_id   = "ci-deployer"
  display_name = "GitHub Actions deployer"
}

# Project-scoped roles the CI deployer needs to apply Terraform.
#
# The scope is broad because CI-driven apply touches every resource the state
# manages (APIs, IAM, WIF, Firestore, Cloud Run, Scheduler, Pub/Sub, Secrets,
# Artifact Registry). A future hardening is to split the state into a
# bootstrap tier (human-only) and a runtime tier (CI-only), and trim CI's
# permissions to the runtime tier; tracked as a follow-up.
locals {
  ci_deployer_roles = [
    # Service enablement (google_project_service)
    "roles/serviceusage.serviceUsageAdmin",

    # IAM policy management (google_project_iam_member bindings)
    "roles/resourcemanager.projectIamAdmin",

    # Service accounts (runtime SAs + impersonation)
    "roles/iam.serviceAccountAdmin",
    "roles/iam.serviceAccountUser",

    # Workload Identity Federation
    "roles/iam.workloadIdentityPoolAdmin",

    # Compute + deployment
    "roles/run.admin",
    "roles/artifactregistry.writer",

    # Data + messaging
    "roles/secretmanager.admin",
    # pubsub.editor lacks topics.getIamPolicy which terraform needs to refresh
    # google_pubsub_topic_iam_member bindings (DLQ publisher grant).
    "roles/pubsub.admin",
    "roles/datastore.owner",
    "roles/cloudscheduler.admin",

    # Firebase project link refresh (google_firebase_project)
    "roles/firebase.admin",
    # Identity Platform config refresh (google_identity_platform_config)
    "roles/firebaseauth.admin",
  ]
}

resource "google_project_iam_member" "ci_deployer_roles" {
  for_each = toset(local.ci_deployer_roles)

  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.ci_deployer.email}"
}

# CI applies terraform against a GCS backend; it needs both state R/W
# (objectAdmin) *and* bucket-level getIamPolicy for terraform refresh to be
# able to read google_storage_bucket_iam_member's current state. The latter
# is not in objectAdmin, so we grant storage.admin scoped to just this
# bucket. The bucket itself is created manually before the first apply
# (chicken-and-egg), so we grant IAM via google_storage_bucket_iam_member
# referencing the fixed bucket name rather than managing the bucket in state.
resource "google_storage_bucket_iam_member" "ci_tfstate" {
  bucket = var.tfstate_bucket
  role   = "roles/storage.admin"
  member = "serviceAccount:${google_service_account.ci_deployer.email}"
}
