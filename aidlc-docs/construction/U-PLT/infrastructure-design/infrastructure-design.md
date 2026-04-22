# U-PLT Infrastructure Design

U-PLT で確定する **全 Unit 共通のインフラ設計**。本ドキュメントは U-PLT 自体のインフラに限らず、後続 Unit（U-CSS / U-ING / U-BFF / U-NTF / U-APP）が踏襲する **Shared Infrastructure の根幹** も含む。

---

## 1. 確定事項サマリー

| 項目 | 採用 | 根拠 |
|---|---|---|
| GCP プロジェクト | **`overseas-safety-map`**（単一 prod プロジェクト） | Q1 [X] — MVP 割り切り、dev なし |
| リージョン | **`asia-northeast1`（東京）** 単一 | Q2 [A] — 利用者との距離最短 |
| CI → GCP 認証 | **Workload Identity Federation** | Q3 [A] — 鍵ファイル不要 |
| IaC ツール | **Terraform**（Cloud Run も含む全リソース） | Q4 [A] → Q7 で B に昇格 |
| Docker | distroless マルチステージ + Artifact Registry | Q5 [A] |
| 命名規約 | 統一規約（Q6 参照） | Q6 [A] |
| Env / Secret 注入 | **Terraform `env` ブロック**（`value` + `value_source.secret_key_ref`） | Q7 [X] |

---

## 2. GCP プロジェクト構成

### 2.1 単一プロジェクト方針（Q1 [X]）

- **プロジェクト ID**: `overseas-safety-map`
- **区分**: `prod` 扱いだが、MVP として dev 環境と兼用
- **Firebase**: 同プロジェクトに紐付け（Firebase Project == GCP Project）
- **Billing**: 有効化必須（Cloud Run / Pub/Sub / Artifact Registry が課金対象）

### 2.2 有効化する API

Terraform で `google_project_service` リソースとして管理:

| API | 用途 |
|---|---|
| `run.googleapis.com` | Cloud Run Service / Job |
| `pubsub.googleapis.com` | Pub/Sub（ingestion → notifier） |
| `secretmanager.googleapis.com` | Secret Manager（API キー等） |
| `artifactregistry.googleapis.com` | Docker イメージレジストリ |
| `cloudbuild.googleapis.com` | （必要に応じて）Cloud Build |
| `cloudscheduler.googleapis.com` | Cloud Scheduler（ingestion 5分毎） |
| `firestore.googleapis.com` | Firestore |
| `identitytoolkit.googleapis.com` | Firebase Auth |
| `fcm.googleapis.com` / `fcmregistrations.googleapis.com` | FCM |
| `iamcredentials.googleapis.com` | WIF / 短命トークン |
| `cloudtrace.googleapis.com` | Cloud Trace |
| `monitoring.googleapis.com` | Cloud Monitoring |
| `logging.googleapis.com` | Cloud Logging |

### 2.3 リージョン（Q2 [A]）

- 全リソース `asia-northeast1`（東京）
- Firestore: Native モード、`asia-northeast1`
- Cloud Run / Pub/Sub / Artifact Registry: 同リージョン
- Secret Manager: マルチリージョン（自動）または `asia-northeast1` 指定可能 — Terraform で明示 `asia-northeast1`

---

## 3. CI → GCP 認証（Q3 [A]）

### 3.1 Workload Identity Federation 構成

```
GitHub Actions (OIDC token issuer)
        ↓ OIDC ID Token
GCP Workload Identity Pool  (overseas-safety-map-pool)
        ↓ identity federation
Workload Identity Provider  (github-provider)
        ↓ impersonation via attribute mapping
GCP Service Account         (ci-deployer@overseas-safety-map.iam.gserviceaccount.com)
        ↓ short-lived access token
GCP APIs（Cloud Run deploy / Artifact Registry push / etc）
```

### 3.2 サービスアカウント

