# U-ING Build and Test — Runbook

**Status**: 🟡 **Template only** — 実 MOFA / Anthropic / Mapbox / reearth-cms / Pub/Sub への疎通確認は未実施。3rd-party サービスの API key と reearth-cms が用意でき次第、本ランブックに沿って実行し、結果を §6 に記録する。

**前提**: U-ING Code Generation（PR #33 merged 2026-04-23、PR #34 merged 2026-04-23）が main に取り込まれていること、および U-CSS Build and Test runbook の §1 の手順（Cloud Run Job `cms-migrate` の 1 回以上の正常実行、CMS に 19 フィールドの `safety-incident` Model が存在）が済んでいること。

---

## 1. 目的

`cmd/ingestion` が **実 MOFA XML → LLM → Mapbox → reearth-cms → Pub/Sub** のパイプラインを正しく動かすことを確認する:

1. MOFA `newarrivalA.xml` / `00A.xml` を正しくパースできる（U-ING Design Q C [A] で意図的に Build and Test に持ち越した **設計仮定の答え合わせ**）
2. Claude Haiku で発生地名が抽出できる（JSON parse、confidence が妥当）
3. Mapbox Geocoding で緯度経度に変換できる、低信頼 / 失敗時は国 Centroid にフォールバックする
4. `cmsx.UpsertItemByFieldValue` で 19 フィールドの Item が CMS に登録される（初回 Create、2 回目以降 Update）
5. **重複排除**（`Repository.Exists`）が効く（2 回目の Run で skip）
6. Pub/Sub topic に NewArrivalEvent が publish される
7. 部分失敗時に **skip + 構造化ログ + Metric** で Run 全体は exit 0（self-healing の挙動確認）
8. 実行中断時（SIGTERM）に exit != 0 になる

---

## 2. 事前準備（実行者が用意するもの）

### 2.1 3rd-party サービス

