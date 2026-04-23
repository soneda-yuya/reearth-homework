# U-ING Deployment Architecture

**Unit**: U-ING（Ingestion Unit、Sprint 2）
**Deployable**: `cmd/ingestion` → Cloud Run Job `ingestion`
**起動**: Cloud Scheduler `ingestion-new-arrival-5min` (`*/5 * * * *`)
**参照**: [`U-ING/design/U-ING-design.md`](../design/U-ING-design.md)、[`construction/shared-infrastructure.md`](../../shared-infrastructure.md)

---

## 1. Component Overview

```
┌──────────────────────────── GCP Project: overseas-safety-map (prod) ────────────────────────────┐
│  Region: asia-northeast1                                                                         │
│                                                                                                  │
│   ┌─ Cloud Scheduler: ingestion-new-arrival-5min ───┐                                            │
│   │  Schedule: */5 * * * * (Asia/Tokyo)             │                                            │
│   │  retry_count = 0                                 │                                            │
│   │  HTTP target → Cloud Run Job invoke API          │                                            │
│   │  oauth_token: scheduler-invoker SA               │                                            │
│   └────────────┬────────────────────────────────────┘                                            │
│                │ POST /apis/run.googleapis.com/v1/.../jobs/ingestion:run                         │
│                ▼                                                                                 │
│   ┌─ Cloud Run Job: ingestion ─────────────────────┐                                             │
│   │  image: <AR_URL>/ingestion:<tag>               │                                             │
│   │  cpu = 1, memory = 512Mi                       │                                             │
│   │  timeout = 300s, max_retries = 0  ← 新規        │                                             │
│   │  SA: ingestion-runtime                         │                                             │
│   │                                                │                                             │
│   │  ENV (Terraform 渡し):                          │                                             │
│   │    PLATFORM_*                                  │                                             │
│   │    INGESTION_MODE = incremental    ← 新規      │                                             │
│   │    INGESTION_PUBSUB_TOPIC_ID = ... ← 新規      │                                             │
│   │    INGESTION_MOFA_BASE_URL                     │                                             │
│   │    INGESTION_PUBSUB_TOPIC                      │                                             │
│   │    INGESTION_CMS_BASE_URL                      │                                             │
│   │    INGESTION_CMS_WORKSPACE_ID                  │                                             │
│   │    INGESTION_CLAUDE_API_KEY (Secret) ─┐        │                                             │
│   │    INGESTION_MAPBOX_API_KEY (Secret) ─┤        │                                             │
│   │    INGESTION_CMS_INTEGRATION_TOKEN ───┤        │                                             │
│   │                                       │        │                                             │
│   │  ENV (envconfig default):             │        │                                             │
│   │    INGESTION_CMS_PROJECT_ALIAS, ..._MODEL_ALIAS│                                             │
│   │    INGESTION_CLAUDE_MODEL                      │                                             │
│   │    INGESTION_CONCURRENCY = 5                   │                                             │
│   │    INGESTION_LLM_RATE_LIMIT = 5                │                                             │
│   │    INGESTION_GEOCODE_RATE_LIMIT = 10           │                                             │
│   └─────┬───────────────────────┬─────────┼────────┘                                             │
│         │                       │         │                                                      │
│         │ outbound HTTPS        │         ▼                                                      │
│         │                       │   ┌─ Secret Manager ────────────────┐                          │
│         │                       │   │  ingestion-claude-api-key       │                          │
│         │                       │   │  ingestion-mapbox-api-key       │                          │
│         │                       │   │  cms-integration-token (共有)    │                          │
│         │                       │   └─────────────────────────────────┘                          │
│         │                       │                                                                │
│         │                       ▼                                                                │
│         │       ┌─ Pub/Sub Topic: safety-incident.new-arrival ─┐                                 │
│         │       │  + DLQ topic                                  │                                 │
│         │       │  Publisher: ingestion-runtime                 │                                 │
│         │       │  Subscriber: notifier-runtime (U-NTF, push)   │                                 │
│         │       └────────────────────────────────────────────────┘                                │
│         │                                                                                        │
└─────────┼────────────────────────────────────────────────────────────────────────────────────────┘
          │
          │ HTTPS (outbound)
          ▼
   ┌─────────────────────┐  ┌──────────────────────┐  ┌──────────────────────┐
   │  MOFA OpenData      │  │  Anthropic Claude    │  │  Mapbox Geocoding    │
   │  /00A.xml           │  │  Haiku 4.5           │  │  /geocoding/v5       │
   │  /newarrivalA.xml   │  │  (extract location)  │  │  (geocode location)  │
   └─────────────────────┘  └──────────────────────┘  └──────────────────────┘
   ┌─────────────────────────────────────────────────┐
   │  External reearth-cms instance (U-CSS と同じ)   │
   │  Integration REST API (Item CRUD)               │
   └─────────────────────────────────────────────────┘
```

