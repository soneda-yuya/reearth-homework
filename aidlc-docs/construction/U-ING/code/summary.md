# U-ING Code Generation — Summary

**Unit**: U-ING（Ingestion Unit）
**対象**: `cmd/ingestion`（Cloud Run Job、Cloud Scheduler 5 分毎、`incremental` モード）
**対応する計画**: [`U-ING-code-generation-plan.md`](../../plans/U-ING-code-generation-plan.md)
**上位設計**: [`U-ING/design/U-ING-design.md`](../design/U-ING-design.md)、[`U-ING/infrastructure-design/`](../infrastructure-design/)

---

## 1. 生成ファイル一覧

### Domain（`internal/safetyincident/domain/`）

| ファイル | 役割 |
|---|---|
| `mail_item.go` | `MailItem` 14 フィールド + `Validate()` 必須チェック |
| `point.go` | `Point{Lat, Lng}` + `Validate()` WGS84 範囲 |
| `geocode_source.go` | `GeocodeSource` enum (`mapbox` / `country_centroid`) + `String()` / `IsValid()` |
| `safety_incident.go` | `SafetyIncident` Aggregate + `Build(item, extract, geocode, now)` |
| `ports.go` | 5 Port (`MofaSource` / `LocationExtractor` / `Geocoder` / `Repository` / `EventPublisher`) + DTO |
| `*_test.go` | unit + PBT (point.Validate の WGS84 envelope 双方向) |

### Application（`internal/safetyincident/application/`）

| ファイル | 役割 |
|---|---|
| `result.go` | `IngestResult` + `Phase` 定数 (`fetch` / `validate` / `lookup` / `extract` / `geocode` / `upsert` / `publish`) |
| `ingest_usecase.go` | `IngestUseCase.Execute` — Validate（item） → Fetch → 並列 process per item with skip-and-continue |
| `fake_test.go` | 5 Port の fake 実装（in-memory） |
| `ingest_usecase_test.go` | 7 シナリオテスト（初回 / 差分 / 重複排除 / フォールバック / 部分失敗 / publish 失敗 / invalid item validate） |

### Infrastructure（`internal/safetyincident/infrastructure/`）

| パッケージ | 役割 |
|---|---|
| `mofa/` | encoding/xml ベースの MOFA Source、`initial`/`incremental` URL 切替、5xx retry、bad-date drop、fixture |
| `llm/` | `LocationExtractor` 実装、Claude prompt（system + user）、JSON parse soft fallback |
| `geocode/` | `MapboxGeocoder` + `CentroidGeocoder` + `ChainGeocoder`、`country_centroids.json`（Natural Earth CC0、~250 国、go:embed） |
| `cms/` | `Repository` 実装（19 フィールド mapping、GeoJSON Point） |
| `eventbus/` | `EventPublisher` 実装、JSON over Topic 抽象 |

### Platform 拡張

| ファイル | 拡張内容 |
|---|---|
| `platform/llm/claude.go` | Anthropic Messages API client |
| `platform/cmsx/item.go` | Item CRUD 系 5 メソッド + URL クエリ escape (regression-tested) |
| `platform/mapboxx/client.go` | 実 HTTP 実装 (Mapbox Geocoding) |
| `platform/ratelimit/ratelimit.go` | `Wait(ctx)` メソッド |
| `platform/pubsubx/client.go` | `cloud.google.com/go/pubsub/v2` wrapper、`Topic` 抽象、Client.Close で全 Topic を Stop |

### Composition Root

| ファイル | 内容 |
|---|---|
| `cmd/ingestion/main.go` | envconfig + DI 配線 + `usecase.Execute` + `run()` パターン (defer 保証) |

### Terraform

| ファイル | 変更 |
|---|---|
| `terraform/modules/ingestion/main.tf` | `max_retries = 0` + env 2 個（`INGESTION_MODE`, `INGESTION_PUBSUB_TOPIC_ID`） |

