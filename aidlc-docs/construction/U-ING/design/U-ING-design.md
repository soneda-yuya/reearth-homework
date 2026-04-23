# U-ING Design (Minimal 合本版)

**Unit**: U-ING（Ingestion Unit、Sprint 2）
**Deployable**: `cmd/ingestion`（Cloud Run Job、Cloud Scheduler 起動、5 分毎）
**Bounded Context**: `safetyincident`（Core domain）
**ワークフロー圧縮**: Option B（Functional Design + NFR Requirements + NFR Design 1 本に集約）

---

## 0. Design Decisions（計画回答の確定）

[`U-ING-design-plan.md`](../../plans/U-ING-design-plan.md) Q1-Q9 すべて **[A]** で確定。

| # | 決定事項 | 選択 | 要旨 |
|---|---------|------|------|
| Q1 | 取り込みモード | **A** | `initial` + `incremental` 両方を `INGESTION_MODE` env で切替 |
| Q2 | ポーリング間隔 | **A** | **5 分毎**（Cloud Scheduler `*/5 * * * *`） |
| Q3 | 重複排除 | **A** | **CMS lookup**（`cmsx.GetItemByAlias`、Source of Truth = CMS） |
| Q4 | LLM プロンプト | **A** | **1 件ずつ + 並列度 5**（errgroup） |
| Q5 | ジオコーディング失敗時 | **A** | **国 Centroid フォールバック**（`geocode_source = "country_centroid"`） |
| Q6 | Pub/Sub publish | **A** | **CMS upsert 直後に 1 件ずつ publish** |
| Q7 | エラーハンドリング | **A** | **失敗 item は skip + ログ + Metric**、Run は exit 0 |
| Q8 | Rate Limit | **A** | **app 側 ratelimit**（LLM 5 req/s、Mapbox 10 req/s） |
| Q9 | テスト戦略 | **A** | **Domain PBT + fake シナリオ**、実 API 統合は Build & Test |

---

## 1. Functional Design

### 1.1 Context — U-ING の責務

**目的**: MOFA 海外安全情報オープンデータを定期取得し、LLM 地名抽出 → Mapbox Geocoding → 国 Centroid フォールバックチェーンを経て、reearth-cms に **upsert** し、新着を Pub/Sub 経由で `notification` Bounded Context に通知する。

**ライフサイクル**:
- `incremental` モード: Cloud Scheduler が **5 分毎** に `cms-migrate` Job を起動（U-CSS と同じ Job 起動モデル）
- `initial` モード: 運用者が **手動 1 回** 実行（バックフィル用、`gcloud run jobs execute ingestion --update-env-vars=INGESTION_MODE=initial`）

**非責務**:
- スキーマ管理（U-CSS の責務、本 Unit が動く前提として CMS スキーマが揃っていること）
- 通知配信そのもの（U-NTF の責務、本 Unit は Pub/Sub publish までで終了）
- Item の読み取り API（U-BFF の責務）
- Drift 検出（U-CSS の責務、Item 単位の drift は概念的に扱わない）

### 1.2 Domain Model（`internal/safetyincident/domain`）

#### 1.2.1 Entity / Value Object

```go
// MailItem — MOFA XML から 1 件抽出した未加工データ
type MailItem struct {
    KeyCd       string    // MOFA 一意 ID
    InfoType    string    // 情報種別（危険・スポット等）
    InfoName    string
    LeaveDate   time.Time // 発出日時 (UTC)
    Title       string
    Lead        string
    MainText    string
    InfoURL     string
    KoukanCd    string
    KoukanName  string
    AreaCd      string
    AreaName    string
    CountryCd   string    // ISO 3166-1 alpha-2 / MOFA 国コード
    CountryName string
}

// SafetyIncident — MailItem を加工した最終形（CMS に upsert される）
type SafetyIncident struct {
    MailItem
    ExtractedLocation string         // LLM 抽出地名
    Geometry          Point          // WGS84 座標
    GeocodeSource     GeocodeSource  // mapbox / country_centroid
    IngestedAt        time.Time      // 取込時刻
    UpdatedAt         time.Time      // 最終更新時刻
}

// Point — WGS84 座標
type Point struct {
    Lat float64 // [-90, 90]
    Lng float64 // [-180, 180]
}

// GeocodeSource — 座標の出所（drift 警告と同じ列挙）
type GeocodeSource int
const (
    GeocodeSourceUnspecified GeocodeSource = iota
    GeocodeSourceMapbox
    GeocodeSourceCountryCentroid
)

// String() は wire 名を返す: "mapbox" / "country_centroid"
```

