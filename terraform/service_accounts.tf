resource "google_service_account" "ci_deployer" {
  account_id   = "ci-deployer"
  display_name = "GitHub Actions deployer"
}

resource "google_service_account" "ingestion_runtime" {
  account_id   = "ingestion-runtime"
  display_name = "Runtime SA for cmd/ingestion"
}

resource "google_service_account" "bff_runtime" {
  account_id   = "bff-runtime"
  display_name = "Runtime SA for cmd/bff"
}

resource "google_service_account" "notifier_runtime" {
  account_id   = "notifier-runtime"
  display_name = "Runtime SA for cmd/notifier"
}

resource "google_service_account" "setup_runtime" {
  account_id   = "setup-runtime"
  display_name = "Runtime SA for cmd/setup"
}