---

## 2. Infrastructure Decisions（計画回答の確定）

| # | 決定事項 | 値 | 備考 |
|---|---|---|---|
| Q1 | Cloud Run Job `max_retries` | **`0`** | **新規追加**。Scheduler tick (5min) が事実上 retry を担う |
| Q2 | Scheduler 重複実行抑止 | **何もしない** | idempotent upsert (Q3 [A]) + U-NTF dedup で対応 |
| Q3 | `INGESTION_MODE` デフォルト | Terraform で **`incremental`** | initial は `gcloud --update-env-vars` で実行時 override |
| Q4 | Cloud Run Job リソース | `cpu = 1` / `memory = 512Mi` | 現状維持 (U-PLT 雛形のまま) |
| Q5 | env の Terraform 反映粒度 | **運用ポリシー / 依存関係のみ Terraform** | Tuning パラメータ (concurrency, rate limit) は envconfig default |

---

## 3. Cloud Run Job 仕様

### 3.1 `google_cloud_run_v2_job` 最終形

```hcl
resource "google_cloud_run_v2_job" "ingestion" {
  name     = "ingestion"
  location = var.region

  template {
    template {
      service_account = google_service_account.runtime.email
      timeout         = "300s"
      max_retries     = 0     # ← U-ING で追加 (Q1 [A])

      containers {
        image = "${var.artifact_registry_url}/ingestion:${var.image_tag}"

        # PLATFORM_* envs (観測共通)
        env { name = "PLATFORM_SERVICE_NAME"    value = "ingestion" }
        env { name = "PLATFORM_ENV"             value = var.env }
        env { name = "PLATFORM_GCP_PROJECT_ID"  value = var.project_id }
        env { name = "PLATFORM_OTEL_EXPORTER"   value = "gcp" }

        # INGESTION_MODE は Terraform で incremental 固定 (Q3 [A])
        env { name = "INGESTION_MODE" value = "incremental" }

        # INGESTION_PUBSUB_TOPIC_ID は shared module の output から渡す (Q5 [A])
        env { name = "INGESTION_PUBSUB_TOPIC_ID" value = var.new_arrival_topic_id }

        # 既存 envs (U-PLT 雛形のまま)
        env { name = "INGESTION_MOFA_BASE_URL"     value = var.mofa_base_url }
        env { name = "INGESTION_PUBSUB_TOPIC"      value = var.new_arrival_topic_name }
        env { name = "INGESTION_CMS_BASE_URL"      value = var.cms_base_url }
        env { name = "INGESTION_CMS_WORKSPACE_ID"  value = var.cms_workspace_id }

        # Secret refs (U-PLT 雛形のまま)
        env {
          name = "INGESTION_CLAUDE_API_KEY"
          value_source { secret_key_ref { secret = var.claude_api_key_secret_name version = "latest" } }
        }
        env {
          name = "INGESTION_MAPBOX_API_KEY"
          value_source { secret_key_ref { secret = var.mapbox_api_key_secret_name version = "latest" } }
        }
        env {
          name = "INGESTION_CMS_INTEGRATION_TOKEN"
          value_source { secret_key_ref { secret = var.cms_integration_token_secret_name version = "latest" } }
        }

        resources {
          limits = {
            cpu    = "1"
            memory = "512Mi"
          }
        }
      }
    }
  }

  depends_on = [
    google_secret_manager_secret_iam_member.claude,
    google_secret_manager_secret_iam_member.mapbox,
    google_secret_manager_secret_iam_member.cms,
  ]
}
```

### 3.2 envconfig default に任せる env (Q5 [A])

以下は Terraform に **書かない**。`cmd/ingestion/main.go` の `ingestionConfig` で `default:"..."` タグで吸収する:

| envconfig タグ | default | 役割 |
|---|---|---|
| `INGESTION_CMS_PROJECT_ALIAS` | `overseas-safety-map` | スキーマ alias、U-CSS と整合 |
| `INGESTION_CMS_MODEL_ALIAS` | `safety-incident` | 同上 |
| `INGESTION_CLAUDE_MODEL` | `claude-haiku-4-5` | LLM モデル指定 |
| `INGESTION_CONCURRENCY` | `5` | 並列度 |
| `INGESTION_LLM_RATE_LIMIT` | `5` | LLM req/s |
| `INGESTION_GEOCODE_RATE_LIMIT` | `10` | Mapbox req/s |

### 3.3 実行モード

#### 通常運用 (incremental)

Cloud Scheduler が `*/5 * * * *` で自動起動。運用者の操作不要。

#### 初回バックフィル (initial)

```bash
gcloud run jobs execute ingestion \
  --region=asia-northeast1 \
  --project=overseas-safety-map \
  --update-env-vars=INGESTION_MODE=initial \
  --wait
```

- `--update-env-vars` はその Run のみ env を override (永続化されない)
- `--wait` で完了まで block、stdout に終了ステータス
- 完了後、Cloud Logging で `mode=initial` 属性付きの summary ログを確認

---

## 4. Cloud Scheduler 仕様 (現状維持)

```hcl
resource "google_cloud_scheduler_job" "ingestion" {
  name        = "ingestion-new-arrival-5min"
  schedule    = "*/5 * * * *"
  time_zone   = "Asia/Tokyo"
  region      = var.region
  retry_config { retry_count = 0 }

  http_target {
    http_method = "POST"
    uri         = "https://${var.region}-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/${var.project_id}/jobs/${google_cloud_run_v2_job.ingestion.name}:run"
    oauth_token {
      service_account_email = google_service_account.scheduler_invoker.email
      scope                 = "https://www.googleapis.com/auth/cloud-platform"
    }
  }
}
```

`retry_count = 0` を意識的に維持 (Cloud Run Job 側で `max_retries = 0` と整合、Scheduler 自体の retry は不要)。

---

## 5. IAM / セキュリティ

### 5.1 Runtime SA (`ingestion-runtime`)

| Role | 付与先 | 目的 |
|---|---|---|
| `roles/secretmanager.secretAccessor` | `ingestion-claude-api-key` | Claude API key 読取 |
| `roles/secretmanager.secretAccessor` | `ingestion-mapbox-api-key` | Mapbox API key 読取 |
| `roles/secretmanager.secretAccessor` | `cms-integration-token` | CMS Token 読取 (U-CSS と共有) |
| `roles/pubsub.publisher` | `safety-incident.new-arrival` | Pub/Sub への publish |
| (暗黙) `roles/logging.logWriter` | プロジェクト | Cloud Logging |
| (暗黙) `roles/monitoring.metricWriter` | プロジェクト | OTel Metric |
| (暗黙) `roles/cloudtrace.agent` | プロジェクト | OTel Trace |

明示的に **付与しない**:
- `run.invoker` — 本 Job は Scheduler から起動される（Scheduler invoker SA に付与済み）
- `datastore.*` / `firestore.*` — Firestore 不使用
- `storage.*` — GCS 不使用

### 5.2 Scheduler Invoker SA (`scheduler-invoker`)

U-PLT 雛形のまま:
- `roles/run.invoker` on `ingestion` Job
- `roles/iam.serviceAccountTokenCreator` を Cloud Scheduler service agent (`service-{project_number}@gcp-sa-cloudscheduler.iam.gserviceaccount.com`) に付与

### 5.3 ネットワーク

- Cloud Run Job egress = ALL (デフォルト)
- 外部宛先: MOFA / Anthropic / Mapbox / reearth-cms (全て public HTTPS)
- VPC コネクタ未使用 (MVP)

### 5.4 Secret rotation

U-CSS と同じ手順 (NFR-CSS-SEC-01 と同型):

```bash
echo -n "<新 Token>" | gcloud secrets versions add <SECRET_NAME> \
  --data-file=- --project=overseas-safety-map
```

`version = "latest"` 追従なので、次回 Run で自動的に新 Token が使われる。

---

## 6. 可観測性

### 6.1 ログ (slog)

- 形式: JSON、Cloud Run 自動収集
- 必須属性:
  - `service.name = ingestion`
  - `env = prod`
  - `trace_id` / `span_id`
  - `app.ingestion.phase` (`fetch` / `lookup` / `extract` / `geocode` / `upsert` / `publish` / `done`)
  - `app.ingestion.mode` (`initial` / `incremental`)
