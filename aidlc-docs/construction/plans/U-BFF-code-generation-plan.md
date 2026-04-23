# U-BFF Code Generation Plan

## Overview

U-BFF（Connect Server / BFF Unit、Sprint 3、**実装順で最後のバックエンド Unit**）の Code Generation 計画。[`U-BFF/design/U-BFF-design.md`](../U-BFF/design/U-BFF-design.md) と [`U-BFF/infrastructure-design/`](../U-BFF/infrastructure-design/) に基づいて実装する。

## Goals

- Flutter モバイルアプリが叩く Connect サーバ `cmd/bff` を完成させる
- 11 RPC を 3 Service で提供（SafetyIncidentService / CrimeMapService / UserProfileService）
- Firebase ID Token を AuthInterceptor で検証、全 RPC 認証必須
- `errs.Kind` → `connect.Code` 自動変換、prod ではメッセージマスク
- U-PLT 共通規約 + U-ING の `cmsx` + `firebasex` + `connectserver` を完全に流用
- 実 API 疎通は Build and Test で手動、Code Gen 完了時点で envconfig が揃えば起動する

## Non-Goals

- Flutter 側実装（U-APP の責務、別レポ）
- FCM 配信（U-NTF の責務）
- CMS への write（U-ING / U-CSS の責務、BFF は readonly）
- 複雑な authZ ルール（全 RPC で uid があれば通す MVP 仕様）
- GeoJSON 巨大データのストリーミング（1 回読み切り、limit 上限）

---

## Context — 既に存在するもの

- `proto/v1/safetymap.proto`: 3 Service / 11 RPC の定義済み
- `cmd/bff/main.go`: skeleton (observability setup + connectserver.Start、handler は TODO)
- `internal/platform/cmsx`: Client + FindItemByFieldValue + 書き込み系（U-CSS/U-ING 由来）。**Read 系 ListItems / Search / ListNearby は未実装、本 Unit で追加**
- `internal/platform/firebasex`: App + Firestore() + Messaging()（U-NTF 由来）。**Auth() accessor は未実装、本 Unit で追加**
- `internal/platform/connectserver`: New(cfg, handlers, probers) + Start(ctx) + HandlerRegistration + Prober
- `internal/safetyincident/domain`: `SafetyIncident` / `Point` / `GeocodeSource` / write 系 Port（U-ING 由来）、**read port は未実装、本 Unit で追加**
- `internal/platform/errs`: `AppError` + `KindOf` + Wrap / Kinds（U-PLT 由来）
- `internal/platform/observability`: Trace / Metric / Logger / Interceptor（U-PLT 由来）
- `gen/go/v1/*.pb.go` / `safetymap_connect.gen.go`: **未生成、本 Unit で buf generate して commit**

---

## Step-by-step Checklist

### Phase 1: Proto 生成

- [ ] `buf generate` 実行し `gen/go/v1/{common,safetymap,pubsub}.pb.go` + `safetymap_connect/safetymap.connect.go` を生成
- [ ] `.gitignore` の `gen/**/*.pb.go.tmp` は残しつつ、`gen/` を commit

### Phase 2: Domain Layer

- [ ] `internal/safetyincident/domain/read_ports.go` 新規
  - `SafetyIncidentReader` interface: `List / Get / Search / ListNearby`
  - `ListFilter` / `SearchFilter`（area_cd / country_cd / info_types / from / to / limit / cursor）
  - `List` / `Search` は `(items, nextCursor, err)`、`ListNearby` は top-N で `([]SafetyIncident, error)`
- [ ] `internal/safetyincident/crimemap/domain/` 新規
  - `crimemap.go`: `CountryChoropleth` / `HeatmapPoint` / `ChoroplethResult` / `HeatmapResult` / `CrimeMapFilter`
  - `color.go`: `ColorFromCount(count, max int) string`（5 段階グラデーション）
  - `color_test.go`: table-driven、境界値検証
- [ ] `internal/user/domain/` 新規
  - `profile.go`: `UserProfile` / `NotificationPreference`（struct shape は U-NTF `notification/domain/user_profile.go` と一致、tag も同じ）
  - `ports.go`: `ProfileRepository` / `AuthVerifier`
  - `profile_test.go`: VO validate（uid 非空、preference fields）
- [ ] `internal/shared/authctx/` 新規
  - `context.go`: `WithUID` / `UIDFrom`（ctx 埋め込み）
  - `context_test.go`

### Phase 3: Application Layer

