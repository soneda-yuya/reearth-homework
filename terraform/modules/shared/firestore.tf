resource "google_firestore_database" "default" {
  project     = var.project_id
  name        = "(default)"
  location_id = var.region
  type        = "FIRESTORE_NATIVE"

  depends_on = [google_project_service.enabled]
}

# U-NTF: TTL policy on notifier_dedup.expireAt.
# The notifier writes expireAt = now() + 24h on each dedup record; Firestore
# reaps documents whose expireAt is in the past (typically within minutes,
# worst-case 24h per Google's documented SLA).
resource "google_firestore_field" "notifier_dedup_ttl" {
  project    = var.project_id
  database   = google_firestore_database.default.name
  collection = "notifier_dedup"
  field      = "expireAt"

  ttl_config {}

  depends_on = [google_firestore_database.default]
}

# U-NTF: composite index for the subscriber resolution query.
#
#   users
#     .where("notification_preference.enabled", "==", true)
#     .where("notification_preference.target_country_cds", "array-contains", <cc>)
#
# Firestore does not auto-create composite indexes, so this declaration is
# required. Construction is asynchronous (a few minutes up to tens of minutes
# on first apply); run U-NTF after the index reports READY.
resource "google_firestore_index" "users_notification" {
  project    = var.project_id
  database   = google_firestore_database.default.name
  collection = "users"

  fields {
    field_path = "notification_preference.enabled"
    order      = "ASCENDING"
  }
  fields {
    field_path   = "notification_preference.target_country_cds"
    array_config = "CONTAINS"
  }

  depends_on = [google_firestore_database.default]
}
