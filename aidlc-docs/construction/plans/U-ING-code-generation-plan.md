# U-ING Code Generation Plan

## Overview

U-ING (Ingestion Unit、Sprint 2) の Code Generation 計画。[`U-ING/design/U-ING-design.md`](../U-ING/design/U-ING-design.md) と [`U-ING/infrastructure-design/`](../U-ING/infrastructure-design/) に基づいて実装する。

## Goals

- MOFA XML を定期取得し、LLM 地名抽出 → Mapbox Geocoding → 国 Centroid フォールバックを経て CMS に upsert、新着を Pub/Sub publish する Cloud Run Job `ingestion` を完成させる
- U-PLT 共通規約 (slog / OTel / envconfig / errs / retry / ratelimit / Clock) と U-CSS の `cmsx.Client` (Project/Model/Field CRUD) を **踏襲する**
- U-CSS と同じく **実 API 疎通確認は Build and Test で手動実施**。Code Gen 完了時点で `cmd/ingestion` は envconfig が揃えば起動する

## Non-Goals

- reearth-cms 本体のデプロイ / 管理 (U-CSS と同じく外部既存を利用)
- Item 読み取り API (U-BFF の責務)
- 通知配信 (U-NTF の責務、本 Unit は Pub/Sub publish まで)
- 国 Centroid データの外部公開 / メンテ (static JSON を embed)

---

## Step-by-step Checklist (計画段階、承認後に実装)

### Phase 1: Domain Layer

- [ ] 1-1. `internal/safetyincident/domain/mail_item.go`
  - `MailItem` struct (key_cd, info_type, ..., country_name の MOFA 生データ 14 フィールド)
  - `MailItem.Validate()` — key_cd / leave_date / title / country_cd の必須チェック
- [ ] 1-2. `internal/safetyincident/domain/safety_incident.go`
  - `SafetyIncident` struct (MailItem embed + ExtractedLocation / Geometry / GeocodeSource / IngestedAt / UpdatedAt)
  - `Build(item MailItem, extract ExtractResult, geocode GeocodeResult, now time.Time) SafetyIncident`
- [ ] 1-3. `internal/safetyincident/domain/point.go`
  - `Point struct { Lat, Lng float64 }`
  - `Point.Validate()` — WGS84 範囲 (lat ∈ [-90, 90], lng ∈ [-180, 180])
- [ ] 1-4. `internal/safetyincident/domain/geocode_source.go`
  - `GeocodeSource` enum (`Unspecified` / `Mapbox` / `CountryCentroid`)
  - `String()` wire 名
- [ ] 1-5. `internal/safetyincident/domain/ports.go`
  - 5 Port interface: `MofaSource` / `LocationExtractor` / `Geocoder` / `Repository` / `EventPublisher`
  - DTO: `ExtractResult` / `GeocodeResult` / `NewArrivalEvent` / `IngestionMode`
- [ ] 1-6. Tests:
  - `mail_item_test.go` (Validate のテーブル駆動)
  - `point_test.go` (PBT: WGS84 不変条件、rapid で ±10 の範囲外入力で Validate が reject)
  - `safety_incident_test.go` (Build の合成ロジック: ExtractedLocation 空文字、Geocoder Source の伝播)

### Phase 2: Application Layer

- [ ] 2-1. `internal/safetyincident/application/ingest_usecase.go`
  - `IngestUseCase` struct (5 Port + clock + logger + tracer + meter + llm/geocode Limiter + concurrency)
  - `Execute(ctx, IngestInput) (IngestResult, error)` — アルゴリズム (design §1.3.1 に基づく)
  - per-item 処理を `processItem` に抽出、errgroup で並列実行
  - skip-and-continue + idempotent upsert (Q3/Q7 [A])
  - OTel Span + Metric 更新
- [ ] 2-2. `internal/safetyincident/application/result.go`
  - `IngestResult` + `Phase` 定数 (`fetch` / `lookup` / `extract` / `geocode` / `upsert` / `publish`)
- [ ] 2-3. `internal/safetyincident/application/fake_test.go`
  - 5 Port の fake 実装 (in-memory map)
  - `fakeMofaSource` / `fakeLocationExtractor` / `fakeGeocoder` / `fakeRepository` / `fakeEventPublisher`
  - 各 fake に「次の呼び出しで error を返す」フックを仕込む (skip-continue シナリオ用)
- [ ] 2-4. `internal/safetyincident/application/ingest_usecase_test.go`
  - シナリオ 6 本: 初回モード / 差分モード / 重複排除 (exists=true) / Mapbox 失敗 → Centroid フォールバック / 部分失敗 (LLM/Geocode/Upsert 個別) / publish 失敗
  - `IngestResult.Fetched/Skipped/Processed/Failed/Published` の整合性を検証