- [ ] `internal/safetyincident/application/` に read 系 UseCase 追加
  - `list_usecase.go`: `ListUseCase.Execute(ctx, filter) ([]SafetyIncident, nextCursor, error)`
  - `get_usecase.go`
  - `search_usecase.go`
  - `nearby_usecase.go`
  - `geojson_usecase.go`: List 結果を GeoJSON FeatureCollection に変換（`type=FeatureCollection`、`features[]`、`geometry.type="Point"`）
- [ ] `internal/safetyincident/crimemap/application/aggregator.go`: `Aggregator.Choropleth / Heatmap`（in-memory 集計、centroid fallback 除外）
- [ ] `internal/user/application/` 新規
  - `get_profile.go`: `GetProfileUseCase.Execute(ctx, uid) (*UserProfile, error)`、初回は `CreateIfMissing`
  - `toggle_favorite.go`
  - `update_preference.go`
  - `register_fcm_token.go`
- [ ] 各 `*_test.go`: Port fake + table-driven（正常 / 読取エラー / domain エラー）

### Phase 4: Infrastructure

- [ ] `internal/platform/cmsx` 拡張
  - `item.go`: `ListItems(ctx, modelID, query ListItemsQuery) ([]ItemDTO, nextCursor, error)` 追加
    - query = filter + cursor + limit
    - CMS API の `?filter=...&cursor=...&limit=...` クエリを URL.Values.Encode() で組み立て
  - `item_test.go`: httptest.Server で挙動検証
- [ ] `internal/platform/firebasex/app.go` 拡張
  - `Auth(ctx)` accessor を sync.Once 化（既存の Firestore / Messaging と同じパターン）
- [ ] `internal/safetyincident/infrastructure/cms/reader.go` 新規
  - `CMSReader` struct（cmsx.Client + projectAlias + modelAlias）
  - `List / Get / Search / ListNearby` 実装
  - `ItemDTO → domain.SafetyIncident` マッパー（U-ING の `toFields` の逆、19 フィールド）
  - `reader_test.go`: `cmsx.Client` を stub 化して検証
- [ ] `internal/user/infrastructure/firestore/profile_repo.go` 新規
  - `FirestoreProfileRepository` struct
  - `Get / CreateIfMissing / ToggleFavoriteCountry / UpdateNotificationPreference / RegisterFcmToken`
  - Firestore document-id 直接アクセス、`ArrayUnion` / `ArrayRemove` 使用
  - `profile_repo_test.go`: Firestore stub or fake
- [ ] `internal/user/infrastructure/firebaseauth/verifier.go` 新規
  - `FirebaseAuthVerifier` struct（firebasex.App の Auth client を保持）
  - `Verify(ctx, idToken) (uid, error)`、期限切れ/署名不正は `KindUnauthorized`
  - `verifier_test.go`: Firebase auth client stub

### Phase 5: Interfaces — RPC Handlers + Interceptors

- [ ] `internal/interfaces/rpc/auth_interceptor.go` 新規
  - `NewAuthInterceptor(verifier, logger) connect.UnaryInterceptorFunc`
  - `Authorization: Bearer <idToken>` ヘッダ読み、verifier.Verify、uid を ctx 埋込
  - missing token → `errs.Wrap(..., KindUnauthorized, errors.New("missing bearer token"))`
- [ ] `internal/interfaces/rpc/error_interceptor.go` 新規
  - `NewErrorInterceptor(env) connect.UnaryInterceptorFunc`
  - `errs.KindOf(err)` → `connect.Code`（mapping table）
  - prod かつ `CodeInternal / CodeUnavailable` のときメッセージマスク（`"internal server error"`）
- [ ] `internal/interfaces/rpc/safety_incident_server.go` 新規
  - `SafetyIncidentServer`（5 UseCase 保持）
  - 5 RPC 実装、proto ⇄ domain 変換
- [ ] `internal/interfaces/rpc/crimemap_server.go` 新規（2 RPC）
- [ ] `internal/interfaces/rpc/user_profile_server.go` 新規（4 RPC）
  - 各 RPC で `authctx.UIDFrom(ctx)` で uid 取り出し
- [ ] 各 `*_test.go`: `connecttest.Server` で handler e2e（AuthInterceptor + ErrorInterceptor 込み）

### Phase 6: Composition Root

- [ ] `cmd/bff/main.go` 本実装
  - `bffConfig` 拡張: `FirebaseProjectID`（= `PLATFORM_GCP_PROJECT_ID` を流用）、`CMSBaseURL / WorkspaceID / IntegrationToken`、tuning envconfig default（`RequestBodyLimitBytes=1MiB`、`FCMTokenMax=10`、`UsersCollection="users"`、`ShutdownGraceSeconds=10`）
  - `run()` pattern + defer graceful shutdown
  - Observability setup → cmsx.Client → firebasex.App → Firestore/Auth client → Reader/Repo/Verifier → UseCase → Server → HandlerRegistration → connectserver.Start
  - Prober 登録（CMS ping + Firestore ping）

