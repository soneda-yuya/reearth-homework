# U-NTF Terraform Plan

**Unit**: U-NTF
**対象**: [`terraform/modules/shared/firestore.tf`](../../../../terraform/modules/shared/firestore.tf) の **差分要約**

U-PLT で `modules/notifier/` が **Cloud Run Service + Pub/Sub Push Subscription + 完全な IAM** まで実装済みなので、U-NTF で必要な Terraform 変更は **Firestore 側の 2 リソース追加のみ**。

---

## 1. 変更サマリ

| # | ファイル | 変更 | 根拠 |
|---|---|---|---|
| 1 | [`modules/shared/firestore.tf`](../../../../terraform/modules/shared/firestore.tf) | `google_firestore_field "notifier_dedup_ttl"` を新規追加 | Q1 [A] |
| 2 | [`modules/shared/firestore.tf`](../../../../terraform/modules/shared/firestore.tf) | `google_firestore_index "users_notification"` を新規追加 | Q2 [A] |
| 3 | `modules/notifier/*` | **変更なし** | Q3 [A] / Q4 [A] |

> 影響は **1 ファイルに 2 リソース追加だけ**。既存 Cloud Run / Pub/Sub / IAM の設定は無変更。

---

## 2. 詳細 diff（疑似）

### 2.1 `terraform/modules/shared/firestore.tf`

```diff
 resource "google_firestore_database" "default" {
   project     = var.project_id
   name        = "(default)"
   location_id = var.region
   type        = "FIRESTORE_NATIVE"

   depends_on = [google_project_service.enabled]
 }
+
+# U-NTF: Firestore TTL policy on notifier_dedup.expireAt.
+# Dedup ドキュメントは書き込み時に expireAt = now() + 24h を設定し、
+# Firestore が expireAt < now() のドキュメントを自動削除する (最大 24h 遅延)。
+resource "google_firestore_field" "notifier_dedup_ttl" {
+  project    = var.project_id
+  database   = google_firestore_database.default.name
+  collection = "notifier_dedup"
+  field      = "expireAt"
+
+  ttl_config {}
+
+  depends_on = [google_firestore_database.default]
+}
+
+# U-NTF: 購読者解決クエリ用の複合インデックス。
+#   users.where("notification_preference.enabled", "==", true)
+#        .where("notification_preference.target_country_cds", "array-contains", countryCd)
+# Firestore は複合 query に自動 index を作らないため明示必須。
+resource "google_firestore_index" "users_notification" {
+  project    = var.project_id
+  database   = google_firestore_database.default.name
+  collection = "users"
+
+  fields {
+    field_path = "notification_preference.enabled"
+    order      = "ASCENDING"
+  }
+  fields {
+    field_path   = "notification_preference.target_country_cds"
+    array_config = "CONTAINS"
+  }
+
+  depends_on = [google_firestore_database.default]
+}
```

### 2.2 `terraform/modules/notifier/*`

**変更なし**。

U-PLT で既に以下が揃っているため追加変更不要:
- Cloud Run Service (scaling / probe / env / resources)
- Pub/Sub Push Subscription (retry / DLQ / OIDC)
- Runtime SA + 5 IAM binding
- Pub/Sub service agent 2 IAM binding (tokenCreator + DLQ publisher)

### 2.3 Code 側で実装する env（Terraform 非対象）

```go
// cmd/notifier/main.go（envconfig default で吸収）
type notifierConfig struct {
    config.Common
    Port                     string `envconfig:"NOTIFIER_PORT" required:"true"`            // Terraform 渡し
    PubSubSubscription       string `envconfig:"NOTIFIER_PUBSUB_SUBSCRIPTION" required:"true"`  // Terraform 渡し
    FirestoreDedupCollection string `envconfig:"NOTIFIER_DEDUP_COLLECTION" default:"notifier_dedup"`
    FirestoreDedupTTLHours   int    `envconfig:"NOTIFIER_DEDUP_TTL_HOURS" default:"24"`
    FirestoreUsersCollection string `envconfig:"NOTIFIER_USERS_COLLECTION" default:"users"`
    FCMConcurrency           int    `envconfig:"NOTIFIER_FCM_CONCURRENCY" default:"5"`
    ShutdownGraceSeconds     int    `envconfig:"NOTIFIER_SHUTDOWN_GRACE_SECONDS" default:"10"`
}
```

---

## 3. 新規リソース / 削除リソース

### 3.1 新規作成（2 リソース）

- `google_firestore_field.notifier_dedup_ttl`
- `google_firestore_index.users_notification`

### 3.2 削除（なし）

---

## 4. `terraform apply` 想定 diff

```
# module.shared.google_firestore_field.notifier_dedup_ttl will be created
+ resource "google_firestore_field" "notifier_dedup_ttl" {
    + collection = "notifier_dedup"
    + database   = "(default)"
    + field      = "expireAt"
    + project    = "overseas-safety-map"
    + ttl_config {}
  }

# module.shared.google_firestore_index.users_notification will be created
+ resource "google_firestore_index" "users_notification" {
    + collection = "users"
    + database   = "(default)"
    + project    = "overseas-safety-map"
    + fields {
        + field_path = "notification_preference.enabled"
        + order      = "ASCENDING"
    }
    + fields {
        + field_path   = "notification_preference.target_country_cds"
        + array_config = "CONTAINS"
    }
  }

Plan: 2 to add, 0 to change, 0 to destroy.
```

### 実行時の注意

- **インデックス構築は非同期**: Firebase Console で状態 READY になるまで数分〜数十分
- **TTL policy 適用は即時**: 既存ドキュメントがあれば 24h 以内に削除開始

---

## 5. Code Generation へ渡す TODO

Code Generation 段階で実施する Terraform 変更:

- [ ] `terraform/modules/shared/firestore.tf` に 2 リソース追加
- [ ] `terraform fmt` / `terraform init -backend=false` / `terraform validate` を通す
- [ ] `modules/notifier/*` は **変更なし**（確認のみ）

並行して Code Generation の本丸は Go 側 (`cmd/notifier/main.go` の拡張 + `internal/notification/` 新規パッケージ + 全テスト + Firestore / FCM Admin SDK 依存追加)。

---

## 6. 非 Terraform セットアップ手順（運用ランブック）

実 notifier を動かすため運用者が **事前に** 行うこと:

1. **Firebase プロジェクト設定**（初回のみ）
   - `overseas-safety-map` GCP プロジェクトで Firebase を有効化
   - iOS / Android アプリを Firebase Console で登録
   - `GoogleService-Info.plist` / `google-services.json` を Flutter アプリに配置（U-APP の責務）

2. **Firestore `users` の初期データ投入**（MVP では U-BFF から登録、U-NTF は reader）
   - MVP 初期: ユーザーアカウントは U-BFF の `UserProfileService` で作成される
   - テスト用に `gcloud firestore import` でサンプルデータ投入可能

3. **Terraform apply**:
   ```bash
   terraform apply -var-file=prod.tfvars
   ```
   → Firestore TTL + index + Cloud Run + Subscription が全部 apply される

4. **Firestore index の READY 確認**:
   ```bash
   gcloud firestore indexes composite list --project=overseas-safety-map
   # state が READY なのを確認
   ```

5. **U-ING が publish → U-NTF が push 受信 → FCM → 実機** の疎通確認（Build and Test）

---

## 7. 承認プロセス

- [ ] 本 Terraform Plan のレビュー
- [ ] [`deployment-architecture.md`](./deployment-architecture.md) のレビュー
- [ ] 承認後、U-NTF Code Generation へ進む（Go + Terraform 両方）