---

## 2. NFR-ING-* カバレッジ

| NFR ID | 要件 | 実装 |
|---|---|---|
| NFR-ING-PERF-01 | Run 完了 < 60s | 並列度 5 + LLM 5 req/s + Mapbox 10 req/s で 30 件処理が 6-12 秒 |
| NFR-ING-PERF-02 | initial Run < 30 分 | 同上 + Cloud Run Job timeout=300s（initial は手動運用で許容） |
| NFR-ING-PERF-03 | per-item p95 < 5s | LLM 1s + Mapbox 0.5s + CMS 0.5s + Pub/Sub 0.1s |
| NFR-ING-SEC-01 | Secret Manager | 3 Secret (claude / mapbox / cms) を value_source.secret_key_ref で注入 |
| NFR-ING-SEC-02 | Token redact | `cmsx.Config.Token` / `llm.Config.APIKey` / `mapboxx.Config.AccessToken` は struct 内、ログに attr 出さず |
| NFR-ING-SEC-03 | 最小権限 SA | Runtime SA は Secret 読取 + Pub/Sub publisher + OTel のみ |
| NFR-ING-SEC-04 | PII redact | `MainText` は slog Debug のみ、Info 以上には出さない |
| NFR-ING-REL-01 | 冪等 | `Repository.Exists` で短絡、CMS が SoT |
| NFR-ING-REL-02 | 部分失敗 → exit 0 | `processItem` で skip-and-continue、Run 集計で Failed[phase] を記録 |
| NFR-ING-REL-03 | retry | MOFA / Claude / Mapbox / cmsx すべて `retry.Do(DefaultPolicy)` |
| NFR-ING-REL-04 | フォールバック | `ChainGeocoder` で Mapbox 失敗 → Centroid 自動切替 |
| NFR-ING-OPS-01 | 構造化ログ | slog `app.ingestion.phase` / `key_cd` / `mode` 属性 |
| NFR-ING-OPS-02 | OTel Metric | `app.ingestion.run.{fetched,skipped,processed,failed,published}` + `app.ingestion.geocode.fallback`（centroid 時のみ） |
| NFR-ING-OPS-03 | Run summary | done log で `fetched / skipped / processed / published / failed_*` 7 属性 |
| NFR-ING-OPS-04 | per-item ERROR | `recordItemFailure` で `key_cd` + `phase` + `err` 必須 |
| NFR-ING-TEST-01 | PBT | `point.Validate` の WGS84 envelope 双方向 |
| NFR-ING-TEST-02 | fake シナリオ | 7 本（plan 6 本 + Copilot 対応で validate 追加） |
| NFR-ING-TEST-03 | MOFA fixture | `testdata/mofa/sample_newarrival.xml`（4 件、bad-date 含む） |
| NFR-ING-TEST-04 | httptest | LLM / Mapbox / CMS Item / MOFA すべて httptest 主要パス |
| NFR-ING-TEST-05 | カバレッジ | §3 参照、全パッケージ目標達成 |
| NFR-ING-EXT-01 | 別 LLM 差替 | `LocationExtractor` Port 経由、`platform/llm/claude.go` は薄い concrete |
| NFR-ING-EXT-02 | 別 Geocoder 追加 | `ChainGeocoder` の MapboxLookup / CentroidLookup interface 経由 |
| NFR-ING-EXT-03 | 別 Source 追加 | `MofaSource` Port 経由、別実装で `IngestionMode` 切替できる |

---

## 3. テストカバレッジ実績

