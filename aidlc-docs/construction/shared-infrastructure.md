# Shared Infrastructure — overseas-safety-map

**全 Unit（U-PLT / U-CSS / U-ING / U-BFF / U-NTF / U-APP）が共有するインフラ規約**。U-PLT の Infrastructure Design で確定。後続 Unit の Infrastructure Design は本ドキュメントに追加で **固有事項のみ** 記述する。

詳細は [U-PLT/infrastructure-design/](./U-PLT/infrastructure-design/) 参照。

---

## 1. GCP プロジェクト

- **プロジェクト ID**: `overseas-safety-map`
- **リージョン**: `asia-northeast1`（東京）単一
- **環境**: MVP は単一（prod 扱い）、dev 兼用
- **Billing**: 有効（月額見積 ~$10）
- **有効 API**: Run / Pub/Sub / Secret Manager / Artifact Registry / Cloud Scheduler / Firestore / Identity Toolkit / FCM / IAM Credentials / Cloud Trace / Monitoring / Logging

## 2. IAM / Identity

- **CI 認証**: Workload Identity Federation（GitHub OIDC）
- **Pool / Provider**:
  - Pool: `overseas-safety-map-pool`
  - Provider: `github-provider`
  - 条件: `assertion.repository == 'soneda-yuya/reearth-homework'`
- **Service Accounts**（すべて `@overseas-safety-map.iam.gserviceaccount.com`）:
  - `ci-deployer` — CI デプロイ用（Terraform / Cloud Run / Artifact Registry 管理権限）
  - `ingestion-runtime` — Pub/Sub Publisher + Secret Accessor（ingestion 関連のみ）
  - `bff-runtime` — Firestore User + Firebase Auth Admin + Secret Accessor（bff 関連のみ）
  - `notifier-runtime` — Pub/Sub Subscriber + Firestore User + FCM Admin
  - `setup-runtime` — Secret Accessor（CMS token のみ）
- **原則**: 最小権限。各 SA は担当 Secret のみ `secretmanager.versions.access` binding

## 3. IaC: Terraform

- **ディレクトリ**: `terraform/`
- **Backend**: GCS `overseas-safety-map-tfstate`（versioning 有効）
- **バージョン**: Terraform 1.9+、google provider 6.x
- **管理対象**: **全 GCP リソース**（Cloud Run / Pub/Sub / Secret Manager / Artifact Registry / IAM / Scheduler / Firestore）
- **Secret 値は未管理**（Terraform では secret 定義のみ、実値は `gcloud secrets versions add` で手動投入）
- **各 Deployable の image tag** は `-var='{deployable}_image_tag=<git-sha>'` で上書き

## 4. Docker

- **ベース**: `gcr.io/distroless/static-debian12:nonroot`
- **ビルド**: マルチステージ、`CGO_ENABLED=0` 静的ビルド
- **Registry**: `asia-northeast1-docker.pkg.dev/overseas-safety-map/app/{deployable}:{git-sha}`
- **Tag 戦略**: `{git-sha}` + `latest`（main のみ）

## 5. 命名規約

| リソース | パターン | 例 |
|---|---|---|
| GCP Project | `overseas-safety-map` | — |
| Service Account | `{purpose}@...` | `ingestion-runtime@...` |
| Secret | `{deployable}-{purpose}` または `{scope}-{purpose}` | `ingestion-claude-api-key`、`cms-integration-token`（共有） |
| Pub/Sub Topic | `{domain}.{event}` | `safety-incident.new-arrival` |
| Pub/Sub Subscription | `{subscriber-deployable}-{topic}` | `notifier-safety-incident-new-arrival` |
| Cloud Run Service/Job | `{deployable}` | `bff` / `ingestion` / `notifier` / `setup` |
| Cloud Scheduler Job | `{deployable}-{purpose}` | `ingestion-new-arrival-5min` |
| Artifact Registry | `app`（リポジトリ名） | `.../app/` |

## 6. 環境変数 / Secrets 注入

