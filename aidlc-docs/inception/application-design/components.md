# コンポーネント定義 — overseas-safety-map

## リポジトリと配置

| リポジトリ | 役割 | 使用技術 |
|---|---|---|
| **reearth-homework**（このリポジトリ） | Go サーバーモノレポ（ingestion / bff / setup / notifier） | Go 単一モジュール + internal サブパッケージ |
| **overseas-safety-map-app**（新規作成予定） | Flutter アプリ | Dart / Flutter、Clean Architecture + MVVM、Riverpod |

> **暗黙決定（設計者注）**: Q2 で単一 Go モジュールを選択、Q7 で Pub/Sub 分離型を選択したため、通知 Cloud Function（Pub/Sub サブスクライバ）は **Go で実装し、同じ Go モノレポの `cmd/notifier` に配置** する。言語統一のメリットとドメイン／リポジトリの再利用性を優先。

---

## コンポーネント一覧

各コンポーネントは「名前」「責務」「公開インターフェイス」の3点で簡潔に定義する。詳細なメソッドは [component-methods.md](./component-methods.md)、関係は [component-dependency.md](./component-dependency.md) を参照。

---

### サーバー側（Go モノレポ）

#### C-01: `MofaXmlClient`
- **責務**: MOFA オープンデータの XML（`area/00A.xml` 初回 / `area/newarrivalA.xml` 継続）を HTTP で取得し、ドメインモデル `domain.MailItem` の列に変換する。
- **公開インターフェイス**: `mofa.Client`（FetchAll / FetchNewArrivals / Parse）
- **パッケージ**: `internal/mofa`

#### C-02: `LocationExtractor`
- **責務**: `domain.MailItem`（title + mainText）を入力に、事故・事件の発生地名（文字列）を抽出する。LLM 依存は **抽象インターフェイス越し** に隔離する。
- **公開インターフェイス**: `llm.LocationExtractor`
- **既定実装**: `llm.ClaudeExtractor`（Anthropic Claude、Haiku クラスを想定、MVP 第一候補）
- **パッケージ**: `internal/llm`

#### C-03: `Geocoder`
- **責務**: 地名文字列を緯度経度に変換する。プライマリは外部サービス、フォールバックは国セントロイド。
- **公開インターフェイス**: `geocode.Geocoder`
- **既定実装**:
  - `geocode.MapboxGeocoder`（primary）
  - `geocode.CountryCentroidFallback`（国コード → セントロイド座標、オフライン内蔵データ）
  - `geocode.Chain(primary, fallback)` で合成
- **パッケージ**: `internal/geocode`

#### C-04: `SafetyIncidentRepository`
- **責務**: 安全情報の **読み書きを一貫した単一インターフェイス** として提供する。MVP の実装は reearth-cms Integration API、将来は DB 直接化へ差し替え可能（Q9 [B] 両方抽象化）。
- **公開インターフェイス**: `repository.SafetyIncidentRepository`
- **既定実装**: `repository.CMSRepository`（reearth-cms Integration API 経由）
- **パッケージ**: `internal/repository`

#### C-05: `ReearthCmsClient`
- **責務**: reearth-cms Integration REST API（`integration.yml` で定義される Project/Model/Field/Item CRUD）を呼ぶ低レベルクライアント。`SafetyIncidentRepository` から利用される。
- **公開インターフェイス**: `cms.Client`（CreateItem / UpdateItem / ListItems / ItemsAsGeoJSON / CreateProject / CreateModel / CreateField など）
- **パッケージ**: `internal/cms`

#### C-06: `PubSubPublisher` / `PubSubSubscriber`
- **責務**: Google Cloud Pub/Sub への publish/subscribe を提供し、ingestion → notifier の疎結合連携を実現する。
- **公開インターフェイス**: `pubsub.Publisher` / `pubsub.Subscriber`
- **トピック**: `safety-incident.new-arrival`（メッセージは `{keyCd, countryCd, infoType, title}`）
- **パッケージ**: `internal/pubsub`

#### C-07: `FirebaseGateway`
- **責務**: Firebase の 3 機能（Auth ID Token 検証 / Firestore 読み書き / FCM 配信）を横断する Gateway。
- **公開インターフェイス**:
  - `firebase.AuthVerifier`（ID Token 検証）
  - `firebase.UserStore`（お気に入り・通知設定 の CRUD、Firestore 上の `users/{uid}` ドキュメント）
  - `firebase.FcmSender`（トークン配列 → 通知配信）
- **パッケージ**: `internal/firebase`

#### C-08: `CrimeMapAggregator`
- **責務**: 「犯罪」相当の `infoType` コード集合でフィルタし、国別カロプレス集計 or ヒートマップ用ポイント列を返す。フォールバック座標アイテムをヒートマップから除外するロジックも保持。
- **公開インターフェイス**: `crimemap.Aggregator`
- **パッケージ**: `internal/crimemap`

#### C-09: `IngestionApp`（`cmd/ingestion`）
- **責務**: ingestion の main。スケジューラ実行エントリ。
- **起動: CLI + 環境変数**。
- **依存**: MofaXmlClient, LocationExtractor, Geocoder, SafetyIncidentRepository, PubSubPublisher, Observability

#### C-10: `BffApp`（`cmd/bff`）
- **責務**: Flutter 向け Connect サーバーの main。
- **起動: HTTP サーバー（Cloud Run 等）**。
- **依存**: SafetyIncidentRepository, CrimeMapAggregator, FirebaseGateway(Auth), Observability

#### C-11: `SetupApp`（`cmd/setup`）
- **責務**: CMS の Project / Model / Field を冪等に作成する一回限りスクリプト。
- **依存**: ReearthCmsClient, Observability

#### C-12: `NotifierApp`（`cmd/notifier`）
- **責務**: Pub/Sub サブスクライバとして `safety-incident.new-arrival` を受け、対象ユーザーを Firestore から取得して FCM で配信する。
- **起動: Cloud Run（Eventarc トリガー）または Cloud Functions**。
- **依存**: PubSubSubscriber, FirebaseGateway(UserStore+FcmSender), SafetyIncidentRepository (title 等を補完取得、必要時), Observability

#### C-13: `Observability`
- **責務**: `log/slog` による構造化ログ、OpenTelemetry による Metrics / Traces、共通コンテキストの付与。
- **公開インターフェイス**: `observability.Setup(ctx) (shutdownFn, error)`、`observability.Logger(ctx)`、`observability.Tracer(ctx)`、`observability.Meter(ctx)`
- **パッケージ**: `internal/observability`

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

| 機能／要件 | 担当コンポーネント |
|---|---|
| MOFA XML 取得・パース | C-01 |
| LLM 地名抽出 | C-02 |
| Mapbox ジオコーディング ＋ セントロイドフォールバック | C-03 |
| CMS への書き込み・読み取り | C-04 + C-05 |
| 新着検知による通知連携 | C-06 + C-12 + C-07 |
| ID Token 認証 | C-07 + C-10 (AuthMiddleware) |
| 犯罪マップ集計 | C-08 |
| ingestion 実行エントリ | C-09 |
| BFF 実行エントリ | C-10 |
| CMS セットアップ | C-11 |
| 通知配信 | C-12 |
| ログ・メトリクス・トレース | C-13 |
| Flutter 画面・状態管理 | C-20 / C-21 / C-22 / C-23 |