| SA 名 | 用途 | IAM Roles |
|---|---|---|
| `ci-deployer@...` | GitHub Actions デプロイ | `roles/run.admin` / `roles/iam.serviceAccountUser` / `roles/artifactregistry.writer` / `roles/secretmanager.admin`（Terraform で Secret 作成用） |
| `ingestion-runtime@...` | `cmd/ingestion` ランタイム | `roles/secretmanager.secretAccessor`（必要な Secret のみ）/ `roles/pubsub.publisher`（topic） |
| `bff-runtime@...` | `cmd/bff` ランタイム | `roles/secretmanager.secretAccessor` / `roles/datastore.user`（Firestore）/ `roles/firebase.auth.admin`（Auth 検証） |
| `notifier-runtime@...` | `cmd/notifier` ランタイム | `roles/pubsub.subscriber` / `roles/datastore.user` / `roles/firebase.fcm.admin` |
| `cmsmigrate-runtime@...` | `cmd/cmsmigrate` ランタイム | `roles/secretmanager.secretAccessor` |

IAM は **最小権限原則**。各 runtime SA は自分の必要 Secret だけにアクセスできるよう `secretmanager.versions.access` を個別 binding。

### 3.3 WIF 設定（Terraform 例）

```hcl
resource "google_iam_workload_identity_pool" "github" {
  workload_identity_pool_id = "overseas-safety-map-pool"
}

resource "google_iam_workload_identity_pool_provider" "github" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-provider"
  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
    "attribute.ref"        = "assertion.ref"
  }
  attribute_condition = "assertion.repository == 'soneda-yuya/reearth-homework'"
  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account_iam_binding" "ci_deployer_wif" {
  service_account_id = google_service_account.ci_deployer.name
  role               = "roles/iam.workloadIdentityUser"
  members = [
    "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/soneda-yuya/reearth-homework",
  ]
}
```

GitHub Actions 側:

```yaml
- uses: google-github-actions/auth@v2
  with:
    workload_identity_provider: 'projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/overseas-safety-map-pool/providers/github-provider'
    service_account: 'ci-deployer@overseas-safety-map.iam.gserviceaccount.com'
```

---

## 4. IaC: Terraform（Q4 → B 昇格）

### 4.1 ディレクトリ構造

```
terraform/
├── main.tf                 # backend + providers
├── versions.tf             # terraform / provider バージョン固定
├── variables.tf            # project_id / region / image_tag 等
├── outputs.tf
├── apis.tf                 # google_project_service 群
├── artifact_registry.tf
├── secret_manager.tf       # Secret 定義のみ、値は別途 version 作成
├── pubsub.tf               # topic + subscription
├── service_accounts.tf     # runtime SA 群 + ci-deployer
├── wif.tf                  # Workload Identity Federation
├── iam.tf                  # project-level bindings
├── cloud_run_bff.tf        # BFF Service
├── cloud_run_ingestion.tf  # Ingestion Job
├── cloud_run_notifier.tf   # Notifier Service (Pub/Sub push)
├── cloud_run_setup.tf      # Setup Job
├── cloud_scheduler.tf      # ingestion 5min schedule
├── firestore.tf            # Firestore (Native mode 初期化、rules は別途 firebase CLI)
└── README.md
```

### 4.2 State バックエンド

```hcl
terraform {
  backend "gcs" {
    bucket = "overseas-safety-map-tfstate"
    prefix = "prod"
  }
}
```

- Bucket `overseas-safety-map-tfstate` は手動作成（bootstrap）、以降は Terraform 管理対象外（chicken-and-egg 回避）
- Versioning 有効、Uniform access

### 4.3 Provider バージョン

```hcl
terraform {
  required_version = "~> 1.9"
  required_providers {
    google      = { source = "hashicorp/google",      version = "~> 6.0" }
    google-beta = { source = "hashicorp/google-beta", version = "~> 6.0" }
  }
}
```

### 4.4 Cloud Run Env / Secret バインディング（Q7 [X] の実装）

```hcl
resource "google_cloud_run_v2_service" "bff" {
  name     = "bff"
  location = var.region

  template {
    service_account = google_service_account.bff_runtime.email

    containers {
      image = "asia-northeast1-docker.pkg.dev/${var.project_id}/app/bff:${var.bff_image_tag}"

      # 非機密 env
      env {
        name  = "PLATFORM_SERVICE_NAME"
        value = "bff"
      }
      env {
        name  = "PLATFORM_ENV"
        value = "prod"
      }
      env {
        name  = "PLATFORM_GCP_PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "PLATFORM_OTEL_EXPORTER"
        value = "gcp"
      }

      # Secret Manager バインディング（CloudRun が起動前に解決）
      env {
        name = "BFF_CMS_INTEGRATION_TOKEN"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.bff_cms_token.secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "BFF_FIREBASE_SERVICE_ACCOUNT_JSON"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.firebase_sa.secret_id
            version = "latest"
          }
        }
      }

      # Health Check
      startup_probe {
        http_get { path = "/healthz" }
        initial_delay_seconds = 3
        period_seconds        = 5
        failure_threshold     = 3
      }
      liveness_probe {
        http_get { path = "/healthz" }
        period_seconds    = 30
        failure_threshold = 3
      }

      resources {
        limits = { cpu = "1", memory = "512Mi" }
      }
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }
}
```