### Phase 3: Infrastructure Adapter — MOFA

- [ ] 3-1. `internal/safetyincident/infrastructure/mofa/xml_types.go`
  - `mofaXML` struct with encoding/xml tags (RSS-like / Atom-like の両対応かは実物を見て決定、design では仮定)
- [ ] 3-2. `internal/safetyincident/infrastructure/mofa/source.go`
  - `MofaSource` 実装、`Fetch(ctx, mode)` で URL 切替
  - HTTP GET (5xx/429 は `retry.Do`)、body parse、`MailItem` へ変換
  - レスポンスが想定形式でなかったら KindExternal で包んで fail
- [ ] 3-3. `internal/safetyincident/infrastructure/mofa/testdata/`
  - `sample_newarrival.xml` (5 件程度の代表 item)
  - `sample_initial.xml` (同じく数件で OK、parser 検証用)
- [ ] 3-4. `internal/safetyincident/infrastructure/mofa/source_test.go`
  - fixture からのパーステスト (全 14 フィールドの decode)
  - `httptest` で URL 切替 (initial/incremental) を検証
  - 5xx retry 動作の検証

### Phase 4: Infrastructure Adapter — LLM (Claude)

- [ ] 4-1. `internal/platform/llm/claude.go` (新規 package、将来他 LLM でも使える基盤)
  - `Client` 構造体 (base URL, API key, model name, http.Client)
  - 低レベルメソッド `Complete(ctx, systemPrompt, userPrompt string) (string, error)` — Claude API `/v1/messages` エンドポイント
  - 429 / 5xx は `retry.Do`、application 層で rate limit 済み
- [ ] 4-2. `internal/safetyincident/infrastructure/llm/extractor.go`
  - `LocationExtractor` 実装、prompt テンプレート (design §1.4.2)
  - レスポンスの JSON parse (Location / Confidence)、parse 失敗は empty + confidence=0 で返す (centroid に流れる)
- [ ] 4-3. `internal/safetyincident/infrastructure/llm/extractor_test.go`
  - `httptest` で Claude API モック
  - 正常 / JSON parse fail / API error の各パス

### Phase 5: Infrastructure Adapter — Geocoder Chain

- [ ] 5-1. `internal/safetyincident/infrastructure/geocode/mapbox.go`
  - `MapboxGeocoder` 実装、`/geocoding/v5/mapbox.places/{location}.json` を `mapboxx.Client` 経由で叩く
  - レスポンスから relevance と座標を取り出す
  - relevance < 0.5 は失敗扱い (err return、ChainGeocoder で centroid に流れる)
- [ ] 5-2. `internal/safetyincident/infrastructure/geocode/centroid.go`
  - `CentroidGeocoder` 実装
  - `go:embed` で `country_centroids.json` (ISO alpha-2 → {lat, lng}) を埋め込み
  - `Lookup(countryCd) (Point, error)` — unknown country → KindNotFound
- [ ] 5-3. `internal/safetyincident/infrastructure/geocode/country_centroids.json`
  - ISO 3166-1 alpha-2 → 国 centroid の静的データ
  - 出典: public domain のデータセット (natural earth や CC0) を使用
  - MOFA `country_cd` は ISO 準拠なのでそのまま lookup 可能
- [ ] 5-4. `internal/safetyincident/infrastructure/geocode/chain.go`
  - `ChainGeocoder` 実装、design §3.2 に従ってフォールバック
- [ ] 5-5. Tests:
  - `mapbox_test.go`: httptest で Mapbox モック、relevance 閾値判定
  - `centroid_test.go`: 既知国の lookup + unknown country の KindNotFound
  - `chain_test.go`: fake Mapbox/Centroid で 3 パス (Mapbox 成功 / Mapbox 失敗 → centroid 成功 / 両方失敗)

### Phase 6: Infrastructure Adapter — CMS Repository

- [ ] 6-1. `internal/platform/cmsx/item.go` (U-CSS の拡張)
  - `GetItemByFieldValue(ctx, modelID, fieldAlias, value string) (*ItemDTO, error)` — 404 → (nil, nil)、重複排除用
  - `UpsertItemByFieldValue(ctx, modelID, fieldAlias, value string, fields map[string]any) error` — GET して ID 取得 → PATCH または POST
  - `ItemDTO`、`FieldValue` DTO
- [ ] 6-2. `internal/platform/cmsx/item_test.go`
  - httptest でモック、GET 200/404、POST 201、PATCH 200 のパス
