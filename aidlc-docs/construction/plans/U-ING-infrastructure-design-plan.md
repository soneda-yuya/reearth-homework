# U-ING Infrastructure Design Plan

## Overview

U-ING (Ingestion Unit、Sprint 2) の **Infrastructure Design** 計画。U-PLT で `terraform/modules/ingestion/` の雛形が **ほぼ完成** しているため、本ステージは **U-ING Design に基づく差分追加** が中心。U-CSS Infra Design よりさらに薄く済む見込み。

## Context — すでに U-PLT で決まっていること

[`terraform/modules/ingestion/`](../../../terraform/modules/ingestion/) に以下が実装済み:

- `google_cloud_run_v2_job "ingestion"` — `cpu = 1`, `memory = 512Mi`, `timeout = 300s`
- Runtime SA: `ingestion-runtime`
- Env: `PLATFORM_*` 一式 + `INGESTION_MOFA_BASE_URL` / `INGESTION_PUBSUB_TOPIC` / `INGESTION_CMS_BASE_URL` / `INGESTION_CMS_WORKSPACE_ID` + 3 Secret (`INGESTION_CLAUDE_API_KEY` / `INGESTION_MAPBOX_API_KEY` / `INGESTION_CMS_INTEGRATION_TOKEN`)
- IAM:
  - Runtime SA に Secret 3 種 (`secretmanager.secretAccessor`)
  - Runtime SA に Pub/Sub topic publisher (`roles/pubsub.publisher`)
- Cloud Scheduler `ingestion-new-arrival-5min` (cron `*/5 * * * *`、`Asia/Tokyo`)
  - `scheduler-invoker` SA + `run.invoker` + `serviceAccountTokenCreator` IAM
  - `retry_config { retry_count = 0 }`
  - HTTP target で Cloud Run Job invoke API を叩く

[`environments/prod/main.tf`](../../../terraform/environments/prod/main.tf) で `module "ingestion"` を wire 済み。

## U-ING Design で確定済みの前提

[`U-ING/design/U-ING-design.md`](../U-ING/design/U-ING-design.md) より:

- `INGESTION_MODE` env で `initial` / `incremental` を切替（Q1 [A]）
- 5 分毎 Cloud Scheduler (Q2 [A])
- skip-and-continue + idempotent upsert で Run は exit 0 が常態（Q7 [A]）
- Rate limit は app 側で先制制御 (Q8 [A])
- LLM Concurrency = 5 (Q4 [A])

---

## Step-by-step Checklist

- [ ] Q1〜Q5 すべて回答
- [ ] 成果物を生成:
  - [ ] `construction/U-ING/infrastructure-design/deployment-architecture.md` — Cloud Run Job + Scheduler + IAM + Secret の最終形
  - [ ] `construction/U-ING/infrastructure-design/terraform-plan.md` — `modules/ingestion/` に対して必要な diff 要約
- [ ] 承認後、U-ING Code Generation へ進む

---

## Questions

### Question 1 — Cloud Run Job の `max_retries` 設定

現状: 未指定（GCP 既定 = **3 回**）。

U-ING Design Q7 [A] では「失敗 item は skip + 構造化ログ + Metric、**Run は exit 0 が常態**」。Run 全体が exit 1 で終わるのは MOFA fetch 失敗のような **本質的に再試行で回復する見込みが薄い** ケース。Cloud Run Job の自動リトライ (max_retries=3) は:
- exit 1 → 自動で 3 回まで Run 再実行
- 部分成功と相性が悪い (ただし Q3 idempotent upsert で多重実行は安全)

**選択肢**:

A) **推奨**: **`max_retries = 0`**（自動リトライ無効）。Run が exit 1 で終わったら次の Cloud Scheduler tick (5 分後) で fresh Run が起動する。Job retry のループを排除し、運用がシンプル
B) `max_retries = 1`（一度だけ再試行）。Cloud Run の起動エラー等の一時障害に保険
C) GCP 既定の 3 回維持。MOFA 一時障害を Job retry でカバー、5 分待たない
X) Other（[Answer]:）

[A]: A

### Question 2 — Scheduler の重複実行抑止 (max_concurrent_executions)

`google_cloud_run_v2_job` には `max_concurrent_executions` がない代わりに、Cloud Scheduler 側で **重複起動を抑止する仕組み** (前回が完了してから次を起動) が必要かどうか。

**前提**: Run は通常 < 60 秒で終わる。5 分間隔なので普通は重複しない。ただし MOFA 一時障害で Run が timeout (300s) まで延びると、次の tick (5 分後 = 同タイミング) と被る可能性がある。

**選択肢**:

A) **推奨**: **何もしない**（重複起動はあり得るが、Q3 [A] idempotent upsert があるので CMS 側で重複登録にはならない。Pub/Sub の重複 publish は U-NTF 側で受信時に dedup する想定。コスト的にも 1 Run = LLM/Mapbox の数分の負荷で許容範囲）
B) Cloud Scheduler の job に `attempt_deadline` を 4 分にする（Run timeout の前に Scheduler が諦めて次回に任せる）
C) Pub/Sub-based queue + worker パターンに変更（複雑、MVP 過剰）
X) Other（[Answer]:）

