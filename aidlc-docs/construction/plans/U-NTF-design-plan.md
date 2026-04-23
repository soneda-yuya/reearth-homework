# U-NTF Design Plan (Minimal 合本版)

## Overview

U-NTF（Notifier Unit、Sprint 4）は **Pub/Sub Push Subscription で U-ING の `NewArrivalEvent` を受信 → Firestore の購読者を解決 → FCM に push 配信 → 無効 token を Firestore から除去** する **Cloud Run Service**。

ワークフロー圧縮 Option B に従い、**Functional Design + NFR Requirements + NFR Design** を 1 ドキュメントに合本します。

## Context（確定済み）

- **Bounded Context**: `notification`（Supporting domain）
- **Deployable**: `cmd/notifier`（Cloud Run Service、Pub/Sub Push Subscription のターゲット）
- **責務**: Pub/Sub 受信 → Firestore 購読者解決 → FCM 配信 → 無効 token 除去
- **依存**: U-PLT（共通基盤）、U-ING（Pub/Sub `safety-incident.new-arrival` topic が publish 済み前提）
- **受信**: Pub/Sub Push Subscription（U-PLT shared infra に DLQ あり、`notifier-safety-incident-new-arrival`）
- **配信**: Firebase Cloud Messaging（FCM）via Firebase Admin SDK
- **永続層**: Firestore（ユーザー token + 通知設定）

U-PLT 共通規約（slog + OTel / envconfig + Secret Manager / `errs.Wrap` / retry / rate limit / `Clock` / terraform module 構成 / CI / Dockerfile）は**全てそのまま踏襲**します。Pub/Sub は U-ING で wire 済みの `pubsubx`（v2）を subscription 側で拡張。Firebase は `firebasex`（U-PLT のプレースホルダ）を本 Unit で本実装化。

---

## Step-by-step Checklist

- [ ] Q1〜Q8 すべて回答
- [ ] 矛盾・曖昧さの検証、必要なら clarification
- [ ] 成果物を生成:
  - [ ] `construction/U-NTF/design/U-NTF-design.md` — Functional + NFR Req + NFR Design 合本
- [ ] 承認後、U-NTF Infrastructure Design へ進む

---

## Questions

### Question 1 — Pub/Sub 受信方式（Push vs Pull）

A) **推奨**: **Push Subscription**（HTTP POST を notifier に送る）
  - Terraform でも既に Push 前提で設定済み（`subscription.tf` が push_config を持つ）
  - Cloud Run Service として HTTP サーバを立てて `/pubsub/push` エンドポイントで受信
  - Pub/Sub が ACK / NACK を HTTP レスポンスコードで判定（2xx=ACK、4xx/5xx=NACK）
  - ✅ 既存雛形と整合、スケーリングが Cloud Run の autoscaling に委ねられる
  - ⚠️ HTTP 公開（INGRESS_ALL）だが IAM で Pub/Sub service agent のみ invoker に制限済み

B) Pull Subscription（notifier が長時間 Pull する Worker 型）
  - Cloud Run Service の長寿命モデルに合わない（Cloud Run は req-based autoscaling）
  - Cloud Run Worker pools を使う選択肢もあるが MVP には過剰

[A]: A

### Question 2 — Dedup 戦略（U-ING からの重複メッセージ対策）

U-ING Code Gen summary.md §5 で「U-ING は at-least-once publish、U-NTF 側で dedup」と決定済み。具体的にどう dedup するか:

A) **推奨**: **Firestore ベース dedup（TTL 付き）**
  - `Collection: notifier_dedup` に `docId = key_cd` で書き込み
  - TTL 24 時間（Firestore の automatic TTL 機能、`expireAt` フィールド）
  - 受信時に `doc = dedup.doc(key_cd).get()`、存在すれば ACK して即終了
  - 存在しなければ transactional `create` でマーキング → FCM 配信
  - ✅ Firestore は notifier/user 両方で既に使う前提、追加依存なし
  - ⚠️ Firestore RW コストが若干増える（1 message = 1 read + 1 write = ~$0.01/日@想定規模）