- [ ] 6-3. `internal/safetyincident/infrastructure/cms/repository.go`
  - `Repository` 実装、`cmsx.Client` に委譲
  - `Exists(ctx, keyCd)` → `GetItemByFieldValue` の nil 判定
  - `Upsert(ctx, incident)` → 19 フィールドを map に詰めて `UpsertItemByFieldValue`
- [ ] 6-4. `internal/safetyincident/infrastructure/cms/repository_test.go`
  - stub cmsx.Client を interface 化して注入 (U-CSS と同じパターン)

### Phase 7: Infrastructure Adapter — Pub/Sub Publisher

- [ ] 7-1. `internal/safetyincident/infrastructure/eventbus/publisher.go`
  - `EventPublisher` 実装、`internal/platform/pubsubx` を使用
  - `NewArrivalEvent` を proto (gen/go/v1/pubsub.pb.go) にシリアライズして publish
  - Pub/Sub attributes: `key_cd` / `country_cd` / `info_type`
- [ ] 7-2. `internal/safetyincident/infrastructure/eventbus/publisher_test.go`
  - `pstest` (Pub/Sub emulator のテストライブラリ) か `pubsubx` を interface 化
  - proto encode を検証

### Phase 8: Composition Root

- [ ] 8-1. `cmd/ingestion/main.go` を拡張 (現状は U-PLT スケルトン)
  - `ingestionConfig` struct (design §1.5 参照)
  - `observability.Setup` → DI 配線 (全 5 Port + Client)
  - `usecase.Execute` 呼び出し、summary ログ、fail-fast (Run 全体失敗時のみ)
  - `run()` function + defer パターン (U-CSS の Copilot 対応と同じ)

### Phase 9: Terraform

- [ ] 9-1. `terraform/modules/ingestion/main.tf` に `max_retries = 0` を追加
- [ ] 9-2. `terraform/modules/ingestion/main.tf` に env 2 個追加 (`INGESTION_MODE`, `INGESTION_PUBSUB_TOPIC_ID`)
- [ ] 9-3. `terraform fmt` / `terraform init -backend=false` / `terraform validate` を通す

### Phase 10: Docs

- [ ] 10-1. `aidlc-docs/construction/U-ING/code/summary.md` を新規作成 (U-CSS と同じ形式)
- [ ] 10-2. `README.md` に ingestion セクション追記 (必須 env、ローカル実行手順、initial mode コマンド)
- [ ] 10-3. `aidlc-docs/aidlc-state.md` 更新

### Phase 11: CI / Verification

- [ ] 11-1. `go test ./... -race` 全緑
- [ ] 11-2. `go vet` / `gofmt -s` / `golangci-lint run` 全緑
- [ ] 11-3. `govulncheck` 全緑
- [ ] 11-4. Docker build `ingestion` 緑
- [ ] 11-5. カバレッジ `internal/safetyincident/` + `internal/platform/llm/` > 85%

---

## 設計上の要判断事項 (計画段階で確定したい)

### Question A — 実装範囲の分割 / PR 分割

U-ING の Code Gen は U-CSS (推定 +1,500 Go 行) より **さらに大きい** (推定 +3,000 行)。

A) **推奨**: **2 PR に分割**
  - **PR A**: Phase 1-7 (Domain + Application + 5 Infrastructure Adapter + 全テスト)
  - **PR B**: Phase 8-11 (Composition Root + Terraform + Docs + CI verification)
  - 利点: レビュー負荷を分散、PR A は「ピュアに Go のビジネスロジック」で見やすい
B) 1 PR で全部
  - U-CSS は 1 PR にまとめたが規模感が違う (+2,500 行 vs U-CSS +1,500)
  - レビュー巨大化、Copilot コメントも大量になる想定
C) 3 PR (domain+app / infra adapters / cmd+terraform+docs)
  - 依存関係的に 2 PR で十分、3 分割は過剰

[A]: A

### Question B — LLM プロバイダ抽象化の粒度

Q4 A で Claude Haiku を採用したが、将来 GPT-5 / Gemini などへの差し替えを想定:

A) **推奨**: **`LocationExtractor` Port のみを抽象化**
  - `internal/platform/llm/claude.go` は Claude 固有の HTTP client (薄い)
  - `internal/safetyincident/infrastructure/llm/extractor.go` が `LocationExtractor` を実装 (Claude 特化)
  - 将来 GPT に差し替える場合は `extractor.go` を別ファイル (`gpt_extractor.go`) で並走、Composition Root で選択
  - ✅ シンプル、YAGNI
B) **汎用 `LLMClient` interface を `platform/llm` に切り出す**
  - `type LLMClient interface { Complete(ctx, system, user string) (string, error) }`
  - Claude / GPT / Gemini すべて `LLMClient` 実装
  - extractor.go は `LLMClient` を受け取る
  - ✅ LLM 差し替えが最も容易
  - ⚠️ 現時点で差し替え予定がないなら過剰
