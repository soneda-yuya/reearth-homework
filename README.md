# overseas-safety-map

外務省 海外安全情報オープンデータを取り込み、LLM で発生地を抽出してジオコーディングし、[reearth-cms](https://github.com/reearth/reearth-cms) に蓄積して Flutter アプリ（iOS/Android）で地図・一覧・詳細・犯罪マップとして閲覧できる MVP プロジェクトです。

本リポジトリは **Go サーバーモノレポ**（ingestion / bff / notifier / setup）です。Flutter アプリは別リポジトリ `overseas-safety-map-app`（未作成、U-APP で追加）。

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
- **Supporting domains**: `user` / `notification` / `cmssetup`
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
make build-setup
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
  <bounded-context>/ 後続 Unit で追加（safetyincident / user / notification / cmssetup）
    domain/
    application/
    infrastructure/
  interfaces/        後続 Unit で追加（rpc / job）
proto/v1/            Connect + Pub/Sub スキーマ（Go/Dart の生成ソース）
gen/go/v1/           buf generate 出力
terraform/           GCP インフラ IaC
.github/
  workflows/         CI / deploy / terraform-plan / setup-go smoke
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

## ライセンス / データ出典

本プロジェクトが扱う安全情報は **外務省 海外安全情報オープンデータ**（政府標準利用規約 第2.0 版、CC BY 4.0 互換）を出典としています。

> 出典：外務省 海外安全情報オープンデータ（<https://www.ezairyu.mofa.go.jp/html/opendata/>）

本アプリでは位置情報を LLM およびジオコーダで加工しています。アプリ UI 上でも同様の出典表記を行います。

## Contributing

PR は CI（`ci.yml`）が緑になってから main に merge してください。
Terraform の変更は `terraform-plan.yml` が PR で `terraform fmt -check` + `terraform validate` を実行します。WIF 権限は main ブランチに限定しているため CI では `terraform plan` は走りません。本物の diff が必要なときは `gcloud auth application-default login` のうえでローカル `terraform plan` を実行してください。