B) Pub/Sub Message ID ベース dedup（Pub/Sub の exactly-once delivery に依存）
  - Pub/Sub exactly-once を有効化すれば重複 0 を保証（SDK 側で 5 分以内の重複を排除）
  - ✅ コードが最小、Firestore 書き込み不要
  - ⚠️ 5 分超の再配信 / U-ING の並行 Run で重複 publish された場合は防げない
  - ⚠️ Pub/Sub exactly-once の SLA は Google 保証だが完全ではない（参考: [docs](https://cloud.google.com/pubsub/docs/exactly-once-delivery)）

C) In-memory LRU（Cloud Run instance 内のみ、クロスインスタンスで効かない）
  - ⚠️ Cloud Run の max_instance_count=2 で dedup が効かない
  - 採用しない

[A]: A

### Question 3 — 購読者解決ロジック（誰に通知を届けるか）

Firestore のユーザー設定に基づいて「この `NewArrivalEvent` を受け取るべきユーザー」を抽出する:

A) **推奨**: **`country_cd` 一致 + 通知設定 ON** のユーザーを Firestore クエリで抽出
  ```
  Query: users
    .where("notification_preference.enabled", "==", true)
    .where("notification_preference.target_country_cds", "array-contains", event.country_cd)
  ```
  - 各ユーザーの `fcm_tokens`（複数端末対応）に対して FCM `multicast` で配信
  - ✅ シンプル、Firestore インデックス 1 個追加で OK
  - ⚠️ ユーザー数 × 端末数 の爆発に注意（MVP は数百ユーザー想定）

B) A + **`info_type` フィルタも評価**（ユーザーが「危険情報」のみ欲しい等）
  - `UserProfile.notification_preference.info_types` 配列をチェック
  - 空配列 → 全 info_type 受信、指定あり → そのリストと `event.info_type` の AND
  - ✅ 細かい制御可能
  - ⚠️ Firestore クエリに array-contains を 2 個使えないので、country で絞った後 in-memory でフィルタ

C) A + B + **地理的距離フィルタ**（ユーザーの現在位置 ±500km 内の事象のみ）
  - MVP 過剰、Flutter 側で GPS 取得が必要、privacy も考慮事項増

[A]: A

### Question 4 — FCM 配信戦略

Firebase Cloud Messaging の呼び出し単位:

A) **推奨**: **ユーザーごとに `SendMulticast` で複数 token 一括配信**
  - 1 ユーザーあたり 1 回 Admin SDK 呼び出し（multicast で最大 500 token）
  - `BatchResponse.Responses[i].Error` で各 token ごとの失敗を判定
  - 失敗 token は invalid-argument / registration-token-not-registered で判定して Firestore から除去
  - ✅ FCM SDK の標準的な使い方
  - ⚠️ 1 ユーザー 500 token 超は現実的に無い（MVP では 1-3 端末想定）

B) ユーザー × token 個別送信（`Send` を token 数だけ呼ぶ）
  - SDK の overhead が高い、レート制限に近づく
  - 採用しない

C) Topic messaging（FCM の Topic 機能で country_cd ごとに topic を持つ）
  - ✅ 最もシンプル（FCM が配信を管理）
  - ⚠️ info_type / 個別ユーザー設定との組み合わせが難しい、A より柔軟性↓
  - ⚠️ Token が Topic から明示的に subscribe する必要、Flutter アプリ側の責務

[A]: A

### Question 5 — 無効 token の除去タイミング

FCM 配信で `registration-token-not-registered` / `invalid-argument` を検知したとき:

A) **推奨**: **同一 Request 内で Firestore から削除**
  - `BatchResponse` を見て無効 token を特定 → 該当 `UserProfile.fcm_tokens` から `ArrayRemove`
  - ユーザー側で re-login → 新 token 取得 → アプリで Firestore に再登録、のフローを想定
  - ✅ 即時反映、stale token が積み重なるリスク無し
  - ⚠️ Firestore write が増えるが、通常は無効 token は稀

B) 別の Cloud Run Job / Scheduler で週次クリーンアップ
  - Stale 判定が難しい（最終使用時刻を持つ必要）
  - 採用しない

[A]: A

### Question 6 — エラーハンドリング / Pub/Sub ACK 戦略

Pub/Sub Push の HTTP レスポンスコードで Pub/Sub 側の挙動が変わる:
- `2xx`: ACK → メッセージ削除
- `4xx` (client error): NACK → retry（Pub/Sub のバックオフに従う）→ 最終的に DLQ
- `5xx` (server error): 同上

