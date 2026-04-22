# Unit of Work 定義 — overseas-safety-map

## 分割軸と前提

- **分割軸**: Deployable 単位（計画質問 Q1 [A]）
- **U-PLT の位置づけ**: 独立 Unit 0、他の全 Unit の基盤（Q2 [A]）
- **実装順序**: 依存順（Q3 [A]）— PLT → CSS → ING → BFF → NTF → APP
- **デプロイモデル**: Cloud Run 系で統一（Q4 [A]）
- **体制**: 1 人開発（Q5 [A]）
- **デモ節目**: U-CSS / U-ING / U-BFF / U-APP（Q6 [A,B,C,E]）。U-NTF は単独デモ節目を設けず、U-APP 完了時の実機検証に統合する。

各 Unit は Construction フェーズで 1 サイクル（Functional Design → NFR Requirements → NFR Design → Infrastructure Design → Code Generation → Build and Test）を回す対象となる。

---

## Unit 一覧

| ID | 名称 | タイプ | リポジトリ | Deployable | 順序 |
|---|---|---|---|---|---|
| U-PLT | Platform & Proto 基盤 | Supporting（Shared Infra） | reearth-homework | なし（ライブラリ / 契約） | 0 |
| U-CSS | CMS セットアップ | Supporting（Context） | reearth-homework | Cloud Run Job 単発 | 1 |
| U-ING | 取り込みパイプライン | Core（Context） | reearth-homework | Cloud Run Job（スケジュール） | 2 |
| U-BFF | Connect サーバ（BFF） | Core（Context 横断） | reearth-homework | Cloud Run Service | 3 |
| U-NTF | 通知配信 | Supporting（Context） | reearth-homework | Cloud Run Service（Pub/Sub push） | 4 |
| U-APP | Flutter アプリ | Core（フロントエンド） | overseas-safety-map-app（別リポ） | TestFlight / Play Console Internal | 5 |

---

## U-PLT: Platform & Proto 基盤

### 責務
- 全 Unit が参照する横断基盤（`internal/platform/*`、`internal/shared/*`）を整備する。
- `proto/v1/*.proto` と `buf` 設定を確定し、Go / Dart 両方のコード生成パイプラインを完成させる。
- Go モジュール・依存関係・CI 基盤（GitHub Actions の再利用可能ワークフロー）を構築する。

### 含むパッケージ／成果物
- `internal/platform/observability`（slog + OTel セットアップ）
- `internal/platform/config`（env 読み込み）
- `internal/platform/connectserver`（HTTP + Connect 組み立て）
- `internal/platform/pubsubx`（Pub/Sub クライアント factory）
- `internal/platform/cmsx`（reearth-cms HTTP クライアント）
- `internal/platform/firebasex`（Firebase SDK factory）
- `internal/platform/mapboxx`（Mapbox SDK factory）
- `internal/shared/errs`（`%w` ラップ規約、AppError 型）
- `internal/shared/clock`（時計抽象）
- `proto/v1/safetymap.proto`（Connect サービス定義）
- `proto/v1/pubsub.proto`（Domain Event メッセージ）
- `buf.yaml` / `buf.gen.yaml` / `gen/go/v1/*`
- `go.mod` / `tools.go`
- `.github/workflows/ci.yml`（lint / test / buf check / build）

### 完了条件（Definition of Done）
- `go build ./...` / `go test ./...` が通る（空実装でも良い）
- `buf lint` / `buf breaking` / `buf generate` がエラーゼロ
- `internal/platform/observability.Setup()` を呼べば Cloud Run ログに JSON 構造化ログが出力される（検証スクリプト付き）
- 各 SDK factory（`cmsx`, `firebasex`, `pubsubx`, `mapboxx`）に対して最小疎通テストが CI で動く
- CI が main ブランチで緑

### デモ節目
なし（基盤 Unit、以降の前提）。ただし CI 緑化とコード生成成功が内部マイルストーン。

---

## U-CSS: CMS セットアップ

### 責務
- reearth-cms 上の Project / Model / Field を宣言的に冪等適用する。
- `SafetyIncident` Model と必要フィールド（FR-CMS-05）を定義し、CMS UI 相当の初期化作業を自動化する。

### 含むパッケージ／成果物
- `internal/cmssetup/domain`（`SchemaDefinition` / `ModelDefinition` / `FieldDefinition`）
- `internal/cmssetup/application`（`EnsureSchemaUseCase`）
- `internal/cmssetup/infrastructure/cms`（`SchemaApplier`）
- `internal/interfaces/job/setup_runner.go`
- `cmd/setup/main.go`
- `deploy/setup/`（Dockerfile + Cloud Run Job デプロイ定義）

### 完了条件
- `cmd/setup` をローカル / Cloud Run Job で実行すると CMS に Project / Model / Field が存在する（既存なら no-op）
- ユニットテストで `SchemaApplier` の冪等性を検証
- Infrastructure Design 時に GitHub Actions から Cloud Run Job 起動手順を整備

