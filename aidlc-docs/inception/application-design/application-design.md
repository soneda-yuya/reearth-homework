# アプリケーション設計（統合版） — overseas-safety-map

## 1. 目的とスコープ

本ドキュメントは AI-DLC Application Design ステージの成果物を統合した索引です。高レベルのコンポーネント識別・サービス設計・依存関係までを定義します。**詳細なビジネスロジック（分岐条件・閾値・エラー分類・監視メトリクス定義など）は Construction フェーズ（Functional Design / NFR Requirements / NFR Design / Infrastructure Design）で詳細化** します。

- 成果物1: [components.md](./components.md) — コンポーネントの定義と責務
- 成果物2: [component-methods.md](./component-methods.md) — 各コンポーネントの公開インターフェイスとメソッドシグネチャ
- 成果物3: [services.md](./services.md) — サービスレイヤのオーケストレーション
- 成果物4: [component-dependency.md](./component-dependency.md) — 依存関係マトリクス・データフロー図
- 本ドキュメント: 上記の要約と決定事項一覧

---

## 2. 確定した設計方針

Application Design Plan の質問に基づく確定事項:

| 項目 | 決定 | 出典 |
|---|---|---|
| リポジトリ構成 | **2リポジトリ分割** — 本リポジトリを Go サーバーモノレポ、別途 Flutter 専用リポジトリ（`overseas-safety-map-app`）を新規作成 | Q1 [B] |
| Go モジュール構成 | **単一 Go モジュール + `cmd/*` + `internal/*`** | Q2 [A] |
| Flutter プロジェクト構造 | **Clean Architecture + MVVM**（domain / data / presentation + core） | Q3 [X] |
| BFF API スタイル | **Connect（gRPC 互換）** — `proto/v1/*.proto` をスキーマ契約に | Q4 [C] |
| Go HTTP フレームワーク | **不要**（Connect サーバが同梱） | Q5 [D] |
| Flutter 状態管理 | **Riverpod**（`AsyncNotifier` + `riverpod_generator`） | Q6 [A] |
| 通知配信 | **Pub/Sub 経由**（ingestion → Pub/Sub → 通知 Cloud Function → FCM） | Q7 [B] |
| LLM プロバイダ | **Anthropic Claude**（Haiku クラス、プロバイダ抽象越しに差し替え可能） | Q8 [A] |
| Repository 抽象化 | **ingestion / BFF の両方** で共通インターフェイスを挟む | Q9 [B] |
| 横断ルール | **%w エラーラップ + `log/slog` + OpenTelemetry Metrics / Traces** を採用 | Q10 [A,B,C,D] |

### 暗黙の設計決定（設計者判断）
- **通知 Cloud Function は Go で実装**し、同じ Go モノレポの `cmd/notifier` に配置（Q2 の単一モジュールと整合）
- **Repository は読み書き統合インターフェイス**（`SafetyIncidentRepository` に `Get/Exists/List/Upsert/Delete`）。これが一番素直に Q9 [B] を実現する
- **Pub/Sub のトピック**: `safety-incident.new-arrival`。メッセージ proto も `proto/v1/` 配下に配置
- **Proto の共有方法**: 実際の共有手段（サブモジュール / CI コピー / 発行）は Infrastructure Design で決定
- **犯罪マップの `infoType` コード集合**: Functional Design で確定（`infotype.xlsx` から選定）

---

## 3. コンポーネント一覧（サーバー側）

| ID | 名前 | 役割 | パッケージ |
|---|---|---|---|
| C-01 | MofaXmlClient | MOFA XML 取得・パース | `internal/mofa` |
| C-02 | LocationExtractor | 地名抽出（Claude 実装） | `internal/llm` |
| C-03 | Geocoder | 緯度経度変換（Mapbox + 国セントロイドフォールバック） | `internal/geocode` |
| C-04 | SafetyIncidentRepository | 安全情報の読み書き I/F（CMS 実装） | `internal/repository` |
| C-05 | ReearthCmsClient | reearth-cms Integration API クライアント | `internal/cms` |
| C-06 | PubSub Publisher/Subscriber | Pub/Sub 連携 | `internal/pubsub` |
| C-07 | FirebaseGateway | Auth ID Token 検証・Firestore・FCM | `internal/firebase` |
| C-08 | CrimeMapAggregator | 犯罪マップ集計（カロプレス/ヒートマップ） | `internal/crimemap` |
| C-09 | IngestionApp | ingestion main | `cmd/ingestion` |
| C-10 | BffApp | BFF main | `cmd/bff` |
| C-11 | SetupApp | CMS セットアップ main | `cmd/cmsmigrate` |
| C-12 | NotifierApp | 通知配信 main | `cmd/notifier` |
| C-13 | Observability | slog + OTel Metrics/Traces | `internal/observability` |

