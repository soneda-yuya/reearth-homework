# コンポーネント定義 — overseas-safety-map

## リポジトリと配置

| リポジトリ | 役割 | 使用技術 |
|---|---|---|
| **reearth-homework**（このリポジトリ） | Go サーバーモノレポ（ingestion / bff / notifier / setup） | Go 単一モジュール、**DDD: Bounded Context × Layered Architecture** |
| **overseas-safety-map-app**（新規作成予定） | Flutter アプリ | Dart / Flutter、Clean Architecture + MVVM、Riverpod |

> **暗黙決定（設計者注）**: Q2 で単一 Go モジュールを選択、Q7 で Pub/Sub 分離型を選択したため、通知 Cloud Function（Pub/Sub サブスクライバ）は **Go で実装し、同じ Go モノレポの `cmd/notifier` に配置** する。言語統一のメリットとドメイン／リポジトリの再利用性を優先。

## Bounded Context（バックエンド）

| Context | 種別 | 説明 |
|---|---|---|
| `safetyincident` | **Core** | MOFA 取り込み・LLM 抽出・ジオコード・CMS 永続化・読み取り/検索。サブドメイン `crimemap` を内包。 |
| `notification` | Supporting | 新着 Domain Event を受けて対象ユーザーへ FCM 配信。 |
| `user` | Supporting | Firebase Auth 検証 + Firestore 上のユーザープロファイル（お気に入り・通知設定・FCM トークン）。 |
| `cmssetup` | Supporting | reearth-cms の Project / Model / Field 冪等作成。 |