- **非機密 env**: Terraform `google_cloud_run_v2_service/_job.template.containers.env { name, value }`
- **機密 env**: `google_cloud_run_v2_service/_job.template.containers.env { name, value_source { secret_key_ref { secret, version } } }`
- **アプリ側**: 通常の `os.Getenv` / `envconfig`（Secret Manager SDK は使わない）
- **ローカル開発**: `.env` ファイルに平文、`.gitignore` 登録

## 7. 共通 Secrets 一覧

| Secret | 用途 | Consumer SA |
|---|---|---|
| `ingestion-claude-api-key` | Claude | `ingestion-runtime` |
| `ingestion-mapbox-api-key` | Mapbox | `ingestion-runtime` |
| `cms-integration-token` | reearth-cms（共有） | `ingestion-runtime` / `bff-runtime` / `setup-runtime` |
| `firebase-service-account-json` | Firebase Admin SDK（共有） | `bff-runtime` / `notifier-runtime` |

## 8. 観測性

- **Logging**: Cloud Logging（slog JSON → 自動構造化）
- **Metrics**: OpenTelemetry → Cloud Monitoring（`PLATFORM_OTEL_EXPORTER=gcp`）
- **Tracing**: OpenTelemetry → Cloud Trace
- **相関**: `trace_id` / `span_id` を slog attr に自動付与

## 9. 共通の Pub/Sub Topic / Subscription

| Topic | Publisher | Subscription | Subscriber |
|---|---|---|---|
| `safety-incident.new-arrival` | `ingestion-runtime` | `notifier-safety-incident-new-arrival`（push 配信） | `notifier-runtime` |

- **Push Endpoint**: `https://notifier-{hash}-an.a.run.app/pubsub/push`
- **DLQ**: `safety-incident.new-arrival.dlq`（リトライ 5 回で移送）
- **Ack Deadline**: 60 秒（notifier 処理時間 + 余裕）

## 10. CI/CD

- **Platform**: GitHub Actions（`ubuntu-latest`）
- **ワークフロー**:
  - `.github/workflows/ci.yml` — PR / main push で静的チェック + test + build
  - `.github/workflows/deploy.yml` — main push（CI 成功後）に docker push + terraform apply
  - `.github/workflows/terraform-validate.yml` — terraform/ 配下 PR で fmt + validate（WIF は main 限定のため plan はローカル実行）
  - `.github/workflows/setup-go.yml`（reusable） — Go 1.26 + buf + govulncheck セットアップ

## 11. コストガードレール

- GCP Budget Alert: 月次予算 $30（MVP 実コスト ~$10 の 3 倍を閾値に）
- 50% / 90% / 100% で Email 通知

## 12. Flutter アプリ側（U-APP、別リポ）

Flutter アプリは **別リポジトリ `overseas-safety-map-app`** で独立 CI。以下のみ共有:

- **Firebase プロジェクト**: 同じ GCP プロジェクト `overseas-safety-map` に紐付く
- **BFF URL**: Cloud Run Service `bff` の URL を `.env.production` に記録
- **Connect スキーマ**: Go 側と同じ `proto/v1/*.proto`（サブモジュール or CI コピーで同期、詳細は U-APP Infrastructure Design で決定）

---

## 13. 各 Unit の Infrastructure Design の記述範囲

本ドキュメントで確定した共通部分を前提に、各 Unit は以下に絞って Infrastructure Design を記述する:

| Unit | 固有で記述する範囲 |
|---|---|
| U-CSS | `cmd/setup` 用 Cloud Run Job の実行タイミング、SA 権限調整 |
| U-ING | `cmd/ingestion` 用 Cloud Run Job、Cloud Scheduler 設定、Pub/Sub Publisher 権限 |
| U-BFF | `cmd/bff` 用 Cloud Run Service、Ingress / ドメイン、Firestore Security Rules |
| U-NTF | `cmd/notifier` 用 Cloud Run Service（Pub/Sub push）、DLQ 設定、FCM 権限 |
| U-APP | Flutter リポ初期化、Firebase プロジェクトとの連携、TestFlight / Play Console 配信パイプライン |