### Phase 7: Terraform

- [ ] **変更なし**（U-BFF Infrastructure Design の結論どおり）
- [ ] 念のため `terraform fmt` / `terraform validate` 実行して緑確認

### Phase 8: Docs

- [ ] `aidlc-docs/construction/U-BFF/code/summary.md` 新規
- [ ] `README.md` に bff セクション追記（env 一覧、起動方法、疎通確認手順）
- [ ] `aidlc-docs/aidlc-state.md` 更新

### Phase 9: CI / Verification

- [ ] `buf format` / `buf lint` 緑
- [ ] `go test ./... -race` 全緑（BFF 追加分のカバレッジ 85%+）
- [ ] `go vet` / `gofmt -s` / `golangci-lint` 全緑
- [ ] `govulncheck` 全緑
- [ ] Docker build `bff` 緑

---

## 設計上の要判断事項

### Question A — PR 分割

Design Q9 [A] で「2 PR（U-ING パターン）」が確定済み。本計画ではどこで切るか決める。

**選択肢**:

A) **推奨**: **PR A = Phase 1-5 (proto + domain/app/infra/interfaces)、PR B = Phase 6-9 (main + docs + CI)**
  - U-ING と同じ境界（実装層 vs 結線層）
  - PR A で Go コードの型整合 + テストを先に担保、PR B で main.go の配線と Docker build を確認
  - レビュー観点を分けやすい

B) PR A = Phase 1-4 (proto + domain/app/infra)、PR B = Phase 5-9 (interfaces + main + docs + CI)
  - Interfaces Layer を後ろに寄せる
  - PR A が Go コード型安定、PR B で Connect + main の結線
  - ⚠️ PR A 単体でテストが回る範囲が狭くなる（interceptor / server が無い）

C) 1 PR にまとめる
  - ⚠️ Design Q9 [A] に矛盾、却下

[Answer]: A

### Question B — `UserProfile` struct の共通化

U-NTF と U-BFF で `UserProfile` / `NotificationPreference` の struct shape が完全一致する（Q4 Design [A] で shared collection）。コード上の struct は:

- **(a)** 別 BC でそれぞれ定義（`internal/notification/domain` と `internal/user/domain` に同じ struct を書く）
- **(b)** 共通の `internal/user/domain` に一元化し、U-NTF 側からも import

**選択肢**:

A) **推奨**: **(a) 別 BC でそれぞれ定義**
  - DDD の Bounded Context 原則に忠実（各 BC が自分の domain 型を持つ）
  - shape 変更時は両方を同期する運用（proto 共通 + struct tag 共通で整合性担保）
  - BC 同士の依存方向が綺麗（user ↔ notification が循環しない）

B) (b) `internal/user/domain` に一元化、U-NTF が import
  - 重複コード削減
  - ⚠️ notification BC が user BC に依存する形になり、Deployable 間の境界が曖昧
  - ⚠️ U-NTF の既存コードを書き換える必要（既存 PR が merged 済み、差分が散る）

C) 両方を `internal/shared/userprofile` にリファクタ
  - ⚠️ user BC の所有権が曖昧になる（BFF が owner のはず）
  - ⚠️ shared BC を作る justifications 薄い

[Answer]: A

### Question C — `FirebaseAuthVerifier` の local token 検証 vs Admin SDK オンライン検証

Firebase Admin SDK の `auth.VerifyIDToken` は **公開鍵を Google からキャッシュ取得してローカルで JWT 検証** する方式（revocation は `VerifyIDTokenAndCheckRevoked` を使えばオンライン追加検証可能）。

**選択肢**:

A) **推奨**: **`VerifyIDToken` のみ使用**（ローカル検証、リフレッシュは SDK 任せ）
  - ✅ p95 < 500ms の SLO を満たしやすい（revocation check は Firebase Auth API へのラウンドトリップが発生）
  - ✅ SDK が公開鍵を自動キャッシュ（内部で時間ベース invalidate）
  - ⚠️ アカウント BAN 直後〜数時間は有効な token が通る可能性（MVP 仕様では許容）

B) `VerifyIDTokenAndCheckRevoked` を使用
  - ✅ revoke 即座反映
  - ⚠️ 毎 RPC で Firebase Auth API へラウンドトリップ（+50-100ms × rps、コストと SLO に影響）
  - ⚠️ MVP としては過剰

