# Terraform — overseas-safety-map

GCP プロジェクト `overseas-safety-map`（単一 prod、`asia-northeast1`）の全インフラを Terraform で管理します。

## ファイル構成

| ファイル | 役割 |
|---|---|
| `versions.tf` | Terraform / provider バージョン固定、GCS backend |
| `main.tf` | google / google-beta provider |
| `variables.tf` | project_id / region / image_tag / CMS 設定 |
| `outputs.tf` | 重要 URL / WIF provider / CI SA email |
| `apis.tf` | GCP API 有効化（16 API） |
| `artifact_registry.tf` | Docker レジストリ `app` |
| `secret_manager.tf` | 3 Secret 定義 + 最小権限 IAM |
| `pubsub.tf` | `safety-incident.new-arrival` topic + subscription + DLQ |
| `service_accounts.tf` | 5 SA（ci-deployer + 4 runtime）+ scheduler-invoker |
| `wif.tf` | Workload Identity Federation（GitHub OIDC） |
| `iam.tf` | project-scoped IAM bindings |
| `cloud_run_bff.tf` | Service `bff`（Connect サーバ）、公開 |
| `cloud_run_notifier.tf` | Service `notifier`（Pub/Sub push）、内部のみ |
| `cloud_run_ingestion.tf` | Job `ingestion`（5 分毎実行） |
| `cloud_run_setup.tf` | Job `setup`（手動実行、冪等） |
| `cloud_scheduler.tf` | ingestion を 5 分毎にキック |
| `firestore.tf` | Native mode database |

## 初回 Bootstrap

Terraform state を GCS に置くため、bucket は **手動で** 作成する（chicken-and-egg 回避）。

```bash
# GCP プロジェクトと billing を先に用意する前提
gcloud auth application-default login
gcloud config set project overseas-safety-map

gsutil mb -l asia-northeast1 gs://overseas-safety-map-tfstate
gsutil versioning set on gs://overseas-safety-map-tfstate
```

その後:

```bash
cd terraform
terraform init
terraform plan -var="project_number=$(gcloud projects describe overseas-safety-map --format='value(projectNumber)')" \
               -var='cms_base_url=https://cms.example.com' \
               -var='cms_workspace_id=<ワークスペース ID>'
terraform apply ...
```

## Secret の初期投入（手動）

```bash
echo -n "sk-ant-..." | gcloud secrets versions add ingestion-claude-api-key --data-file=-
echo -n "pk.ey..."   | gcloud secrets versions add ingestion-mapbox-api-key --data-file=-
echo -n "<token>"    | gcloud secrets versions add cms-integration-token --data-file=-
```

## デプロイの流れ

1. main に merge される PR が CI を通過
2. GitHub Actions が `docker build + push` を 4 Deployable で実行
3. `terraform apply -var='bff_image_tag=<git-sha>' ...` で各 image tag を更新
4. Cloud Run が新しい revision に切り替わる（無停止）

## ロールバック

Cloud Run はリビジョン単位でイミュータブルなので、過去の git-sha を渡して再 apply すれば戻せます:

```bash
terraform apply -var='bff_image_tag=abc1234'
```
