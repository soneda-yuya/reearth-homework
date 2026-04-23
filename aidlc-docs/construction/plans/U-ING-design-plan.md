# U-ING Design Plan (Minimal 合本版)

## Overview

U-ING（Ingestion Unit、Sprint 2）は **MOFA 海外安全情報オープンデータを取り込み、LLM で発生地名抽出 → Mapbox でジオコード → reearth-cms に upsert → 新着を Pub/Sub に publish** する **Cloud Run Job** です。プロジェクトのコア機能。

ワークフロー圧縮 Option B に従い、**Functional Design + NFR Requirements + NFR Design** を 1 ドキュメントに合本します。

## Context（確定済み）

- **Bounded Context**: `safetyincident`（Core domain）
- **Deployable**: `cmd/ingestion`（Cloud Run Job、Cloud Scheduler 起動、定期実行）
- **責務**: MOFA データ取込 → LLM/Mapbox 加工 → CMS 永続化 → 新着 publish
- **依存**: U-PLT（共通基盤）、U-CSS（CMS スキーマ適用済み前提）
- **データソース**: MOFA `https://www.ezairyu.mofa.go.jp/html/opendata/`
  - `00A.xml`（全件アーカイブ、初回モード用）
  - `newarrivalA.xml`（差分、5 分毎モード用）
- **Pub/Sub Topic**: `safety-incident.new-arrival`（U-PLT shared infra で作成済み）
- **必要な Secret**: `ingestion-claude-api-key`、`ingestion-mapbox-api-key`、`cms-integration-token`（全て shared/secrets.tf にて定義済み）

U-PLT 共通規約（slog + OTel / envconfig + Secret Manager / `errs.Wrap` / retry / rate limit / Clock / terraform module 構成 / CI / Dockerfile）は **全てそのまま踏襲** します。U-CSS の `cmsx` Client もそのまま使い、Item CRUD 系メソッドだけを U-ING で追加拡張します。

---

## Step-by-step Checklist

- [ ] Q1〜Q9 すべて回答
- [ ] 矛盾・曖昧さの検証、必要なら clarification
- [ ] 成果物を生成:
  - [ ] `construction/U-ING/design/U-ING-design.md` — Functional + NFR Req + NFR Design 合本
- [ ] 承認後、U-ING Infrastructure Design へ進む

---

## Questions

### Question 1 — 取り込みモード（初回 vs 差分）

MOFA は 2 つのエンドポイントを公開:
- `00A.xml`: **全件アーカイブ**（数年分。サイズ大、初回バックフィル用）
- `newarrivalA.xml`: **直近 24 時間分の新着**（小、定期更新用）

A) **推奨**: **両方サポート、env で切替**（`INGESTION_MODE=initial` / `INGESTION_MODE=incremental`）。`incremental` を Cloud Scheduler で 5 分毎定期実行、`initial` は手動で 1 回だけ運用者が叩く（U-CSS と同じ手動実行モデル）
B) `incremental` のみサポート（バックフィルは諦める。MVP 開始時の過去データは無し）
C) `initial` を Cloud Scheduler で 1 日 1 回 + `incremental` を 5 分毎の 2 系統並走（重複排除に依存して安全）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 2 — ポーリング間隔 / Scheduler 設定

`incremental` モードの実行頻度は？

A) **推奨**: **5 分毎**（`*/5 * * * *`、要件 NFR-PERF と整合）。MOFA 側の更新頻度に合わせる
B) 1 分毎（極端なリアルタイム性、MOFA 側に負担。CMS / LLM / Mapbox のレート制限にも近づく）
C) 15 分毎（コスト最小、通知遅延が許容範囲なら）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 3 — 重複排除（idempotency）戦略

XML には全件含まれるので、毎回処理すると同じ `key_cd` を何度も LLM/Mapbox に投げてしまう。重複検知を **どこ** で行うか:

A) **推奨**: **CMS への lookup**（`cmsx.GetItemByAlias("key_cd", value)` を最初に叩いて存在確認）。LLM/Mapbox を呼ぶ前に short-circuit。CMS 側だけが Source of Truth で local state なし
B) **Firestore キャッシュ**（処理済み `key_cd` を Firestore に書き出し、次回以降は Firestore lookup で短絡）。CMS 呼び出し回数は減るが Firestore 維持コスト
C) **In-memory のみ**（毎回起動時にリセット、5 分毎の同じ実行内では重複しないが Job 跨ぎでは重複処理）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 4 — LLM プロンプト戦略

Anthropic Claude（Haiku、コスト重視で選定済み）に **発生地名抽出** を依頼する形:

A) **推奨**: **1 件ずつ呼び出し**（title + lead + main_text を渡し、JSON で `{"location": "...", "confidence": 0.0-1.0}` を返させる）。並列度を制御（`golang.org/x/sync/errgroup` で max concurrency = 5 程度）。シンプル + デバッグ容易
B) **バッチ呼び出し**（10 件まとめて 1 リクエスト）。コスト/レイテンシ↓、ただしプロンプト長 / 失敗時の影響範囲↑
C) **Few-shot**（過去成功例をプロンプトに含める）。精度↑だがプロンプト長 + 維持コスト
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 5 — ジオコーディング失敗時のフォールバック

