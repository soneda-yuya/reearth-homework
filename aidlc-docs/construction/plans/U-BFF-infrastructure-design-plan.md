# U-BFF Infrastructure Design Plan

## Overview

U-BFF（Connect Server Unit、Sprint 3）の **Infrastructure Design** 計画。U-PLT で `terraform/modules/bff/` が **Cloud Run Service + BFF Runtime SA + CMS Secret IAM + Firestore R/W IAM + public invoker** まで実装済みなので、本ステージは **Flutter クライアント接続（CORS / ingress）、Cloud Run scaling / concurrency 最終値、Firebase Admin SDK 利用に必要な追加設定、env の Terraform 反映粒度** が中心。U-NTF と同様、薄く済む見込み。

## Context — すでに U-PLT で決まっていること

[`terraform/modules/bff/`](../../../terraform/modules/bff/) に以下が実装済み:

- `google_cloud_run_v2_service "bff"`
  - `ingress = INGRESS_TRAFFIC_ALL`（Flutter クライアントが public URL で叩く前提）
  - `scaling { min=0, max=3 }`
  - `cpu=1 / memory=512Mi`
  - startup / liveness probe on `/healthz`
  - port 8080（`BFF_PORT` env で bind）
- `google_cloud_run_v2_service_iam_member "invoker"` = `allUsers`（認証は AuthInterceptor で実施）
- Runtime SA (`bff-runtime`) の IAM:
  - `roles/datastore.user`（Firestore `users` R/W）
  - `roles/secretmanager.secretAccessor`（CMS Integration Token 読取）
- 既存 env:
  - `PLATFORM_SERVICE_NAME`, `PLATFORM_ENV`, `PLATFORM_GCP_PROJECT_ID`, `PLATFORM_OTEL_EXPORTER`
  - `BFF_PORT`, `BFF_CMS_BASE_URL`, `BFF_CMS_WORKSPACE_ID`, `BFF_CMS_INTEGRATION_TOKEN`（secret_key_ref）

## U-BFF Design で確定済みの前提

[`U-BFF/design/U-BFF-design.md`](../U-BFF/design/U-BFF-design.md) より:

- **Q1**: 全 RPC で Firebase ID Token 必須 → Firebase Admin SDK で検証（ADC で公開鍵キャッシュ、追加 IAM 不要）
- **Q2**: SafetyIncident は毎回 CMS 直アクセス（キャッシュなし、Memorystore 等の追加リソース不要）
- **Q3**: CrimeMap は `crimemap/application.Aggregator` で in-memory 集計（CloudRun pod 内で完結、追加リソース不要）
- **Q4**: UserProfile は U-NTF と `users` collection 共有 → U-NTF 側で既に Firestore DB + 複合インデックス + `notifier_dedup` TTL を `modules/shared/firestore.tf` に定義済み
- **Q5**: FCM Token は `ArrayUnion`（Firestore 追加インデックス不要、document-id で直接アクセス）
- **Q6**: errs → Connect code 自動変換（inline、追加リソース不要）
- **Q7**: Span + 4 Metric + `app.bff.phase` 属性ログ（U-PLT `otelx` / `slogx` が既にセットアップ済み、追加リソース不要）
- **Q8**: 層別カバレッジ + `connecttest.Server` で handler e2e（CI でのみ実行、インフラ影響なし）
- **Q9**: 2 PR 分割（U-ING パターン）

---

## Step-by-step Checklist

- [ ] Q1〜Q6 すべて回答
- [ ] 成果物を生成:
  - [ ] `construction/U-BFF/infrastructure-design/deployment-architecture.md`
  - [ ] `construction/U-BFF/infrastructure-design/terraform-plan.md`
- [ ] 承認後、U-BFF Code Generation 計画へ進む

---

## Questions

### Question 1 — Flutter クライアント接続方式（CORS / Cloud Run ingress）

U-BFF は Flutter アプリ（別レポ `overseas-safety-map-app`）から叩かれる。Flutter が **mobile のみ** か **web も含む** かで CORS ポリシーが変わる。現在は `ingress = INGRESS_TRAFFIC_ALL + invoker = allUsers + AuthInterceptor` で public 公開、AuthInterceptor で Firebase ID Token 検証して認可する構成。

**選択肢**:

A) **推奨**: **Flutter mobile のみ（iOS/Android）と仮定し、CORS 設定なし**
  - ✅ Mobile クライアントは CORS を適用されないので、Connect サーバ側で CORS ヘッダーを付ける必要がない
  - ✅ `connectcors` middleware を入れるコストと認知負荷を避けられる
  - ✅ Flutter web を将来対応するとき、その時点で `connectcors` を追加すれば済む（可逆）
  - ⚠️ Flutter web で開発する（`flutter run -d chrome`）ケースは暫定的にブラウザ CORS エラーで弾かれる

