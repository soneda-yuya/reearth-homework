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
| C-11 | SetupApp | CMS セットアップ main | `cmd/setup` |
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

### 本リポジトリ（`reearth-homework`）: Go サーバーモノレポ
```
/
├── cmd/
│   ├── ingestion/         # C-09
│   ├── bff/               # C-10
│   ├── setup/             # C-11
│   └── notifier/          # C-12
├── internal/
│   ├── domain/            # 純粋ドメイン型
│   ├── mofa/              # C-01
│   ├── llm/               # C-02
│   ├── geocode/           # C-03
│   ├── repository/        # C-04
│   ├── cms/               # C-05
│   ├── pubsub/            # C-06
│   ├── firebase/          # C-07
│   ├── crimemap/          # C-08
│   ├── bff/               # BffApiService 本体
│   ├── ingestion/         # IngestionService 本体
│   ├── notifier/          # NotifierService 本体
│   ├── setup/             # CmsSetupService 本体
│   └── observability/     # C-13
├── proto/
│   └── v1/
│       ├── safetymap.proto      # Connect サービス
│       └── pubsub.proto          # Pub/Sub メッセージ
├── gen/go/v1/              # buf generate 出力（Go）
├── buf.yaml / buf.gen.yaml
├── go.mod
├── tools.go                # buf, connect-go, etc の tools
└── aidlc-docs/             # AI-DLC ドキュメント（既存）
```

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