| 項目 | 例 / 取得方法 |
|---|---|
| reearth-cms インスタンス | U-CSS 手順で準備済みのものを流用（専用 test workspace） |
| Claude API key | [console.anthropic.com](https://console.anthropic.com) で発行、**usage limit を低めに設定**（例 $5/月） |
| Mapbox API key | [mapbox.com](https://www.mapbox.com/) で発行、**Geocoding scope のみ** 許可、rate 600 req/min |
| Pub/Sub Topic | U-PLT Terraform で作成済み（`safety-incident.new-arrival`、DLQ 付き） |

> ⚠️ **本番 workspace / 本番 Pub/Sub で実行しない**。test workspace + test project を用意するか、Pub/Sub は `overseas-safety-map-test` プロジェクトで別 topic を用意する。

### 2.2 ローカル環境

- Go 1.26+
- gcloud CLI + ADC (`gcloud auth application-default login`) — Pub/Sub 認証用
- Pub/Sub エミュレータで済ませたい場合: `gcloud beta emulators pubsub start`（別ターミナル）+ `export PUBSUB_EMULATOR_HOST=localhost:8085`

```bash
git checkout main && git pull --ff-only
make build-ingestion       # bin/ingestion を生成
```

### 2.3 環境変数

`.env`（`.gitignore` 済み）を作成:

```bash
# Platform 共通
PLATFORM_SERVICE_NAME=ingestion
PLATFORM_ENV=dev
PLATFORM_GCP_PROJECT_ID=overseas-safety-map-test
PLATFORM_LOG_LEVEL=DEBUG           # 検証時は DEBUG で attr を全部見たい
PLATFORM_OTEL_EXPORTER=stdout      # 実 GCP へ送らない

# ingestion 必須
INGESTION_CMS_BASE_URL=https://cms.example.com
INGESTION_CMS_WORKSPACE_ID=wkp_XXXXXXXX
INGESTION_CMS_INTEGRATION_TOKEN=it_XXXXXXXX
INGESTION_CLAUDE_API_KEY=sk-ant-XXXXXXXX
INGESTION_MAPBOX_API_KEY=pk.XXXXXXXX
INGESTION_PUBSUB_TOPIC_ID=projects/overseas-safety-map-test/topics/safety-incident.new-arrival
```

必要に応じて任意 env をオーバーライド（`INGESTION_MODE=initial`、`INGESTION_CONCURRENCY=1` など）。

---

## 3. 実行手順

### 3.1 Incremental 初回実行

```bash
set -a; source .env; set +a
./bin/ingestion 2>&1 | tee /tmp/ingestion.run1.log
```

**期待されるログの骨格**（抜粋）:

```json
{"level":"INFO","msg":"ingestion starting","app.ingestion.phase":"start","app.ingestion.mode":"incremental",...}
{"level":"INFO","msg":"ingestion finished","app.ingestion.phase":"done","app.ingestion.mode":"incremental","fetched":N,"skipped":0,"processed":N,"published":N,...}
```

**確認項目**:

- [ ] exit code = 0
- [ ] `fetched` > 0（MOFA 新着があった日時）、`fetched == processed`（全件成功の想定）
- [ ] `skipped == 0`（初回なので重複なし）
- [ ] `published == processed`
- [ ] `failed_*` すべて 0（または運用許容範囲）
- [ ] reearth-cms 管理画面で `safety-incident` Model に新規 Item が増えている
- [ ] 各 Item の 19 フィールドがすべて埋まっている（空文字許容のものを除く）
- [ ] `geometry.coordinates = [lng, lat]` が妥当な範囲
- [ ] `geocode_source` が `mapbox` / `country_centroid` のどちらか

### 3.2 2 回目実行（重複排除 + no-op 確認）

```bash
./bin/ingestion 2>&1 | tee /tmp/ingestion.run2.log
```

**期待**:

- `fetched == run1.fetched`（MOFA の間に新しい記事が増えていなければ）
- `skipped == run1.fetched`（全件が `Repository.Exists=true` で短絡）
- `processed == 0`
- `published == 0`
- LLM / Mapbox API は一切叩かれていない（DEBUG ログで `llm.Extract` / `mapbox.Geocode` スパンが無い）

**確認項目**:

- [ ] exit code = 0
- [ ] Claude / Mapbox のコンソール（dashboard）で **Run 2 分の呼び出しが 0 件**
- [ ] CMS 側に重複 Item が発生していない（Model 行数が変わらない）
- [ ] Pub/Sub に重複メッセージが publish されていない（Subscriber を手動で pull して確認）

### 3.3 Initial モード（バックフィル）

```bash
INGESTION_MODE=initial ./bin/ingestion 2>&1 | tee /tmp/ingestion.initial.log
```

`00A.xml`（全件アーカイブ）から数百〜数千件取得。

**確認項目**:

- [ ] exit code = 0
- [ ] `fetched` が数百〜数千件
- [ ] `processed + skipped = fetched`
- [ ] 完了時間が妥当（数分〜数十分、並列度 5 × ratelimit 300/600 RPM なので 1,000 件で ~3 分目安）
- [ ] Anthropic / Mapbox の usage dashboard でコストが想定内

### 3.4 エラーケースの確認（任意）

以下は任意。LLM / Mapbox / CMS のいずれかを人為的に障害にして skip-and-continue + self-healing が動くことを確認する:

- **LLM API key 無効化**: `INGESTION_CLAUDE_API_KEY=invalid` で実行 → 全 item が `Failed[extract]` に振られるが Run は exit 0
- **CMS 到達不能**: `INGESTION_CMS_BASE_URL=https://localhost:65535` で実行 → `fetched` は成功、`Failed[lookup]` or `Failed[upsert]` が積まれるが Run は exit 0
- **SIGTERM**: 実行中に Ctrl+C → `ingestion interrupted` WARN ログ + **exit != 0**

### 3.5 Production 反映手順

ローカルで疎通確認 OK なら:

1. Anthropic / Mapbox の **prod 用 API key を Secret Manager に投入**
   ```bash
   echo -n "<prod claude key>" | gcloud secrets versions add ingestion-claude-api-key --data-file=-
   echo -n "<prod mapbox key>" | gcloud secrets versions add ingestion-mapbox-api-key --data-file=-
   ```
2. `terraform apply`（deploy.yml main merge で自動実行、U-ING Code Gen PR B の差分を反映）
3. Cloud Scheduler がすでに Terraform で作成されているため、**追加操作不要で 5 分毎に自動起動**
4. 初回バックフィルを手動で 1 回（任意）:
   ```bash
   gcloud run jobs execute ingestion \
     --region=asia-northeast1 \
     --update-env-vars=INGESTION_MODE=initial \
     --wait
   ```
5. Cloud Logging (`resource.labels.job_name=ingestion`) で `app.ingestion.phase=done` ログを確認

---

## 4. トラブルシューティング

### 4.1 MOFA XML パースエラー

**症状**: `mofa.decode` エラーで exit 1、または全 item が `Failed[validate]` に振られる

**原因と対処**:
- 実 XML の root element が想定と違う → `internal/safetyincident/infrastructure/mofa/xml_types.go` の `mofaFeed.XMLName` を修正
- フィールド名が違う（例: `keyCd` vs `key_cd`）→ `rawItem` の xml tag を修正
- `leave_date` のフォーマットが違う → `leaveDateFormats` に追加

### 4.2 LLM 抽出失敗多発

**症状**: `geocode_source=country_centroid` の比率が 50% 超

**原因と対処**:
- プロンプトが効いていない / Haiku の性能不足 → `internal/safetyincident/infrastructure/llm/extractor.go` の system prompt を調整、`INGESTION_CLAUDE_MODEL=claude-sonnet-4-6` 等に上げる
- Claude が JSON 以外を返す → prompt で "前置きを入れない" を強調、extractor.go の parseExtractJSON() のフォールバック動作は正常

### 4.3 Mapbox 結果が不正確

**症状**: CMS 上の Item の `geometry` が実際の事象発生地と乖離

**原因と対処**:
- Mapbox が全く違う場所をヒット → `INGESTION_MAPBOX_MIN_SCORE` を上げる（0.5 → 0.7）
- LLM の抽出地名が大雑把すぎる → プロンプトで「地理的に最も具体的なものを」を強調、few-shot 例を追加（Design Q B の C に切替検討）

### 4.4 CMS 401 / 403

- Token 期限切れ → Secret Manager で新 version 追加
- Token の権限不足 → Integration Settings で Item Create / Update / Read を付与

### 4.5 CMS 400（バッドリクエスト）

**症状**: `cmsx.upsert` で HTTP 400

**原因と対処**:
- Item の field key が Model の alias と不一致 → `internal/safetyincident/infrastructure/cms/repository.go` の `toFields` のキー名が CMS の `key_cd` / `title` / ... と一致しているか確認
- `geometry` の GeoJSON 形式を CMS が受け付けない → CMS の `geometryObject` 型の受け入れ形式を確認

### 4.6 Pub/Sub publish 失敗

**症状**: `failed_publish > 0` が常態化

**原因と対処**:
- Runtime SA に `roles/pubsub.publisher` が無い → Terraform の `iam.tf` を確認
- Topic が存在しない → `gcloud pubsub topics list --project=<proj>` で確認、shared module が apply されているか

### 4.7 `max_retries = 0` で Run が失敗 → 再実行は自動?

- **自動で再 Run される**のは 5 分後の Cloud Scheduler tick から。即時 retry はしない（設計通り）
- 緊急時は `gcloud run jobs execute ingestion` で手動実行

---

## 5. 観測ポイント

Run 実行時に **必ず見るメトリック / ログ属性**:

| 観測対象 | 見る場所 | 期待 |
|---|---|---|
| `app.ingestion.run.fetched` | Metric / summary log | 新着数（Incremental で 0〜30） |
| `app.ingestion.run.skipped` | Metric / summary log | 2 回目以降は `== fetched` |
| `app.ingestion.run.processed` | Metric / summary log | 1 回目は `== fetched`、2 回目は 0 |
| `app.ingestion.run.failed{phase=*}` | Metric | 常時 0 であるのが健全。MOFA 不調時に `phase=fetch` が立つ |
| `app.ingestion.geocode.fallback{source=country_centroid}` | Metric | < 20% であるのが健全。超えたら LLM / Mapbox 調整 |
| `app.ingestion.item.duration` (Histogram) | Metric | p95 < 5s（NFR-ING-PERF-03） |
| `app.ingestion.phase=interrupted` | Log filter | **あれば Scheduler timeout / job cancel の兆候** |

---

## 6. 実行記録

> 実 パイプラインで実行する都度、ここに追記する。

### 6.1 [日付未定] ローカル初回実行（test workspace + test keys）

**実行者**: TBD
**Workspace / Topic**: TBD
**MOFA XML 構造の答え合わせ結果**: TBD
**LLM 抽出成功率**: TBD
**Mapbox ヒット率**: TBD
**Centroid フォールバック率**: TBD
**API 仕様の差分（あれば）**: TBD

### 6.2 [日付未定] Production 初回バックフィル

**実行者**: TBD
**fetched / processed**: TBD
**所要時間**: TBD
**コスト（Anthropic + Mapbox 合計）**: TBD

### 6.3 [日付未定] Production 継続運用開始

**実行者**: TBD
**Cloud Scheduler 初回 tick**: TBD
**一週間の `run.failed` 合計**: TBD

---

## 7. 関連ドキュメント

- [`U-ING/design/U-ING-design.md`](../design/U-ING-design.md) — Functional + NFR Req + NFR Design 合本
- [`U-ING/infrastructure-design/`](../infrastructure-design/) — Cloud Run Job / Scheduler / IAM 設計
- [`U-ING/code/summary.md`](../code/summary.md) — Code Generation 成果物一覧 + U-NTF 申し送り
- [`U-CSS/build-and-test/runbook.md`](../../U-CSS/build-and-test/runbook.md) — U-CSS の runbook（本 Unit の前提として U-CSS の実行が必要）
- [`construction/shared-infrastructure.md`](../../shared-infrastructure.md) — Secret 投入手順 / WIF セットアップ