B) **Flutter web も視野に入れて最初から CORS を有効化**
  - ✅ Web dev 時も flutter run -d chrome で即疎通
  - ⚠️ 使っていない段階で middleware を入れる（YAGNI）
  - ⚠️ 許可 origin の管理（dev / prod）が増える

C) Cloud Run 前段に **Cloud Armor + CDN + カスタムドメイン** を立て、そこで CORS 統制
  - ⚠️ MVP として明らかに過剰、構築コスト高

[Answer]: A

### Question 2 — Cloud Run scaling / concurrency 値

現状: `min_instance_count=0, max_instance_count=3, cpu=1, memory=512Mi`、concurrency は明示しておらず Cloud Run デフォルト (= 80 req/instance)。

**トラフィック想定（MVP）**:
- Flutter モバイルアプリ利用者数は MVP として **~10-100 DAU** を想定
- 1 画面表示 ≒ 1-3 RPC（`ListSafetyIncidents` + `GetChoropleth` + `GetProfile`）
- ピーク: **1-5 req/s**、1 request の処理 ~200-500ms（CMS 読取 + in-memory 集計）

**選択肢**:

A) **推奨**: **現状維持**（`min=0 / max=3 / cpu=1 / memory=512Mi`、concurrency デフォルト 80）
  - `max=3` × concurrency 80 = 240 並列、MVP トラフィックには十分
  - ✅ コスト最小（通常は instance 0）
  - ✅ 雛形のまま、追加作業なし
  - ⚠️ Cold start が発生するが、Flutter 起動時に `GetProfile` で一発ウォームアップされるので UX 影響は軽微

B) `min=1` で常時起動（Cold start 回避）
  - ✅ Flutter 起動直後の p95 が改善する
  - ⚠️ 24h instance 稼働で月 $15-20 追加（MVP として過剰）
  - ⚠️ U-BFF の SLO p95 < 500ms は `min=0` でも達成できる見込み

C) concurrency を明示的に下げる（例: 30）
  - ⚠️ BFF は I/O bound（CMS HTTP + Firestore）なので concurrency を下げる理由が乏しい
  - ⚠️ 過剰チューニング

[Answer]: A

### Question 3 — Firestore リソースの所有権整理

U-NTF で `modules/shared/firestore.tf` に以下を既に定義済み:
- `google_firestore_database "default"`（Firestore DB 本体）
- `google_firestore_field "notifier_dedup_ttl"`（U-NTF 専用）
- `google_firestore_index "users_notification"`（U-NTF の query 用、`users` collection 上）

U-BFF は `users` collection の **主オーナー**（Get/Create/Update/Toggle）で、U-NTF はその reader。BFF の query パターンは **document-id 直接アクセスのみ**（`GetProfile`, `ToggleFavoriteCountry`, `UpdateNotificationPreference`, `RegisterFcmToken` は全部 `users/{uid}` に対するピンポイント操作）。追加の複合インデックスは不要。

**選択肢**:

A) **推奨**: **`shared/firestore.tf` に変更を加えない**（BFF 独自のインデックス・TTL を追加しない）
  - ✅ BFF の query は全て document-id アクセスなので、Firestore は自動で index を作ってくれる（単一フィールド index は自動生成）
  - ✅ shared module のスコープを広げない
  - ⚠️ 将来 `ListNearby` で Firestore を使うなら、その時点で geohash index を追加

B) BFF 用の新しい複合インデックスを `shared/firestore.tf` に追加
  - ⚠️ Q3 [A] で明らかに不要なので追加しない

[Answer]: A

### Question 4 — Firebase Admin SDK 利用に伴う追加 IAM / 設定

U-BFF は **Firebase Admin SDK** で以下を実行:
1. **ID Token 検証**（`auth.VerifyIDToken`）— 公開鍵を Google から取得してキャッシュ検証（IAM 不要）
2. **Firestore R/W**（`firestore.NewClient` 経由）— 既存 `roles/datastore.user` で充足

既存の BFF Runtime SA IAM:
- `roles/datastore.user`（Q4 の Firestore R/W に必要、充足）
- `roles/secretmanager.secretAccessor`（CMS 用）

[`bff/iam.tf`](../../../terraform/modules/bff/iam.tf) には次のコメントが既にある:
> "No Firebase Auth role is granted here: the BFF only verifies Firebase ID Tokens (which Firebase Admin SDK does against cached Google public certificates — no IAM required)."

**選択肢**:

A) **推奨**: **IAM 変更なし**（既存の 2 role で充足、コメントはそのまま残す）
  - ✅ 最小権限、追加ロールなし
  - ✅ 既存コメントが設計意図を正しく説明している
  - ⚠️ 将来 Admin API（`createUser` / `disableUser`）を呼ぶ必要が出たら、その時点で narrow role を追加

B) 念のため `roles/firebase.viewer` を追加
  - ⚠️ 不要（ID Token 検証は IAM ゲートされない）、YAGNI

C) Firebase Auth 用の別 SA を作成してキー交換
  - ⚠️ 明確に過剰、ADC で完結する

