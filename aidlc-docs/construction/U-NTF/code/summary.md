# U-NTF Code Generation — Summary

**Unit**: U-NTF（Notifier Unit、Sprint 4）
**対象**: `cmd/notifier`（Cloud Run Service、Pub/Sub Push Subscription target）
**対応する計画**: [`U-NTF-code-generation-plan.md`](../../plans/U-NTF-code-generation-plan.md)
**上位設計**: [`U-NTF/design/U-NTF-design.md`](../design/U-NTF-design.md)、[`U-NTF/infrastructure-design/`](../infrastructure-design/)

---

## 1. 生成ファイル一覧

### Domain（`internal/notification/domain/`）

| ファイル | 役割 |
|---|---|
| `user_profile.go` | `UserProfile` / `NotificationPreference` + `WantsInfoType` / `IsDeliverable` |
| `batch_result.go` | `BatchResult` / `FCMMessage` |
| `event.go` | `NewArrivalEvent`（Pub/Sub message の domain 表現） |
| `ports.go` | 4 Port (`Dedup` / `UserRepository` / `FCMClient` / `EventDecoder`) |

### Application（`internal/notification/application/`）

| ファイル | 役割 |
|---|---|
| `result.go` | `DeliverOutcome` enum + `DeliverResult` |
| `deliver_usecase.go` | `DeliverNotificationUseCase.Execute` — dedup → resolve → 並列 send + cleanup |
| `fake_test.go` | 4 Port の fake 実装 |
| `deliver_usecase_test.go` | 6 シナリオ（Delivered / Deduped / NoSubscribers / PartialFailure / DedupError / ResolveError） |

### Interfaces（`internal/interfaces/job/`）

| ファイル | 役割 |
|---|---|
| `notifier_runner.go` | `NotifierHandler.Push` + `.Health`、Q6 status code 戦略実装 |
| `notifier_runner_test.go` | 7 シナリオ（200×3 + 400 + 500 + MethodNotAllowed + Health） |

### Infrastructure（`internal/notification/infrastructure/`）

| パッケージ | 役割 |
|---|---|
| `dedup/` | `FirestoreDedup.CheckAndMark` with `RunTransaction` + TTL 書込 |
| `userrepo/` | `FirestoreUserRepository.FindSubscribers` + `.RemoveInvalidTokens` |
| `fcm/` | `FirebaseFCM.SendMulticast` + 失敗 token 分類 (`Invalid` vs `Transient`) |
| `eventdecoder/` | `PubSubEnvelopeDecoder.Decode` — base64 + JSON parse + attr fallback |

### Platform 拡張

| ファイル | 内容 |
|---|---|
| `platform/firebasex/app.go` | 本実装化（firebase.NewApp、Firestore/Messaging クライアント lazy init、Close） |

### Composition Root

| ファイル | 内容 |
|---|---|
| `cmd/notifier/main.go` | envconfig + DI 配線 + `run()` pattern + graceful shutdown（SIGTERM drain） |

### Terraform

| ファイル | 変更 |
|---|---|
| `terraform/modules/shared/firestore.tf` | `google_firestore_field "notifier_dedup_ttl"` + `google_firestore_index "users_notification"` 新規 |

---

## 2. NFR-NTF-* カバレッジ

| NFR ID | 要件 | 実装 |
|---|---|---|
| NFR-NTF-PERF-01 | p95 < 3s | dedup (Firestore tx ~100ms) + users query (~200ms) + 並列 FCM (~1s) + cleanup |
| NFR-NTF-PERF-02 | ack_deadline 60s 内 | concurrency=5 で 100+ 購読者でも 2-3s |
| NFR-NTF-SEC-01 | Pub/Sub agent のみ invoker | Terraform IAM 既存 |
| NFR-NTF-SEC-02 | ADC 認証 | firebasex.NewApp (ServiceAccountJSON=nil でADC) |
| NFR-NTF-SEC-03 | 最小権限 SA | Runtime SA に datastore.user + cloudmessaging.messagesSender のみ |
| NFR-NTF-REL-01 | dedup | `FirestoreDedup.CheckAndMark` (transactional) |
| NFR-NTF-REL-02 | Pub/Sub retry + DLQ | Terraform subscription 既存 |
| NFR-NTF-REL-03 | 無効 token 即時除去 | `users.RemoveInvalidTokens` in-request |
| NFR-NTF-OPS-01 | 構造化ログ | slog `app.notifier.phase` / `key_cd` / `uid` |
| NFR-NTF-OPS-02 | OTel Metric | 6 種 (`received` / `deduped` / `recipients` / `fcm.sent` / `token_invalidated` / `duration`) |
| NFR-NTF-OPS-03 | HTTP status 別監視 | handler が status を明示、Cloud Monitoring で集計可 |
| NFR-NTF-TEST-* | 層別カバレッジ | §3 参照 |
| NFR-NTF-EXT-01 | 他 channel 拡張 | `FCMClient` port 経由 |