## 4. コンポーネント一覧（Flutter 側）

| ID | 名前 | 役割 |
|---|---|---|
| C-20 | domain レイヤ | エンティティ・ユースケース・Repository I/F |
| C-21 | data レイヤ | Repository 実装・DataSource（Connect / Firebase） |
| C-22 | presentation レイヤ (MVVM) | View + ViewModel（feature-first） |
| C-23 | core レイヤ | Providers / Router / Theme / DI |

---

## 5. サービス一覧

### サーバー側

| ID | 名前 | 責務 |
|---|---|---|
| S-01 | IngestionService | MOFA XML → LLM 抽出 → Mapbox Geocode → CMS 保存 → Pub/Sub 発行 |
| S-02 | NotifierService | Pub/Sub → Firestore 購読者解決 → FCM 配信 |
| S-03 | BffApiService | Connect サーバ、ID Token 検証、Repository / CrimeMapAggregator 経由 |
| S-04 | CmsSetupService | CMS の Project/Model/Field を冪等作成 |

### Flutter 側（UseCase 層）
ListSafetyIncidents / GetSafetyIncidentDetail / ListNearby / SearchSafetyIncidents / GetCrimeMapData / ToggleFavoriteCountry / UpdateNotificationPreference / SignIn / SignUp / SignOut / ObserveAuthState / HandleNotificationTap

詳細は [services.md](./services.md) を参照。

---

## 6. リポジトリとディレクトリ構造（予定）

### 本リポジトリ（`overseas-safety-map`）: Go サーバーモノレポ — **DDD（Bounded Context × Layered Architecture）**

#### 設計原則
- **Bounded Context を `internal/` 直下で分離**: それぞれに閉じた `domain` / `application` / `infrastructure` レイヤを持つ。
- **Layered Architecture（依存は内向き）**: `domain` は他レイヤを知らない → `application` は `domain` のみ参照 → `infrastructure` は `domain` の Port（I/F）を実装し、`application` は依存性注入で受け取る。
- **Interfaces（入口）レイヤは `internal/interfaces/`**: Connect ハンドラや Job ランナーなど、外部トリガーに対して Application Service を呼ぶだけの薄い層。
- **Platform / Shared**: 技術基盤（Pub/Sub、HTTP サーバ、OTel、設定読み込み）は `internal/platform/`、純粋な共有ユーティリティは `internal/shared/`。
- **cmd/\* は Composition Root**: DI ワイヤリングと main のみ。ビジネスロジックは置かない。
- **循環依存禁止**: Bounded Context 間は **Port/Adapter + Domain Event** でのみ結合（例: `safetyincident.domain.NewArrivalEvent` を `notification.infrastructure.eventbus` が受け取る）。

#### Bounded Context 一覧
| Context | タイプ | 説明 |
|---|---|---|
| `safetyincident` | **Core** | MOFA 取り込み・LLM 抽出・ジオコード・CMS 永続化・読み取り / 検索。サブドメイン `crimemap` を内包。 |
| `notification` | Supporting | 新着 Domain Event を受けて対象ユーザーへ FCM 配信。 |
| `user` | Supporting | Firebase Auth 検証 + Firestore 上のユーザープロファイル（お気に入り・通知設定・FCM トークン）。 |
| `cmsmigrate` | Supporting | reearth-cms の Project / Model / Field 冪等作成。 |