### デモ節目（Q6 [A]）
**U-CSS 完了時**: CMS UI で SafetyIncident Model とフィールドを確認できる。手動で Item を作成して UI で表示確認できる。

### 依存
- U-PLT（`platform/cmsx`、`platform/observability`、`platform/config`、`shared/errs`）

---

## U-ING: 取り込みパイプライン

### 責務
- MOFA オープンデータを定期取得し、LLM 地名抽出 → Mapbox Geocoding → 国セントロイドフォールバックのチェーンを経て、CMS へ Item を登録する。
- 新着発生を Pub/Sub に publish する（`notification` コンテキストへの Domain Event）。

### 含むパッケージ／成果物
- `internal/safetyincident/domain` のうち取り込みに必要な部分（`MailItem`, `SafetyIncident`, `MofaSource`, `LocationExtractor`, `Geocoder`, `Repository`, `EventPublisher`, `NewArrivalEvent`）
- `internal/safetyincident/application/ingest_usecase.go`
- `internal/safetyincident/infrastructure/mofa`
- `internal/safetyincident/infrastructure/llm`（Claude Haiku 実装）
- `internal/safetyincident/infrastructure/geocode`（Mapbox + Centroid + Chain）
- `internal/safetyincident/infrastructure/cms`（Repository 書き込み側）
- `internal/safetyincident/infrastructure/eventbus`（Pub/Sub Publisher）
- `internal/interfaces/job/ingestion_runner.go`
- `cmd/ingestion/main.go`
- `deploy/ingestion/`（Dockerfile + Cloud Run Job + Cloud Scheduler 定義）

### 完了条件
- 初回モード（`00A.xml`）で全件取り込みが成功
- 継続モード（`newarrivalA.xml`）で差分取り込みが成功、既存 `keyCd` はスキップ
- フォールバック UX（ジオコード失敗時の国セントロイド、LLM 抽出失敗時の空文字ハンドリング）を検証
- PBT（NFR-TEST-02）: XML パーサ、地名抽出プロンプト I/O、ジオコード合成の 3 つに適用
- Cloud Run Job + Cloud Scheduler で 5 分毎実行が稼働

### デモ節目（Q6 [B]）
**U-ING 完了時**: Cloud Scheduler 実行 → CMS 上に安全情報 Item が自動蓄積される。ログでジオコード成功率 / フォールバック件数 / LLM 失敗件数が確認できる。

### 依存
- U-PLT / U-CSS（CMS スキーマが前提）

---

## U-BFF: Connect サーバ（BFF）

### 責務
- Flutter アプリに向けた Connect RPC を提供する。`ListSafetyIncidents` / `GetSafetyIncident` / `SearchSafetyIncidents` / `ListNearby` / `GeoJSON` / `CrimeMap.Choropleth` / `CrimeMap.Heatmap` / `UserProfile` 系。
- Firebase ID Token 検証（`AuthInterceptor`）を実装し、未認証リクエストを弾く。
- Repository 読み取りと `crimemap.Aggregator` による犯罪マップ集計を提供。
- ユーザー設定（お気に入り・通知 pref・FCM Token）の読み書きを `user.application` 経由で行う。

### 含むパッケージ／成果物
- `internal/safetyincident/application` 読み取り系（`ListUseCase` / `GetUseCase` / `SearchUseCase` / `NearbyUseCase`）
- `internal/safetyincident/crimemap/{domain,application,infrastructure}`
- `internal/user/{domain,application,infrastructure}`（`AuthVerifier` + `ProfileRepository`）
- `internal/interfaces/rpc/*`（Connect ハンドラと `AuthInterceptor`）
- `cmd/bff/main.go`
- `deploy/bff/`（Dockerfile + Cloud Run Service 定義）

### 完了条件
- `buf curl` で認証付きリクエストが通り、CMS に登録済みの Item を返す
- 未認証リクエストが 401 で弾かれる
- 犯罪マップ API がカロプレス・ヒートマップ（フォールバック除外）を返す
- ウィジェット／結合テストで Flutter が読み取り可能な proto フォーマットを返す
- Firestore セキュリティルールのユニットテストが緑（NFR-SEC-04）

### デモ節目（Q6 [C]）
**U-BFF 完了時**: `buf curl` / Postman で Connect 経由のすべての RPC が動作。API レスポンスを手動確認できる。

### 依存
- U-PLT / U-CSS / U-ING（実データがあると検証価値が高い、疎通は CSS 完了時点で可能）

---

## U-NTF: 通知配信

### 責務
- ingestion の `NewArrivalEvent` を Pub/Sub 経由で受け、Firestore の購読者を解決して FCM を配信する。
- 配信後の無効トークンは Firestore から除去する。