- レベル指針:
  - `INFO`: phase 開始/終了、Run summary
  - `WARN`: publish 失敗 (CMS には入っているのでスキップ可)、Mapbox 失敗 → Centroid フォールバック
  - `ERROR`: per-item の致命的失敗 (skip but record)、MOFA fetch 失敗 (Run exit 1)
  - `DEBUG`: MOFA 本文 (PII の可能性ありなので INFO 以上には出さない)

### 6.2 OTel Metric

| Metric | Kind | Attributes |
|---|---|---|
| `app.ingestion.run.fetched` | Counter | `mode` |
| `app.ingestion.run.skipped` | Counter | (none) |
| `app.ingestion.run.processed` | Counter | (none) |
| `app.ingestion.run.failed` | Counter | `phase` |
| `app.ingestion.run.published` | Counter | (none) |
| `app.ingestion.geocode.fallback` | Counter | `source` (`mapbox`/`country_centroid`) |
| `app.ingestion.run.duration` | Histogram (ms) | `mode`, `result` (`success`/`failure`) |
| `app.ingestion.item.duration` | Histogram (ms) | `phase` |

### 6.3 Trace

- Root: `ingestion.Run`
- 子: `ingestion.Fetch` / `ingestion.ProcessItem` (per item)
- 孫: `cms.GetItem` / `llm.Extract` / `geocode.Geocode` / `cms.UpsertItem` / `pubsub.Publish`

### 6.4 アラート (将来拡張)

MVP では作成しないが、将来のアラート候補:
- `app.ingestion.run.failed{phase=fetch} > 3 / hour` → MOFA 障害の可能性
- `app.ingestion.geocode.fallback{source=country_centroid} / total > 0.5` → LLM/Mapbox 精度劣化
- Run 失敗率 (`failure / (success + failure)`) > 0.2 → 重大障害

---

## 7. 運用ランブック (簡略、詳細は Build and Test で)

### 7.1 通常運用

Cloud Scheduler が 5 分毎に自動起動。運用者の操作不要。

### 7.2 障害時の復旧

1. Cloud Logging で `severity=ERROR` を確認
2. `app.ingestion.phase` で失敗段階を特定
3. 必要な対処 (API key rotation、CMS スキーマ確認、MOFA 障害情報確認)
4. **何もしなくても次の Run 5 分後に自動リトライ** (Q3 + Q7 の合わせ技、self-healing)

### 7.3 初回バックフィル (一度だけ)

```bash
gcloud run jobs execute ingestion \
  --region=asia-northeast1 \
  --update-env-vars=INGESTION_MODE=initial \
  --wait
```

数千件処理で 30 分程度。完了後は env override が剥がれて自動的に `incremental` に戻る。

### 7.4 API Key Rotation

```bash
# Claude
gcloud secrets versions add ingestion-claude-api-key --data-file=- < key.txt
# Mapbox
gcloud secrets versions add ingestion-mapbox-api-key --data-file=- < key.txt
# CMS
gcloud secrets versions add cms-integration-token --data-file=- < token.txt
```

次回 Run で自動反映 (`version = "latest"` 追従)。

---

## 8. 非スコープ (U-ING Infrastructure Design 範囲外)

- **reearth-cms 本体のデプロイ** — 外部既存を利用 (U-CSS と同じ)
- **Anthropic / Mapbox の選定** — 利用前提、API key 入手は運用者責務
- **Alerting / Monitoring Policy** — Q6 (Design) と同方針、MVP では未実装
- **VPC Service Controls** — public エンドポイント対象なので意味なし
- **Multi-Region / DR** — 単一リージョン (`asia-northeast1`)、再実行で復旧可能なステートレス Job
- **Pub/Sub DLQ 設定** — shared module で土台あり、U-NTF Infra Design で確認

---

## 9. トレーサビリティ

| 上位要件 | U-ING Infra 対応 |
|---|---|
| NFR-ING-PERF-01/02 (Run 完了時間) | §3.1 timeout=300s、§4 5 分 cron |
| NFR-ING-SEC-01/02/03 (Secret / 最小権限) | §5.1 IAM / §5.4 rotation |
| NFR-ING-REL-01/02/03 (冪等・self-healing) | Q1 max_retries=0 + Q2 重複容認 |
| NFR-ING-OPS-01/02/03 (ログ/Metric/ランブック) | §6 可観測性 / §7 運用 |
| NFR-EXT-01 (拡張性) | envconfig default で tuning パラメータが env override 可 |
