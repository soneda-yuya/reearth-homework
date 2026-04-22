resource "google_service_account" "runtime" {
  account_id   = "ingestion-runtime"
  display_name = "Runtime SA for cmd/ingestion"
}