#### 1.2.2 Port（Outbound）

```go
// MofaSource — MOFA エンドポイントから XML を取得して MailItem に変換する
type MofaSource interface {
    Fetch(ctx context.Context, mode IngestionMode) ([]MailItem, error)
}

type IngestionMode string
const (
    IngestionModeInitial     IngestionMode = "initial"      // 00A.xml
    IngestionModeIncremental IngestionMode = "incremental"  // newarrivalA.xml
)

// LocationExtractor — title + lead + main_text から地名を抽出する (LLM 実装)
type LocationExtractor interface {
    Extract(ctx context.Context, item MailItem) (ExtractResult, error)
}

type ExtractResult struct {
    Location   string  // 抽出した地名（空文字 = 抽出失敗）
    Confidence float64 // 0.0-1.0
}

// Geocoder — 地名 + 国コードから座標を得る (Mapbox + Centroid フォールバックの合成)
type Geocoder interface {
    Geocode(ctx context.Context, location, countryCd string) (GeocodeResult, error)
}

type GeocodeResult struct {
    Point  Point
    Source GeocodeSource
}

// Repository — CMS への永続化と存在確認
type Repository interface {
    Exists(ctx context.Context, keyCd string) (bool, error)
    Upsert(ctx context.Context, incident SafetyIncident) error
}

// EventPublisher — Pub/Sub topic への通知
type EventPublisher interface {
    PublishNewArrival(ctx context.Context, ev NewArrivalEvent) error
}

type NewArrivalEvent struct {
    KeyCd     string
    CountryCd string
    InfoType  string
    Geometry  Point
    LeaveDate time.Time
}
```

### 1.3 Application Layer（`internal/safetyincident/application`）

#### 1.3.1 `IngestUseCase`

```go
type IngestUseCase struct {
    source    MofaSource
    extractor LocationExtractor
    geocoder  Geocoder
    repo      Repository
    publisher EventPublisher
    clock     clock.Clock
    logger    *slog.Logger
    tracer    trace.Tracer
    meter     metric.Meter

    llmLimiter     *ratelimit.Limiter // 5 req/s
    geocodeLimiter *ratelimit.Limiter // 10 req/s
    concurrency    int                // 5
}

type IngestInput struct {
    Mode IngestionMode
}

type IngestResult struct {
    Fetched   int                       // MOFA から取得した件数
    Skipped   int                       // 重複排除で skip した件数
    Processed int                       // 成功して CMS に upsert した件数
    Failed    map[Phase]int             // フェーズ別失敗件数
    Published int                       // Pub/Sub に publish した件数
}

type Phase string
const (
    PhaseFetch     Phase = "fetch"      // MOFA fetch
    PhaseLookup    Phase = "lookup"     // CMS Exists
    PhaseExtract   Phase = "extract"    // LLM
    PhaseGeocode   Phase = "geocode"    // Mapbox / Centroid
    PhaseUpsert    Phase = "upsert"     // CMS Upsert
    PhasePublish   Phase = "publish"    // Pub/Sub
)

func (u *IngestUseCase) Execute(ctx context.Context, in IngestInput) (IngestResult, error)
```

