# U-BFF Design Plan (Minimal 合本版)

## Overview

U-BFF（Connect Server / BFF Unit、Sprint 3）は **Flutter アプリが叩く唯一の Connect RPC サーバ** を提供する **Cloud Run Service**。認証（Firebase ID Token 検証）と、SafetyIncident の読み取り系 RPC + CrimeMap 集計 + UserProfile CRUD をすべてこの Unit が担当。

ワークフロー圧縮 Option B に従い、**Functional Design + NFR Requirements + NFR Design** を 1 ドキュメントに合本します。

## Context（確定済み）

- **Bounded Context**: `safetyincident`（読み取り系）+ `safetyincident.crimemap`（subdomain）+ `user`
- **Deployable**: `cmd/bff`（Cloud Run Service、Connect HTTP/gRPC 両対応）
- **責務**:
  - **SafetyIncidentService**: `ListSafetyIncidents` / `GetSafetyIncident` / `SearchSafetyIncidents` / `ListNearby` / `GetSafetyIncidentsAsGeoJSON`
  - **CrimeMapService**: `GetChoropleth` / `GetHeatmap`（犯罪マップ集計）
  - **UserProfileService**: `GetProfile` / `ToggleFavoriteCountry` / `UpdateNotificationPreference` / `RegisterFcmToken`
  - Firebase ID Token 検証（AuthInterceptor）
- **依存**: U-PLT（基盤）、U-CSS（CMS スキーマ前提）、U-ING（実データが CMS に蓄積されている前提）、U-NTF（UserProfile の `fcm_tokens` / `notification_preference` を共同利用）

U-PLT 共通規約（slog + OTel / envconfig + Secret Manager / `errs.Wrap` / retry / rate limit / Clock / `connectserver` / terraform module 構成 / CI / Dockerfile）は **全てそのまま踏襲**。U-CSS の `cmsx` + U-NTF の `firebasex` を読み取り側にも活用。

### 既存の proto 定義（再実装不要）

`proto/v1/safetymap.proto` で全 service + request/response message が既に定義済み（U-PLT で承認）:
- `SafetyIncidentService`（5 RPC）
- `CrimeMapService`（2 RPC）
- `UserProfileService`（4 RPC）
- 対応する Go コードは `gen/go/v1/*` に buf generate 済み

U-BFF で実装するのは **サーバ側ハンドラ + application 層 + 認証 interceptor + UserProfile 永続層**。

### 既存の Terraform 雛形

`terraform/modules/bff/` で Cloud Run Service + Runtime SA + IAM（`datastore.user` / `secretmanager.secretAccessor`）+ env 一部が U-PLT で整備済み。

---

## Step-by-step Checklist

- [ ] Q1〜Q9 すべて回答
- [ ] 矛盾・曖昧さの検証、必要なら clarification
- [ ] 成果物を生成:
  - [ ] `construction/U-BFF/design/U-BFF-design.md` — Functional + NFR Req + NFR Design 合本
- [ ] 承認後、U-BFF Infrastructure Design へ進む

---

## Questions

### Question 1 — 認証方式（Firebase ID Token の検証）

`UserProfileService.*` と CMS を叩く API は認証必須。匿名ユーザーへの公開範囲は?

A) **推奨**: **全 RPC で Firebase ID Token 必須**（Anonymous Auth も Firebase が発行するので「ログインしていれば誰でも」）
  - Flutter アプリ起動時に `FirebaseAuth.signInAnonymously()` で強制ログイン
  - `AuthInterceptor` が全 RPC の metadata から `Authorization: Bearer <id_token>` を検証
  - 未認証は Connect/gRPC の `unauthenticated` で 401
  - ✅ 単純、MVP では十分
  - ⚠️ Anonymous でも token 検証が発生する分のオーバーヘッド（~数 ms）

B) 読み取り系（SafetyIncident / CrimeMap）は **匿名許可**、UserProfile のみ認証必須
  - ✅ 未ログインでも地図が見られる（UX が柔らかい）
  - ⚠️ Interceptor のパス判定ロジックが複雑化、一部 endpoint だけ認証スキップ

