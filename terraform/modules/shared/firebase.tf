# GCP プロジェクトを Firebase プロジェクトとしてリンクする。
# これが無いと Firebase Console から参照できず、Identity Platform の UI も
# Firebase Authentication として表示されない。google-beta 専用リソース。
resource "google_firebase_project" "default" {
  provider = google-beta
  project  = var.project_id

  depends_on = [google_project_service.enabled]
}

# Identity Platform (Firebase Auth の上位互換) を初期化し、Anonymous Auth を有効化する。
# BFF の AuthInterceptor は Firebase Admin SDK で ID Token を検証するため、
# このリソースが apply された時点で Flutter 側は匿名サインインが可能になる。
#
# 課金について: Identity Platform は 50K MAU までは実質無料。Cloud Run / Firestore で
# どのみち billing account を紐付けるので追加コストは発生しない想定。
resource "google_identity_platform_config" "default" {
  project = var.project_id

  sign_in {
    anonymous {
      enabled = true
    }
  }

  depends_on = [
    google_project_service.enabled,
    google_firebase_project.default,
  ]
}
