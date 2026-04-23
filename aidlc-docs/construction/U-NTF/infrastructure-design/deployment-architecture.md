# U-NTF Deployment Architecture

**Unit**: U-NTF（Notifier Unit、Sprint 4）
**Deployable**: `cmd/notifier` → Cloud Run Service `notifier`
**受信**: Pub/Sub Push Subscription `notifier-safety-incident-new-arrival`
**参照**: [`U-NTF/design/U-NTF-design.md`](../design/U-NTF-design.md)、[`construction/shared-infrastructure.md`](../../shared-infrastructure.md)

---

## 1. Component Overview

```
┌──────────────────────────── GCP Project: overseas-safety-map (prod) ──────────────────────────────┐
│  Region: asia-northeast1                                                                          │
│                                                                                                   │
│  U-ING (publisher)                                                                                │
│       ↓                                                                                           │
│  ┌─ Pub/Sub Topic: safety-incident.new-arrival ──┐                                                │
│  │  + DLQ topic: safety-incident.new-arrival.dlq │                                                │
│  └───────┬─────────────────────────────────────┬─┘                                                │
│          │                                     │                                                  │
│          ▼                                     │ (max_delivery_attempts=5)                        │
│  ┌─ Push Subscription ──────────┐              ▼                                                  │
│  │  notifier-safety-incident-   │         (DLQ on failure)                                        │
│  │    new-arrival               │                                                                 │
│  │  ack_deadline = 60s          │                                                                 │
│  │  retry: 10s-600s exponential │                                                                 │
│  │  push with OIDC token        │                                                                 │
│  │    (runtime SA が subject)   │                                                                 │
│  └─────────┬────────────────────┘                                                                 │
│            │ HTTPS POST /pubsub/push                                                              │
│            ▼                                                                                      │
│  ┌─ Cloud Run Service: notifier ───────────────────────┐                                          │
│  │  image: <AR_URL>/notifier:<tag>                     │                                          │
│  │  ingress = INGRESS_TRAFFIC_ALL                      │                                          │
│  │  cpu=1 / memory=512Mi                               │                                          │
│  │  scaling: min=0 / max=2                             │                                          │
│  │  startup/liveness probe on /healthz                 │                                          │
│  │  SA: notifier-runtime                               │                                          │
│  │                                                     │                                          │
│  │  ENV (Terraform 渡し):                              │                                          │
│  │    PLATFORM_*                                       │                                          │
│  │    NOTIFIER_PORT = "8080"                           │                                          │
│  │    NOTIFIER_PUBSUB_SUBSCRIPTION = "notifier-..."   │                                          │
│  │                                                     │                                          │
│  │  ENV (envconfig default):                           │                                          │
│  │    NOTIFIER_DEDUP_COLLECTION, ..._TTL_HOURS,        │                                          │
│  │    NOTIFIER_USERS_COLLECTION,                       │                                          │
│  │    NOTIFIER_FCM_CONCURRENCY, ..._SHUTDOWN_GRACE     │                                          │
│  └────┬────────────────────────────────────┬───────────┘                                          │
│       │                                    │                                                      │
│       │ (Firestore SDK, ADC via runtime SA)                                                      │
│       ▼                                    ▼                                                      │
│  ┌─ Firestore (default) ──────┐   ┌─ Firebase Cloud Messaging (FCM) ─┐                           │
│  │  notifier_dedup            │   │  (Admin SDK 経由、ADC)             │                           │
│  │    TTL: expireAt + 24h     │   │  SendEachForMulticast             │                           │
│  │                            │   │                                   │                           │
│  │  users (U-BFF 主オーナー)  │   │  → iOS / Android デバイス          │                           │
│  │    複合 index:             │   └───────────────────────────────────┘                           │
│  │      enabled + country_cds │                                                                   │
│  └────────────────────────────┘                                                                   │
└───────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Infrastructure Decisions（計画回答の確定）

| # | 決定事項 | 値 | 備考 |
|---|---|---|---|
| Q1 | Firestore TTL policy | **`modules/shared/firestore.tf`** | 他 Unit と共通管理 |
| Q2 | Firestore 複合インデックス | **`modules/shared/firestore.tf`** | U-BFF と共用 |
| Q3 | Cloud Run scaling | 現状維持（`min=0 / max=2 / cpu=1 / memory=512Mi`） | U-PLT 雛形のまま |
| Q4 | env の Terraform 反映粒度 | **Terraform 追加 env ゼロ**、tuning は envconfig default | U-ING Q5 [A] と同方針 |

---

## 3. Cloud Run Service 仕様（現状維持）

U-PLT で完成済みの `google_cloud_run_v2_service.notifier` をそのまま使用。U-NTF での変更は **なし**。

### 要点

- **`ingress = INGRESS_TRAFFIC_ALL`**: Pub/Sub push が public URL に届けるため ALL。IAM で Pub/Sub service agent のみ invoker 許可
- **`min_instance_count = 0`**: アイドル時 0 instance、Cold start は Pub/Sub の retry policy (min backoff 10s) で吸収
- **`max_instance_count = 2`**: 80 req/instance concurrency × 2 で 160 concurrent、MVP トラフィック（~0.03 req/s）に対して十分
- **`cpu = 1, memory = 512Mi`**: Firestore / FCM SDK の HTTP/gRPC + Go runtime に十分
- **`startup_probe`, `liveness_probe`**: `/healthz` エンドポイント

### Cloud Run Service autoscaling 動作

```
req/s  → Cloud Run 判断
0      → min_instance_count (0) まで scale down
スパイク → concurrency を超えたら instance 追加（max=2 が上限）
サステイン後 → 15 分 idle で scale down
```

---

## 4. Pub/Sub Push Subscription 仕様（現状維持）

U-PLT で完成済みの `google_pubsub_subscription.new_arrival`。変更なし。

### 要点

- **`ack_deadline_seconds = 60`**: notifier は 60s 以内に HTTP response を返す必要（U-NTF Design NFR-NTF-PERF-01 `p95 < 3s` に余裕）
- **`retry_policy { minimum_backoff=10s, maximum_backoff=600s }`**: 5xx / timeout で指数バックオフ retry
- **`dead_letter_policy { max_delivery_attempts=5 }`**: 5 回失敗で DLQ へ
- **`push_config.oidc_token.service_account_email = runtime_sa`**: Pub/Sub が runtime SA の identity token を発行して POST

### IAM（現状維持）

| Binding | 目的 |
|---|---|
| Runtime SA: `datastore.user` | Firestore R/W |
| Runtime SA: `cloudmessaging.messagesSender` | FCM 送信 |
| Runtime SA: `run.invoker` (on Cloud Run Service) | Push 時の OIDC token subject |
| Pub/Sub service agent: `iam.serviceAccountTokenCreator` (on runtime SA) | OIDC token 発行 |
| Pub/Sub service agent: `pubsub.publisher` (on DLQ topic) | DLQ ルーティング |

---

## 5. Firestore 仕様（新規追加）

### 5.1 `notifier_dedup` TTL policy

```hcl
# modules/shared/firestore.tf (新規追加)
resource "google_firestore_field" "notifier_dedup_ttl" {
  project    = var.project_id
  database   = google_firestore_database.default.name
  collection = "notifier_dedup"
  field      = "expireAt"

  ttl_config {}

  depends_on = [google_firestore_database.default]
}
```

**動作**:
- `notifier_dedup` collection のドキュメントに `expireAt` (timestamp) フィールド
- `expireAt < now()` になったドキュメントを Firestore が自動削除（遅延 最大 24h）
- TTL policy は collection 全体に適用、app が書き込み時に `expireAt = now() + 24h` を設定

### 5.2 `users` 複合インデックス

```hcl
# modules/shared/firestore.tf (新規追加)
resource "google_firestore_index" "users_notification" {
  project    = var.project_id
  database   = google_firestore_database.default.name
  collection = "users"

  fields {
    field_path = "notification_preference.enabled"
    order      = "ASCENDING"
  }
  fields {
    field_path   = "notification_preference.target_country_cds"
    array_config = "CONTAINS"
  }

  depends_on = [google_firestore_database.default]
}
```

**動作**:
- Q3 [A] の `where(enabled=true) + where(target_country_cds array-contains)` クエリを有効化
- Terraform apply 後、インデックス構築に **数分〜数十分** かかる（非同期）
- 初回 apply 後に U-NTF を動かすなら少し待つ必要あり（運用ランブックに注記）

### 5.3 Database

既存の `google_firestore_database.default`（U-PLT で作成済み）をそのまま使用。追加変更なし。

---

## 6. 運用ランブック（簡略、詳細は Build and Test で）

### 6.1 通常運用

Pub/Sub が自動 push。運用者の操作不要。

### 6.2 初回デプロイ時の注意

Terraform apply の順序:
1. Firestore index 作成（数分〜数十分、非同期）
2. Cloud Run Service デプロイ（数秒）
3. Pub/Sub Subscription 作成（数秒）

Index 構築完了前に Pub/Sub が push 始めると、Firestore query が `FAILED_PRECONDITION` エラーで失敗する可能性。初回は:
- **Terraform apply 後 30 分待つ** か
- Firebase Console で index status が READY になるまで確認してから Pub/Sub Subscription を有効化

### 6.3 障害時の復旧

1. Cloud Logging (`resource.labels.service_name=notifier`) で `severity=ERROR / WARN` を確認
2. `app.notifier.phase` で失敗段階を特定
3. **DLQ topic** (`safety-incident.new-arrival.dlq`) にメッセージが溜まっているか確認:
   ```bash
   gcloud pubsub subscriptions pull notifier-dlq-sub \
     --limit=10 --auto-ack --project=overseas-safety-map
   ```
4. 原因対処後、必要なら手動 replay（DLQ から元 topic に再 publish）

### 6.4 通知 UI のカスタマイズ

`cmd/notifier/main.go` の `buildFCMMessage(event)` 関数で `Title` / `Body` / `Data` を生成。変更は PR + デプロイで反映。

### 6.5 インスタンスメトリック確認

Cloud Monitoring で:
- `run.googleapis.com/container/instance_count`: 平常時 0、スパイク時 1-2
- `run.googleapis.com/request_count{response_code_class=5xx}`: 異常検知
- `app.notifier.duration` (p95): < 3s 維持

---

## 7. 非スコープ

- **Firestore の PII redact**（ユーザーの email 等は notifier が触らない、U-BFF の責務）
- **DLQ の自動 replay**（MVP では手動で十分、自動化は Ops フェーズで）
- **Alerting Policy**（MVP では未実装、Cloud Logging 目視）
- **Multi-Region / DR**（単一リージョン、Firestore は multi-region で自動対応）
- **FCM Topic messaging**（Q4 Design [A] で direct send を選択）

---

## 8. トレーサビリティ

| 上位要件 | U-NTF Infra 対応 |
|---|---|
| NFR-NTF-PERF-01 (p95 < 3s) | §3 Cloud Run scaling、§5 Firestore index で query 高速化 |
| NFR-NTF-SEC-01/02/03 (IAM 最小権限) | §4 IAM 一覧 |
| NFR-NTF-REL-01 (dedup) | §5.1 TTL policy |
| NFR-NTF-REL-02 (Pub/Sub retry + DLQ) | §4 retry_policy + dead_letter_policy |
| NFR-NTF-OPS-01-04 (ログ / Metric / DLQ 監視) | §6 運用 |
| NFR-NTF-EXT-01 (他 channel 拡張) | §7 非スコープ |