| パッケージ | 実績 | 目標（Q F [A]） | 達成 |
|---|---|---|---|
| `internal/safetyincident/domain` | **100.0%** | 95%+ | ✓ |
| `internal/safetyincident/application` | **90.7%** | 90%+ | ✓ |
| `internal/safetyincident/infrastructure/cms` | **93.8%** | 70%+ | ✓ |
| `internal/safetyincident/infrastructure/eventbus` | **87.5%** | 70%+ | ✓ |
| `internal/safetyincident/infrastructure/geocode` | **90.9%** | 70%+ | ✓ |
| `internal/safetyincident/infrastructure/llm` | **100.0%** | 70%+ | ✓ |
| `internal/safetyincident/infrastructure/mofa` | **88.1%** | 70%+ | ✓ |
| `internal/platform/llm` | **83.3%** | 70%+ | ✓ |
| **U-ING 全体** | **90.9%** | 85%+ | ✓ |

---

## 4. 設計のキモ（合体技）

### Q3 + Q7 で Self-healing
1. Run 中の per-item 失敗は skip + 構造化ログ + Metric
2. 失敗 item は CMS に未登録のまま残る
3. 5 分後の次 Run で `Repository.Exists` が false → 自動再試行
4. → Run 単位は常に exit 0、failure は別レイヤから検知

### Q5 ChainGeocoder
- Mapbox（成功 + relevance ≥ 0.5）→ `mapbox` source
- Mapbox 失敗 / 信頼度低 → 国 Centroid → `country_centroid` source
- Centroid lookup 失敗（unknown country）→ `Failed[geocode]` で skip

### Q8 Rate Limit
- LLM 5 req/s（300 RPM）+ Mapbox 10 req/s（600 RPM）を app 側で先制制御
- 並列度 5 = errgroup の semaphore、ratelimit が req/s 上限
- 30 件 Run で 6-12 秒、Mapbox 無料枠 600 RPM 内に収まる

---

## 5. U-NTF への申し送り

次 Unit（U-NTF）が U-ING の資産を使う時の要点:

1. **Pub/Sub Subscriber は `safety-incident.new-arrival` topic を購読**。Push delivery で notifier Cloud Run Service が起動される
2. **メッセージ形式は JSON**（`internal/safetyincident/infrastructure/eventbus/publisher.go` の `newArrivalEventWire`）:
   - `key_cd`, `country_cd`, `info_type`, `geometry{lat, lng}`, `leave_date` (RFC3339)
3. **Pub/Sub attributes** で軽量フィルタ可能: `key_cd` / `country_cd` / `info_type`
4. **Dedup** が必要: U-ING は publish at-least-once。同じ `key_cd` の重複メッセージがあり得る（並行実行 race、retry 等）。U-NTF 側で Firestore に最近処理した key_cd を保持して dedup する想定（U-NTF Design で確定）
5. **DLQ** は shared module で既に作成済み。U-NTF Subscription 設定で DLQ ターゲットを指定する

---

## 6. 実行確認（Build and Test で実施）

本 PR の範囲外。Build and Test ランブックで以下を確認:

- `gcloud run jobs execute ingestion --region=asia-northeast1 --wait` で exit 0
- Cloud Logging で `app.ingestion.phase=done` + summary 属性
- 2 回目実行で `skipped > 0`（重複排除動作確認）
- `app.ingestion.geocode.fallback` Metric が centroid fallback でのみ increment
- 実 MOFA XML との struct 整合性（不一致あれば `mofa/xml_types.go` 修正）
- Pub/Sub `safety-incident.new-arrival` に publish されること
- 19 フィールドが reearth-cms 上で正しく upsert されること

---

## 7. 将来の拡張ポイント

- **U-NTF 完成前は Pub/Sub publish が「捨てメッセージ」になる**（受信者なし）。Topic は U-PLT で作成済みなので publish 自体は成功する
- LLM プロバイダ切替（GPT / Gemini）: `internal/safetyincident/infrastructure/llm/extractor.go` に並走実装、Composition Root で env 切替
- MOFA 以外のソース追加（米国 State Department など）: `MofaSource` Port の別実装
- `country_centroids.json` の網羅: 必要に応じて MOFA `country_cd` の出現値で穴を埋める