---

## 3. テストカバレッジ実績

**方針**: U-NTF Design Q8 [A] — Firestore / FCM / Firebase SDK 依存部分は **Build and Test で実クライアント検証**、Application 層は fake Port で完全網羅。

| パッケージ | 実績 | 目標 | 判定 |
|---|---|---|---|
| `notification/domain` | **100.0%** | 95%+ | ✓ |
| `notification/application` | **92.8%** | 90%+ | ✓ |
| `notification/infrastructure/eventdecoder` | **96.4%** | 70%+ | ✓ |
| `interfaces/job` | **89.3%** | 70%+ | ✓ |
| `notification/infrastructure/dedup` | **27.8%** | (Build & Test) | 設計通り、SDK 依存 |
| `notification/infrastructure/userrepo` | **9.7%** | (Build & Test) | 設計通り、SDK 依存 |
| `notification/infrastructure/fcm` | **5.9%** | (Build & Test) | 設計通り、SDK 依存 |
| `platform/firebasex` | **7.1%** | 50%+ | 設計通り、smoke test のみ |
| **架構上テスト可能な部分** | **>90%** | 85%+ | ✓ |

SDK 依存パッケージ 4 種は smoke test + Build and Test で完結という U-NTF Design Q8 [A] の方針通り。

---

## 4. 設計のキモ

### Q2 + Q5 で **誤通知 0 件 + stale token 自動除去**

1. Pub/Sub から message 受信
2. `FirestoreDedup.CheckAndMark` で 24h 以内の重複を排除（`alreadySeen=true` なら 200 OK で早期 return）
3. `users.FindSubscribers` で購読者を Firestore query 1 回 + in-memory filter
4. 並列度 5 で `SendEachForMulticast` 実行
5. `BatchResponse.Responses[i]` で invalid 判定された token は **同 request 内で `ArrayRemove`**

### Q6 HTTP Status Code 戦略

| シナリオ | Status | Pub/Sub の挙動 |
|---|---|---|
| Delivered / Deduped / NoSubscribers | 200 | ACK |
| Malformed envelope / missing key_cd | 400 | 即 DLQ |
| Firestore / FCM transient | 500 | retry → DLQ |

### Composition Root の `run()` pattern + graceful shutdown

- `main` は `run()` の戻り値で exit code 決定 → defer（Firebase Close、HTTP Shutdown、OTel flush）が必ず実行
- `signal.NotifyContext` + `srv.Shutdown(shutdownCtx)` で SIGTERM 時に in-flight request を drain（デフォルト 10s）

---

## 5. U-APP への申し送り

次 Unit（U-APP = Flutter）で本 Unit の FCM 通知を受信する時の要点:

1. **FCM Token 登録**: `FirebaseMessaging.instance.getToken()` → `UserProfileService.RegisterFcmToken()` RPC で Firestore `users/{uid}.fcm_tokens` に追加（U-BFF 経由）
2. **通知ペイロード**: `Notification.title` / `Notification.body` は OS が自動表示、`data.key_cd` / `data.country_cd` / `data.info_type` は tap 時に取得
3. **Tap handling**: `key_cd` を受けて `GetSafetyIncident(key_cd)` を U-BFF に問い合わせて詳細画面へ遷移
4. **Token rotation**: アプリ起動時に `getToken()` 呼び出し、変化していたら U-BFF に再登録。無効化された token は U-NTF が Firestore から自動除去する
5. **通知設定の編集 UI**: `UserProfileService.UpdateNotificationPreference` で `enabled` / `target_country_cds` / `info_types` を書き込み

---

## 6. 実行確認（Build and Test で実施する項目）

本 PR の範囲外。Build and Test ランブックで以下を確認:

- Pub/Sub に手動で message publish → notifier が受信 → 200 応答
- Firestore `notifier_dedup/{key_cd}` に `expireAt` フィールド付きの doc が作られる
- 同じ key_cd で再 publish → `deduped` outcome で 200 返却
- 実機で FCM 通知を受信
- 無効 token（古い simulator のトークン等）を `users.fcm_tokens` に仕込んで Pub/Sub → `ArrayRemove` 実行確認
- DLQ が `max_delivery_attempts=5` で機能

---

## 7. 将来の拡張ポイント

- **他通知 channel**: Slack / Email など `FCMClient` と同型の Port を追加 → UseCase は port を 1 個増やすだけ
- **Dedup ストア差し替え**: Redis / Memcached など `Dedup` port の別実装
- **地理的フィルタ**: Q3 で C として却下したが、ユーザー位置 + 半径 X km 以内の通知が必要なら Firestore geohash + where で実装可能