C) 読み取り系も UserProfile も全匿名
  - ⚠️ UserProfile が uid ベースなので匿名だと Firestore doc を引けない
  - 採用しない

[A]: 

### Question 2 — SafetyIncident 読み取り系の実装（CMS 直アクセス vs キャッシュ）

Flutter アプリが地図を開くたびに `ListSafetyIncidents` を呼ぶ想定。CMS Item は U-ING が書いた最新データ:

A) **推奨**: **毎回 `cmsx.Client` 経由で CMS を叩く**（キャッシュなし）
  - ✅ 実装シンプル、reearth-cms が正式なデータソース
  - ✅ MVP の read トラフィック（数十〜数百 req/day）なら CMS の負荷問題なし
  - ⚠️ CMS が落ちたら BFF も応答できなくなる（MVP では許容）

B) `sync.Map` で in-memory キャッシュ、TTL 1 分
  - ✅ CMS への負荷削減
  - ⚠️ Cloud Run max_instance=N のたびに N コピーのキャッシュが別々
  - ⚠️ ユーザーが書いたばかりの item が反映遅延（U-ING の書込から最大 1 分遅延）

C) Redis / Memcached（Cloud Memorystore）
  - ⚠️ MVP 過剰、運用コスト増
  - 採用しない

[A]: 

### Question 3 — CrimeMap 集計ロジックの実装場所

`CrimeMapService.GetChoropleth` / `GetHeatmap` は、期間・フィルタに応じて Item を集計:
- `Choropleth`: 国別 count + color（heat 強度）
- `Heatmap`: 個別 point + weight

A) **推奨**: **`internal/safetyincident/crimemap/application.Aggregator`** で集計（CMS から引いた Item 配列を in-memory で集計）
  - 集計ロジックは domain 側で明確化（SafetyIncident の `geocode_source`、`geometry` 等を使う）
  - ✅ CMS クエリ 1 回 + in-memory 集計で動く（MVP 規模なら ~1,000 件が最大）
  - ✅ Flutter 側でフィルタ切替のたびに再計算、即応答
  - ⚠️ Item 数が数万件規模になると in-memory 集計がつらい（MVP 後に Firestore / BQ 集計に切替）

B) CMS 側 aggregation API を使う
  - ⚠️ reearth-cms に集計機能があるか未確認
  - 採用しない

C) Firestore の `aggregationQuery` で count
  - ⚠️ Firestore に Item を二重持ちすることになる（現在は CMS が SoT）
  - 採用しない

[A]: 

### Question 4 — UserProfile の永続化（Firestore `users` コレクション）

`UserProfileService` が書き込む Firestore `users/{uid}` のフィールド:

```
users/{uid}:
  favorite_country_cds: [string]
  notification_preference: { enabled, target_country_cds, info_types }
  fcm_tokens: [string]
```

A) **推奨**: **U-NTF と同じ Firestore `users` collection を直接 R/W**
  - U-NTF と同じ domain（`user`）なので collection を共有するのが自然
  - U-NTF は Read + ArrayRemove、U-BFF は Create / Update / ArrayUnion / ArrayRemove
  - ✅ single source of truth、重複定義なし
  - ✅ 競合は Firestore transaction / update で serialize
  - ⚠️ 1 collection を 2 Unit が触る設計、スキーマ変更時は両方で同期必要

B) U-BFF 独自 collection（`user_profiles`）+ U-NTF は reader で join
  - ⚠️ 冗長、スキーマが 2 箇所に割れる
  - 採用しない

[A]: 

### Question 5 — FCM Token 登録の冪等性

`RegisterFcmToken(token, device_id)` が Flutter から呼ばれる。同じ device が再起動時に同じ token を送ってくる可能性:

A) **推奨**: **`ArrayUnion` で追加**（既に含まれていれば no-op、Firestore が保証）
  - ✅ 冪等、Flutter 側は呼び放題
  - ⚠️ `device_id` は将来の拡張（どの device の token かを分離管理）のため、MVP では tokens 配列のみ管理

