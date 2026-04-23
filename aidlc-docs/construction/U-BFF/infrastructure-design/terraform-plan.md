# U-BFF Terraform Plan

**Unit**: U-BFF（Connect Server / BFF Unit、Sprint 3）
**参照**: [`deployment-architecture.md`](./deployment-architecture.md)、[`construction/plans/U-BFF-infrastructure-design-plan.md`](../../plans/U-BFF-infrastructure-design-plan.md)

---

## 1. サマリ

**結論: Terraform コード変更ゼロ**。

U-PLT で `terraform/modules/bff/` が既に完全実装されており、U-NTF で Firestore 共通リソース（`modules/shared/firestore.tf`）が整備済みのため、U-BFF の要件は既存定義のみで充足する。

| 対象 | 既存の状態 | U-BFF での変更 |
|---|---|---|
| `google_cloud_run_v2_service.bff` | `min=0 / max=3 / cpu=1 / memory=512Mi`、`ingress=ALL` | **なし** |
| `google_cloud_run_v2_service_iam_member.invoker` | `allUsers`（認可は AuthInterceptor） | **なし** |
| `google_service_account.runtime` (`bff-runtime`) | `roles/datastore.user` + `roles/secretmanager.secretAccessor` | **なし** |
| `google_secret_manager_secret.cms_integration_token` | shared module で管理 | **なし** |
| `google_firestore_database.default` | shared module で作成済み | **なし** |
| `google_firestore_field.notifier_dedup_ttl` | U-NTF 専用、U-BFF は利用せず | **なし** |
| `google_firestore_index.users_notification` | U-NTF 専用、U-BFF は利用せず | **なし** |

---

## 2. 既存 `terraform/modules/bff/` のファイル一覧

```
terraform/modules/bff/
├── main.tf              # Cloud Run v2 Service 定義 + invoker IAM
├── iam.tf               # Runtime SA の IAM binding
├── service_account.tf   # bff-runtime SA
├── variables.tf         # project_id, region, env, image_tag, AR URL, CMS env, secret id/name
└── outputs.tf           # URL などの出力
```

`terraform/environments/prod/main.tf` から `module "bff"` として使用されており、既に配線済み。

---

## 3. Cloud Run Service（現状維持）

```hcl
# terraform/modules/bff/main.tf  (抜粋、変更なし)

resource "google_cloud_run_v2_service" "bff" {
  name     = "bff"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.runtime.email
    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }
    containers {
      image = "${var.artifact_registry_url}/bff:${var.image_tag}"
      ports { container_port = 8080 }

      env { name = "PLATFORM_SERVICE_NAME"    value = "bff" }
      env { name = "PLATFORM_ENV"             value = var.env }
      env { name = "PLATFORM_GCP_PROJECT_ID"  value = var.project_id }
      env { name = "PLATFORM_OTEL_EXPORTER"   value = "gcp" }
      env { name = "BFF_PORT"                 value = "8080" }
      env { name = "BFF_CMS_BASE_URL"         value = var.cms_base_url }
      env { name = "BFF_CMS_WORKSPACE_ID"     value = var.cms_workspace_id }
      env {
        name = "BFF_CMS_INTEGRATION_TOKEN"
        value_source {
          secret_key_ref {
            secret  = var.cms_integration_token_secret_name
            version = "latest"
          }
        }
      }

      resources {
        limits = { cpu = "1", memory = "512Mi" }
      }

      startup_probe  { http_get { path = "/healthz" } ; ... }
      liveness_probe { http_get { path = "/healthz" } ; ... }
    }
  }

  traffic { type = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST", percent = 100 }
  depends_on = [google_secret_manager_secret_iam_member.bff_cms]
}

resource "google_cloud_run_v2_service_iam_member" "invoker" {
  location = google_cloud_run_v2_service.bff.location
  name     = google_cloud_run_v2_service.bff.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
```

### env の流入方針

| env | 供給元 | 補足 |
|---|---|---|
| `PLATFORM_GCP_PROJECT_ID` | Terraform | Firebase Admin SDK の `ProjectID` にも流用（Q5 [A]） |
| `BFF_PORT` | Terraform | Cloud Run port と一致必須 |
| `BFF_CMS_BASE_URL` / `WORKSPACE_ID` | Terraform | 運用ポリシー |
| `BFF_CMS_INTEGRATION_TOKEN` | Terraform（Secret Manager 経由） | secret_key_ref |
| `BFF_REQUEST_BODY_LIMIT_BYTES` | envconfig default（1 MiB） | tuning |
| `BFF_FCM_TOKEN_MAX` | envconfig default（10） | tuning |
| `BFF_USERS_COLLECTION` | envconfig default（"users"） | tuning |
| `BFF_SHUTDOWN_GRACE_SECONDS` | envconfig default（10） | tuning |