C) A の亜流: `platform/llm` 自体作らず、Claude は `infrastructure/llm/claude_client.go` に同居
  - ✅ パッケージ数最小
  - ⚠️ 将来 BFF 側や他 Bounded Context でも LLM 使うなら platform に出したほうが良い (footprint が予想しやすい)

[A]: A

### Question C — MOFA XML のパーサー戦略

MOFA の XML は実サーバを見て形状確定が必要 (U-ING Design でも Q9 [A] で「実 API は Build and Test」と決めた):

A) **推奨**: **仮定した構造で先行実装**
  - Design §1.4.1 の XML 構造を `xml_types.go` に仮置き (standard RSS-like 想定)
  - fixture `sample_newarrival.xml` も仮のものを自作 (`testdata/mofa/`)
  - Build and Test で実 XML を取得 → 構造が違えば修正
  - ✅ 他 Phase と並行して進められる
  - ⚠️ 実 XML と乖離しているリスク、Build and Test で修正作業発生
B) **実 XML を先に取得して fixture を作る**
  - Code Gen 開始前に外務省のサイトから `00A.xml` / `newarrivalA.xml` を DL
  - 実物に合わせた struct を書く
  - ✅ Build and Test での手戻りが減る
  - ⚠️ 開始前の準備ステップが増える
C) XML ではなく JSON API を探す
  - ⚠️ MOFA 公式は XML のみ、選択肢なし
  - 除外

[A]: A

### Question D — Terraform 変更を本 PR に含めるか、別途か

U-ING Design で「実 CMS 接続は Build and Test で実施」方針。Terraform apply も同じく実運用フェーズ。

A) **推奨**: **Terraform 変更は本 PR に含める** (Phase 9)
  - U-CSS Code Gen と同じ流れ (Code + Terraform を同 PR で)
  - `terraform validate` は CI の `terraform-validate.yml` で緑化される
  - 実 apply は運用フェーズで
B) Terraform は別 PR に分離
  - Go / Terraform で関心分離
  - ⚠️ 2 PR をセットでマージしないとインフラとコードが整合しない期間ができる
  - 採用しない

[A]: A

### Question E — `country_centroids.json` データの出典

A) **推奨**: **[Natural Earth](https://www.naturalearthdata.com/) の admin 0 centroid** (public domain / CC0)
  - 国 centroid は約 250 件程度、JSON 形式で数十 KB
  - ISO 3166-1 alpha-2 コード付き、MOFA country_cd と互換
  - `go:embed` で埋め込み (実行時ファイルシステム不要)
B) 自前計算 (各国の首都座標を手書きで `centroid.go` にハードコード)
  - ⚠️ メンテ困難、分離独立国 (パレスチナなど) の扱いで困る
  - 採用しない
C) 外部 API を叩く
  - ⚠️ ランタイム依存 ↑、キャッシュ複雑度 ↑
  - 採用しない

[A]: A

### Question F — テスト戦略 (U-CSS と同じ方針で OK か確認)

U-CSS と同じ **層別カバレッジ** を継承:

A) **推奨**:
  - `internal/safetyincident/domain/`: **95%+** (PBT 含む)
  - `internal/safetyincident/application/`: **90%+** (fake シナリオ 6 本)
  - `internal/safetyincident/infrastructure/`: **70%+** (httptest ベース主要パス)
  - `internal/platform/llm/`: **70%+**
  - 全体: **85%+**
B) カバレッジ目標なし、質的テスト重視
C) 全パッケージで 90%+

[A]: A

---

## 承認前の最終確認 (回答確定)

- **Q A [A]**: PR 分割 = **2 PR** (PR A: Phase 1-7 Go ロジック、PR B: Phase 8-11 結線)
- **Q B [A]**: LLM 抽象化 = `LocationExtractor` Port のみ (`platform/llm/claude.go` は薄い concrete、YAGNI)
- **Q C [A]**: MOFA XML パーサー = 仮定構造で先行実装、実 XML は Build and Test で答え合わせ
- **Q D [A]**: Terraform = PR B に同梱 (U-CSS と同じ運用)
- **Q E [A]**: country centroid データ = Natural Earth 由来 CC0 (~250 国、go:embed)
- **Q F [A]**: カバレッジ目標 = U-CSS と同じ層別 (domain 95% / app 90% / infra 70% / 全体 85%)、PBT は point.Validate のみ

回答確定済み。Phase 1-11 を 2 PR で順次実装する。PR A → merge → PR B の順序。
