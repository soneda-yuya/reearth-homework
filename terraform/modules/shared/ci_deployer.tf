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
    "roles/pubsub.editor",
    "roles/datastore.owner",
    "roles/cloudscheduler.admin",
  ]
}

resource "google_project_iam_member" "ci_deployer_roles" {
  for_each = toset(local.ci_deployer_roles)

  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.ci_deployer.email}"
}

# CI applies terraform against a GCS backend; without object admin on the
# tfstate bucket, init/apply fails. The bucket itself is created manually
# before the first apply (chicken-and-egg), so we grant IAM via
# google_storage_bucket_iam_member referencing the fixed bucket name rather
# than managing the bucket in state.
resource "google_storage_bucket_iam_member" "ci_tfstate" {
  bucket = var.tfstate_bucket
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.ci_deployer.email}"
}