**アルゴリズム**:
```
1. items = source.Fetch(ctx, in.Mode)
   - エラー時は Run 全体 fail（exit 1）。fetch できなければ続行不能
2. result.Fetched = len(items)
3. errgroup で並列度 = u.concurrency 個まで:
   For each item in items:
     a. exists = repo.Exists(ctx, item.KeyCd)
        if err != nil → Failed[PhaseLookup]++、continue (skip)
        if exists → Skipped++、continue
     b. extract = llmLimiter.Wait → extractor.Extract(ctx, item)
        if err != nil → Failed[PhaseExtract]++、continue
        ※ extract.Location が空文字 → centroid に直行
     c. geocode = geocodeLimiter.Wait → geocoder.Geocode(ctx, extract.Location, item.CountryCd)
        if err != nil → Failed[PhaseGeocode]++、continue
        ※ Geocoder 内部で Mapbox 失敗時に Centroid フォールバック (合成)
     d. incident = build(item, extract, geocode, clock.Now())
     e. repo.Upsert(ctx, incident)
        if err != nil → Failed[PhaseUpsert]++、continue
     f. publisher.PublishNewArrival(ctx, NewArrivalEvent{...})
        if err != nil → Failed[PhasePublish]++、log Warn（CMS には入っているのでスキップ可）
     g. result.Processed++、Published++
4. return result, nil  (Run は exit 0、ログ・Metric で結果報告)
```

**Observability**:
- Span: `ingestion.Run` (root) → `ingestion.ProcessItem` (per item) → `cms.GetItem` / `llm.Extract` / `geocode.Geocode` / `cms.UpsertItem` / `pubsub.Publish`
- Metric:
  - `app.ingestion.run.fetched` (Counter, attr: mode)
  - `app.ingestion.run.skipped` (Counter)
  - `app.ingestion.run.processed` (Counter)
  - `app.ingestion.run.failed` (Counter, attr: phase)
  - `app.ingestion.run.published` (Counter)
  - `app.ingestion.geocode.fallback` (Counter, attr: source)
  - `app.ingestion.run.duration` (Histogram, ms)

### 1.4 Infrastructure Adapters（`internal/safetyincident/infrastructure`）

#### 1.4.1 `mofa/source.go`

- HTTP GET で MOFA XML 取得（`MofaSource` 実装）
- URL は `INGESTION_MOFA_BASE_URL` で settable（デフォルト `https://www.ezairyu.mofa.go.jp/html/opendata`）
- モード切替: `Fetch(ctx, IngestionModeInitial)` → `/00A.xml`、`Fetch(ctx, IngestionModeIncremental)` → `/newarrivalA.xml`
- XML パーサー: encoding/xml で構造体に decode、`MailItem` に変換
- `httpx.GetWithRetry` で 5xx / 429 を retry（U-PLT `platform/retry`）

#### 1.4.2 `llm/claude.go`

- Anthropic Claude Haiku を呼び出す `LocationExtractor` 実装
- プロンプトテンプレート（system + user 分離）:
  ```
  System: あなたは外務省 海外安全情報の本文から発生地名を抽出する地理アシスタントです。
  
  以下のフォーマットで JSON のみを返してください:
  {"location": "<地名 or 空文字>", "confidence": 0.0-1.0}
  
  地名は地理的に最も具体的なものを選んでください (例: "東京都新宿区"、"パリ 9 区"、"ジャカルタ南部")。
  抽出できない場合は location を空文字、confidence を 0 にしてください。
  
  User:
  Title: {{.Title}}
  Lead: {{.Lead}}
  MainText: {{.MainText}}
  ```
- 並列度制御は呼び出し側 (Application 層の errgroup)
- response の JSON parse 失敗 → ExtractResult{Location: "", Confidence: 0}, err = nil として fallback (Centroid に流れる)

#### 1.4.3 `geocode/chain.go` — Mapbox + Centroid 合成