B) `device_id` ごとに sub-collection `fcm_tokens/{device_id}` で管理
  - ✅ device 毎の状態管理が豊か
  - ⚠️ MVP 過剰、単一 array で十分

[A]: 

### Question 6 — エラーハンドリング / Connect Error Code

Connect RPC の error code マッピング:

A) **推奨**: **errs.KindOf ベースの一律マッピング**（`connectserver` の既存パターン継承）
  - `KindNotFound` → `connect.CodeNotFound`
  - `KindInvalidInput` → `connect.CodeInvalidArgument`
  - `KindUnauthorized` → `connect.CodeUnauthenticated`
  - `KindPermissionDenied` → `connect.CodePermissionDenied`
  - `KindConflict` → `connect.CodeAlreadyExists`
  - `KindExternal` → `connect.CodeUnavailable`（Flutter 側で retry）
  - `KindInternal` → `connect.CodeInternal`
  - ✅ errs.AppError のまま return すれば interceptor が自動変換
  - ✅ Flutter 側で error code でハンドリング

B) RPC ごとに個別マッピング
  - ⚠️ 冗長、DRY 違反

[A]: 

### Question 7 — OTel observability

U-ING / U-NTF と同レベル:

A) **推奨**:
  - Span: `bff.<service>.<method>` (e.g. `bff.SafetyIncidentService.ListSafetyIncidents`)
  - Metric:
    - `app.bff.rpc.requests` (Counter, attr: `service`, `method`, `status`)
    - `app.bff.rpc.duration` (Histogram ms, attr: `service`, `method`)
    - `app.bff.auth.failures` (Counter, attr: `reason`)
    - `app.bff.cms.calls` (Counter, attr: `operation`)
  - Log: `app.bff.phase` (`auth` / `handle` / `done`) + `uid` (authenticated user)

B) 全 RPC で共通 interceptor がログ + metric を吐く（メソッドごとの attribute）
  - ✅ DRY
  - A と併用可

[A]: 

### Question 8 — テスト戦略

U-CSS / U-ING / U-NTF と同じ層別 + Connect handler の e2e:

A) **推奨**:
  - **Domain** (`crimemap/domain`): 集計ロジックの unit test
  - **Application**: `ListUseCase` / `GetUseCase` / `CrimeMapAggregator` / `UserProfile*UseCase` を fake Repository で検証（5-8 シナリオ）
  - **Infrastructure**: `cmsx` は U-CSS/U-ING で既に httptest、U-BFF では read 系の追加テスト
  - **Interfaces (`rpc/`)**: Connect handler を `connecttest` or `httptest` で検証（各 RPC の success / error パス）
  - **AuthInterceptor**: Firebase ID Token 検証の stub VerifierIDToken でテスト
  - カバレッジ目標: domain 95% / app 90% / infra 70% / rpc 80% / 全体 85%

B) A + e2e（実 Firebase + 実 CMS）
  - ⚠️ MVP 過剰、Build and Test で手動
  - 採用しない

[A]: 

### Question 9 — PR 分割

U-BFF は U-ING 並 (~3,000 行) の規模が見込まれる:

A) **推奨**: **2 PR 分割**（U-ING と同じ）
  - PR A: Phase 1-N（Domain + Application + Infrastructure + Interfaces handler + AuthInterceptor）
  - PR B: Composition Root + Terraform + Docs + CI

B) 1 PR
  - U-CSS と同じだが、U-BFF は規模が大きいので 2 PR が安全

C) 3 PR（SafetyIncidentService / CrimeMapService / UserProfileService で分割）
  - ⚠️ 依存関係が複雑、レビュー順序が混乱
  - 採用しない

[A]: 

---

## 承認前の最終確認（回答後に AI が埋めます）

- 認証方式: _TBD_
- CMS キャッシュ: _TBD_
- CrimeMap 集計: _TBD_
- UserProfile 永続化: _TBD_
- FCM Token 登録冪等性: _TBD_
- エラーハンドリング: _TBD_
- OTel observability: _TBD_
- テスト戦略: _TBD_
- PR 分割: _TBD_

回答完了後、矛盾・曖昧さがなければ `U-BFF-design.md`（合本版）を生成します。