C) ハイブリッド（機微 RPC だけ AndCheckRevoked、読取は Verify のみ）
  - ⚠️ MVP では BFF の全 RPC は read or プロファイル更新のみ、機密度はほぼ同じ
  - ⚠️ RPC ごとに interceptor が分岐するのは運用が複雑

[Answer]: A

### Question D — `cmsx` Read API の実装範囲

現状 `cmsx.Client` には:
- `FindItemByFieldValue(ctx, modelID, fieldKey, value) (*ItemDTO, error)`（= Get に流用可）
- `CreateItem / UpdateItem / UpsertItemByFieldValue`（write 系）

U-BFF が必要な新規 read API:
- **`ListItems(ctx, modelID, query) ([]ItemDTO, nextCursor, error)`**: filter + cursor + limit
- **`SearchItems(ctx, modelID, query) (...)`**: keyword 検索（reearth-cms の検索 API 仕様に合わせる）
- **`ListNearby(ctx, modelID, center, radiusKm, limit)` 相当**: reearth-cms は空間フィルタを持たない可能性が高く、**BFF 側で広めに ListItems して距離計算する** 方式が現実的

**選択肢**:

A) **推奨**: **`ListItems` + `SearchItems` を cmsx に追加、`ListNearby` は BFF 側（CMSReader）で ListItems → 距離フィルタ**
  - cmsx は汎用 HTTP ラッパに徹する（domain 知識を持ち込まない）
  - BFF 側で ListNearby の「広めに fetch → Haversine 距離計算 → limit で打ち切り」を実装
  - ⚠️ 大量データ時の効率は悪いが MVP として許容（実データは国別 centroid fallback が多数を占めるため、実使用で数百件程度）

B) reearth-cms 側に空間フィルタを追加 → cmsx も対応
  - ⚠️ CMS 側の変更が要り、スコープ肥大

C) `ListNearby` は当面未実装にして proto から削除
  - ⚠️ Design の 11 RPC 仕様を満たさない

[Answer]: A

### Question E — カバレッジ目標

**選択肢**:

A) **推奨**: U-CSS / U-ING / U-NTF と同じ **層別**（domain 95 / app 90 / infra 70 / interceptor 90 / rpc 80 / 全体 85）
  - 既存 Unit と一貫性
  - interceptor は 90 基準（ロジック集中、fake で外部依存切れる）
  - rpc は 80（`connecttest.Server` で e2e 網羅するが proto → domain 変換は分岐多め）

B) 一律 85
C) 数値目標なし、質的カバレッジ

[Answer]: A

### Question F — RPC Handler テスト方式

**選択肢**:

A) **推奨**: **`connecttest.Server` で e2e**（AuthInterceptor + ErrorInterceptor 込み、ユースケース fake）
  - ✅ Design Q8 [A] どおり
  - ✅ Interceptor の結線バグが検出される
  - ✅ proto ⇄ domain 変換の往復が単一テストで確認できる

B) Server struct を直接インスタンス化してメソッド呼び出し
  - ⚠️ Interceptor を通らず、`connect.Code` 変換の確認が別テストに分散
  - ⚠️ header-based auth の挙動が確認できない

C) 実 Firebase Auth emulator / 実 Firestore
  - ⚠️ Code Gen の範囲外（Build and Test で実施）

[Answer]: A

---

## 承認前の最終確認（回答確定）

- **Q A [A]**: PR 分割 = **PR A (Phase 1-5: proto + domain/app/infra/interfaces) + PR B (Phase 6-9: main + docs + CI)**
- **Q B [A]**: `UserProfile` struct 共通化 = **別 BC でそれぞれ定義**（DDD 原則、proto 共通 + struct tag 共通で整合性担保）
- **Q C [A]**: Firebase Auth 検証 = **`VerifyIDToken` のみ**（ローカル検証、p95 優先、revocation 即時反映は諦める）
- **Q D [A]**: cmsx Read API 範囲 = **`ListItems` + `SearchItems` を `cmsx` に追加、`ListNearby` は BFF で ListItems → Haversine 距離フィルタ**
- **Q E [A]**: カバレッジ = **層別**（domain 95 / app 90 / infra 70 / interceptor 90 / rpc 80 / 全体 85）
- **Q F [A]**: RPC Handler テスト = **`connecttest.Server` で e2e**（Interceptor 込み、UseCase fake）

回答確定済み。Phase 1-9 を PR A (Phase 1-5) / PR B (Phase 6-9) に分けて順次実装する。