- **Outer**: `Geocoder` interface 実装、Mapbox → Centroid の順に試行
  ```go
  type ChainGeocoder struct {
      mapbox   MapboxGeocoder
      centroid CentroidGeocoder
      logger   *slog.Logger
  }
  
  func (g *ChainGeocoder) Geocode(ctx context.Context, location, countryCd string) (GeocodeResult, error) {
      if location != "" {
          if result, err := g.mapbox.Geocode(ctx, location); err == nil && result.Confidence >= 0.5 {
              return GeocodeResult{Point: result.Point, Source: GeocodeSourceMapbox}, nil
          }
          // Mapbox 失敗 / 信頼度低 → fall through
      }
      point, err := g.centroid.Lookup(countryCd)
      if err != nil {
          return GeocodeResult{}, errs.Wrap("geocode.centroid_lookup", errs.KindNotFound, err)
      }
      return GeocodeResult{Point: point, Source: GeocodeSourceCountryCentroid}, nil
  }
  ```
- **MapboxGeocoder**: `internal/platform/mapboxx` の Client を使用、`/geocoding/v5/mapbox.places/{location}.json?country={iso}` を叩く
- **CentroidGeocoder**: 静的 JSON `country_centroids.json`（ISO 3166-1 alpha-2 → {lat, lng}） を `go:embed` で埋め込んで lookup

#### 1.4.4 `cms/repository.go` — `cmsx` 経由

- `cmsx.Client` に **U-ING で新規追加** するメソッド:
  - `GetItemByFieldValue(ctx, projectAlias, modelAlias, fieldAlias, value string) (*ItemDTO, error)` — 重複排除用 (404 → nil, nil)
  - `UpsertItemByFieldValue(ctx, projectAlias, modelAlias, fieldAlias, value string, item ItemDTO) error` — 既存があれば PATCH、無ければ POST
- 内部で `errs.Kind*` に変換 (KindNotFound / KindConflict / KindUnauthorized / KindExternal)

#### 1.4.5 `eventbus/publisher.go` — Pub/Sub publisher

- `internal/platform/pubsubx` の Client を使用
- Topic = `safety-incident.new-arrival` (U-PLT shared infra で作成済み)
- メッセージは `proto/v1/pubsub.proto` の `NewArrivalEvent` を proto エンコード

### 1.5 Composition Root — `cmd/ingestion/main.go`（拡張計画）

```go
type ingestionConfig struct {
    config.Common
    Mode                  string `envconfig:"INGESTION_MODE" default:"incremental"`  // initial | incremental
    MofaBaseURL           string `envconfig:"INGESTION_MOFA_BASE_URL" default:"https://www.ezairyu.mofa.go.jp/html/opendata"`
    CMSBaseURL            string `envconfig:"INGESTION_CMS_BASE_URL" required:"true"`
    CMSWorkspaceID        string `envconfig:"INGESTION_CMS_WORKSPACE_ID" required:"true"`
    CMSIntegrationToken   string `envconfig:"INGESTION_CMS_INTEGRATION_TOKEN" required:"true"`
    CMSProjectAlias       string `envconfig:"INGESTION_CMS_PROJECT_ALIAS" default:"overseas-safety-map"`
    CMSModelAlias         string `envconfig:"INGESTION_CMS_MODEL_ALIAS" default:"safety-incident"`
    ClaudeAPIKey          string `envconfig:"INGESTION_CLAUDE_API_KEY" required:"true"`
    ClaudeModel           string `envconfig:"INGESTION_CLAUDE_MODEL" default:"claude-haiku-4-5"`
    MapboxAPIKey          string `envconfig:"INGESTION_MAPBOX_API_KEY" required:"true"`
    PubSubTopicID         string `envconfig:"INGESTION_PUBSUB_TOPIC_ID" required:"true"`
    Concurrency           int    `envconfig:"INGESTION_CONCURRENCY" default:"5"`
    LLMRateLimit          int    `envconfig:"INGESTION_LLM_RATE_LIMIT" default:"5"`     // req/s
    GeocodeRateLimit      int    `envconfig:"INGESTION_GEOCODE_RATE_LIMIT" default:"10"` // req/s
}

func main() {
    if err := run(); err != nil {
        slog.Error("ingestion failed", "err", err)
        os.Exit(1)
    }
}

func run() error {
    var cfg ingestionConfig
    config.MustLoad(&cfg)
    // ... observability.Setup ...
    // ... cmsx.NewClient / mapboxx.NewClient / claude client / pubsub client ...
    
    usecase := application.NewIngestUseCase(
        mofaSource, llmExtractor, chainGeocoder, cmsRepo, pubsubPublisher,
        clock.Real{}, logger, tracer, meter,
        cfg.LLMRateLimit, cfg.GeocodeRateLimit, cfg.Concurrency,
    )
    
    result, err := usecase.Execute(ctx, application.IngestInput{
        Mode: domain.IngestionMode(cfg.Mode),
    })
    if err != nil {
        // fetch-level failure only (per-item failures are inside result)
        return err
    }
    
    logger.InfoContext(ctx, "ingestion finished",
        "mode", cfg.Mode,
        "fetched", result.Fetched,
        "skipped", result.Skipped,
        "processed", result.Processed,
        "failed_lookup", result.Failed[application.PhaseLookup],
        "failed_extract", result.Failed[application.PhaseExtract],
        "failed_geocode", result.Failed[application.PhaseGeocode],
        "failed_upsert", result.Failed[application.PhaseUpsert],
        "failed_publish", result.Failed[application.PhasePublish],
        "published", result.Published,
    )
    return nil
}
```