### 4.5 Image Tag の扱い

- `variables.tf` で `bff_image_tag` / `ingestion_image_tag` / ... を宣言
- GitHub Actions が `terraform apply -var='bff_image_tag=<git-sha>'` で更新
- main ブランチへの merge でビルド → push → `terraform apply` が発火

---

## 5. Docker イメージ（Q5 [A]）

### 5.1 マルチステージ Dockerfile（雛形、後続 Unit が踏襲）

```dockerfile
# ============ Build stage ============
FROM golang:1.26-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG DEPLOYABLE
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/app ./cmd/${DEPLOYABLE}

# ============ Final stage ============
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
```

### 5.2 `.dockerignore`

```
.git
.github
aidlc-docs
.aidlc-rule-details
terraform
*.md
```

### 5.3 イメージタグ戦略

- **main ブランチ**: `:{git-sha}` + `:latest`
- **feature ブランチ**: タグなし（CI でビルド検証のみ、push しない）
- **rollback**: `terraform apply -var='bff_image_tag=<過去の git-sha>'`

---

## 6. 命名規約（Q6 [A]）

### 6.1 各リソース命名

| リソース | パターン | 例 |
|---|---|---|
| GCP Project | `overseas-safety-map` | `overseas-safety-map` |
| Service Account | `{deployable}-runtime` | `ingestion-runtime@...`、`bff-runtime@...` |
| Service Account (CI) | `ci-deployer` | `ci-deployer@...` |
| Secret | `{deployable}-{purpose}` | `ingestion-claude-api-key`、`bff-cms-integration-token`、`firebase-service-account-json`（共有）|
| Pub/Sub Topic | `{domain}.{event}` | `safety-incident.new-arrival` |
| Pub/Sub Subscription | `{subscriber-deployable}-{topic}` | `notifier-safety-incident-new-arrival` |
| Cloud Run Service/Job | `{deployable}` | `bff` / `ingestion` / `notifier` / `cmsmigrate` |
| Cloud Scheduler Job | `{deployable}-{purpose}` | `ingestion-new-arrival-5min` |
| Artifact Registry Repo | `app` | `asia-northeast1-docker.pkg.dev/overseas-safety-map/app/` |
| Docker イメージ | `{deployable}:{tag}` | `bff:abc1234`、`ingestion:abc1234` |
| GCS Bucket (tfstate) | `overseas-safety-map-tfstate` | — |
| Firestore Collection | `users/{uid}` / `notification_logs/{keyCd}_{uid}` | — |

### 6.2 Terraform モジュール命名

`terraform/cloud_run_{deployable}.tf` のように Deployable 毎にファイル分割。モジュール化は MVP では不要（ファイル分割で十分）。

---

## 7. 環境変数注入（Q7 [X]）

### 7.1 方針の統合

NFR Design の `Secret Resolver` パターンと整合させるため、**2 モードをサポート**:

- **モード A（本番 / dev クラウド）**: Cloud Run の `env.value_source.secret_key_ref` で Terraform 管理 → アプリは **普通の環境変数として読む** だけ
- **モード B（ローカル開発）**: `.env` ファイルに平文の Secret を書く、アプリは同じく普通の環境変数として読む

これにより **アプリ側コードは単純**（`os.Getenv` / `envconfig` のみ）で、`Secret Resolver` は実装しなくてよい。NFR Design の `Secret Resolver` パターンは **参考設計** として残しつつ、実際は未実装で進める。

### 7.2 Config スキーマの簡素化

Functional Design の Config スキーマから `*_SECRET_NAME` という間接参照を削除し、**直接値が入る想定** にする:

```go
// Before (Functional Design)
ClaudeSecretName string `envconfig:"INGESTION_CLAUDE_SECRET_NAME" required:"true"`

// After (Infrastructure Design を反映)
ClaudeAPIKey string `envconfig:"INGESTION_CLAUDE_API_KEY" required:"true"`
```