[Answer]: A

### Question 5 — 追加 env の Terraform 反映粒度（U-NTF Q4 / U-ING Q5 と同パターン）

U-BFF design で新規に出てくる tuning パラメータ候補:

| env | Terraform 渡し or envconfig default? |
|---|---|
| `BFF_REQUEST_BODY_LIMIT_BYTES` | envconfig default (`1 MiB`)? |
| `BFF_FCM_TOKEN_MAX` | envconfig default (`10`)? |
| `BFF_USERS_COLLECTION` | envconfig default (`users`)? |
| `BFF_SHUTDOWN_GRACE_SECONDS` | envconfig default (`10`)? |
| `BFF_FIREBASE_PROJECT_ID` | Terraform（= `var.project_id`、明示的に渡す） |

既存 env（Terraform 渡し）:
- `PLATFORM_GCP_PROJECT_ID`（= `var.project_id`）は既にあるので、`BFF_FIREBASE_PROJECT_ID` は「Platform env を flat に読むか、BFF 専用に別の env を立てるか」の判断になる。

**選択肢**:

A) **推奨**: **U-ING Q5 [A] / U-NTF Q4 [A] と同じ方針**
  - Terraform 渡し: **既存の `PLATFORM_GCP_PROJECT_ID` を Firebase 用にも流用**（= 追加 env ゼロ）
  - envconfig default に任せる: `REQUEST_BODY_LIMIT_BYTES`, `FCM_TOKEN_MAX`, `USERS_COLLECTION`, `SHUTDOWN_GRACE_SECONDS`（tuning パラメータ）
  - ✅ Terraform 追加 env ゼロ、変更面積最小
  - ✅ tuning は Go コード側で調整可能、運用で Cloud Run Revision 更新不要

B) 全部 Terraform で明示
  - ⚠️ 過剰、雑多な tuning を IaC に混ぜる

C) 全部 envconfig default
  - ⚠️ `FIREBASE_PROJECT_ID` を default 値にすると env ごとに baking が必要になり運用が辛い

[Answer]: A

### Question 6 — 作業着手の PR 戦略（インフラ変更は極小か）

上記 Q1-Q5 で確定する変更量が極めて小さい可能性がある:
- Q1 [A] で CORS middleware 追加なし → Terraform 変更ゼロ
- Q2 [A] で scaling 現状維持 → Terraform 変更ゼロ
- Q3 [A] で Firestore 変更なし → Terraform 変更ゼロ
- Q4 [A] で IAM 変更なし → Terraform 変更ゼロ
- Q5 [A] で env 追加ゼロ → Terraform 変更ゼロ

**結果として Terraform コード変更がゼロ** になる可能性が高い。

**選択肢**:

A) **推奨**: **変更ゼロでも Infrastructure Design ドキュメントは生成する**（`deployment-architecture.md` + `terraform-plan.md`）
  - ✅ 「なぜ U-PLT の既存インフラで充足するのか」を明示的に残すことで、後続開発者が同じ判断を再検討せずに済む
  - ✅ ドキュメントは設計プロセスの一部、コード変更の有無とは独立
  - ⚠️ 退屈な「変更なし」ドキュメントになるが、正しい状態を示す

B) Terraform 変更ゼロならドキュメント生成をスキップ
  - ⚠️ AI-DLC は stage ごとの artefact を原則とするので skip は避ける

C) 変更ゼロの場合は `terraform-plan.md` だけ省略
  - △ 妥協案だが、deployment-architecture.md で終端点や制約を書けるので OK

[Answer]: A

---

## 承認前の最終確認（回答確定）

- **Q1 [A]**: Flutter mobile のみ前提、CORS 設定なし（web 対応時に `connectcors` 追加、可逆）
- **Q2 [A]**: Cloud Run scaling / concurrency = **現状維持**（`min=0 / max=3 / cpu=1 / memory=512Mi` + concurrency デフォルト 80）
- **Q3 [A]**: Firestore = **`shared/firestore.tf` に変更なし**（BFF は document-id 直接アクセスのみ）
- **Q4 [A]**: IAM = **変更なし**（既存 `roles/datastore.user` + `roles/secretmanager.secretAccessor` で充足）
- **Q5 [A]**: env = **U-ING Q5 / U-NTF Q4 と同方針**（`PLATFORM_GCP_PROJECT_ID` 流用、tuning は envconfig default、Terraform 追加 env ゼロ）
- **Q6 [A]**: Terraform 変更ゼロでも Infrastructure Design ドキュメント 2 種生成

**結果**: Terraform コード変更ゼロ。`deployment-architecture.md` + `terraform-plan.md` で「既存インフラで充足する根拠」を文書化する。

回答確定済み。以下を生成:

- `construction/U-BFF/infrastructure-design/deployment-architecture.md`
- `construction/U-BFF/infrastructure-design/terraform-plan.md`