### 1.6 Sequence

```
Cloud Scheduler (5min cron)
   ↓ HTTP target (Cloud Run Job invoke)
ingestion Job
   ↓
MofaSource.Fetch(incremental) → [MailItem × N]
   ↓ for each (concurrency=5)
Repository.Exists(keyCd) ── true → skip
   ↓ false
LLM.Extract → location
   ↓
Geocoder.Geocode(location, countryCd)
   ├─ Mapbox 成功 → Point + GeocodeSourceMapbox
   └─ Mapbox 失敗 / 信頼度低 → CountryCentroid → Point + GeocodeSourceCountryCentroid
   ↓
Repository.Upsert(incident)
   ↓
EventPublisher.PublishNewArrival(NewArrivalEvent) → Pub/Sub
   ↓
(loop end)
   ↓
Run summary log + Metric flush → exit 0
```

---

## 2. NFR Requirements（U-ING 固有）

U-PLT の NFR 要件（[`U-PLT/nfr-requirements/nfr-requirements.md`](../../U-PLT/nfr-requirements/nfr-requirements.md)）を **前提として継承**。以下は U-ING 固有値。

### 2.1 性能

- **NFR-ING-PERF-01**: 5 分毎の `incremental` Run 完了時間 **< 60 秒**（新着 0〜30 件想定、並列度 5 + LLM 5 req/s で 6 秒 + Mapbox 3 秒 + CMS 数秒）
- **NFR-ING-PERF-02**: `initial` Run の完了時間 **< 30 分**（数千件想定、並列度 5 + ratelimit で逆算）
- **NFR-ING-PERF-03**: per-item 処理時間 (extract → geocode → upsert → publish) **p95 < 5 秒**

### 2.2 セキュリティ

- **NFR-ING-SEC-01**: 3 つの Secret (`ingestion-claude-api-key`, `ingestion-mapbox-api-key`, `cms-integration-token`) は GCP Secret Manager に保管、`env.value_source.secret_key_ref` で注入
- **NFR-ING-SEC-02**: ログに API key / Token を出さない (slog の attr filter + Marshaler で redact)
- **NFR-ING-SEC-03**: Runtime SA (`ingestion-runtime`) は Secret 読み取り + Pub/Sub publish + OTel 送信のみの最小権限
- **NFR-ING-SEC-04**: MOFA 本文 (PII の可能性あり) は `slog.Debug` のみ、`Info` 以上には出さない

### 2.3 信頼性 / 冪等性

- **NFR-ING-REL-01**: 同じ XML を N 回処理しても CMS には 1 件しか登録されない (Q3 [A] CMS lookup による idempotency)
- **NFR-ING-REL-02**: 部分失敗 (一部 item で LLM/Mapbox/CMS エラー) で Run 全体は exit 0、失敗 item は次回 Run で自然リトライ (Q7 [A])
- **NFR-ING-REL-03**: MOFA fetch 失敗 (5xx, 429, network) は `retry.Do` で吸収。それでもダメなら Run 全体 fail (exit 1) して Cloud Scheduler 履歴で気付かせる
- **NFR-ING-REL-04**: Mapbox 失敗時は **必ず** Centroid にフォールバック (Q5 [A])。Centroid lookup も失敗するケース (未知の `country_cd`) は `Failed[PhaseGeocode]++` で skip

