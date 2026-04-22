# U-CSS Infrastructure Design Plan

## Overview

U-CSS（CMS Migrate Unit、Sprint 1）の **Infrastructure Design** 計画。U-PLT で Terraform `modules/cmsmigrate/` がすでに雛形として整備されているため、本ステージは **差分確認 + U-CSS 固有値の確定** が中心。薄く済む見込み。

## Context — すでに U-PLT で決まっていること

[`terraform/modules/cmsmigrate/`](../../../terraform/modules/cmsmigrate/) に以下が実装済み:

- `google_cloud_run_v2_job "cmsmigrate"` — name = `cms-migrate`、region = `asia-northeast1`
- Runtime SA: `cmsmigrate-runtime`
- Env: `PLATFORM_*`（service/env/project/otel 共通）、`CMSMIGRATE_CMS_BASE_URL`、`CMSMIGRATE_CMS_WORKSPACE_ID`、`CMSMIGRATE_CMS_INTEGRATION_TOKEN`（Secret Manager）
- IAM: Runtime SA に `secretmanager.secretAccessor`（CMS Token 向け）
- 現状値: `timeout = 120s`、`cpu = 1`、`memory = 256Mi`、`max_retries` 未指定（= GCP 既定 3）

[`environments/prod/main.tf`](../../../terraform/environments/prod/main.tf) で `module "cmsmigrate"` を wire 済み。

## U-CSS Design で確定済みの前提

[`U-CSS/design/U-CSS-design.md`](../U-CSS/design/U-CSS-design.md) より:

- 初回実行 **< 60 秒**、再実行（全 no-op）**< 10 秒**（NFR-CSS-PERF-01/02）
- **手動実行のみ**（Scheduler 不要、Q3 [A]）
- **fail-fast**（exit 1、Cloud Run Job 側でリトライされる意味は薄い、Q4 [A]）
- Runtime SA は Secret 読み取り + OTel 送信のみ（NFR-CSS-SEC-03）
- reearth-cms 側へ Bearer Token で認証（IAM 依存なし）

---

## Step-by-step Checklist

- [ ] Q1〜Q6 すべて回答
- [ ] reearth-cms 本体のホスト前提（Q5）を確認
- [ ] 成果物を生成:
  - [ ] `construction/U-CSS/infrastructure-design/deployment-architecture.md` — Cloud Run Job + IAM + Secret の最終形
  - [ ] `construction/U-CSS/infrastructure-design/terraform-plan.md` — `modules/cmsmigrate/` に対して必要な追加 / 調整の diff 要約
- [ ] 承認後、U-CSS Code Generation へ進む

---

## Questions

### Question 1 — Job リソース制限（CPU / Memory）

現状: `cpu = 1`、`memory = 256Mi`。

想定負荷: 19 Field × HTTP リクエスト（各 500ms〜1s 程度）+ slog + OTel。主にネットワーク I/O、CPU / メモリ消費は小さい。

A) **推奨**: 現状維持（`cpu = 1` / `memory = 256Mi`）。`go run` の最小構成 + HTTP ペイロードは数十 KB なので 256Mi で十分。
B) 余裕を持って `memory = 512Mi` に引き上げ（他 Job とのベースライン揃え優先）
C) より絞って `cpu = 0.5` / `memory = 128Mi`（コスト最小、ただし Cold Start + OTel exporter 起動時に瞬間的に足りなくなる懸念）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 2 — Task Timeout

現状: `timeout = 120s`。

U-CSS Design の NFR-CSS-PERF-01 は「初回実行 < 60 秒」。ただし初回 CMS プロジェクト作成がアカウント初期化を伴うと重くなる可能性、また retry `200ms→400ms→800ms` が 19 Field × 最大 3 回発生し得ると理論上 45 秒程度追加。

A) **推奨**: 現状維持 **120s**（初回想定 60s + バッファ 2x、fail-fast なので長すぎる必要なし）
B) 余裕を持って **300s**（retry 連鎖 + 想定外の遅延に対して広めのマージン）
C) より絞って **60s**（NFR ぴったり、タイムアウトで気付きやすい）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 3 — Max Retries（Cloud Run Job レベル）

