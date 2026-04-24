locals {
  enabled_apis = [
    "run.googleapis.com",
    "pubsub.googleapis.com",
    "secretmanager.googleapis.com",
    "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com",
    "cloudscheduler.googleapis.com",
    "firestore.googleapis.com",
    "identitytoolkit.googleapis.com",
    "fcm.googleapis.com",
    "fcmregistrations.googleapis.com",
    "iamcredentials.googleapis.com",
    "sts.googleapis.com",
    "cloudtrace.googleapis.com",
    "monitoring.googleapis.com",
    "logging.googleapis.com",
    "iam.googleapis.com",
    # Bootstrap APIs: terraform 自身が google provider / google_project_service /
    # tfstate bucket を扱うために必要。事前に手動 enable していても、
    # 管理下に置いておくと disable 事故を防げる (disable_on_destroy = false)。
    "cloudresourcemanager.googleapis.com",
    "serviceusage.googleapis.com",
    # Firebase: GCP プロジェクトを Firebase プロジェクトとして扱うのに必要。
    "firebase.googleapis.com",
  ]
}

resource "google_project_service" "enabled" {
  for_each = toset(local.enabled_apis)

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}