A) **推奨**: **細かく使い分け**
  - Dedup hit（既処理）→ **200 OK**（ACK、retry 不要）
  - 購読者 0（対象ユーザーなし）→ **200 OK**（ACK、通知すべき相手がいないだけ）
  - FCM 配信成功（一部 token 失敗含む）→ **200 OK**（本流は成功、stale token 除去は副次的）
  - Firestore / FCM API が **transient error**（500/429/context deadline）→ **500** 返却（Pub/Sub が retry、最終的に DLQ）
  - Pub/Sub payload parse error（malformed JSON）→ **400**（client error、即 DLQ、再試行しない）
  - ✅ 各エラーの semantics が Pub/Sub 側の挙動と一致
  - ⚠️ 実装分岐が増えるが複雑度は中程度

B) 常に 200 を返す（エラーもログのみ）
  - Pub/Sub の再配信機能を一切使わず、notifier 内で自力 retry
  - ⚠️ transient error で通知が永久に届かなくなる
  - 採用しない

C) エラー時は全て 500（Pub/Sub retry + DLQ に任せる）
  - Dedup hit も 500 → retry 無限ループ or DLQ 入り
  - 採用しない

[A]: A

### Question 7 — OTel Observability

U-ING と同レベルの observability:

A) **推奨**:
  - **Span**: `notifier.Receive` (HTTP handler) → `dedup.Check` / `users.Resolve` / `fcm.SendMulticast` / `tokens.Cleanup`
  - **Metric**:
    - `app.notifier.received` (Counter, attr: `country_cd` / `info_type`)
    - `app.notifier.deduped` (Counter)
    - `app.notifier.recipients` (Histogram, ユーザー数)
    - `app.notifier.fcm.sent` (Counter, attr: `status=success/failure`)
    - `app.notifier.fcm.token_invalidated` (Counter)
    - `app.notifier.duration` (Histogram)
  - **Log**: `app.notifier.phase` 属性（`receive` / `dedup` / `resolve` / `send` / `cleanup` / `done`）
- ✅ U-ING と一貫した観測性

B) Metric 絞り込み（`received` / `sent` のみ）
  - ⚠️ 障害時の切り分けが困難

[A]: A

### Question 8 — テスト戦略

U-CSS / U-ING と同じ方針:

A) **推奨**:
  - **Domain**: `UserProfile` / `SubscriberFilter` / `FCMToken` の unit test
  - **Application**: `DeliverNotificationUseCase` を fake implementation (Firestore / FCM / Dedup Port) でシナリオ 5 本（初回 / dedup hit / subscriber 0 / 部分 token 失敗 / transient error → 500 return）
  - **Infrastructure**: Firestore / FCM の httptest は現実的に困難（両方 gRPC / SDK ベース）→ Interface 抽象 + fake で application 層を網羅、実 SDK 呼び出しは Build and Test で手動
  - **HTTP handler**: `httptest.Server` で Pub/Sub push payload を POST して各 status code パスを検証
  - Q F と同じ層別カバレッジ（domain 95%+ / app 90%+ / infra 70%+ / 全体 85%+）

B) A + Firebase Admin SDK の httptest ベース e2e テスト（SDK mock が複雑すぎる）
  - 採用しない

[A]: A

---

## 承認前の最終確認（回答確定）

- **Q1 [A]**: Pub/Sub 受信方式 = **Push Subscription**（Cloud Run Service + HTTP handler）
- **Q2 [A]**: Dedup 戦略 = **Firestore `notifier_dedup` collection + TTL 24h**（クロスインスタンス / 5 分超の重複にも対応）
- **Q3 [A]**: 購読者解決 = `country_cd` 一致 + `enabled=true` を Firestore query で絞り、`info_types` は in-memory filter（Firestore array-contains × 1 制約のため）
- **Q4 [A]**: FCM 配信戦略 = **`SendMulticast`** でユーザーごとに複数 token 一括配信
- **Q5 [A]**: 無効 token 除去 = **同一 Request 内で Firestore `ArrayRemove`** で即時除去
- **Q6 [A]**: エラーハンドリング = HTTP status code **細かく使い分け**（dedup/no-sub/delivered = 200、parse error = 400、transient = 500）
- **Q7 [A]**: OTel observability = Span 4 + Metric 6 + 構造化ログ属性（`app.notifier.phase`）
- **Q8 [A]**: テスト戦略 = U-CSS/U-ING と同じ層別（domain 95%+ / app 90%+ / infra 70%+ / 全体 85%+）、SDK 部分は Port 抽象 + fake、実 API は Build and Test

回答確定済み。`U-NTF-design.md`（合本版）を生成する。
