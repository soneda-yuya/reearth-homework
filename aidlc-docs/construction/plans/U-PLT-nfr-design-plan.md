# U-PLT NFR Design Plan (Minimal)

## Overview

NFR Requirements で確定した性能・信頼性・セキュリティ・観測性要件を **どのパターン／論理コンポーネントで実現するか** を決めます。U-PLT は基盤 Unit のため、ここで決めたパターンは **他 Unit がすべて踏襲** する共通規約となります。

5 項目に絞って確定します。推奨 A で進めば最短です。

---

## Step-by-step Checklist

- [ ] Q1〜Q5 すべて回答
- [ ] 矛盾・曖昧さの検証
- [ ] 成果物 2 点を生成:
  - [ ] `construction/U-PLT/nfr-design/nfr-design-patterns.md`
  - [ ] `construction/U-PLT/nfr-design/logical-components.md`
- [ ] 承認後、U-PLT Infrastructure Design へ進む

---

## Context Summary

- **所属 Unit**: U-PLT
- **参照 NFR**: NFR-PLT-REL-01〜03（信頼性）/ NFR-PLT-PERF-01〜04（性能）/ NFR-PLT-SEC-01〜04（セキュリティ）/ NFR-PLT-OBS-01〜04（観測性）
- **Deployable 形態**: 全サービス Cloud Run（Job または Service）
- **決定済み**: slog + OTel / envconfig + Secret Manager / Connect / Pub/Sub / Firebase Admin

---

## Questions

### Question 1 — パニック復帰 + グレースフルシャットダウン

`interfaces/rpc` と `interfaces/job` での Panic 復帰 / SIGTERM 対応のパターンは？

A) **推奨**: Connect Interceptor と Job Runner の両方で `defer recover()` を実装、`errs.KindInternal` にラップして上位に返却。SIGTERM / SIGINT 受信時は `context.WithCancel` でキャンセル伝播 → 進行中ハンドラは **最大 10 秒** で完了待ち → `observability.shutdown(ctx)` → `os.Exit(0)`。
B) Panic はプロセスを落とす（Cloud Run が再起動）、Graceful shutdown なし
C) Panic 復帰のみ実装、Graceful shutdown は未対応
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 2 — 外部 SDK の Retry / Backoff

Mapbox / Claude / reearth-cms / Firebase への呼び出しでの再試行ポリシー。

A) **推奨**: 各 Adapter（`platform/mapboxx` など）内で **指数バックオフ**（初期 500ms、最大 8 秒、最大 3 回）+ **ジッター**。冪等な GET / HEAD / LLM 推論のみ再試行、POST / PATCH は再試行しない（冪等キーを持つ場合のみ再試行可）。共通ヘルパを `platform/retry` に配置し、各 Adapter で利用。
B) `github.com/cenkalti/backoff/v4` のようなライブラリに寄せる
C) 再試行は行わない（失敗即エラー）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 3 — ヘルスチェック（Cloud Run / Pub/Sub push）

BFF / Notifier（Cloud Run Service）のヘルスチェック。

A) **推奨**: `platform/connectserver` が自動で `/healthz`（liveness、常に 200）と `/readyz`（readiness、依存先疎通チェック — CMS / Firebase / Secret Manager の ping）を提供。Cloud Run は `/healthz` を使う。起動時 `/readyz` は Secret 取得完了までは 503。
B) `/healthz` のみ（readiness なし、Cloud Run のデフォルト挙動に委ねる）
C) ヘルスチェック未実装（MVP として割愛）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 4 — 外部 API のレート制限保護（Server-side）

Claude / Mapbox / CMS への呼び出しをレート制限する仕組み（自分たちの使いすぎ防止）。

A) **推奨**: 各 Adapter で **`golang.org/x/time/rate.Limiter`** による **トークンバケット** を実装。Claude = 10 RPM、Mapbox = 600 RPM、CMS = 60 RPM（Infrastructure Design で env で調整可能に）。超過時は `errs.KindExternal` で即エラー（ブロック待機しない）。
B) レート制限なし（外部サービスのレート制限を信じる）
C) Circuit Breaker（`sony/gobreaker` 等）を先に導入
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 5 — SDK クライアントのライフサイクル

Firebase App / Pub/Sub Client / Mapbox Client / CMS Client のインスタンス化方針。

A) **推奨**: 各 Client は **プロセス単位でシングルトン**（`main` で 1 回生成 → DI）。HTTP / gRPC のコネクションプールは SDK に任せる。ローテーション不要な前提。
B) リクエスト毎に生成（Firebase / Pub/Sub のコールドスタート許容）
C) `sync.Pool` で再利用
X) Other（[Answer]: の後ろに自由記述）

[A]: 

---

## 承認前の最終確認（回答後に AI が埋めます）

- Panic / Graceful shutdown: _TBD_
- Retry / Backoff: _TBD_
- Health check: _TBD_
- Rate limit: _TBD_
- SDK ライフサイクル: _TBD_

回答完了後、矛盾・曖昧さがなければ `nfr-design-patterns.md` と `logical-components.md` の 2 成果物を生成します。