#### ディレクトリ構造
```
/
├── cmd/                                  # Composition roots（DI + main のみ）
│   ├── ingestion/main.go
│   ├── bff/main.go
│   ├── notifier/main.go
│   └── cmsmigrate/main.go
│
├── internal/
│   ├── safetyincident/                   # 🟩 Core Bounded Context
│   │   ├── domain/                       # 純粋ドメイン（依存ゼロ）
│   │   │   ├── safety_incident.go        # Aggregate Root
│   │   │   ├── mail_item.go              # Entity（MOFA raw）
│   │   │   ├── key_cd.go                 # VO
│   │   │   ├── lat_lng.go                # VO
│   │   │   ├── country_code.go           # VO
│   │   │   ├── area_code.go              # VO
│   │   │   ├── info_type.go              # VO
│   │   │   ├── geocode_source.go         # VO (enum)
│   │   │   ├── filter.go                 # List/Search 用の VO
│   │   │   ├── repository.go             # Port: Repository I/F
│   │   │   ├── mofa_source.go            # Port: MOFA 取得 I/F
│   │   │   ├── location_extractor.go     # Port: LLM 抽出 I/F（Domain Service）
│   │   │   ├── geocoder.go               # Port: ジオコーダ I/F（Domain Service）
│   │   │   ├── event_publisher.go        # Port: Domain Event 発行 I/F
│   │   │   └── events.go                 # NewArrivalEvent ほか
│   │   ├── application/                  # ユースケース（Application Service）
│   │   │   ├── ingest_usecase.go         # 取り込みオーケストレーション
│   │   │   ├── list_usecase.go
│   │   │   ├── get_usecase.go
│   │   │   ├── search_usecase.go
│   │   │   ├── nearby_usecase.go
│   │   │   └── dto.go                    # Application DTO（proto と domain の翻訳）
│   │   ├── crimemap/                     # 🟨 Subdomain（safetyincident 内部）
│   │   │   ├── domain/
│   │   │   │   ├── choropleth.go         # VO
│   │   │   │   ├── heatmap_point.go      # VO
│   │   │   │   ├── infotype_policy.go    # Policy: 何を「犯罪」とみなすか
│   │   │   │   └── aggregator.go         # Domain Service I/F
│   │   │   ├── application/
│   │   │   │   ├── get_choropleth_usecase.go
│   │   │   │   └── get_heatmap_usecase.go
│   │   │   └── infrastructure/
│   │   │       └── repository_aggregator.go  # safetyincident.Repository を使う実装
│   │   └── infrastructure/               # Port の実装（Adapter）
│   │       ├── mofa/
│   │       │   ├── http_source.go        # MofaSource 実装
│   │       │   └── parser.go
│   │       ├── cms/
│   │       │   ├── repository.go         # Repository 実装（reearth-cms Integration API）
│   │       │   ├── client.go             # 低レベル HTTP クライアント
│   │       │   └── dto.go                # CMS JSON ↔ domain マッピング
│   │       ├── llm/
│   │       │   └── claude_extractor.go   # LocationExtractor 実装
│   │       ├── geocode/
│   │       │   ├── mapbox.go             # Geocoder 実装（primary）
│   │       │   ├── centroid.go           # Geocoder 実装（fallback）
│   │       │   └── chain.go              # 合成（primary → fallback）
│   │       └── eventbus/
│   │           └── pubsub_publisher.go   # EventPublisher 実装（Pub/Sub）
│   │
│   ├── notification/                     # 🟦 Supporting Bounded Context
│   │   ├── domain/
│   │   │   ├── notification.go           # VO（Title/Body/Payload）
│   │   │   ├── subscriber.go             # VO
│   │   │   ├── dispatch_policy.go        # Domain Service: 誰に送るか
│   │   │   ├── subscriber_store.go       # Port I/F
│   │   │   ├── push_sender.go            # Port I/F
│   │   │   └── new_arrival_consumer.go   # Port: 他コンテキストからの Event 受信 I/F
│   │   ├── application/
│   │   │   └── dispatch_usecase.go       # DispatchOnNewArrivalUseCase
│   │   └── infrastructure/
│   │       ├── firestore/
│   │       │   └── subscriber_store.go   # SubscriberStore 実装
│   │       ├── fcm/
│   │       │   └── push_sender.go        # PushSender 実装
│   │       └── eventbus/
│   │           └── pubsub_consumer.go    # NewArrivalConsumer 実装（Pub/Sub 購読）
│   │
│   ├── user/                             # 🟦 Supporting Bounded Context
│   │   ├── domain/
│   │   │   ├── user_profile.go           # Aggregate
│   │   │   ├── favorite_country.go       # VO
│   │   │   ├── notification_pref.go      # Entity
│   │   │   ├── fcm_token.go              # VO
│   │   │   ├── verified_user.go          # VO（Auth から）
│   │   │   ├── profile_repository.go     # Port I/F
│   │   │   └── auth_verifier.go          # Port I/F
│   │   ├── application/
│   │   │   ├── get_profile_usecase.go
│   │   │   ├── toggle_favorite_usecase.go
│   │   │   ├── update_notification_pref_usecase.go
│   │   │   └── register_fcm_token_usecase.go
│   │   └── infrastructure/
│   │       ├── firestore/
│   │       │   └── profile_repository.go # ProfileRepository 実装
│   │       └── firebaseauth/
│   │           └── verifier.go           # AuthVerifier 実装
│   │
│   ├── cmsmigrate/                         # 🟦 Supporting Bounded Context
│   │   ├── domain/
│   │   │   └── schema_definition.go      # 安全情報 Model / Field の宣言的定義
│   │   ├── application/
│   │   │   └── ensure_schema_usecase.go  # 冪等適用
│   │   └── infrastructure/
│   │       └── cms/
│   │           └── schema_applier.go     # reearth-cms Integration API 経由
│   │
│   ├── interfaces/                       # 🎯 入口レイヤ（薄いアダプター）
│   │   ├── rpc/                          # Connect RPC ハンドラ
│   │   │   ├── safetyincident_handler.go # safetyincident.application を呼ぶ
│   │   │   ├── crimemap_handler.go       # safetyincident/crimemap.application を呼ぶ
│   │   │   ├── usersetting_handler.go    # user.application を呼ぶ
│   │   │   ├── auth_interceptor.go       # user.AuthVerifier を使う Interceptor
│   │   │   └── server.go                 # Connect mux 組み立て
│   │   └── job/                          # スケジューラ／CLI 起動
│   │       ├── ingestion_runner.go
│   │       ├── notifier_runner.go
│   │       └── setup_runner.go
│   │
│   ├── shared/                           # Shared Kernel
│   │   ├── errs/                         # errors.Is/As ヘルパ、%w ラップ規約
│   │   └── clock/                        # time 抽象（テスト容易性）
│   │
│   └── platform/                         # 技術基盤（Adapter 供給側の素材）
│       ├── config/                       # 環境変数読み込み
│       ├── observability/                # slog + OTel セットアップ
│       ├── connectserver/                # HTTP サーバ + Connect 組み立て
│       ├── pubsubx/                      # Pub/Sub クライアント factory
│       ├── cmsx/                         # reearth-cms HTTP クライアント（adapter 用の素材）
│       ├── firebasex/                    # Firebase SDK factory（Auth/Firestore/FCM）
│       └── mapboxx/                      # Mapbox SDK factory
│
├── proto/
│   └── v1/
│       ├── safetymap.proto               # Connect サービス
│       └── pubsub.proto                  # Domain Event メッセージ
├── gen/go/v1/                            # buf generate 出力
├── buf.yaml / buf.gen.yaml
├── go.mod
├── tools.go                              # buf, connect-go, etc
└── aidlc-docs/                           # AI-DLC ドキュメント（既存）
```