### 2.4 運用 / 可観測性

- **NFR-ING-OPS-01**: 構造化 JSON ログ (slog) 必須属性: `service.name=ingestion`, `env`, `trace_id`, `span_id`, `app.ingestion.phase`
- **NFR-ING-OPS-02**: OTel Metric 一式 (§1.3.1 参照)
- **NFR-ING-OPS-03**: Run 末に summary INFO ログ (`fetched` / `skipped` / `processed` / `failed_*` / `published`)
- **NFR-ING-OPS-04**: 失敗 item は **必ず** ERROR ログ + `key_cd` + `phase` + `err` を含める (運用者が grep で特定可能)

### 2.5 テスト / 品質

- **NFR-ING-TEST-01**: Domain 層 (`MailItem`, `SafetyIncident`, `Point`) に PBT (rapid)
  - Point の lat/lng が WGS84 範囲内 (∀ valid input)
  - Geocoder chain の合成 (Mapbox 失敗 → Centroid に切替) が安全に成立
- **NFR-ING-TEST-02**: Application 層 `IngestUseCase` を fake 実装 5 種で組み立てたシナリオテスト 5〜6 本
  - 初回モード / 差分モード / 重複排除 / フォールバック / 部分失敗 / publish 失敗
- **NFR-ING-TEST-03**: MOFA XML パーサーは `testdata/mofa/sample_*.xml` の fixture で回帰テスト
- **NFR-ING-TEST-04**: LLM / Mapbox / CMS / Pub/Sub の HTTP は `httptest` でモック (任意、最小 1 本ずつ)
- **NFR-ING-TEST-05**: カバレッジ (Q9 [A] 層別目標)
  - domain: **95%+**
  - application: **90%+**
  - infrastructure: **70%+**
  - 全体: **85%+**

### 2.6 拡張性

- **NFR-ING-EXT-01**: 新しい `LocationExtractor` 実装 (例: GPT 系、Gemini) に差し替えできる Port 設計
- **NFR-ING-EXT-02**: 新しい `Geocoder` 実装 (例: Google Maps Geocoding) を Chain に追加できる構造
- **NFR-ING-EXT-03**: MOFA 以外のソース追加 (例: 米国 State Department) は `MofaSource` Port の別実装で対応可能

---

## 3. NFR Design Patterns（U-ING 固有）

### 3.1 Skip-and-Continue パターン (Q7 [A])

**問題**: 30 件処理中の 5 件目で LLM エラー。残り 25 件はどうするか。

**解法**: per-item の error は continue で吸収、Run 全体は exit 0、失敗は Metric / Log で記録。

```go
for _, item := range items {
    if err := u.processItem(ctx, item, &result); err != nil {
        u.logger.ErrorContext(ctx, "item processing failed",
            "key_cd", item.KeyCd,
            "phase", phaseOf(err),
            "err", err,
        )
        u.failedCounter.Add(ctx, 1, metric.WithAttributes(
            attribute.String("phase", phaseOf(err)),
        ))
        continue
    }
    result.Processed++
}
return result, nil  // 常に nil
```

**性質**:
- **Liveness**: 部分失敗でも残り item は処理される
- **Self-healing**: 失敗 item は CMS に未登録のまま残り、次回 Run の `Exists(keyCd)` で再試行される (Q3 [A] と組み合わせ)
- **観測性**: 失敗は Metric / Log で別レイヤから検知 (例: `app.ingestion.run.failed > N` で Cloud Monitoring アラート)

### 3.2 Geocoder Chain パターン (Q5 [A])

**問題**: 3 段階 (LLM 抽出 → Mapbox → Centroid) のフォールバックチェーン。