**Terraform 追加 env: ゼロ**。

---

## 4. IAM（現状維持）

```hcl
# terraform/modules/bff/iam.tf  (抜粋、変更なし)

resource "google_secret_manager_secret_iam_member" "bff_cms" {
  secret_id = var.cms_integration_token_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_project_iam_member" "bff_datastore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

# No Firebase Auth role is granted here: the BFF only verifies Firebase
# ID Tokens (which Firebase Admin SDK does against cached Google public
# certificates — no IAM required). If a future use case needs to call admin
# APIs (e.g. createUser, disableUser), add the narrowest role that covers
# only those operations at that time.
```

---

## 5. Firestore（既存 shared 流用、変更なし）

U-NTF で追加済みの `modules/shared/firestore.tf` が以下を定義:

```hcl
resource "google_firestore_database" "default" {
  project     = var.project_id
  name        = "(default)"
  location_id = var.region
  type        = "FIRESTORE_NATIVE"
  depends_on  = [google_project_service.enabled]
}

resource "google_firestore_field" "notifier_dedup_ttl" { ... }   # U-NTF 専用
resource "google_firestore_index" "users_notification" { ... }   # U-NTF 専用
```

U-BFF は `users/{uid}` を document-id で直接アクセスするため、**追加 index / TTL policy なし**。

---

## 6. Terraform apply フロー

U-BFF のデプロイに伴う `terraform plan` / `apply` の差分は **通常ゼロ**（image tag 差し替えを除く）:

```
$ terraform plan -var-file=prod.tfvars -var="bff_image_tag=<new-sha>"
...
module.bff.google_cloud_run_v2_service.bff will be updated in-place
  ~ template { containers { image = "<old-sha>" → "<new-sha>" } }

Plan: 0 to add, 1 to change, 0 to destroy.
```

image tag 以外の変更は発生しない。CI/CD は既存の U-PLT デプロイパイプライン（WIF + Artifact Registry push + Cloud Run deploy）をそのまま使う。

---

## 7. Terraform 変更ゼロの根拠（Q1-Q6 サマリ）

| Q | 決定 | Terraform 変更 |
|---|---|---|
| Q1 | Flutter mobile 前提、CORS 設定なし | ゼロ（middleware は Go コード側のみ） |
| Q2 | Cloud Run scaling 現状維持 | ゼロ |
| Q3 | Firestore 変更なし（document-id 直接アクセス） | ゼロ |
| Q4 | IAM 変更なし（既存 2 role で充足） | ゼロ |
| Q5 | env 追加ゼロ（tuning は envconfig default） | ゼロ |
| Q6 | 変更ゼロでもドキュメント生成 | 本ドキュメント |

---

## 8. 将来の拡張ポイント（参考）

本 MVP の範囲外。運用結果を見ながら判断:

| 拡張 | 追加リソース | トリガ |
|---|---|---|
| Flutter web 対応 | `connectcors` Go 実装（Terraform 変更なし） | Web ビルドを公開 |
| CDN / カスタムドメイン | `google_cloud_run_domain_mapping`、Cloud CDN | ドメイン取得後 |
| Cold start 削減 | `min_instance_count = 1` | p95 が SLO を外す |
| Rate limiting | Cloud Armor `google_compute_security_policy` | 悪意トラフィック観測時 |
| Firestore geohash index | `google_firestore_index` | `ListNearby` を Firestore 実装に切替 |
| Alerting Policy | `google_monitoring_alert_policy` | Ops フェーズで一括導入 |

---

## 9. 適用チェックリスト

- [ ] `terraform/modules/bff/` に未コミットの差分がない（`git status terraform/modules/bff/`）
- [ ] `terraform/environments/prod/main.tf` の `module "bff"` ブロックが変更されていない
- [ ] `modules/shared/firestore.tf` が U-NTF の定義どおり残っている
- [ ] Runtime SA の IAM binding 一覧が §4 と一致
- [ ] `terraform plan` で image tag 以外の差分が出ない
