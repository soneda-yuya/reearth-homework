resource "google_service_account" "runtime" {
  account_id   = "setup-runtime"
  display_name = "Runtime SA for cmd/setup"
}