LLM が地名を抽出 → Mapbox に送る → 結果が無い / 信頼度が低い場合:

A) **推奨**: **国 Centroid フォールバック**（MOFA の `country_cd` から国の代表座標を取得して使用）。`geocode_source = "country_centroid"` で記録、Flutter 側は UI で「概算位置」を示す（NFR-FUN-04 既定）
B) **Skip**（Item を CMS に保存しない）。位置不明データは誰にも届かない
C) **保存はする、geometry は null** にして Flutter 側で「位置不明」表示
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 6 — Pub/Sub publish のタイミング

新着 1 件処理完了 → publish のタイミング:

A) **推奨**: **CMS upsert 成功直後に 1 件ずつ publish**（実装シンプル + メッセージごとの failure を独立して扱える）。Pub/Sub 側のメッセージ数が増えるが、コストは小さい
B) **バッチ末で一度に publish**（Run 1 回分の新着を 1 メッセージで送る）。Subscriber 側のロジックが複雑化
C) **Publish しない**（Notifier 側が Cloud Logging Sink で拾う）。MVP 簡略化、ただし設計から外れる
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 7 — エラーハンドリング / 部分失敗

Run 中に LLM や Mapbox や CMS への呼び出しが一部失敗した場合:

A) **推奨**: **失敗した item は skip + 構造化ログ + Metric カウンタ**、Run 自体は exit 0（成功した分は CMS に入る）。次の Run 5 分後に重複排除を経て再試行（key_cd が CMS に無いまま残るので自然リトライ）。エラーレートが閾値を超えたら別途アラート（Run 単位の throw はしない）
B) **fail-fast**（最初のエラーで Run 終了 exit 1）。Cloud Scheduler が次回 5 分後に新規 Run を起動して継続
C) **Run 単位 retry**（Cloud Run Job の `max_retries` で 3 回試行）。部分成功と相性悪い（同じ item を 3 回処理）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 8 — Rate Limit / 並列度

LLM (Anthropic) と Mapbox の API レート制限への配慮:

A) **推奨**: **app 側で `platform/ratelimit` を使ってグローバルに絞る**。LLM = 5 req/s、Mapbox = 10 req/s（無料枠 600 req/min を踏まえた上で安全マージン）。並列度は errgroup で制御し、burst も rate limiter で吸収
B) **並列度のみ制御**（rate limit 無し、API 側の 429 を retry でハンドリング）。実装シンプルだが API ベンダー側の reputation 落とすリスク
C) **完全直列**（並列無し）。安全だが遅い（500 件処理に 数分〜十数分）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 9 — テスト戦略

`safetyincident` BC のテスト:

A) **推奨**:
  - **Domain**: `MailItem`、`SafetyIncident`、`Geocoder` チェーンの unit test + PBT（座標バリデーション、フォールバック合成）
  - **Application**: `IngestUseCase` を fake `MofaSource` / fake `LocationExtractor` / fake `Geocoder` / fake `Repository` / fake `EventPublisher` で組み立てたシナリオテスト（初回 / 差分 / 重複排除 / フォールバック / 部分失敗）
  - **Infrastructure**: MOFA XML パーサーは固定 fixture を `testdata/` に置いて回帰テスト、LLM / Mapbox / CMS は `httptest` モック（任意）
B) A + Live integration test（実 MOFA + Anthropic + Mapbox を呼ぶ。CI では `-tags=integration` で skip、Build and Test でローカル実行）
C) A のみ（実 API 統合は Build and Test 手動）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

---

## 承認前の最終確認（回答確定）

- **Q1 [A]**: 取り込みモード = `initial` + `incremental` 両方サポート、`INGESTION_MODE` env で切替（incremental は Cloud Scheduler、initial は手動 1 回）
- **Q2 [A]**: ポーリング間隔 = **5 分毎**（`*/5 * * * *`）
- **Q3 [A]**: 重複排除 = **CMS への lookup**（`cmsx.GetItemByAlias("key_cd", value)` で短絡、Source of Truth は CMS）
- **Q4 [A]**: LLM プロンプト = **1 件ずつ + 並列度 5**（errgroup + ratelimit）
- **Q5 [A]**: ジオコーディング失敗時 = **国 Centroid フォールバック**（`geocode_source = "country_centroid"` で記録、Flutter 側で「概算位置」UI）
- **Q6 [A]**: Pub/Sub publish = **CMS upsert 直後に 1 件ずつ publish**
- **Q7 [A]**: エラーハンドリング = **失敗 item は skip + 構造化ログ + Metric**、Run は exit 0、次回 Run で自然リトライ（重複排除と組み合わせ）
- **Q8 [A]**: Rate Limit = **app 側で `platform/ratelimit`** で LLM 5 req/s + Mapbox 10 req/s、burst で時々超える程度
- **Q9 [A]**: テスト戦略 = Domain unit + PBT、Application は fake シナリオ 5〜6 本、Infrastructure は MOFA fixture + httptest、実 API 統合は Build and Test 手動。カバレッジ層別 (domain 95% / app 90% / infra 70%、全体 85%+)

回答確定済み。`U-ING-design.md`（合本版）を生成する。
