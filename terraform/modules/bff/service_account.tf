resource "google_service_account" "runtime" {
  account_id   = "bff-runtime"
  display_name = "Runtime SA for cmd/bff"
}