### 含むパッケージ／成果物
- `internal/notification/{domain,application,infrastructure}`
- `internal/interfaces/job/notifier_runner.go`
- `cmd/notifier/main.go`
- `deploy/notifier/`（Dockerfile + Cloud Run Service + Pub/Sub Push Subscription）

### 完了条件
- ingestion が Pub/Sub へ発行 → notifier が受信 → テスト端末に通知が届く
- 無効トークンの除去ロジックを結合テストで検証
- リトライポリシー（Pub/Sub 側）と DLQ の設定完了

### デモ節目
単独のデモ節目は設けない（Q6 で D を除外）。**U-APP 完了時の実機検証に統合**する。

### 依存
- U-PLT / U-ING（Pub/Sub 発行イベントが前提）

---

## U-APP: Flutter アプリ

### 責務
- iOS / Android 向けに Clean Architecture + MVVM + Riverpod で 13 MVP ストーリー（US-01〜US-13）を実現する。
- Firebase Auth ログインを必須化し、Connect クライアント経由で BFF と通信、Firestore でユーザー設定を保存、FCM で通知を受信する。
- `flutter_map` で地図表示、OSM タイル、ピン／クラスタ／ヒートマップ／カロプレス。
- MOFA 出典表記を規定通り表示（NFR-LIC-01/02）。

### 含むパッケージ／成果物
- 別リポジトリ **`overseas-safety-map-app`**（新規作成）
- `lib/domain/`（Entities / UseCases / Repository I/F）
- `lib/data/`（DataSources / Repository Impl / DTO）
- `lib/presentation/features/*`（auth / onboarding / map / list / detail / search / nearby / favorites / notifications / crime_map / about）
- `lib/core/`（Providers / Router / Theme / i18n / Connect factory）
- `proto/v1/*` と `lib/gen/v1/*`（Go と同一 .proto から Dart コード生成）
- `.github/workflows/flutter-ci.yml`
- `fastlane/` or EAS 的なビルド配信設定（TestFlight / Play Console Internal）

### 完了条件
- iOS シミュレータ + Android エミュレータ + 実機の 3 環境で 13 MVP ストーリーの GWT 基準が通る
- ウィジェット＋結合テスト CI が緑
- TestFlight / Play Console Internal Testing に配信済み
- 通知（U-NTF）の実機検証がここで完了（統合確認）

### デモ節目（Q6 [E]）
**U-APP 完了時**: iOS / Android 実機で MVP 全画面が動作し、通知受信 → 詳細遷移が成立する。

### 依存
- U-PLT（proto 共有）/ U-BFF（API）/ U-NTF（通知）

---

## コード組織戦略（Greenfield 指針）

Application Design [application-design.md §6](./application-design.md) の DDD 構造が確定しているため、基本的にはそれに従う。以下は Unit 化に伴う追加の運用ルール:

- **リポジトリ 2 つ**: `reearth-homework`（Go サーバーモノレポ）、`overseas-safety-map-app`（Flutter）
- **ディレクトリは Unit 単位で独立ビルド可能** にする:
  - U-PLT / U-CSS / U-ING / U-BFF / U-NTF は全て `cmd/{deployable}` と `internal/*` の組み合わせで独立ビルドでき、CI で個別 Unit をテスト可能にする。
  - `deploy/{unit}/` にそれぞれの Dockerfile / Cloud Run 定義を置く。
- **proto は単一ソース・双方生成**: `proto/v1/*.proto` を Go 側リポが owner とする。Flutter リポへは **CI が生成コードを同期**（サブモジュール or 生成済みコードのコピー同期ワークフロー。具体手段は Infrastructure Design で確定）。
- **CI 分岐**:
  - Go 側: Unit 毎のテストターゲットを make / go test のビルドタグで分ける
  - Flutter 側: 別リポなので独立 CI
- **Build タグ**: Unit 毎に `//go:build unit_<name>` は使わず、package の疎結合で分離する（build tag を使うと import エラーが見えなくなるため）。
- **環境変数の名前空間**: Unit 毎に `INGESTION_*` / `BFF_*` / `NOTIFIER_*` / `SETUP_*` のプレフィックスで分離し、Config loader で検証する。

---

## Unit 実装ロードマップ

```
(Sprint 0) U-PLT  — 基盤
   ↓
(Sprint 1) U-CSS  — CMS スキーマ確定 (デモ節目: CMS UI で Model 確認)
   ↓
(Sprint 2) U-ING  — ingestion 稼働 (デモ節目: CMS に自動蓄積)
   ↓
(Sprint 3) U-BFF  — Connect API 提供 (デモ節目: buf curl で全 RPC 動作)
   ↓
(Sprint 4) U-NTF  — 通知配信
   ↓
(Sprint 5) U-APP  — Flutter アプリ (デモ節目: 実機で MVP 全機能 + 通知受信)
```

各 Sprint は Construction フェーズの 1 サイクル（Functional Design → NFR Req → NFR Design → Infra Design → Code Gen → Build & Test）を回す単位。
