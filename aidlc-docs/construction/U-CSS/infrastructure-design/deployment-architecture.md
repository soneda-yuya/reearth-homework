# U-CSS Deployment Architecture

**Unit**: U-CSS（CMS Migrate Unit、Sprint 1）
**Deployable**: `cmd/cmsmigrate` → Cloud Run Job `cms-migrate`
**参照**: [`U-CSS/design/U-CSS-design.md`](../design/U-CSS-design.md)、[`construction/shared-infrastructure.md`](../../shared-infrastructure.md)

---

## 1. Component Overview

```
┌──────────────────────────── GCP Project: overseas-safety-map (prod) ────────────────────────────┐
│  Region: asia-northeast1                                                                         │
│                                                                                                  │
│   ┌─ Cloud Run Job: cms-migrate ────────────────────────┐                                        │
│   │  image: <AR_URL>/cmsmigrate:<tag>                   │                                        │
│   │  cpu = 1, memory = 256Mi                            │                                        │
│   │  timeout = 120s, max_retries = 0                    │                                        │
│   │  SA: cmsmigrate-runtime@...                         │                                        │
│   │                                                     │                                        │
│   │  ENV:                                               │                                        │
│   │    PLATFORM_SERVICE_NAME = cmsmigrate               │                                        │
│   │    PLATFORM_ENV = prod                              │                                        │
│   │    PLATFORM_GCP_PROJECT_ID = overseas-safety-map    │                                        │
│   │    PLATFORM_OTEL_EXPORTER = gcp                     │                                        │
│   │    CMSMIGRATE_CMS_BASE_URL = <var>                  │                                        │
│   │    CMSMIGRATE_CMS_WORKSPACE_ID = <var>              │                                        │
│   │    CMSMIGRATE_CMS_INTEGRATION_TOKEN = <secret ref> ─┼───┐                                    │
│   └────┬───────────────────────────────────────────┬────┘   │                                    │
│        │ (HTTPS + Bearer Token)                    │        │                                    │
│        │                                           │        ▼                                    │
│        │                                   ┌───────┴──────────────────────┐                      │
│        │                                   │  Secret Manager: cms-integration-token              │
│        │                                   │  (shared module、values manual)                    │
│        │                                   └─────────────────────────────┘                      │
│        │                                                                                        │
│        │   OTel exporter (logs / metrics / traces) → Cloud Logging / Cloud Monitoring           │
│        │                                                                                        │
└────────┼────────────────────────────────────────────────────────────────────────────────────────┘
         │
         │ HTTPS (outbound, public internet)
         ▼
┌────────────────────────────────────────┐
│  External reearth-cms instance         │
│  (別管理プロジェクト、Q5 [A] に準拠)    │
│                                        │
│  Integration REST API                  │
│  /api/workspaces/{ws}/projects         │
│  /api/projects/{id}/models             │
│  /api/models/{id}/fields               │
└────────────────────────────────────────┘
```

---

## 2. Infrastructure Decisions（計画回答の確定）

| # | 決定事項 | 値 | 備考 |
|---|---|---|---|
| Q1 | Cloud Run Job リソース | `cpu = 1` / `memory = 256Mi` | 現状値維持（U-PLT 雛形のまま） |
| Q2 | Task Timeout | `timeout = 120s` | NFR-CSS-PERF-01（< 60s）にバッファ 2x |
| Q3 | **Max Retries** | **`0`** | **新規設定**。U-PLT 雛形は未指定（GCP 既定 3）、U-CSS で明示 0 を追加 |
| Q4 | Secret rotation | 手動 + `version = "latest"` | Terraform 変更不要で値差し替え可 |
| Q5 | reearth-cms ホスト | **外部既存** | `cms_base_url` / `cms_workspace_id` は tfvars で与える。CMS 本体は本プロジェクト外 |
| Q6 | 監視 / アラート | **ログ運用のみ** | Alerting Policy は作成しない（MVP） |

---

## 3. Cloud Run Job 仕様

### 3.1 `google_cloud_run_v2_job` 最終形

U-PLT 雛形（[`terraform/modules/cmsmigrate/main.tf`](../../../../terraform/modules/cmsmigrate/main.tf)）に対して **2 箇所の追加** が必要:

1. `template.template.max_retries = 0` の追加（Q3 [A]）
2. （差分なし、U-PLT 雛形のまま Q1/Q2/Q4 を満たす）

