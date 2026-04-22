# U-PLT Infrastructure Design Plan (Minimal)

## Overview

U-PLT の Infrastructure Design は **全 Unit が共有する基盤インフラ** を確定します。GCP プロジェクト構成、CI/CD、Artifact Registry、IaC ツール、命名規約など、後続 Unit の Infrastructure Design で繰り返し参照される部分です。

成果物として `construction/shared-infrastructure.md` を生成し、各 Unit が参照する形にします。

7 項目に絞って確定します。推奨 A で進めば最短です。

---

## Step-by-step Checklist

- [ ] Q1〜Q7 すべて回答
- [ ] 矛盾・曖昧さの検証
- [ ] 成果物 3 点を生成:
  - [ ] `construction/U-PLT/infrastructure-design/infrastructure-design.md`
  - [ ] `construction/U-PLT/infrastructure-design/deployment-architecture.md`
  - [ ] `construction/shared-infrastructure.md`（全 Unit 共有、新規）
- [ ] 承認後、U-PLT Code Generation へ進む

---

## Context Summary

- **所属 Unit**: U-PLT
- **NFR Design 決定済み**: 2-tier Health Check / Token Bucket Rate Limit / Retry / Graceful Shutdown / Secret Resolver
- **Deployable 形態**: 全サービス Cloud Run（Job または Service）
- **外部依存**: GCP（Secret Manager / Pub/Sub / Cloud Run / Artifact Registry）/ Firebase / MOFA / Mapbox / Anthropic

---

## Questions

### Question 1 — GCP プロジェクト戦略

GCP プロジェクトをどう分割しますか？

A) **推奨**: **dev / prod の 2 プロジェクト**（`overseas-safety-map-dev` / `overseas-safety-map-prod`）。Firebase も同プロジェクト内に相乗り（Firebase は GCP プロジェクトに紐付く）。
B) **単一プロジェクト**（`overseas-safety-map`）でリソース名のサフィックス（`-dev` / `-prod`）で分離 — 一番安い
C) **dev / staging / prod の 3 プロジェクト**
D) 既存プロジェクトがある（ユーザー指定）
X) Other（[Answer]: の後ろに自由記述）

[X,prodだけ良いです。overseas-safety-map]: 

### Question 2 — リージョン

Cloud Run / Pub/Sub / Firestore のリージョン。

A) **推奨**: **`asia-northeast1`（東京）** 単一リージョン。アプリ利用者（日本人海外渡航者）との距離最短。
B) `us-central1`（無料枠が大きい）
C) `asia-northeast1` + `us-central1` のマルチリージョン（可用性優先、MVP スコープ超過）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 3 — GitHub Actions → GCP 認証

CI から GCP にデプロイする際の認証方式。

A) **推奨**: **Workload Identity Federation**（GitHub OIDC トークン → GCP サービスアカウント）。鍵ファイルをリポジトリにコミットせず、セキュリティ的に最良。
B) サービスアカウントキー（JSON）を GitHub Secrets に格納
C) デプロイは手動（gcloud CLI）、CI からは行わない
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 4 — IaC ツール

GCP リソース（Secret / Pub/Sub / Cloud Run / Artifact Registry / IAM）の管理方法。

A) **推奨**: **Terraform**（`terraform/` ディレクトリに tfstate を GCS backend で管理）。最初は Cloud Run を除いたリソースのみ、Cloud Run は GitHub Actions + `gcloud` でデプロイ。
B) **Terraform** で全リソース（Cloud Run も含む）
C) **IaC なし** — 初期は `gcloud` コマンドを `Makefile` / シェルスクリプトに記録、後で Terraform 化
D) **Pulumi**（Go で記述、言語統一）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 5 — Docker イメージ

Cloud Run にデプロイする Go アプリの Docker イメージ。

A) **推奨**: **マルチステージビルド**、最終段は **`gcr.io/distroless/static-debian12:nonroot`**（バイナリのみ、最小）。`CGO_ENABLED=0` で静的ビルド。イメージは **Artifact Registry**（`asia-northeast1-docker.pkg.dev/{project}/app/{deployable}:{tag}`）にプッシュ。
B) Alpine ベース（シェルが欲しい場合、デバッグしやすい）
C) Ubuntu / Debian フル（互換性重視、イメージは大きい）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 6 — 命名規約（Secret / Pub/Sub / Cloud Run / Artifact Registry）

全リソースの命名規約。

A) **推奨**: 以下の統一規約
  - Secret: `{deployable}-{purpose}`（例: `ingestion-claude-api-key`、`bff-cms-integration-token`）
  - Pub/Sub Topic: `safety-incident.new-arrival`（ドメイン基準、Deployable 非依存）
  - Pub/Sub Subscription: `{subscriber-deployable}-{topic-name}`（例: `notifier-safety-incident-new-arrival`）
  - Cloud Run Service / Job: `{deployable}`（`ingestion` / `bff` / `notifier` / `setup`）
  - Artifact Registry: リポジトリ名 `app`、タグ `{git-sha}` + `latest`（main ブランチのみ）
B) すべてに `overseas-safety-map-` プレフィックス付与
C) 個別に決定（統一しない）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 7 — 環境変数の注入方法

Cloud Run での環境変数設定方法。

A) **推奨**: **`--set-env-vars` で非機密**（`PLATFORM_SERVICE_NAME`、`PLATFORM_ENV` など）+ **`--set-secrets` で機密**（Secret Manager 参照を直接）。起動時に Secret Manager SDK を明示的に呼ぶ必要がなくなり、コードがシンプル。
B) すべて環境変数で渡す（Secret も `gcloud run deploy --update-secrets` で実値展開）
C) アプリ側で Secret Manager SDK を呼び出し（NFR Design の `Secret Resolver` パターン通り）
X) Other（[Answer]: の後ろに自由記述）

**注**: A / B / C いずれも NFR Design の `Secret Resolver` パターンと互換。A/B は **Cloud Run が起動前に解決**、C は **アプリが起動時に解決**（既定）。

[X,terraformからコンテナに割り当てできないですか？]: 

---

## 承認前の最終確認（回答後に AI が埋めます）

- GCP プロジェクト: _TBD_
- リージョン: _TBD_
- CI → GCP 認証: _TBD_
- IaC ツール: _TBD_
- Docker イメージ: _TBD_
- 命名規約: _TBD_
- 環境変数注入: _TBD_

回答完了後、矛盾・曖昧さがなければ 3 成果物（`infrastructure-design.md` / `deployment-architecture.md` / `shared-infrastructure.md`）を生成します。
