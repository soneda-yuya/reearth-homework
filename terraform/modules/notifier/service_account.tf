resource "google_service_account" "runtime" {
  account_id   = "notifier-runtime"
  display_name = "Runtime SA for cmd/notifier"
}
