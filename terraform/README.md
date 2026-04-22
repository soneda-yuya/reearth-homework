# Terraform — overseas-safety-map

GCP インフラを **module × environments** 構成で管理します。将来 `dev` 環境を追加する際は `environments/dev/` を増設し、同じ modules を別の backend prefix / 変数で呼び出すだけで済みます。

## ディレクトリ構成

```
terraform/
├── modules/
│   ├── shared/       # プロジェクト横断: API 有効化 / Artifact Registry / Firestore /
│   │                 #                 Pub/Sub topic + DLQ / Secret 定義 / WIF / CI SA
│   ├── bff/          # Cloud Run Service bff + runtime SA + IAM
│   ├── ingestion/    # Cloud Run Job ingestion + runtime SA + IAM + Cloud Scheduler
│   ├── notifier/     # Cloud Run Service notifier + runtime SA + IAM + Pub/Sub Subscription
│   └── setup/        # Cloud Run Job setup + runtime SA + IAM
└── environments/
    └── prod/         # 本番相当の root module（backend / providers / module 呼び出し）
```

各 application module は自身の `runtime` Service Account / Cloud Run リソース / その SA に紐づく IAM を持ちます。共有リソース（Secret / Pub/Sub Topic 等）は `shared` module で一括定義し、各 application module が ID を受け取って **consumer 側の IAM binding だけ自分で張る** 構造にしています。

## 初回 Bootstrap

Terraform state を GCS に置くため、bucket は **手動で** 作成します（chicken-and-egg 回避）:

```bash
gcloud auth application-default login
gcloud config set project overseas-safety-map

gsutil mb -l asia-northeast1 gs://overseas-safety-map-tfstate
gsutil versioning set on gs://overseas-safety-map-tfstate
```

その後 prod 環境で:

```bash
cd terraform/environments/prod
terraform init
terraform plan \
  -var="project_number=$(gcloud projects describe overseas-safety-map --format='value(projectNumber)')" \
  -var="cms_base_url=https://cms.example.com" \
  -var="cms_workspace_id=<ワークスペース ID>"
terraform apply ...
```

## Secret 実値の初期投入（手動）

Terraform は secret *定義* のみ扱います。実値は state に漏らさないよう手動登録:

```bash
echo -n "sk-ant-..." | gcloud secrets versions add ingestion-claude-api-key --data-file=-
echo -n "pk.ey..."   | gcloud secrets versions add ingestion-mapbox-api-key --data-file=-
echo -n "<token>"    | gcloud secrets versions add cms-integration-token --data-file=-
```

## デプロイ（CI）

main ブランチに merge されると GitHub Actions が:

1. `docker build + push` を 4 deployable で実行
2. `terraform apply -var='bff_image_tag=<git-sha>' ...` で各 Cloud Run の image tag を更新

`.github/workflows/deploy.yml` と `terraform-validate.yml` は `working-directory: terraform/environments/prod` で動作します。`terraform-validate.yml` は PR で fmt + validate のみ（WIF が main 限定のため plan はローカル実行）。

## Dev 環境を追加したくなったら

`environments/dev/` を作り、`environments/prod/` をコピーして以下を変える:

- `backend "gcs" { prefix = "dev" }`
- 変数の default（`project_id`、`cms_base_url`、`cms_workspace_id`）
- `local.env = "dev"`

modules には触らずに済むのがこの構成の狙いです。

## ロールバック

Cloud Run はリビジョン単位でイミュータブルなので、過去の git-sha を渡して再 apply で戻せます:

```bash
cd terraform/environments/prod
terraform apply -var='bff_image_tag=abc1234'
```
