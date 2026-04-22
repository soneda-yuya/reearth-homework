# U-CSS Terraform Plan

**Unit**: U-CSS
**対象**: [`terraform/modules/cmsmigrate/`](../../../../terraform/modules/cmsmigrate/) と [`terraform/environments/prod/`](../../../../terraform/environments/prod/) の **差分要約**

U-PLT ですでに雛形が整備されているため、U-CSS で必要な変更は **最小限**。本ドキュメントは Code Generation で実装する Terraform 変更の diff 要約。

---

## 1. 変更サマリ

| # | ファイル | 変更 | 根拠 |
|---|---|---|---|
| 1 | [`terraform/modules/cmsmigrate/main.tf`](../../../../terraform/modules/cmsmigrate/main.tf) | `max_retries = 0` を追加 | Q3 [A]、fail-fast と整合 |
| 2 | （なし） | — | Q1/Q2/Q4 は現状値のまま |
| 3 | [`terraform/environments/prod/variables.tf`](../../../../terraform/environments/prod/variables.tf) | `cms_base_url` / `cms_workspace_id` に **description** を追加（既存だが運用者向けコメント強化） | Q5 [A]、外部 CMS の URL を tfvars で供給することを明示 |
| 4 | `terraform/environments/prod/prod.tfvars.example`（新規）| `cms_base_url` / `cms_workspace_id` のプレースホルダ付きサンプル | Q5 [A]、運用セットアップ手順の一部 |

> 影響は **1 resource への 1 行追加** + **環境側の description / 運用ドキュメント** のみ。新しい Google API 有効化、IAM 付与、Secret 追加などは不要。

---

## 2. 詳細 diff（疑似）

### 2.1 `terraform/modules/cmsmigrate/main.tf`

```diff
     template {
       service_account = google_service_account.runtime.email
       timeout         = "120s"
+      max_retries     = 0
 
       containers {
```

**意味**:
- `max_retries = 0` により Cloud Run Job が失敗したとき GCP 側で自動リトライしない
- fail-fast（U-CSS Design Q4 [A]）と整合：運用者が Cloud Logging を見て原因切り分け → 手動再実行
- 一時的な 5xx / 429 は application 層の `retry.Do`（max 3、exp backoff）で吸収済み

### 2.2 `terraform/environments/prod/variables.tf`

```diff
 variable "cms_base_url" {
-  type = string
+  type        = string
+  description = "Base URL of the external reearth-cms instance (e.g. https://cms.example.com). The CMS itself is managed outside this project."
 }
 
 variable "cms_workspace_id" {
-  type = string
+  type        = string
+  description = "Workspace ID in the external reearth-cms where the SafetyIncident schema is applied."
 }
```

**意味**:
- Q5 [A] の前提（CMS は外部既存）を Terraform definition 内に明示
- 新しい変数ではなく既存変数への description 追加

### 2.3 `terraform/environments/prod/prod.tfvars.example`（新規）

```hcl
# Example tfvars for the prod environment.
# Copy to prod.tfvars (gitignored) and fill in the actual values.
#
# NOTE: tfstate bucket + WIF + GitHub secrets are assumed to be provisioned
# already. See construction/shared-infrastructure.md for bootstrap steps.

project_number = "000000000000"  # gcloud projects describe overseas-safety-map --format='value(projectNumber)'

cms_base_url     = "https://cms.example.com"
cms_workspace_id = "wkp_XXXXXXXXXXXX"

# image tags are normally overridden by CI per deploy.
# bff_image_tag        = "latest"
# ingestion_image_tag  = "latest"
# notifier_image_tag   = "latest"
# cmsmigrate_image_tag = "latest"
```

**意味**:
- 新規運用者が prod デプロイに必要な tfvars をセットアップする手順を明文化
- `prod.tfvars` 自体は `.gitignore` 済み（既存）
- CMS 本体が外部にあるという前提（Q5 [A]）を example 値で示す

---

## 3. 新規リソース / 削除リソース

### 3.1 新規作成（なし）

Secret Manager の `cms-integration-token` はすでに [`terraform/modules/shared/secrets.tf`](../../../../terraform/modules/shared/secrets.tf) に存在。U-CSS では **追加しない**。

Cloud Run Job `cms-migrate` の `google_cloud_run_v2_job` もすでに [`terraform/modules/cmsmigrate/main.tf`](../../../../terraform/modules/cmsmigrate/main.tf) に存在。U-CSS では **追加しない**（`max_retries = 0` の 1 行追加のみ）。

### 3.2 削除（なし）

---

## 4. `terraform apply` 時の想定 diff

`main.tf` の `max_retries = 0` 追加で発生する Plan 出力の予想:

```
# module.cmsmigrate.google_cloud_run_v2_job.cmsmigrate will be updated in-place
~ resource "google_cloud_run_v2_job" "cmsmigrate" {
    name = "cms-migrate"
    ...
    ~ template {
        ~ template {
            + max_retries = 0
            ...
        }
    }
}

Plan: 0 to add, 1 to change, 0 to destroy.
```

- 実行は in-place update（リソース再作成なし）
- Downtime なし（Job は on-demand 実行のため、設定変更で動作中の実行に影響しない）

---

## 5. Code Generation へ渡す TODO

Code Generation 段階で実施する Terraform 変更:

- [ ] `terraform/modules/cmsmigrate/main.tf` に `max_retries = 0` を追記
- [ ] `terraform/environments/prod/variables.tf` の `cms_base_url` / `cms_workspace_id` に description を追加
- [ ] `terraform/environments/prod/prod.tfvars.example` を新規作成
- [ ] `terraform fmt` / `terraform validate` を通す
- [ ] `terraform/environments/prod/README.md` が既にあれば「`prod.tfvars` のセットアップ手順」を追記（なければ作成は任意）

上記と並行して、Code Generation の本丸は Go 側（`cmd/cmsmigrate/main.go` の拡張 + `internal/cmsmigrate/` の新規パッケージ + test）。詳細は次の U-CSS Code Generation Plan で決める。

---

## 6. 非 Terraform セットアップ手順（運用ランブック）

実際に `terraform apply` + `gcloud run jobs execute` するために運用者が **事前に** 行うこと:

1. **reearth-cms 側で Workspace 準備 + Integration Token 発行**（Q5 [A] の前提）
2. **`gcloud secrets versions add cms-integration-token`** で Token 値を投入
3. **`prod.tfvars` 作成**（リポジトリ内で `.gitignore` 済み、example をコピー）
   ```hcl
   project_number   = "<actual project number>"
   cms_base_url     = "<actual CMS URL>"
   cms_workspace_id = "<actual workspace id>"
   ```
4. CI/CD が `terraform apply -var-file=prod.tfvars` を実行（WIF 経由）
5. 運用者が `gcloud run jobs execute cms-migrate --region=asia-northeast1 --wait` で初回スキーマ適用

> 詳細な bootstrap 手順（tfstate bucket / WIF / GitHub Secrets）は [`construction/shared-infrastructure.md`](../../shared-infrastructure.md) 参照。

---

## 7. 承認プロセス

- [ ] 本 Terraform Plan のレビュー
- [ ] deployment-architecture.md のレビュー
- [ ] 承認後、U-CSS Code Generation へ進む（Terraform + Go の両方を含む）