```hcl
resource "google_cloud_run_v2_job" "cmsmigrate" {
  name     = "cms-migrate"
  location = var.region

  template {
    template {
      service_account = google_service_account.runtime.email
      timeout         = "120s"
      max_retries     = 0               # ← U-CSS で追加（Q3 [A]）

      containers {
        image = "${var.artifact_registry_url}/cmsmigrate:${var.image_tag}"

        # PLATFORM_* envs（観測共通）
        env { name = "PLATFORM_SERVICE_NAME"    value = "cmsmigrate" }
        env { name = "PLATFORM_ENV"             value = var.env }
        env { name = "PLATFORM_GCP_PROJECT_ID"  value = var.project_id }
        env { name = "PLATFORM_OTEL_EXPORTER"   value = "gcp" }

        # CMSMIGRATE_* envs
        env { name = "CMSMIGRATE_CMS_BASE_URL"     value = var.cms_base_url }
        env { name = "CMSMIGRATE_CMS_WORKSPACE_ID" value = var.cms_workspace_id }
        env {
          name = "CMSMIGRATE_CMS_INTEGRATION_TOKEN"
          value_source {
            secret_key_ref {
              secret  = var.cms_integration_token_secret_name
              version = "latest"        # Q4 [A]: 手動 rotation + latest 追従
            }
          }
        }

        resources {
          limits = {
            cpu    = "1"
            memory = "256Mi"
          }
        }
      }
    }
  }

  depends_on = [google_secret_manager_secret_iam_member.cms]
}
```

### 3.2 実行モード（Q3 [A] 手動、U-CSS Design §4.1 に基づく）

```bash
gcloud run jobs execute cms-migrate \
  --region=asia-northeast1 \
  --project=overseas-safety-map \
  --wait
```

- `--wait`: 完了まで待機、stdout に終了ステータス
- CI/CD からの自動起動は **なし**（U-CSS Design Q3 [A]）
- 失敗したら運用者が Cloud Logging で `app.cmsmigrate.phase` / 失敗 `field.alias` を確認 → 原因対処 → 同コマンドで再実行

### 3.3 Cold Start / 起動時間

- Go バイナリ（`distroless/static-debian12`）+ `observability.Setup` + `envconfig.Process` で約 200-400ms
- 19 Field × 各 500ms〜1s のネットワーク I/O ≈ 10-20 秒
- 初回 `CreateProject` / `CreateModel` が追加 1-2 秒
- **総実行時間（初回想定）**: 15-30 秒 → NFR-CSS-PERF-01（< 60s）は十分余裕
- 再実行（全 no-op）: Find のみ × 20 回 ≈ 5-10 秒 → NFR-CSS-PERF-02（< 10s）ギリギリだが許容

---

## 4. IAM / セキュリティ

### 4.1 Runtime SA（`cmsmigrate-runtime`）

**付与する Role**:

| Role | 付与先 | 目的 |
|---|---|---|
| `roles/secretmanager.secretAccessor` | `cms-integration-token` Secret | Bearer Token 読取 |
| （暗黙）`roles/logging.logWriter` | プロジェクト | Cloud Logging へのログ書込（Cloud Run デフォルト） |
| （暗黙）`roles/monitoring.metricWriter` | プロジェクト | OTel → Cloud Monitoring 書込 |
| （暗黙）`roles/cloudtrace.agent` | プロジェクト | OTel Trace 書込 |

**明示的に付与しない**:
- `run.invoker` — 本 Job は外部トリガなし、Pub/Sub push や Scheduler から呼ばれない
- `pubsub.*` — Pub/Sub 不使用
- `datastore.*` / `firestore.*` — Firestore 不使用
- `storage.*` — GCS 不使用

### 4.2 外部 CMS への認可

- **Google IAM に依存しない**。reearth-cms Integration API は Bearer Token で認可
- Runtime SA には CMS 側の権限ロールは付与できない（別システム）
- Token の持つ権限は Workspace レベル（Create Project / Create Model / Create Field）。権限スコープの絞り込みは CMS 管理者側で実施（本 Terraform 範囲外）

### 4.3 ネットワーク

- Cloud Run Job は **egress = ALL**（デフォルト、VPC コネクタ未使用）
- 外部 reearth-cms（公開 URL）への HTTPS アウトバウンドのみ
- VPC-SC 等は MVP では適用しない

### 4.4 Secret の取り扱い（Q4 [A]）

**Rotation 手順**:
1. reearth-cms 管理画面で新しい Integration Token を発行
2. `gcloud secrets versions add cms-integration-token --data-file=- --project=overseas-safety-map`（stdin で Token 投入）
3. 次回 `gcloud run jobs execute cms-migrate` で新 Token が自動注入（`version = "latest"`）
4. 旧 version は `gcloud secrets versions disable <VERSION_ID>` で無効化（任意、監査目的）

**値の Terraform 管理しない理由**:
- Terraform state に secret material が乗らないよう Secret の「定義」のみ Terraform で管理（[`terraform/modules/shared/secrets.tf`](../../../../terraform/modules/shared/secrets.tf) 既存）
- 値は `gcloud secrets versions add` で手動投入（state に含まれない）
- 詳細は [`construction/shared-infrastructure.md`](../../shared-infrastructure.md) の Secret 管理方針に従う

---

## 5. 可観測性

### 5.1 ログ

