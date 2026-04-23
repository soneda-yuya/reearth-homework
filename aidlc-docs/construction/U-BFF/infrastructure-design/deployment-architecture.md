# U-BFF Deployment Architecture

**Unit**: U-BFF（Connect Server / BFF Unit、Sprint 3）
**Deployable**: `cmd/bff` → Cloud Run Service `bff`
**受信**: HTTPS / HTTP2（Connect RPC）from Flutter モバイルアプリ
**参照**: [`U-BFF/design/U-BFF-design.md`](../design/U-BFF-design.md)、[`construction/plans/U-BFF-infrastructure-design-plan.md`](../../plans/U-BFF-infrastructure-design-plan.md)

---

## 1. Component Overview

```
┌──────────────────────────── GCP Project: overseas-safety-map (prod) ──────────────────────────────┐
│  Region: asia-northeast1                                                                          │
│                                                                                                   │
│  Flutter App (別レポ、iOS / Android)                                                              │
│     └── Firebase Anonymous Auth で ID Token 取得                                                   │
│           │                                                                                       │
│           ▼                                                                                       │
│  HTTPS (public URL、Connect RPC、Authorization: Bearer <ID Token>)                                │
│           │                                                                                       │
│           ▼                                                                                       │
│  ┌─ Cloud Run Service: bff ─────────────────────────────┐                                        │
│  │  image: <AR_URL>/bff:<tag>                            │                                        │
│  │  ingress = INGRESS_TRAFFIC_ALL  (invoker = allUsers)  │                                        │
│  │  cpu=1 / memory=512Mi                                 │                                        │
│  │  scaling: min=0 / max=3                               │                                        │
│  │  concurrency: 80 (default)                            │                                        │
│  │  startup/liveness probe on /healthz                   │                                        │
│  │  SA: bff-runtime                                      │                                        │
│  │                                                       │                                        │
│  │  AuthInterceptor (Firebase ID Token 検証) →           │                                        │
│  │   ErrorInterceptor (errs → Connect code 自動変換) →   │                                        │
│  │    MetricInterceptor / TraceInterceptor (U-PLT) →     │                                        │
│  │     Service Handlers (3 Service / 11 RPC)             │                                        │
│  │                                                       │                                        │
│  │  ENV (Terraform 渡し):                                │                                        │
│  │    PLATFORM_*                                         │                                        │
│  │    BFF_PORT = "8080"                                  │                                        │
│  │    BFF_CMS_BASE_URL / BFF_CMS_WORKSPACE_ID            │                                        │
│  │    BFF_CMS_INTEGRATION_TOKEN (secret_key_ref)         │                                        │
│  │                                                       │                                        │
│  │  ENV (envconfig default):                             │                                        │
│  │    BFF_REQUEST_BODY_LIMIT_BYTES (1 MiB)               │                                        │
│  │    BFF_FCM_TOKEN_MAX (10)                             │                                        │
│  │    BFF_USERS_COLLECTION ("users")                     │                                        │
│  │    BFF_SHUTDOWN_GRACE_SECONDS (10)                    │                                        │
│  └────┬────────────────────┬─────────────────┬───────────┘                                        │
│       │                    │                 │                                                    │
│       │ (ADC via runtime SA + Firebase Admin SDK + cmsx HTTP)                                     │
│       ▼                    ▼                 ▼                                                    │
│  ┌─ reearth-cms ──┐  ┌─ Firestore ──────┐  ┌─ Firebase Auth ─────────┐                           │
│  │  Integration   │  │  users/{uid}     │  │  ID Token 検証          │                           │
│  │  REST API      │  │   (U-BFF オーナー)│  │  (公開鍵キャッシュ、    │                           │
│  │  (read only)   │  │                  │  │   IAM 不要)              │                           │
│  │                │  │  U-NTF が reader │  │                          │                           │
│  └────────────────┘  └──────────────────┘  └──────────────────────────┘                           │
└───────────────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Infrastructure Decisions（計画回答の確定）

| # | 決定事項 | 値 | 備考 |
|---|---|---|---|
| Q1 | Flutter クライアント接続方式 | **mobile 前提、CORS 設定なし** | Web 対応時に `connectcors` 追加で可逆 |
| Q2 | Cloud Run scaling / concurrency | 現状維持（`min=0 / max=3 / cpu=1 / memory=512Mi`、concurrency デフォルト 80） | U-PLT 雛形のまま |
| Q3 | Firestore リソース所有権 | **`shared/firestore.tf` 変更なし** | BFF は document-id 直接アクセスのみ |
| Q4 | Firebase Admin SDK IAM | **変更なし**（既存 `datastore.user` + `secretmanager.secretAccessor` で充足） | ID Token 検証は IAM ゲートされない |
| Q5 | 追加 env の Terraform 反映粒度 | **Terraform 追加 env ゼロ**、tuning は envconfig default | U-ING Q5 / U-NTF Q4 と同方針 |
| Q6 | PR 戦略 | 変更ゼロでも Infrastructure Design ドキュメント 2 種生成 | 設計意図を明文化 |

**結論**: U-PLT で BFF module（Cloud Run + BFF Runtime SA + CMS Secret IAM + Firestore R/W IAM + public invoker）が既に充足、U-NTF で Firestore 共通リソースが shared module に収まっているため、**Terraform コード変更ゼロ** で U-BFF の要件を満たす。

---

## 3. Cloud Run Service 仕様（現状維持）

U-PLT で完成済みの `google_cloud_run_v2_service.bff` をそのまま使用。U-BFF での変更は **なし**。

### 要点

- **`ingress = INGRESS_TRAFFIC_ALL`**: Flutter モバイルアプリが public URL に到達するため ALL。**認可は AuthInterceptor** で実施（`invoker = allUsers` は IAM ゲートを外すだけ、認証は app layer）
- **`min_instance_count = 0`**: アイドル時 0 instance、Flutter 起動時の `GetProfile` が warm-up として機能
- **`max_instance_count = 3`**: 80 req/instance concurrency × 3 = 240 concurrent、MVP トラフィック（~1-5 req/s）に対して十分
- **`cpu = 1, memory = 512Mi`**: CMS HTTP + Firestore SDK + Firebase Admin SDK + Go runtime に十分
- **`startup_probe`, `liveness_probe`**: `/healthz` エンドポイント

### Cloud Run Service autoscaling 動作

```
req/s  → Cloud Run 判断
0      → min_instance_count (0) まで scale down
スパイク → concurrency 80 を超えたら instance 追加（max=3 が上限）
サステイン後 → 15 分 idle で scale down
```

### Connect RPC 配信

- Cloud Run v2 は HTTP/2 を自動でネゴシエーション（Connect の binary / gRPC トランスポート対応）
- `connectserver.Start(ctx)` が SIGTERM 受信で graceful shutdown（10s grace、`BFF_SHUTDOWN_GRACE_SECONDS` で tuning）

---

## 4. IAM（現状維持）

U-BFF Runtime SA (`bff-runtime`) に付与済みの IAM binding:

| Binding | 用途 |
|---|---|
| Runtime SA: `roles/datastore.user` | Firestore `users` collection R/W + Firebase Admin SDK Firestore client |
| Runtime SA: `roles/secretmanager.secretAccessor` on `cms-integration-token` secret | CMS Integration Token 読取 |
| Cloud Run Service: `roles/run.invoker` に `allUsers` | Public 到達許可（認可は AuthInterceptor） |

### Firebase ID Token 検証に IAM が不要な理由

Firebase Admin SDK の `VerifyIDToken` は、Google の公開鍵エンドポイント（public、認証不要）から署名検証用の公開鍵をキャッシュして JWT を検証する。この一連の操作は GCP IAM を介さないため、BFF Runtime SA に Firebase 関連ロール（`roles/firebase.viewer` 等）を付与する必要は **ない**。

将来 Admin API（`createUser` / `disableUser` 等）を呼ぶ必要が出たら、その時点で必要な narrowest role を追加する方針（`bff/iam.tf` のコメントに既に記載）。

---

## 5. Firestore（既存 shared リソース流用）

U-NTF で追加済みの `modules/shared/firestore.tf` を流用:

- `google_firestore_database.default`（DB 本体、U-PLT で作成、asia-northeast1）
- `google_firestore_field.notifier_dedup_ttl`（U-NTF 専用、U-BFF は使わない）
- `google_firestore_index.users_notification`（U-NTF 用の composite index、U-BFF には不要）

### U-BFF の Firestore アクセスパターン

全て **document-id 直接アクセス** で完結:

| RPC | Firestore operation |
|---|---|
| `GetProfile` | `Collection("users").Doc(uid).Get(ctx)` |
| `ToggleFavoriteCountry` | `Doc(uid).Update(... ArrayUnion / ArrayRemove)` |
| `UpdateNotificationPreference` | `Doc(uid).Update(... NotificationPreference)` |
| `RegisterFcmToken` | `Doc(uid).Update(... ArrayUnion(fcmTokens))` |
| `CreateIfMissing`（Get 初回時） | `Doc(uid).Create(UserProfile)` |

**複合インデックス不要**: Firestore は document-id / 単一フィールドの index を自動生成するため、追加 index は不要。

将来 `ListNearby` が Firestore 上で動く設計に変わるなら、その時点で geohash index を追加する（現在の設計では CMS 経由で完結）。

---

## 6. 外部依存関係

### 6.1 reearth-cms Integration REST API

- **エンドポイント**: `BFF_CMS_BASE_URL` で Cloud Run env から注入
- **認証**: Integration Token（Secret Manager `cms-integration-token`）を `Authorization: Bearer` ヘッダで送る
- **呼出元**: `safetyincident/infrastructure/CMSReader`（`cmsx.Client` を経由）
- **読取のみ**: U-BFF は CMS に write しない（write は U-ING の責務）

### 6.2 Firebase Anonymous Auth / ID Token

- **ID Token 発行**: Flutter アプリ側で Firebase Anonymous Auth を使って匿名ユーザーを作成、ID Token を取得
- **ID Token 検証**: BFF の AuthInterceptor が `Authorization: Bearer <idToken>` を読み、Firebase Admin SDK の `auth.VerifyIDToken` で検証
- **uid の取り出し**: 検証後、`ctx` に `uid` を付与して後続 UseCase へ伝播
- **未認証/期限切れ/署名不正**: `KindUnauthorized` の `errs.AppError` を返し、ErrorInterceptor が `connect.CodeUnauthenticated` にマップ

---

## 7. 運用ランブック（簡略、詳細は Build and Test で）

### 7.1 通常運用

Flutter アプリが直接 Cloud Run の public URL を叩く。運用者の操作不要。

### 7.2 初回デプロイ時の注意

Terraform apply の順序:
1. Cloud Run Service デプロイ（数秒）
2. Secret Manager に CMS Integration Token を登録（Terraform 外、手動）
3. 動作確認: Flutter Debug ビルドから ID Token 付きで RPC 疎通

Firestore 側は U-NTF が既に apply している前提（`users` collection を共有）。

### 7.3 障害時の復旧

1. Cloud Logging (`resource.labels.service_name=bff`) で `severity=ERROR / WARN` を確認
2. `app.bff.phase` 属性で失敗段階を特定（auth / cms_read / firestore / aggregate / response）
3. CMS API 側の障害の場合: `KindExternal` が多発 → Status Dashboard で確認
4. Firebase Auth 障害の場合: `KindUnauthorized` が多発 → Firebase Console で confirm
5. Cold start が問題になる場合: `min_instance_count` を暫定的に `1` に引き上げ（Terraform で `var.bff_min_instance_count = 1` を立てる）

### 7.4 インスタンスメトリック確認

Cloud Monitoring で:
- `run.googleapis.com/container/instance_count`: 平常時 0、日中トラフィック時 1-2
- `run.googleapis.com/request_count{response_code_class=5xx}`: 異常検知
- `app.bff.request_duration` (p95): < 500ms（Choropleth は < 1s）維持
- `app.bff.auth_failure` (count): 急増時はクライアント側の Firebase Auth 設定不整合の可能性

### 7.5 CMS 疎通確認

Cloud Run 実行環境で以下の curl 相当が通ることを Build and Test フェーズで確認:
```bash
curl -H "Authorization: Bearer <cms-integration-token>" \
     "${BFF_CMS_BASE_URL}/api/projects/${BFF_CMS_WORKSPACE_ID}/models/safety-incident/items?limit=1"