#### レイヤ依存ルール（Go 側）
- `domain` → ❌ 他レイヤ / 他 Context / `platform` / `shared` いずれにも **依存しない**（`time`・標準ライブラリ・`errors` のみ）
- `application` → ✅ 同 Context の `domain` のみ
- `infrastructure` → ✅ 同 Context の `domain`（Port 実装）と `platform` / `shared` / 必要に応じて generated proto
- `interfaces/rpc` → ✅ 複数 Context の `application`、`shared`、generated proto
- `interfaces/job` → ✅ 同 Context の `application`、`shared`
- `platform` → ✅ `shared` のみ（ドメイン非依存）
- `shared` → ❌ 他 `internal/*` 依存なし

#### Context 間結合ルール（Bounded Context の尊重）
- **禁止**: Context A の `application` / `infrastructure` が Context B の `domain` / `application` / `infrastructure` を直接 import すること
- **許可**:
  - `interfaces/rpc` が複数 Context の `application` を組み合わせる（オーケストレーション）
  - Domain Event を **proto 定義** に変換して Pub/Sub 経由で連携（`safetyincident.NewArrivalEvent` → `notification.NewArrivalConsumer`）
  - `cmd/*` の Composition Root が DI の際に全 Context の型を触るのは OK（本来の責務）

