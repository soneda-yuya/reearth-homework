# overseas-safety-map

外務省 海外安全情報オープンデータを取り込み、LLM で発生地を抽出してジオコーディングし、[reearth-cms](https://github.com/reearth/reearth-cms) に蓄積して Flutter アプリ（iOS/Android）で地図・一覧・詳細・犯罪マップとして閲覧できる MVP プロジェクトです。

本リポジトリは **Go サーバーモノレポ**（ingestion / bff / notifier / cmsmigrate）です。Flutter アプリは別リポジトリ `overseas-safety-map-app`（未作成、U-APP で追加）。

## アーキテクチャ概要

```
MOFA XML (5分毎) ──> Cloud Run Job: ingestion ──┬──> reearth-cms (SaaS)
                                                 └──> Pub/Sub: safety-incident.new-arrival
                                                       │
                                                       ▼
                                                 Cloud Run Service: notifier ──> FCM
                                                                                 │
                                                                                 ▼
Flutter app (iOS/Android) ◀── Connect RPC ── Cloud Run Service: bff ◀── reearth-cms
                                                  ▲
                                                  └── Firebase Auth (ID Token 検証)
```

- **Core domain**: `safetyincident`（crimemap subdomain 含む）
- **Supporting domains**: `user` / `notification` / `cmsmigrate`
- **設計詳細**: [aidlc-docs/inception/application-design/application-design.md](aidlc-docs/inception/application-design/application-design.md)

## 使用技術（要点）

| レイヤ | 選定 |
|---|---|
| 言語 | Go 1.26 |
| RPC / Proto | Connect + buf |
| ログ / 観測性 | log/slog (JSON) + OpenTelemetry Metrics/Traces |
| Config / Secrets | envconfig + GCP Secret Manager |
| SDK | Firebase Admin v4 / Cloud Pub/Sub / Cloud Secret Manager / Mapbox (自前 HTTP) / Anthropic Claude |
| テスト | testing + pgregory.net/rapid (PBT) |
| Lint | golangci-lint + gofmt + buf lint |
| CI/CD | GitHub Actions (ubuntu-latest) |
| IaC | Terraform (GCS backend) |
| デプロイ | Cloud Run Service / Job, Workload Identity Federation |

## Getting Started

### 必要なツール

- Go 1.26+（`brew install go` or [公式](https://go.dev/dl/)）
- [buf](https://buf.build/docs/installation)
- [golangci-lint](https://golangci-lint.run/usage/install/)
- Docker（ローカルビルド用）
- gcloud CLI（デプロイ／Secret 操作用）

### 初期セットアップ

```bash
git clone https://github.com/soneda-yuya/reearth-homework
cd reearth-homework

make setup          # バージョン確認 + go mod download
make test           # 全テスト実行
make lint           # golangci-lint
make vuln           # govulncheck
```

### ローカルでのビルド

```bash
# 全 Deployable
make build

# 個別
make build-bff
make build-ingestion
make build-notifier
make build-cmsmigrate
```

### Proto コード生成

```bash
make proto-lint       # buf lint
make proto            # buf generate -> gen/go/v1/
make proto-breaking   # 破壊的変更チェック
```

### ローカル実行（bff の例）

`.env.example` を `.env` にコピーして必要な値を埋め、

```bash
set -a; source .env; set +a
./bin/bff
```

`/healthz` / `/readyz` で疎通確認可能。

### cmsmigrate（CMS スキーマ適用 Job）

`cmd/cmsmigrate` は reearth-cms の Project / Model / Field を**冪等 CREATE**で適用する一回実行 Job です。初回デプロイと、`internal/cmsmigrate/domain/schema_definition.go` を変更した後に実行します。

**必須 env**:

```
PLATFORM_SERVICE_NAME=cmsmigrate
PLATFORM_ENV=dev
PLATFORM_GCP_PROJECT_ID=overseas-safety-map
CMSMIGRATE_CMS_BASE_URL=https://cms.example.com
CMSMIGRATE_CMS_WORKSPACE_ID=wkp_XXXXXXXX
CMSMIGRATE_CMS_INTEGRATION_TOKEN=<token>
```

**ローカル実行**:

```bash
make build-cmsmigrate
set -a; source .env; set +a
./bin/cmsmigrate
```

このコマンドは実際に `CMSMIGRATE_CMS_BASE_URL` で指定された reearth-cms に HTTP で接続して Project / Model / Field を読み書きします。CMS が到達不能、Token が無効、または必須 env が未設定の場合は exit 1 になります。CMS への接続無しでバイナリの起動だけを試したいときは `go test ./internal/cmsmigrate/... -run Validate` などのユニットテストを実行してください。

**prod 実行**:

```bash
gcloud run jobs execute cms-migrate \
  --region=asia-northeast1 \
  --project=overseas-safety-map \
  --wait
```

- 自動リトライなし（`max_retries = 0`）。失敗したら Cloud Logging でエラー内容を確認し、修正後に同コマンドで再実行（冪等）。
- スキーマ drift を検知した場合は `WARN` ログに集約されますが、**自動上書きしません**。運用者が CMS 側か declaration 側を手動で揃えます。

### ingestion（MOFA 取込パイプライン Job）

`cmd/ingestion` は MOFA 海外安全情報 XML を取得し、Claude で発生地名を抽出 → Mapbox でジオコード（失敗時は国 Centroid フォールバック）→ reearth-cms に upsert → Pub/Sub 通知、までを行う Cloud Run Job です。Cloud Scheduler が `*/5 * * * *` で `incremental` モードを起動します。

**必須 env**:

```
PLATFORM_SERVICE_NAME=ingestion
PLATFORM_ENV=dev
PLATFORM_GCP_PROJECT_ID=overseas-safety-map
INGESTION_CMS_BASE_URL=https://cms.example.com
INGESTION_CMS_WORKSPACE_ID=wkp_XXXXXXXX
INGESTION_CMS_INTEGRATION_TOKEN=<token>
INGESTION_CLAUDE_API_KEY=<key>
INGESTION_MAPBOX_API_KEY=<key>
INGESTION_PUBSUB_TOPIC_ID=projects/overseas-safety-map/topics/safety-incident.new-arrival  # 短 topic 名 (safety-incident.new-arrival) も可
```

任意 env（envconfig default で吸収。本番では Terraform でも明示しない）:

```
INGESTION_MODE=incremental                 # initial | incremental
INGESTION_MOFA_BASE_URL=https://www.ezairyu.mofa.go.jp/html/opendata
INGESTION_CMS_PROJECT_ALIAS=overseas-safety-map
INGESTION_CMS_MODEL_ALIAS=safety-incident
INGESTION_CMS_KEY_FIELD=key_cd
INGESTION_CLAUDE_MODEL=claude-haiku-4-5
INGESTION_MAPBOX_MIN_SCORE=0.5
INGESTION_CONCURRENCY=5
INGESTION_LLM_RATE_LIMIT_RPM=300           # = 5 req/s
INGESTION_GEOCODE_RATE_LIMIT_RPM=600       # = 10 req/s
INGESTION_HTTP_TIMEOUT_SECONDS=30
```

**ローカル実行**:

```bash
make build-ingestion
set -a; source .env; set +a
./bin/ingestion
```

実 MOFA / Claude / Mapbox / reearth-cms / Pub/Sub に HTTP / gRPC 接続するため、いずれかが到達不能だと exit 1 になります。バイナリ起動だけ試したい場合は `go test ./internal/safetyincident/...` を実行してください。

**prod 実行**:

通常運用は **Cloud Scheduler が自動起動**するため操作不要。初回バックフィル（過去全件取込）のみ手動:

```bash
gcloud run jobs execute ingestion \
  --region=asia-northeast1 \
  --project=overseas-safety-map \
  --update-env-vars=INGESTION_MODE=initial \
  --wait
```

- 自動リトライなし（`max_retries = 0`）。失敗しても 5 分後の Scheduler tick で fresh Run が起動するため、`incremental` の連続取りこぼしは発生しにくい
- per-item 失敗は **skip + 構造化ログ + Metric**、Run 自体は exit 0（U-ING design Q7 [A]）。失敗 item は CMS に未登録のまま残り、次の Run で自動再試行される（冪等性 + skip-and-continue の合わせ技）
- ジオコーディング失敗時は **国 Centroid フォールバック**で必ず Item を保存。Flutter 側で `geocode_source = "country_centroid"` を見て「概算位置」UI を表示

## Deployment

GCP プロジェクト `overseas-safety-map`（asia-northeast1）に Cloud Run でデプロイします。詳細は [terraform/README.md](terraform/README.md) を参照。

- main ブランチ merge → GitHub Actions が自動で: docker build → Artifact Registry push → `terraform apply -var='*_image_tag=<git-sha>'`
- Cloud Run は各 Deployable のイメージタグ更新で無停止ロールアウト
- ロールバック: 過去の git-sha を渡して再 apply

## Repository Layout

```
cmd/                 4 Deployable の main（Composition Root）
internal/
  platform/          observability / config / connectserver / retry / ratelimit / SDK wrapper
  shared/            errs / clock / validate
  cmsmigrate/        U-CSS で追加（DDD: domain / application / infrastructure）
  safetyincident/    U-ING で追加（DDD: domain / application / infrastructure）
  <bounded-context>/ 後続 Unit で追加（user / notification）
    domain/
    application/
    infrastructure/
  interfaces/        後続 Unit で追加（rpc / job）
proto/v1/            Connect + Pub/Sub スキーマ（Go/Dart の生成ソース）
gen/go/v1/           buf generate 出力
terraform/           GCP インフラ IaC
.github/
  workflows/         CI / deploy / terraform-validate / setup-go smoke
  actions/setup-go/  composite action（Go + buf + govulncheck インストール）
aidlc-docs/          AI-DLC 設計ドキュメント
```

## AI-DLC Documentation

本プロジェクトは [AI-DLC](https://github.com/soneda-yuya/reearth-homework/tree/main/.aidlc-rule-details) の Adaptive ワークフローで設計・開発されています。各フェーズの設計ドキュメントは [`aidlc-docs/`](aidlc-docs/) 配下:

- [要件定義](aidlc-docs/inception/requirements/requirements.md)
- [ユーザーストーリー](aidlc-docs/inception/user-stories/stories.md)（13 MVP + 3 Post-MVP）
- [アプリケーション設計](aidlc-docs/inception/application-design/application-design.md)（DDD × Bounded Context）
- [Unit of Work](aidlc-docs/inception/application-design/unit-of-work.md)（6 Unit × Construction ループ）
- [Shared Infrastructure](aidlc-docs/construction/shared-infrastructure.md)
- [U-PLT 設計・実装](aidlc-docs/construction/U-PLT/)
- [U-CSS 設計・実装](aidlc-docs/construction/U-CSS/)
- [U-ING 設計・実装](aidlc-docs/construction/U-ING/)

## ライセンス / データ出典

本プロジェクトが扱う安全情報は **外務省 海外安全情報オープンデータ**（政府標準利用規約 第2.0 版、CC BY 4.0 互換）を出典としています。

> 出典：外務省 海外安全情報オープンデータ（<https://www.ezairyu.mofa.go.jp/html/opendata/>）

本アプリでは位置情報を LLM およびジオコーダで加工しています。アプリ UI 上でも同様の出典表記を行います。

## Contributing

PR は CI（`ci.yml`）が緑になってから main に merge してください。
Terraform の変更は `terraform-validate.yml` が PR で `terraform fmt -check` + `terraform validate` を実行します。WIF 権限は main ブランチに限定しているため CI では `terraform plan` は走りません。本物の diff が必要なときは `gcloud auth application-default login` のうえでローカル `terraform plan` を実行してください。