Terraform 側で `env.value_source.secret_key_ref` → Secret Manager の実値がコンテナに注入される。

### 7.3 結果

- NFR Design LC-06 の `Secret Resolver` は **LC-06 (deferred)** として保留
- 代わりに **Terraform 管理の Secret バインディング**を標準とする
- コード側は `envconfig` のみで完結（Secret SDK 呼び出しなし）

---

## 8. 監視・ログ

### 8.1 Cloud Logging

- Cloud Run / Cloud Scheduler / その他 GCP サービスのログは自動で Cloud Logging に集約
- アプリ側の `slog` JSON 出力は Cloud Logging により自動構造化
- `trace_id` / `span_id` フィールドを検出して Trace 相関

### 8.2 Cloud Monitoring

- Cloud Run の組み込みメトリクス（requests / latency / errors）を自動収集
- OpenTelemetry Metrics はアプリから直接 Cloud Monitoring へエクスポート（`PLATFORM_OTEL_EXPORTER=gcp`）
- カスタムダッシュボード（後続 Unit でアラート追加）

### 8.3 Cloud Trace

- OpenTelemetry Traces を Cloud Trace にエクスポート
- Connect Interceptor と Pub/Sub messagingAttributes で trace propagation

### 8.4 アラート方針（MVP 最小）

- Cloud Run Service 5xx 率 > 5%（5 分間）→ Email 通知
- ingestion Job 失敗（exit code != 0）→ Email 通知
- Secret Manager の読み取り失敗 → Email 通知
- 具体的な条件は後続 Unit で詰める

---

## 9. コスト見積（参考、MVP 規模）

| サービス | 想定使用量 | 月額（USD 概算） |
|---|---|---|
| Cloud Run (BFF / Notifier) | 100 req/day、平均 200ms、min=0 | ~$1 |
| Cloud Run Jobs (ingestion/cmsmigrate) | 5分毎 × 30秒 = 288回/日 × 30s | ~$3 |
| Pub/Sub | 10 msg/day × 1KB | < $1 |
| Secret Manager | 10 シークレット、月 1000 アクセス | ~$0 |
| Artifact Registry | 50MB × 50 イメージ = 2.5GB | ~$0.25 |
| Firestore | 500 docs、1000 read/write/day | < $1（無料枠内） |
| Cloud Logging / Monitoring / Trace | 想定 ~500MB logs/month | < $1（無料枠内） |
| Mapbox Geocoding | 500 リクエスト/day × 30 = 15k/month | $0（100k 無料枠内） |
| Anthropic Claude Haiku | 500 呼び出し/day × 30 | ~$5 |
| Firebase Auth / FCM | 100 MAU | 無料 |
| GCS (tfstate) | <10MB | ~$0 |
| **合計** | | **~$10 / 月** |

MVP 規模では月 10USD 程度で運用可能。

---

## 10. 環境セットアップ手順（Bootstrap）

1. GCP プロジェクト作成（CLI or Console）: `gcloud projects create overseas-safety-map`
2. Billing 有効化
3. `gcloud auth application-default login`
4. tfstate Bucket を手動作成: `gsutil mb -l asia-northeast1 gs://overseas-safety-map-tfstate && gsutil versioning set on gs://overseas-safety-map-tfstate`
5. `terraform init` → `terraform apply`（Workload Identity Pool / Secret / Pub/Sub / Artifact Registry / SA / IAM）
6. GitHub Secrets に `GCP_PROJECT_NUMBER` / `GCP_PROJECT_ID` を登録
7. CI が有効化され、main merge で自動デプロイ開始
8. Firebase プロジェクト初期化（`firebase init`）+ Firestore rules 設定（U-USR 時に詳細化）

---

## 11. 受け入れ基準（U-PLT Infrastructure Design 完了条件）

- [ ] `terraform/` 配下に 13 の .tf ファイルが配置されている（Code Generation で生成）
- [ ] Workload Identity Federation で GitHub Actions から `gcloud` が使える
- [ ] Artifact Registry にダミーイメージを push できる
- [ ] Terraform apply が Cloud Run BFF のスケルトン（/healthz のみ返すコンテナ）を deploy できる
- [ ] `shared-infrastructure.md` に全 Unit 共通のインフラ規約が記載され、後続 Unit の Infrastructure Design が参照できる