[A]: A

### Question 3 — `INGESTION_MODE` のデフォルト値と initial 実行手順

U-ING Design Q1 [A] では `initial` と `incremental` を env で切替。

**選択肢**:

A) **推奨**: Terraform で `INGESTION_MODE = "incremental"` を Cloud Run Job env に固定。`initial` 実行時は `gcloud run jobs execute ingestion --update-env-vars=INGESTION_MODE=initial,...` で **実行時 override**（永続化されない、その Run のみ initial）
B) Terraform で env 設定せず、毎回 `gcloud run jobs execute --update-env-vars=` で渡す（incremental も毎回明示）
C) `incremental` 用 Job と `initial` 用 Job を 2 つ作る（簡潔だが重複）
X) Other（[Answer]:）

[A]: A

### Question 4 — Cloud Run Job リソース (CPU / Memory) の見直し

現状: `cpu = 1`, `memory = 512Mi`（U-PLT 初期値）。

U-ING の処理特性:
- 主にネットワーク I/O (LLM, Mapbox, CMS, Pub/Sub の HTTP 呼び出し)
- 並列度 5 で同時 5 件処理 (Q4 [A])
- XML パース (`encoding/xml`) の負荷も小さい (新着 30 件程度)
- LLM レスポンス (JSON) は数 KB
- メモリは 100MB 以下で十分の見込み

**選択肢**:

A) **推奨**: **現状維持** (`cpu = 1` / `memory = 512Mi`)。余裕あり、initial mode (数千件) でも収まる
B) 小型化 (`cpu = 0.5` / `memory = 256Mi`)。コスト削減、incremental では十分だが initial で OOM の懸念
C) 大型化 (`cpu = 2` / `memory = 1Gi`)。並列度を上げる将来の余地、現状コストは小さい
X) Other（[Answer]:）

[A]: A

### Question 5 — Terraform 環境変数の追加

U-ING Design で新たに必要になった env を Terraform `modules/ingestion/main.tf` の Cloud Run Job 定義に追加する必要があります:

| 追加する env | 値の出所 |
|---|---|
| `INGESTION_MODE` | Q3 [A] によりデフォルト `incremental` |
| `INGESTION_CMS_PROJECT_ALIAS` | デフォルト `overseas-safety-map`（envconfig default で吸収する手もあり） |
| `INGESTION_CMS_MODEL_ALIAS` | デフォルト `safety-incident` |
| `INGESTION_CLAUDE_MODEL` | デフォルト `claude-haiku-4-5` |
| `INGESTION_PUBSUB_TOPIC_ID` | shared module output から |
| `INGESTION_CONCURRENCY` | デフォルト `5`（envconfig default でカバー、Terraform で出さない選択肢あり） |
| `INGESTION_LLM_RATE_LIMIT` | デフォルト `5`（同上） |
| `INGESTION_GEOCODE_RATE_LIMIT` | デフォルト `10`（同上） |

これらをどこまで Terraform で渡すか、どこまで envconfig default に任せるか:

A) **推奨**: **可変性が運用上重要なものだけ Terraform で渡す**:
  - Terraform 渡し: `INGESTION_MODE`, `INGESTION_PUBSUB_TOPIC_ID` (shared module の output 経由)
  - envconfig default に任せる: `INGESTION_CMS_PROJECT_ALIAS`, `INGESTION_CMS_MODEL_ALIAS`, `INGESTION_CLAUDE_MODEL`, `INGESTION_CONCURRENCY`, `INGESTION_LLM_RATE_LIMIT`, `INGESTION_GEOCODE_RATE_LIMIT`
  - 理由: tuning パラメータ (concurrency / rate limit) は env で動的に変えられるが、デフォルトで本番運用に十分な値を持つ。コードの値を変えるのと Terraform を変えるのとで二重管理を避ける
B) すべて Terraform で明示的に渡す。Terraform = Source of Truth、envconfig default は使わない
C) すべて envconfig default に任せる。Terraform は最小限。env override は手動操作で
X) Other（[Answer]:）

[A]: A

---

## 承認前の最終確認（回答確定）

- **Q1 [A]**: Cloud Run Job `max_retries = 0`（5 分後の Scheduler tick が事実上 retry を担う）
- **Q2 [A]**: Scheduler 重複実行抑止 = **何もしない**（idempotent upsert + U-NTF 側 dedup に依存）
- **Q3 [A]**: `INGESTION_MODE` デフォルト = Terraform で **`incremental`** 固定、initial は `gcloud --update-env-vars` で実行時 override
- **Q4 [A]**: Cloud Run Job リソース = 現状維持（`cpu = 1` / `memory = 512Mi`）
- **Q5 [A]**: env の Terraform 反映粒度 = **運用ポリシー / 依存関係 (FQ ID) のみ Terraform**、tuning パラメータは envconfig default に任せる

回答確定済み。以下を生成:

- `construction/U-ING/infrastructure-design/deployment-architecture.md`
- `construction/U-ING/infrastructure-design/terraform-plan.md`
