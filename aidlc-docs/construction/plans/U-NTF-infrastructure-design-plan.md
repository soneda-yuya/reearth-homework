# U-NTF Infrastructure Design Plan

## Overview

U-NTF (Notifier Unit、Sprint 4) の **Infrastructure Design** 計画。U-PLT で `terraform/modules/notifier/` が **Cloud Run Service + Pub/Sub Push Subscription + 完全な IAM** まで実装済みなので、本ステージは **Firestore 周り（TTL + インデックス） + scaling 値の最終確認** が中心。薄く済む見込み。

## Context — すでに U-PLT で決まっていること

[`terraform/modules/notifier/`](../../../terraform/modules/notifier/) に以下が実装済み:

- `google_cloud_run_v2_service "notifier"`
  - `ingress = INGRESS_TRAFFIC_ALL`（IAM で Pub/Sub service agent のみ invoker）
  - `scaling { min=0, max=2 }`
  - `cpu=1 / memory=512Mi`
  - startup / liveness probe on `/healthz`
- `google_pubsub_subscription "new_arrival"`（Push Subscription）
  - `ack_deadline_seconds = 60`
  - `push_config.oidc_token.service_account_email = runtime_sa`
  - `retry_policy { minimum_backoff=10s, maximum_backoff=600s }`
  - `dead_letter_policy { max_delivery_attempts=5 }`
- Runtime SA (`notifier-runtime`) の IAM:
  - `roles/datastore.user`（Firestore R/W）
  - `roles/cloudmessaging.messagesSender`（FCM 送信）
  - `roles/run.invoker`（Pub/Sub push の OIDC token subject に必要）
- Pub/Sub service agent の IAM:
  - `roles/iam.serviceAccountTokenCreator` on runtime SA（OIDC token 発行用）
  - `roles/pubsub.publisher` on DLQ topic（DLQ ルーティング用）

## U-NTF Design で確定済みの前提

[`U-NTF/design/U-NTF-design.md`](../U-NTF/design/U-NTF-design.md) より:

- `notifier_dedup` collection + `expireAt` フィールドで Firestore TTL 24h (Q2 [A])
- `users` collection を country + enabled + array-contains query で読む (Q3 [A])
- SendMulticast 並列度 5 (Q4 [A])
- 同一 Request 内 ArrayRemove で無効 token 除去 (Q5 [A])
- HTTP status code 細かく使い分け (Q6 [A])

---

## Step-by-step Checklist

- [ ] Q1〜Q4 すべて回答
- [ ] 成果物を生成:
  - [ ] `construction/U-NTF/infrastructure-design/deployment-architecture.md`
  - [ ] `construction/U-NTF/infrastructure-design/terraform-plan.md`
- [ ] 承認後、U-NTF Code Generation へ進む

---

## Questions

### Question 1 — Firestore `notifier_dedup` の TTL policy（Terraform で作るか）

Q2 [A] で「Firestore 自動 TTL で 24h 後削除」と決定。GCP では `google_firestore_field` リソースで TTL policy を設定:

```hcl
resource "google_firestore_field" "notifier_dedup_ttl" {
  project    = var.project_id
  database   = "(default)"
  collection = "notifier_dedup"
  field      = "expireAt"
  ttl_config {}
}
```

**選択肢**:

A) **推奨**: **shared module に追加**（`modules/shared/firestore.tf` に TTL 定義を置く）
  - ✅ 他 Unit が Firestore TTL を使いたくなっても同 module に集約
  - ✅ notifier module から Firestore 設定が切り離される（notifier は「コンシューマ」に徹する）
  - ⚠️ shared module のスコープがやや広がる

B) notifier module 内に追加（`modules/notifier/firestore.tf` 新規）
  - ✅ notifier 専用リソースは notifier module に集約
  - ⚠️ shared の firestore.tf が既にあるなら整合性を揃えたい

C) Terraform で作らず、Firebase Console で手動設定
  - ⚠️ IaC の一貫性が崩れる
  - 採用しない

[A]: A

### Question 2 — Firestore `users` 複合インデックス

Q3 [A] の Firestore query（`enabled == true` + `target_country_cds array-contains`）には **複合インデックス** が必要:

```hcl
resource "google_firestore_index" "users_notification" {
  project    = var.project_id
  database   = "(default)"
  collection = "users"
  fields {
    field_path = "notification_preference.enabled"
    order      = "ASCENDING"
  }
  fields {
    field_path   = "notification_preference.target_country_cds"
    array_config = "CONTAINS"
  }
}
```

**選択肢**:

A) **推奨**: **shared module に追加**（`modules/shared/firestore.tf`、Q1 と同じ場所）
  - ✅ Firestore リソースを shared に集約、U-BFF も `users` を使うので共通管理が自然
  - ✅ `users` collection は U-BFF が主オーナー、U-NTF が reader

B) notifier module に追加
  - ⚠️ U-BFF 側が同じインデックスを参照するため、所有者が曖昧に

[A]: A

### Question 3 — Cloud Run Service の scaling 値

現状: `min_instance_count=0, max_instance_count=2, cpu=1, memory=512Mi`

**トラフィック想定**:
- Pub/Sub push 頻度 = U-ING Run の publish 数 = **~30 msg/Run × 288 Run/日 = ~8,600 msg/日 平均 ~0.1 req/s**
- Spike: バックフィル時に瞬間的に数百 msg → Pub/Sub が HTTP を並列 push
- 1 request の処理 ~1-2 秒（dedup + resolve + parallel FCM + cleanup）

**選択肢**:

A) **推奨**: **現状維持**（`min=0 / max=2`）
  - `max=2` で突発的な数十並列 push を 80 req/instance のデフォルト concurrency でさばける（~160 concurrent = 十分）
  - ✅ コスト最小（通常は instance 0）
  - ✅ 雛形のまま、追加作業なし
  - ⚠️ Cold start が数秒あるが、Pub/Sub retry で吸収される

B) `min=1 / max=3` で Cold start を回避
  - ⚠️ 24h instance 稼働で月 $15-20 追加（MVP として過剰）

C) `max=5` に拡張
  - ⚠️ 現実のトラフィックに対して過剰、課金 instance が並走する

[A]: A

### Question 4 — env 追加 / Terraform 反映粒度

U-NTF design で新規に必要な env をどこで管理するか（U-ING Q5 と同じ方針判断）:

| env | Terraform 渡し or envconfig default? |
|---|---|
| `NOTIFIER_PORT` | Terraform（= 8080、probe と一致する必要） |
| `NOTIFIER_PUBSUB_SUBSCRIPTION` | Terraform（既存、FQ name で監視に使える） |
| `NOTIFIER_DEDUP_COLLECTION` | envconfig default (`notifier_dedup`)? それとも Terraform? |
| `NOTIFIER_DEDUP_TTL_HOURS` | envconfig default (`24`)? |
| `NOTIFIER_USERS_COLLECTION` | envconfig default (`users`)? |
| `NOTIFIER_FCM_CONCURRENCY` | envconfig default (`5`)? |
| `NOTIFIER_SHUTDOWN_GRACE_SECONDS` | envconfig default (`10`)? |

**選択肢**:

A) **推奨**: **U-ING Q5 [A] と同じ方針**
  - Terraform 渡し: `NOTIFIER_PORT`, `NOTIFIER_PUBSUB_SUBSCRIPTION`（運用ポリシー / 依存関係）
  - envconfig default に任せる: `DEDUP_COLLECTION`, `DEDUP_TTL_HOURS`, `USERS_COLLECTION`, `FCM_CONCURRENCY`, `SHUTDOWN_GRACE_SECONDS`（tuning パラメータ）

B) 全部 Terraform で明示

C) 全部 envconfig default

[A]: A

---

## 承認前の最終確認（回答確定）

- **Q1 [A]**: Firestore `notifier_dedup` TTL policy = **`modules/shared/firestore.tf`** に追加
- **Q2 [A]**: Firestore `users` 複合インデックス = **`modules/shared/firestore.tf`** に追加（U-BFF と共用）
- **Q3 [A]**: Cloud Run scaling = **現状維持**（`min=0 / max=2 / cpu=1 / memory=512Mi`）
- **Q4 [A]**: env の Terraform 反映粒度 = U-ING Q5 [A] と同じ（**既存 2 env はそのまま、tuning は envconfig default で吸収**）。**Terraform 追加 env ゼロ**

回答確定済み。以下を生成:

- `construction/U-NTF/infrastructure-design/deployment-architecture.md`
- `construction/U-NTF/infrastructure-design/terraform-plan.md`