### 別リポジトリ（`overseas-safety-map-app`、新規作成予定）: Flutter アプリ
```
/
├── lib/
│   ├── main.dart
│   ├── app.dart
│   ├── core/               # C-23
│   │   ├── providers/
│   │   ├── router/
│   │   ├── theme/
│   │   ├── i18n/
│   │   └── connect/        # Connect client ファクトリ
│   ├── domain/             # C-20
│   │   ├── entities/
│   │   ├── repositories/
│   │   └── usecases/
│   ├── data/               # C-21
│   │   ├── datasources/
│   │   ├── dto/
│   │   └── repositories/   # Repo Impl
│   └── presentation/       # C-22
│       └── features/
│           ├── auth/
│           ├── onboarding/
│           ├── map/
│           ├── list/
│           ├── detail/
│           ├── search/
│           ├── nearby/
│           ├── favorites/
│           ├── notifications/
│           ├── crime_map/
│           └── about/
├── proto/v1/               # buf で Dart コードを生成（Go 側と同一 .proto を管理）
├── lib/gen/v1/             # 生成コード
├── test/
├── integration_test/
└── pubspec.yaml
```

---

## 7. 要件・ストーリーとの対応

### 要件トレース

| 要件 (FR / NFR) | 主担当コンポーネント／サービス |
|---|---|
| FR-ING-01〜09 | S-01 IngestionService（C-01/02/03/04/06/13） |
| FR-CMS-01〜05 | S-04 CmsSetupService（C-05/13）+ S-01/S-03 の Repository 経由 |
| FR-BFF-01〜04 | S-03 BffApiService（C-04/07/08/13） |
| FR-APP-01〜08 | Flutter C-20/C-21/C-22/C-23（+ BFF S-03） |
| NFR-SEC-01〜05 | C-07 AuthVerifier、各サービスの middleware、`internal/observability`、`internal/firebase` |
| NFR-TEST-01〜04 | 各コンポーネントのインターフェイス境界（テスト容易性）、PBT 対象関数は Functional Design で指定 |
| NFR-OPS-01〜04 | C-13 Observability（slog + OTel） |
| NFR-EXT-01 | C-04 Repository 抽象化（CMS → DB 差し替え可能） |
| NFR-EXT-02 | C-02 LocationExtractor / C-03 Geocoder / 地図タイル（Flutter 側）をインターフェイス化 |
| NFR-LIC-01/02 | Flutter C-22（about feature）、BFF は出典／加工の注記を安全情報レスポンスにメタとして含む（Functional Design で確定） |

### ストーリー主要担当

| Story | 主担当 |
|---|---|
| US-01 オンボーディング/新規登録 | C-23 Router + auth feature + C-21 AuthDataSource + Firebase |
| US-02 ログイン維持・再認証 | auth feature + Connect Interceptor（ID Token 自動再取得） |
| US-03 地図閲覧 | map feature + BFF ListSafetyIncidents + GeoJSON |
| US-04 一覧閲覧 | list feature + BFF ListSafetyIncidents |
| US-05 詳細＋出典表記 | detail feature + BFF GetSafetyIncident |
| US-06 絞り込み | search feature + BFF SearchSafetyIncidents |
| US-07 現在地近く | nearby feature + BFF ListNearby + 端末 GPS |
| US-08 お気に入り | favorites feature + UserProfile 関連 UseCase + Firestore |
| US-09 通知設定 | notifications feature + UserProfile / FCM Token 登録 |
| US-10 通知受信 | Flutter FCM handler + Router + Pub/Sub → Notifier |
| US-11 情報ページ | about feature（MOFA 出典・加工の注記） |
| US-12 オフライン/エラー | core の共通エラー境界 + DataSource のキャッシュ方針 |
| US-13 犯罪マップ | crime_map feature + BFF CrimeMapService（Choropleth/Heatmap） |

---

## 8. 次ステージ（Units Planning）への申し送り

- `cmd/*` と `internal/*` のパッケージ構成は決定済み。次段階で **ユニット（＝独立して実装可能なビルド単位）** に分けてタスク化する。
- `proto/v1/*.proto` をコード生成のソースとして唯一視するルールを Infrastructure Design で明文化する。
- Flutter のリポジトリ作成は Infrastructure Design で取り扱う（新規 GitHub リポジトリ作成、CI セットアップ）。
- Outbox パターン採用可否、`notification_logs` の可否、地名抽出プロンプト設計、`infoType` 犯罪コード集合の確定は **Functional Design** で行う（今フェーズでは選択肢として提示）。