**解法**: `ChainGeocoder` が outer responsibility、各 stage は単純な Port 実装。

```go
type ChainGeocoder struct {
    mapbox   MapboxGeocoder
    centroid CentroidGeocoder
}

func (g *ChainGeocoder) Geocode(ctx context.Context, location, countryCd string) (GeocodeResult, error) {
    // Stage 1: LLM が地名抽出済みなら Mapbox を試す
    if location != "" {
        if result, err := g.mapbox.Geocode(ctx, location); err == nil && result.Confidence >= 0.5 {
            return GeocodeResult{Point: result.Point, Source: GeocodeSourceMapbox}, nil
        }
        // Mapbox 失敗 / 信頼度低 → fall through
    }
    // Stage 2: Centroid フォールバック (常に成功する想定だが unknown country は err)
    point, err := g.centroid.Lookup(countryCd)
    if err != nil {
        return GeocodeResult{}, errs.Wrap("geocode.centroid_lookup", errs.KindNotFound, err)
    }
    return GeocodeResult{Point: point, Source: GeocodeSourceCountryCentroid}, nil
}
```

**性質**:
- **Open-Closed**: 新しい stage (例: Google Maps) は Chain の中段に挿入できる
- **Failure isolation**: 上位 stage の failure は下位 stage に伝播しない (always 何らかの結果を返す、または最終 stage の err を返す)
- **観測性**: Geocoder 内部で Source を識別できるので Flutter 側 / Metric 側で fallback 率が見える

### 3.3 Idempotent Upsert パターン (Q3 [A])

**問題**: MOFA は全件返してくるので、同じ `key_cd` を毎回処理すると LLM/Mapbox を無駄に叩く。

**解法**: Repository に `Exists(keyCd)` を持たせ、Application 層で **LLM/Mapbox を呼ぶ前に short-circuit**。

```go
exists, err := u.repo.Exists(ctx, item.KeyCd)
if err != nil { /* count + continue */ }
if exists {
    result.Skipped++
    continue  // LLM, Mapbox, CMS, Pub/Sub すべて skip
}
```

**性質**:
- **Cost saving**: 重複 item ごとに LLM 1 回 + Mapbox 1 回 + CMS upsert 1 回が省ける (大きい)
- **Source of Truth**: CMS が唯一の真実、local cache 不要
- **Race condition**: 並列実行でも CMS 側で 2 回 upsert になる程度 (重複 publish はあり得るが U-NTF 側で Pub/Sub の dedup でカバー予定 — U-NTF Design で扱う)

### 3.4 Rate Limiting パターン (Q8 [A])

**問題**: LLM / Mapbox の API レート制限 (Mapbox 600 req/min) を超えると 429 ストーム。

**解法**: `platform/ratelimit` で **app 側で先制的に絞る**。errgroup の semaphore は並列度上限、ratelimit は req/s 上限の二段構え。

```go
llmLimiter := ratelimit.New(5, 5)        // 5 req/s, burst 5
geocodeLimiter := ratelimit.New(10, 10)  // 10 req/s, burst 10

// 各 stage の前で:
if err := u.llmLimiter.Wait(ctx); err != nil { /* ctx canceled */ }
extract, err := u.extractor.Extract(ctx, item)
```

**性質**:
- **Predictable**: 自分の API 使用率を事前に予測可能、本番で Mapbox の 429 を踏むことが原理的にない
- **Fair**: 並列度 = 5 で同時に複数 item が rate limiter で wait するが、長時間 starvation にはならない
- **Burst tolerance**: 短時間バースト (Run 開始直後の数秒) は burst で吸収、定常状態で req/s に収束

### 3.5 Mode Switching パターン (Q1 [A])

**問題**: `initial` (一括バックフィル) と `incremental` (5 分毎差分) を 1 つの Job バイナリで両対応する。

**解法**: `INGESTION_MODE` env で MOFA URL を切替、それ以外のロジックは共通。