- **形式**: JSON（slog）、Cloud Run 自動収集 → Cloud Logging
- **必須属性**:
  - `service.name = cmsmigrate`
  - `env = prod`
  - `trace_id` / `span_id`（OTel 連携）
  - `app.cmsmigrate.phase`（`validate` / `find-project` / `create-model` / `create-field-XX` / `done`）
- **レベル**:
  - `INFO`: 各 phase 開始 / 終了、作成件数
  - `WARN`: Drift 検出（Q2 [A]、自動修正しない）
  - `ERROR`: fail-fast 時の例外、retry 最終失敗

### 5.2 Metric（OTel、U-PLT `observability.Meter` 経由）

| Metric | Kind | Attributes |
|---|---|---|
| `app.cmsmigrate.project.created` | Counter | `project.alias` |
| `app.cmsmigrate.model.created` | Counter | `project.alias` / `model.alias` |
| `app.cmsmigrate.field.created` | Counter | `model.alias` / `field.alias` / `field.type` |
| `app.cmsmigrate.drift.detected` | Counter | `resource` / `reason` |
| `app.cmsmigrate.run.failure` | Counter | `phase` / `error.kind` |
| `app.cmsmigrate.run.duration` | Histogram (ms) | `result=success|failure` |

### 5.3 Trace（OTel）

Root Span: `cmsmigrate.EnsureSchema`
子 Span: `cms.FindProject` / `cms.CreateProject` / `cms.FindModel` / `cms.CreateModel` / `cms.FindField` / `cms.CreateField`

各 HTTP リクエストには `http.url` / `http.method` / `http.status_code` / `cms.resource.alias` を付与。

### 5.4 アラート（Q6 [A]）

**MVP では作成しない**。以下の理由:
- 手動実行なので、実行者がその場でログを見る
- 自動リトライなし（Q3 [A]）、放置される可能性が低い
- Drift は WARN なので無視しても問題ない（段階的に直せる）

**将来拡張ポイント**:
- `app.cmsmigrate.run.failure > 0` で Slack / Email 通知
- Log-based Metric + Alerting Policy で実装
- Infrastructure Design の範囲外（`docs/operations.md` 相当で検討）

---

## 6. 運用手順（参考、U-CSS Design §4 と重複）

### 6.1 初回デプロイ

```
(1) main に PR merge（schema_definition.go + terraform/modules/cmsmigrate/*）
(2) deploy.yml が artifact registry に image push + terraform apply
(3) gcloud run jobs execute cms-migrate --region=asia-northeast1 --wait
(4) Cloud Logging で project_created / models_created / fields_created を確認
```

### 6.2 スキーマ変更時

```
(1) schema_definition.go に新 Field 追加 → PR review → main merge
(2) deploy.yml で image + terraform apply
(3) gcloud run jobs execute cms-migrate --region=asia-northeast1 --wait
(4) result.fields_created に新 Field が載っていることを確認
```

### 6.3 失敗時

```
(1) Cloud Logging で severity=ERROR を確認
(2) app.cmsmigrate.phase / field.alias / error.kind を読んで原因特定
(3) 必要な対処（権限追加、CMS 側修正、schema_definition.go 修正など）
(4) 同じコマンドで再実行（冪等なので既作成は保持されたまま再開）
```

### 6.4 Token rotation

```
(1) reearth-cms 管理画面で新 Token 発行
(2) gcloud secrets versions add cms-integration-token --data-file=-
(3) 次回 Job 実行時に自動反映（version=latest）
(4) （任意）古い version を disable
```

---

## 7. 非スコープ（U-CSS Infrastructure Design 範囲外）

- **reearth-cms 本体のデプロイ** — Q5 [A] により外部既存を利用、本 Terraform 範囲外
- **Alerting / Monitoring Policy** — Q6 [A]、MVP では省略
- **VPC / Private Service Connect / Internal Only** — 外部 CMS（public URL）と通信するため意味なし
- **Multi-Region / DR** — 単一リージョン（`asia-northeast1`）、U-CSS Job は一時的実行なので DR 不要
- **バックアップ / リストア** — CMS 側の責務、U-CSS 側では何も保持しない（ステートレス）

---

## 8. トレーサビリティ

| 上位要件 | U-CSS Infra 対応 |
|---|---|
| NFR-CSS-PERF-01/02 (実行時間) | §3.1 timeout=120s、§3.3 Cold Start 分析 |
| NFR-CSS-SEC-01 (Secret Manager) | §4.4 Secret rotation、§3.1 `secret_key_ref` |
| NFR-CSS-SEC-02 (Token redact) | Code Generation で slog handler filter 実装 |
| NFR-CSS-SEC-03 (最小権限 SA) | §4.1 Runtime SA の role 一覧 |
| NFR-CSS-REL-01/02 (冪等・再実行) | §3.2 実行モード（手動再実行）、§6.3 失敗時手順 |
| NFR-CSS-OPS-01/02/03 (ログ/Metric/ランブック) | §5 可観測性、§6 運用手順 |
