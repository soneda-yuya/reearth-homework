# U-ING Terraform Plan

**Unit**: U-ING
**対象**: [`terraform/modules/ingestion/`](../../../../terraform/modules/ingestion/) と [`terraform/environments/prod/`](../../../../terraform/environments/prod/) の **差分要約**

U-PLT で `modules/ingestion/` の雛形が **ほぼ完成** しているため、U-ING で必要な変更は **最小限**。本ドキュメントは Code Generation で実装する Terraform 変更の diff 要約。

---

## 1. 変更サマリ

| # | ファイル | 変更 | 根拠 |
|---|---|---|---|
| 1 | [`terraform/modules/ingestion/main.tf`](../../../../terraform/modules/ingestion/main.tf) | `max_retries = 0` を追加 | Q1 [A] |
| 2 | [`terraform/modules/ingestion/main.tf`](../../../../terraform/modules/ingestion/main.tf) | env `INGESTION_MODE = "incremental"` を追加 | Q3 [A] / Q5 [A] |
| 3 | [`terraform/modules/ingestion/main.tf`](../../../../terraform/modules/ingestion/main.tf) | env `INGESTION_PUBSUB_TOPIC_ID` を追加 (新規 var 経由) | Q5 [A] |
| 4 | [`terraform/environments/prod/main.tf`](../../../../terraform/environments/prod/main.tf) | `module "ingestion"` の引数に `new_arrival_topic_id` (既存) を確認、変更なしの可能性大 | Q5 [A] |

> **影響は 1 module への env 2 個追加 + `max_retries = 0` の 1 行だけ**。新規リソース / 新規 IAM / 新規 Secret はゼロ。

---

## 2. 詳細 diff (疑似)

### 2.1 `terraform/modules/ingestion/main.tf`

```diff
     template {
       service_account = google_service_account.runtime.email
       timeout         = "300s"
+      max_retries     = 0
 
       containers {
         image = "${var.artifact_registry_url}/ingestion:${var.image_tag}"
 
         env { name = "PLATFORM_SERVICE_NAME"   value = "ingestion" }
         env { name = "PLATFORM_ENV"            value = var.env }
         env { name = "PLATFORM_GCP_PROJECT_ID" value = var.project_id }
         env { name = "PLATFORM_OTEL_EXPORTER"  value = "gcp" }
+
+        # U-ING: incremental 固定 (initial は実行時 --update-env-vars で override)
+        env { name = "INGESTION_MODE" value = "incremental" }
+
+        # U-ING: shared module の output を proxy
+        env { name = "INGESTION_PUBSUB_TOPIC_ID" value = var.new_arrival_topic_id }
 
         env { name = "INGESTION_MOFA_BASE_URL"     value = var.mofa_base_url }
         env { name = "INGESTION_PUBSUB_TOPIC"      value = var.new_arrival_topic_name }
         env { name = "INGESTION_CMS_BASE_URL"      value = var.cms_base_url }
         env { name = "INGESTION_CMS_WORKSPACE_ID"  value = var.cms_workspace_id }
         # ... (3 secret refs unchanged)
       }
     }
```

**意味**:
- `max_retries = 0`: Cloud Run Job が exit 1 で終わっても自動 retry しない → 5 分後の Scheduler tick で fresh Run
- `INGESTION_MODE = "incremental"`: 通常運用のデフォルト、Cloud Scheduler 経由は常にこれ
- `INGESTION_PUBSUB_TOPIC_ID`: shared module の `var.new_arrival_topic_id` を proxy。Application 層が FQ ID で publish 先を指定可能に

### 2.2 `terraform/modules/ingestion/variables.tf`

`new_arrival_topic_id` は既存 (U-PLT で追加済み) なので追加なし。

### 2.3 `terraform/environments/prod/main.tf`

`module "ingestion"` の引数渡しは既存のままで OK。`shared` module の output を引き継ぐ:

```hcl
module "ingestion" {
  source = "../../modules/ingestion"

  # ... (既存)
  new_arrival_topic_id    = module.shared.new_arrival_topic_id     # 既存、INGESTION_PUBSUB_TOPIC_ID で参照される
  new_arrival_topic_name  = module.shared.new_arrival_topic_name   # 既存、INGESTION_PUBSUB_TOPIC で参照される
}
```

**変更なし** (既に shared module から渡している)。

### 2.4 envconfig default で吸収する env (Q5 [A])

以下は **Terraform で渡さない**。`cmd/ingestion/main.go` の `ingestionConfig` struct で `default` タグを付ける:

```go
type ingestionConfig struct {
    config.Common
    // Terraform 渡し
    Mode                string `envconfig:"INGESTION_MODE" default:"incremental"`
    PubSubTopicID       string `envconfig:"INGESTION_PUBSUB_TOPIC_ID" required:"true"`
    MofaBaseURL         string `envconfig:"INGESTION_MOFA_BASE_URL" required:"true"`
    CMSBaseURL          string `envconfig:"INGESTION_CMS_BASE_URL" required:"true"`
    CMSWorkspaceID      string `envconfig:"INGESTION_CMS_WORKSPACE_ID" required:"true"`
    CMSIntegrationToken string `envconfig:"INGESTION_CMS_INTEGRATION_TOKEN" required:"true"`
    ClaudeAPIKey        string `envconfig:"INGESTION_CLAUDE_API_KEY" required:"true"`
    MapboxAPIKey        string `envconfig:"INGESTION_MAPBOX_API_KEY" required:"true"`

    // envconfig default で吸収 (Q5 [A])
    CMSProjectAlias  string `envconfig:"INGESTION_CMS_PROJECT_ALIAS" default:"overseas-safety-map"`
    CMSModelAlias    string `envconfig:"INGESTION_CMS_MODEL_ALIAS" default:"safety-incident"`
    ClaudeModel      string `envconfig:"INGESTION_CLAUDE_MODEL" default:"claude-haiku-4-5"`
    Concurrency      int    `envconfig:"INGESTION_CONCURRENCY" default:"5"`
    LLMRateLimit     int    `envconfig:"INGESTION_LLM_RATE_LIMIT" default:"5"`
    GeocodeRateLimit int    `envconfig:"INGESTION_GEOCODE_RATE_LIMIT" default:"10"`
}
```

これらの値を本番で変えたい場合は:
1. **コード PR** (`default:"5"` → `default:"10"`) → デプロイ
2. **緊急時のみ Terraform で env 追加** (`env { name = "INGESTION_LLM_RATE_LIMIT" value = "..." }`) → 平時は外す

---

## 3. 新規リソース / 削除リソース

### 3.1 新規作成 (なし)

- Pub/Sub topic + DLQ: shared module で既存
- Secret 3 種: shared module で既存
- Cloud Run Job, Scheduler, Runtime SA, scheduler-invoker SA, 全 IAM: U-PLT で既存

### 3.2 削除 (なし)

---

## 4. `terraform apply` 想定 diff

```
# module.ingestion.google_cloud_run_v2_job.ingestion will be updated in-place
~ resource "google_cloud_run_v2_job" "ingestion" {
    name = "ingestion"
    ...
    ~ template {
        ~ template {
            + max_retries = 0
            ...
            + env {
                + name  = "INGESTION_MODE"
                + value = "incremental"
              }
            + env {
                + name  = "INGESTION_PUBSUB_TOPIC_ID"
                + value = "projects/overseas-safety-map/topics/safety-incident.new-arrival"
              }
            ...
        }
    }
}

Plan: 0 to add, 1 to change, 0 to destroy.
```

- in-place update (リソース再作成なし)
- 次回 Cloud Scheduler tick から新しい env / `max_retries = 0` で起動

---

## 5. Code Generation へ渡す TODO

Code Generation 段階で実施する Terraform 変更:

- [ ] `terraform/modules/ingestion/main.tf` に `max_retries = 0` を追加
- [ ] `terraform/modules/ingestion/main.tf` に env `INGESTION_MODE = "incremental"` を追加
- [ ] `terraform/modules/ingestion/main.tf` に env `INGESTION_PUBSUB_TOPIC_ID = var.new_arrival_topic_id` を追加
- [ ] `terraform fmt` / `terraform init -backend=false` / `terraform validate` を通す
- [ ] `terraform/environments/prod/main.tf` の `module "ingestion"` 引数を確認 (変更不要見込み)

並行して Code Generation の本丸は Go 側 (`cmd/ingestion/main.go` の拡張 + `internal/safetyincident/` パッケージ群 + 全テスト)。詳細は U-ING Code Generation Plan で決める。

---

## 6. 非 Terraform セットアップ手順 (運用ランブック)

実 ingestion を動かすため運用者が **事前に** 行うこと:

1. **Anthropic Console で API Key 発行**
   ```bash
   gcloud secrets versions add ingestion-claude-api-key --data-file=- --project=overseas-safety-map
   ```
2. **Mapbox Account で API Key 発行** (Geocoding scope)
   ```bash
   gcloud secrets versions add ingestion-mapbox-api-key --data-file=- --project=overseas-safety-map
   ```
3. **CMS Integration Token は U-CSS で投入済みの想定** (`cms-integration-token` を共有)
4. **`prod.tfvars` 更新** (既存変数を確認、変更不要見込み)
5. CI が `terraform apply -var-file=prod.tfvars` を実行 (WIF)
6. Cloud Scheduler が **5 分以内に** 自動的に initial tick を開始 (Scheduler 作成時の挙動)
7. (任意) 過去データのバックフィルは `gcloud run jobs execute ingestion --update-env-vars=INGESTION_MODE=initial --wait`

---

## 7. 承認プロセス

- [ ] 本 Terraform Plan のレビュー
- [ ] [`deployment-architecture.md`](./deployment-architecture.md) のレビュー
- [ ] 承認後、U-ING Code Generation へ進む (Go + Terraform 両方)