```go
type IngestInput struct {
    Mode IngestionMode
}

func (s *MofaSource) Fetch(ctx context.Context, mode IngestionMode) ([]MailItem, error) {
    var path string
    switch mode {
    case IngestionModeInitial:
        path = "/00A.xml"
    case IngestionModeIncremental:
        path = "/newarrivalA.xml"
    default:
        return nil, errs.Wrap("mofa.invalid_mode", errs.KindInvalidInput, fmt.Errorf("mode=%s", mode))
    }
    // ... HTTP fetch + parse
}
```

**性質**:
- **DRY**: Application 層 (`IngestUseCase.Execute`) は mode に依存しない、source の差し替えだけで動く
- **Operability**: 運用者は Cloud Scheduler 設定 (incremental) と手動コマンド (initial) を別々に管理
- **Cost**: `initial` は手動 1 回、`incremental` は 5 分毎自動、で意図通りのコスト分布

---

## 4. 運用ランブック (簡略、詳細は Build and Test で)

### 4.1 通常運用 (incremental)

Cloud Scheduler が `*/5 * * * *` で `cms-migrate` Job を起動。運用者の操作不要。

### 4.2 障害時の復旧

1. Cloud Logging で `severity=ERROR` を確認
2. `app.ingestion.phase` で失敗段階を特定 (fetch / extract / geocode / upsert / publish)
3. 必要な対処 (API key 更新、CMS スキーマ確認、MOFA 障害情報確認)
4. **何もしなくても次の Run 5 分後に自動リトライ** (Q3 + Q7 の合わせ技)

### 4.3 初回バックフィル

```bash
gcloud run jobs execute ingestion \
  --region=asia-northeast1 \
  --update-env-vars=INGESTION_MODE=initial \
  --wait
```

数千件処理で 30 分程度。完了後は `INGESTION_MODE` を戻す (Cloud Scheduler 経由で incremental 起動するので env override は実行時のみ).

### 4.4 LLM / Mapbox の API Key Rotation

`gcloud secrets versions add` で新 version 投入 → 次の Run で自動反映 (`version = "latest"`)。

---

## 5. 次フェーズ（Infrastructure Design）での未決事項

本 design で **決めない**もの (U-ING Infrastructure Design へ持ち越し):

- Cloud Run Job の `cpu` / `memory` / `task.timeout` / `max_retries` 数値
- Cloud Scheduler の auth 方法 (OIDC token、SA = `ingestion-scheduler` 等)
- Pub/Sub Topic の retention policy / DLQ 設定 (U-PLT で土台はあるが U-NTF 連携で確認)
- Runtime SA に必要な追加 IAM (`pubsub.publisher` 等)
- Mapbox / Anthropic Secret の値投入手順

---

## 6. トレーサビリティ

| 上位要件 | U-ING 対応 |
|---|---|
| US-01〜US-13 (MVP Stories) | §1.1 Context (取込パイプラインの実体) |
| NFR-FUN-04 (位置不明時のフォールバック) | §1.4.3 ChainGeocoder、Q5 [A] |
| NFR-SEC-01 (Secret 管理) | NFR-ING-SEC-01/02/03 |
| NFR-OPS-01 (構造化ログ) | NFR-ING-OPS-01/02/03/04 |
| NFR-EXT-01 (拡張性 / DIP) | §1.2.2 Port、NFR-ING-EXT-01/02/03 |
| NFR-PERF (定期実行のスループット) | NFR-ING-PERF-01/02/03、§3.4 Rate Limiting |
| NFR-REL (冪等性 / 再実行性) | NFR-ING-REL-01/02/03/04、§3.1 Skip + §3.3 Idempotent Upsert |

---

## 7. 承認プロセス

- **本ドキュメントの承認**: ユーザーレビュー → LGTM で次ステップへ
- **次ステップ**: U-ING Infrastructure Design (`construction/U-ING/infrastructure-design/`)
  - U-PLT で `terraform/modules/ingestion/` の雛形 + Cloud Scheduler の足場が既にあるため Infra も薄い見込み
  - Pub/Sub publisher IAM、3 つの Secret、Mapbox API key の Secret Manager 投入手順が中心