```

---

## 8. 非スコープ

- **CORS 対応**（Q1 [A] で Flutter mobile 限定の方針、web 対応時に `connectcors` middleware を追加）
- **CDN / Cloud Armor / カスタムドメイン**（MVP では Cloud Run デフォルト URL 直叩き）
- **リクエストキャッシュ**（Q2 Design [A] で毎回 CMS 直アクセス、将来の課題）
- **Rate limiting**（Firebase Auth の RPM 制限で自然に抑制、必要になれば Cloud Armor で追加）
- **Multi-Region / DR**（単一リージョン、Firestore は multi-region で自動対応）
- **RPC-level authorization**（Q1 Design [A] で「全 RPC で ID Token 必須」、RPC ごとの詳細権限分けは将来）

---

## 9. トレーサビリティ

| 上位要件 | U-BFF Infra 対応 |
|---|---|
| NFR-BFF-PERF-01 (p95 < 500ms) | §3 Cloud Run scaling（concurrency 80 × max 3 で余裕） |
| NFR-BFF-PERF-02 (Choropleth p95 < 1s) | §3 CMS 集計を in-memory で完結、スケール必要時は `max` 引き上げ |
| NFR-BFF-SEC-01 (Firebase ID Token 必須) | §6.2 AuthInterceptor + Firebase Admin SDK |
| NFR-BFF-SEC-02 (Firestore Security Rules) | §5 document-id アクセスのみ、owner = uid ルールで十分 |
| NFR-BFF-REL-01 (idempotent Profile 操作) | §5 `CreateIfMissing` + ArrayUnion / ArrayRemove |
| NFR-BFF-OPS-01-04 (ログ / Metric / トレース) | §7.4 Cloud Monitoring、`app.bff.phase` 属性 |
| NFR-BFF-EXT-01 (他 auth provider 拡張) | §6.2 AuthVerifier Port で差し替え可能 |