現状: 未指定（GCP 既定 **3 回**）。

U-CSS Design は Q4 [A] で「fail-fast、exit 1」。既作成分は次回実行で補完されるため、**Cloud Run Job の自動リトライが再度 CMS への無駄な Find を走らせるだけ**で価値が薄い。ただし一時的な 5xx / 429 は application 層の retry で吸収している（NFR-CSS-REL-03）。

A) **推奨**: **`max_retries = 0`**（自動リトライ無効。手動実行が前提なので、失敗したら運用者がログを見て判断 → 手動再実行）
B) 現状維持（GCP 既定 3 回、一時障害を透過的に吸収）
C) `max_retries = 1`（一度だけ再試行 — Cloud Run の起動時失敗などに備える最小保険）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 4 — Secret Manager のライフサイクル

現状: `cms_integration_token` が `modules/shared/` で定義されているかの再確認が必要。U-PLT Infrastructure Design で `cms_integration_token_secret_id` / `_secret_name` の output 経路は整えられている。

Secret の **rotation（値の差し替え）手順** をどうするか？

A) **推奨**: **手動 rotation**（Secret Manager Console / `gcloud secrets versions add` で新 version 追加 → `env.value_source.secret_key_ref.version = "latest"` が自動追従）。プロセス再起動で反映されるため、cmsmigrate Job は次回実行時に自動で新 Token を使う。
B) Terraform で version を固定（`version = "3"` 等）、rotation 時に Terraform 変更として PR を切る（監査性 +、運用手数 -）
C) 外部 Secret Rotation 機構（Scheduler + Cloud Function）を組む（自動化、ただし MVP には過剰）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 5 — reearth-cms 本体のホスト前提

U-CSS は **reearth-cms Integration REST API** を叩く Job。その reearth-cms 本体は今回のプロジェクトではどこで動いている前提ですか？

A) **別の GCP プロジェクト / 既存の reearth-cms 環境**（本プロジェクト `overseas-safety-map` では CMS 本体はデプロイしない。CMS_BASE_URL は既存の reearth-cms インスタンスを指す外部 URL）
B) 同じ `overseas-safety-map` プロジェクト内に CMS 本体も Cloud Run でデプロイする（本ワークフローで CMS 本体インフラも Terraform 管理）
C) SaaS 版の reearth-cms（hosted.reearth.io など）を使う
X) Other（[Answer]: の後ろに自由記述 — CMS_BASE_URL と Workspace の入手方法を明記してください）

> 注: これはインフラ設計の **前提条件** を確定させる質問です。Terraform `tfvars` に入れる `cms_base_url` / `cms_workspace_id` の値をどこから持ってくるかが決まります。

[A]: 

### Question 6 — 監視 / アラート

Cloud Run Job 実行失敗の検知はどこまで実装する？

A) **推奨**: **ログ確認運用のみ**（MVP）。Cloud Logging で `resource.type=cloud_run_job` かつ `severity=ERROR` を目視確認。構造化ログ + OTel Metric（`app.cmsmigrate.run.failure`、`app.cmsmigrate.drift.detected`）は実装するがアラート発報はしない。手動実行運用なので、実行者がその場でログを見る。
B) Cloud Monitoring Alerting Policy を Terraform で定義（`app.cmsmigrate.run.failure > 0` で Email/Slack 通知）
C) Cloud Logging Sink → Pub/Sub → Cloud Function → Slack Webhook（柔軟だが作り込み量が多い）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

---

## 承認前の最終確認（回答後に AI が埋めます）

- Job リソース制限: _TBD_
- Task Timeout: _TBD_
- Max Retries: _TBD_
- Secret rotation 手順: _TBD_
- reearth-cms ホスト前提: _TBD_
- 監視 / アラート: _TBD_

回答完了後、矛盾・曖昧さがなければ以下を生成:

- `construction/U-CSS/infrastructure-design/deployment-architecture.md`（Cloud Run Job + IAM + Secret + 運用の最終形）
- `construction/U-CSS/infrastructure-design/terraform-plan.md`（`modules/cmsmigrate/` に対して必要な diff 要約）
