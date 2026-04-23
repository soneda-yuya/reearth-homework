# U-NTF Code Generation Plan

## Overview

U-NTF (Notifier Unit、Sprint 4) の Code Generation 計画。[`U-NTF/design/U-NTF-design.md`](../U-NTF/design/U-NTF-design.md) と [`U-NTF/infrastructure-design/`](../U-NTF/infrastructure-design/) に基づいて実装する。

## Goals

- Pub/Sub Push Subscription から U-ING の `NewArrivalEvent` を受信し、Firestore 購読者に FCM 配信する Cloud Run Service `cmd/notifier` を完成させる
- U-PLT 共通規約 + U-ING の `pubsubx` v2 + `firebasex` の本実装化を前提
- U-CSS / U-ING と同じく **実 API 疎通は Build and Test で手動**。Code Gen 完了時点で `cmd/notifier` は envconfig が揃えば起動する

## Non-Goals

- Firebase プロジェクト設定 / 実機登録 (U-APP の責務)
- `users` collection への書き込み (U-BFF の責務)
- FCM Topic messaging (Design Q4 [A] で不採用)
- Alerting Policy (MVP では未実装)

---

## Step-by-step Checklist

### Phase 1: Domain Layer

- [ ] `internal/notification/domain/` 新規
  - `user_profile.go`: `UserProfile` / `NotificationPreference`、Validate (tokens 非空、enabled 真偽値等)
  - `batch_result.go`: `BatchResult` / `FCMMessage` / `Subscriber`
  - `event.go`: `NewArrivalEvent` (Pub/Sub message の domain 表現)
  - `ports.go`: 4 Port (`Dedup` / `UserRepository` / `FCMClient` / `EventDecoder`)
  - `*_test.go`: VO 単体テスト、UserProfile.Validate table-driven

### Phase 2: Application Layer

- [ ] `internal/notification/application/`
  - `result.go`: `DeliverOutcome` enum (Delivered/Deduped/NoSubscribers) + `DeliverResult`
  - `deliver_usecase.go`: `DeliverNotificationUseCase.Execute` (dedup → resolve → 並列 send + cleanup)
  - `fake_test.go`: 4 Port の fake 実装 (in-memory)
  - `deliver_usecase_test.go`: 5 シナリオ (初回/dedup hit/購読者 0/部分失敗/transient error→error return)

### Phase 3: Interfaces — HTTP Handler

- [ ] `internal/interfaces/job/notifier_runner.go` (HTTP handler、Q6 status code 戦略)
  - POST /pubsub/push の受信
  - envelope decode → execute → status code 判定 (200/400/500)
  - `/healthz` handler (probe 用)
- [ ] `notifier_runner_test.go`: httptest で status code 分岐検証 (200×3, 400, 500)

### Phase 4: Infrastructure Adapters

- [ ] `internal/notification/infrastructure/dedup/firestore.go` (RunTransaction で CheckAndMark)
- [ ] `internal/notification/infrastructure/userrepo/firestore.go` (Where + array-contains + in-memory info_types filter + ArrayRemove)
- [ ] `internal/notification/infrastructure/fcm/firebase.go` (Firebase Admin SDK SendEachForMulticast + 失敗 token 分類)
- [ ] `internal/notification/infrastructure/eventdecoder/pubsub_envelope.go` (base64 decode + JSON unmarshal)
- [ ] 各 `_test.go`: stub SDK client で検証 (実 Firestore / FCM は Build & Test)

### Phase 5: Platform 拡張

- [ ] `internal/platform/firebasex/client.go` 本実装化
  - Firebase App 初期化 (ADC、`firebase.NewApp`)
  - Firestore client factory
  - FCM (Messaging) client factory
  - Close 処理

### Phase 6: Composition Root

- [ ] `cmd/notifier/main.go` 拡張
  - notifierConfig (既存 env + 新規 envconfig default)
  - observability setup → firebasex init → Firestore/FCM client → 4 Adapter → UseCase → Handler
  - http.Server 起動 + graceful shutdown (SIGTERM で in-flight を drain)
  - `run()` pattern (U-CSS/U-ING と同じ defer 保証)

### Phase 7: Terraform

- [ ] `modules/shared/firestore.tf` に 2 リソース追加 (notifier_dedup TTL + users 複合 index)
- [ ] `terraform fmt` / `validate` 通す

### Phase 8: Docs

- [ ] `aidlc-docs/construction/U-NTF/code/summary.md` 新規
- [ ] `README.md` に notifier セクション追記
- [ ] `aidlc-docs/aidlc-state.md` 更新

### Phase 9: CI / Verification

- [ ] `go test ./... -race` 全緑
- [ ] `go vet` / `gofmt -s` / `golangci-lint` 全緑
- [ ] `govulncheck` 全緑
- [ ] Docker build `notifier` 緑
- [ ] カバレッジ 85%+

---

## 設計上の要判断事項

### Question A — PR 分割

U-NTF は U-ING より小さい (domain/app/infra/handler/cmd で推定 +2,000 行)。

A) **推奨**: **1 PR にまとめる** (Phase 1-9 全部)
  - U-CSS と同じパターン、規模も近い
  - レビュー 1 回で済む

B) 2 PR (Phase 1-5 Go / Phase 6-9 結線)
  - U-ING のパターン、ただし U-NTF は規模が小さいので分割の価値薄い

[A]: A

### Question B — Firebase Admin SDK の依存追加

Firebase Admin SDK (`firebase.google.com/go/v4`) を go.mod に追加する必要。

A) **推奨**: **v4 最新版** を `go get` で追加
  - 現行の Go Admin SDK の最新 major
  - Firestore + FCM 両方をカバー

B) `cloud.google.com/go/firestore` + 独自 FCM HTTP 実装
  - SDK 依存を減らす
  - ⚠️ FCM の HTTP API は独自実装のコストが大きい
  - 採用しない

[A]: A

### Question C — カバレッジ目標

A) **推奨**: U-CSS / U-ING と同じ層別 (domain 95 / app 90 / infra 70 / 全体 85)
B) 一律 85
C) 質的テスト重視、数値目標なし

[A]: A

---

## 承認前の最終確認（回答確定）

- **Q A [A]**: PR 分割 = **1 PR にまとめる** (Phase 1-9 全部、U-CSS と同規模 ~2,000 行)
- **Q B [A]**: Firebase SDK = **`firebase.google.com/go/v4` 最新版**
- **Q C [A]**: カバレッジ = U-CSS/U-ING と同じ層別 (domain 95 / app 90 / infra 70 / interfaces 70 / firebasex 50 / 全体 85)

回答確定済み。Phase 1-9 を順次実装 → 1 PR で提出する。