各 Context 内は `domain` / `application` / `infrastructure` の 3 レイヤで構成し、依存は内向き。詳細なディレクトリ構造は [application-design.md §6](./application-design.md#6-リポジトリとディレクトリ構造予定) 参照。

---

## コンポーネント一覧

各コンポーネントは「名前」「責務」「公開インターフェイス」の3点で簡潔に定義する。詳細なメソッドは [component-methods.md](./component-methods.md)、関係は [component-dependency.md](./component-dependency.md) を参照。

---

### サーバー側（Go モノレポ、DDD）

各コンポーネントのパッケージ名は `internal/{context}/{layer}/...` 形式で示す。

#### 🟩 Context: `safetyincident`（Core）

##### C-01: MOFA Source（Port: `safetyincident.MofaSource`、Adapter: `mofa.HttpSource`）
- **責務**: MOFA オープンデータの XML（`area/00A.xml` 初回 / `area/newarrivalA.xml` 継続）を取得し、ドメイン `MailItem` に変換する。
- **Port**: `internal/safetyincident/domain` の `MofaSource`（FetchAll / FetchNewArrivals / Parse）
- **Adapter**: `internal/safetyincident/infrastructure/mofa`（HTTP + XML パーサ）

##### C-02: Location Extractor（Port: `safetyincident.LocationExtractor`、Adapter: `llm.ClaudeExtractor`）
- **責務**: `MailItem`（title + mainText）を入力に発生地名（文字列）を抽出する。LLM 依存は Port 越しに隔離。
- **Port**: `internal/safetyincident/domain` の `LocationExtractor`
- **Adapter**: `internal/safetyincident/infrastructure/llm`（Anthropic Claude Haiku、MVP 第一候補）

##### C-03: Geocoder（Port: `safetyincident.Geocoder`、Adapter: Mapbox + Centroid + Chain）
- **責務**: 地名文字列を緯度経度に変換。プライマリ外部サービス、フォールバック国セントロイド。
- **Port**: `internal/safetyincident/domain` の `Geocoder`
- **Adapter**:
  - `internal/safetyincident/infrastructure/geocode` に `MapboxGeocoder` / `CountryCentroidFallback` / `Chain(primary, fallback)`

##### C-04: Safety Incident Repository（Port: `safetyincident.Repository`、Adapter: `cms.Repository`）
- **責務**: 安全情報の **読み書き統合 I/F**。ingestion / BFF の両方が同じ Port を利用（Q9 [B]）。MVP は reearth-cms、将来は DB 直接化へ差し替え可能（NFR-EXT-01）。
- **Port**: `internal/safetyincident/domain` の `Repository`
- **Adapter**: `internal/safetyincident/infrastructure/cms`（Integration API 経由）

##### C-05: reearth-cms Client（`platform/cmsx`）
- **責務**: reearth-cms Integration REST API の低レベルクライアント。`safetyincident.infrastructure.cms` と `cmssetup.infrastructure.cms` の双方が利用。
- **パッケージ**: `internal/platform/cmsx`（素材としての HTTP クライアント。ドメインには依存しない）

##### C-06: Event Publisher / Consumer（ingestion→notification の Pub/Sub 連携）
- **責務**: `safetyincident.domain.NewArrivalEvent` を Pub/Sub に publish、`notification` 側で consume する。
- **Port（publish 側）**: `internal/safetyincident/domain.EventPublisher`
- **Adapter（publish 側）**: `internal/safetyincident/infrastructure/eventbus`（Pub/Sub Publisher）
- **Port（consume 側）**: `internal/notification/domain.NewArrivalConsumer`
- **Adapter（consume 側）**: `internal/notification/infrastructure/eventbus`（Pub/Sub Subscriber）
- **素材**: `internal/platform/pubsubx`（クライアント factory）
- **トピック**: `safety-incident.new-arrival`（proto 定義は `proto/v1/pubsub.proto`）

##### C-08: Crime Map Aggregator（Subdomain: `safetyincident/crimemap`）
- **責務**: 「犯罪」相当の `infoType` コード集合でフィルタし、国別カロプレス or ヒートマップ用ポイント列を返す。フォールバック座標アイテムはヒートマップから除外（FR-APP-08）。
- **Port**: `internal/safetyincident/crimemap/domain.Aggregator`
- **Policy**: `infotype_policy.go`（何を「犯罪」とみなすかのドメインポリシー）
- **Adapter**: `internal/safetyincident/crimemap/infrastructure`（`safetyincident.Repository` を使う実装）

#### 🟦 Context: `user`（Supporting）

##### C-07a: Auth Verifier（Port: `user.AuthVerifier`、Adapter: `firebaseauth.Verifier`）
- **責務**: Firebase ID Token を検証し `VerifiedUser` を返す。
- **Port**: `internal/user/domain.AuthVerifier`
- **Adapter**: `internal/user/infrastructure/firebaseauth`
- **素材**: `internal/platform/firebasex`

##### C-07b: User Profile Repository（Port: `user.ProfileRepository`、Adapter: Firestore 実装）
- **責務**: Firestore 上の `users/{uid}` ドキュメントの CRUD。お気に入り国・通知設定・FCM トークンを保持。
- **Port**: `internal/user/domain.ProfileRepository`
- **Adapter**: `internal/user/infrastructure/firestore`

#### 🟦 Context: `notification`（Supporting）

##### C-07c / C-12 統合: Notification Dispatch
- **責務**: Pub/Sub から受信した新着イベントに対して、購読者を解決し FCM で配信する。
- **Domain**: `Notification`（VO）、`DispatchPolicy`（Domain Service）、`SubscriberStore`（Port）、`PushSender`（Port）、`NewArrivalConsumer`（Port）
- **Application**: `DispatchOnNewArrivalUseCase`
- **Adapter**:
  - `internal/notification/infrastructure/firestore` — SubscriberStore 実装（**user コンテキストを直接 import せず、Firestore 上の同一ドキュメントを読む**。Context 独立性のため）
  - `internal/notification/infrastructure/fcm` — PushSender 実装
  - `internal/notification/infrastructure/eventbus` — Pub/Sub Subscriber

#### 🟦 Context: `cmssetup`（Supporting）

##### C-11: CMS Schema Bootstrapper
- **責務**: 安全情報 Model / Field の宣言的定義を冪等に適用する。
- **Domain**: `SchemaDefinition`（宣言的に「安全情報 Model にこのフィールドが必要」を表現する VO 群）
- **Application**: `EnsureSchemaUseCase`
- **Adapter**: `internal/cmssetup/infrastructure/cms/schema_applier.go`（`platform/cmsx` を使う）

#### 🎯 Interface（入口）レイヤ

##### C-10: BFF（Connect RPC ハンドラ群）
- **配置**: `internal/interfaces/rpc`
- **責務**: Connect サーバの RPC ハンドラ（SafetyIncident / CrimeMap / UserSetting）と `AuthInterceptor`。各 Context の Application Service に委譲するのみ。
- **起動 main**: `cmd/bff/main.go`（Composition Root）

##### C-09 / C-11 / C-12: Job ランナー
- **配置**: `internal/interfaces/job`
- **責務**: ingestion / setup / notifier のエントリポイント。`safetyincident.application.IngestUseCase` / `cmssetup.application.EnsureSchemaUseCase` / `notification.application.DispatchOnNewArrivalUseCase` をループ駆動する。
- **起動 main**: `cmd/ingestion/main.go`、`cmd/setup/main.go`、`cmd/notifier/main.go`（Composition Root）

#### 📦 Platform / Shared（横断）

##### C-13: Observability
- **責務**: `log/slog` による構造化ログ、OpenTelemetry Metrics / Traces、共通 context 属性の付与。
- **公開関数**: `observability.Setup(ctx) (shutdownFn, error)`、`observability.Logger(ctx)`、`observability.Tracer(ctx)`、`observability.Meter(ctx)`
- **パッケージ**: `internal/platform/observability`

##### Platform 素材（ドメイン非依存の SDK ラッパー）
- `internal/platform/config` — 環境変数ローダ
- `internal/platform/connectserver` — HTTP + Connect サーバ組み立て
- `internal/platform/pubsubx` — Pub/Sub クライアント factory
- `internal/platform/cmsx` — reearth-cms HTTP クライアント（素材、ドメイン非依存）
- `internal/platform/firebasex` — Firebase SDK factory（Auth/Firestore/FCM）
- `internal/platform/mapboxx` — Mapbox SDK factory

##### Shared Kernel
- `internal/shared/errs` — エラー型・`%w` ラップ規約
- `internal/shared/clock` — `time` 抽象（テスト容易性）

---

### Flutter アプリ側（Clean Architecture + MVVM + Riverpod）

#### C-20: `domain` レイヤー
- **責務**: エンティティ・ユースケース・Repository インターフェイスを定義する（外部依存を持たない純粋層）。
- **主要クラス**:
  - `SafetyIncident`（エンティティ）
  - `CountryCode`, `LatLng`, `SafetyIncidentFilter`（値オブジェクト）
  - `UserProfile`, `NotificationPreference`
  - `SafetyIncidentRepository`（I/F、BFF リモート向け読み取り）
  - `UserProfileRepository`（I/F、Firestore 向け）
  - `AuthRepository`（I/F、Firebase Auth 向け）
  - ユースケース（UseCase）: `ListSafetyIncidentsUseCase`, `GetSafetyIncidentDetailUseCase`, `ListNearbyUseCase`, `SearchSafetyIncidentsUseCase`, `GetCrimeMapDataUseCase`, `ToggleFavoriteCountryUseCase`, `UpdateNotificationPreferenceUseCase`, `SignInUseCase`, `SignOutUseCase` など

#### C-21: `data` レイヤー
- **責務**: domain インターフェイスの実装・リモート／ローカルデータソース・DTO 変換。
- **主要クラス**:
  - `SafetyIncidentRemoteDataSource`（Connect クライアント呼び出し）
  - `UserProfileRemoteDataSource`（Firestore SDK）
  - `AuthRemoteDataSource`（Firebase Auth SDK）
  - 各 Repository 実装（`SafetyIncidentRepositoryImpl` など）
  - DTO ↔ Entity のマッパー

#### C-22: `presentation` レイヤー（MVVM）
- **責務**: 各 feature ごとの画面（View）と ViewModel を配置。
- **主要 feature**: `auth`, `onboarding`, `map`, `list`, `detail`, `search`, `nearby`, `favorites`, `notifications`, `crime_map`, `about`
- **各 feature 内**: `view/`（Widget 群）、`viewmodel/`（`StateNotifier` / `AsyncNotifier` 相当の Riverpod Notifier）、`state/`（UIState クラス）

#### C-23: `core` レイヤー
- **責務**: 横断機能。Riverpod Providers、`GoRouter` ルーティング、DI 構成、テーマ、ローカライズ、エラーハンドリング、Connect クライアントファクトリ、Firebase 初期化。

---

## コンポーネント俯瞰（機能 → コンポーネント対応）

| 機能／要件 | 担当 Context / コンポーネント |
|---|---|
| MOFA XML 取得・パース | `safetyincident` / C-01 |
| LLM 地名抽出 | `safetyincident` / C-02 |
| Mapbox ジオコーディング ＋ セントロイドフォールバック | `safetyincident` / C-03 |
| CMS への書き込み・読み取り | `safetyincident` / C-04（Port）+ C-05（素材） |
| 新着検知による通知連携 | `safetyincident.EventPublisher` + `notification.NewArrivalConsumer`（Pub/Sub 経由） |
| ID Token 認証 | `user.AuthVerifier`（C-07a）+ `interfaces/rpc.AuthInterceptor` |
| 犯罪マップ集計 | `safetyincident/crimemap` / C-08 |
| ingestion 実行エントリ | `cmd/ingestion` → `interfaces/job` → `safetyincident.application.IngestUseCase` |
| BFF 実行エントリ | `cmd/bff` → `interfaces/rpc` |
| CMS セットアップ | `cmd/setup` → `interfaces/job` → `cmssetup.application.EnsureSchemaUseCase` |
| 通知配信 | `cmd/notifier` → `interfaces/job` → `notification.application.DispatchOnNewArrivalUseCase` |
| ログ・メトリクス・トレース | `platform/observability` / C-13 |
| Flutter 画面・状態管理 | C-20 / C-21 / C-22 / C-23 |
