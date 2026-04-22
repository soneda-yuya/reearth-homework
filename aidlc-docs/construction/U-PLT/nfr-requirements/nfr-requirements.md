# U-PLT Non-Functional Requirements

U-PLT（Platform & Proto 基盤 Unit）の非機能要件。上位要件（`aidlc-docs/inception/requirements/requirements.md`）の NFR-SEC-* / NFR-OPS-* / NFR-TEST-* / NFR-EXT-* を U-PLT 固有値に落とし込む。

---

## 1. スケーラビリティ・可用性・性能

### NFR-PLT-PERF-01: ビルド時間
- **基準**: `go build ./...` がローカルで **< 30 秒**、CI（GitHub Actions `ubuntu-latest`）で **< 60 秒**（キャッシュ有効時）
- **理由**: 1 人開発の小さな反復を阻害しないため
- **測定**: CI のステップ時間を GitHub Actions の summary で可視化

### NFR-PLT-PERF-02: CI 全体時間（Q3 [A]）
- **基準**: `go test ./... + buf lint + buf breaking + govulncheck` の合計で **< 5 分**
- **測定**: PR ビルドの wall-clock

### NFR-PLT-PERF-03: ログ書き出しのオーバーヘッド
- **基準**: `slog.Info` 1 回の処理時間が **< 10μs**（ベンチマーク計測）。通常フローのスループットに影響しない
- **理由**: 取り込み（U-ING）や BFF（U-BFF）で高頻度に呼ばれるため

### NFR-PLT-PERF-04: OTel Span 発行のオーバーヘッド
- **基準**: 1 Span あたり **< 50μs**（子 Span 含む）
- **測定**: `go test -bench` で `observability.Tracer` の作成〜 `End()` まで計測

### NFR-PLT-AVAIL-01（該当なし）
U-PLT はライブラリ Unit のため自律可用性は持たない。起動時の `observability.Setup` / `config.Load` の **失敗モード** は以下:
- Config 必須欠落 → プロセス終了（`log.Fatalf`）
- OTel exporter 初期化失敗 → **stdout fallback** で続行、`WARN` ログ
- Firebase / CMS SDK 初期化失敗 → プロセス終了（依存サービスが使えない以上起動不可）

---

## 2. セキュリティ（NFR-SEC-* 拡張有効）

### NFR-PLT-SEC-01: Secrets 管理（Q4 [A]）
- **開発環境**: `.env`（`.gitignore` 登録）/ `.env.example` のみコミット
- **CI**: GitHub Secrets（`MAPBOX_API_KEY`、`CLAUDE_API_KEY`、`CMS_INTEGRATION_TOKEN` など）
- **本番 / dev（クラウド）**: **GCP Secret Manager**、環境変数には Secret のリソース名のみ（Functional Design `business-logic-model.md` §4.4 に従う）
- **取得タイミング**: 起動時 1 回、メモリ内のみ、ログ・エラー出力に値を出さない（marshaler で redact）

### NFR-PLT-SEC-02: 依存脆弱性スキャン（Q4 [A]）
- **ツール**:
  - `govulncheck`（Go 標準、CI 必須）— Critical / High 検出時は PR を落とす
  - **Dependabot**（daily） — GitHub 設定で `gomod` と `github-actions` を有効化
- **対応方針**: Critical は即日対応、High は 3 日以内、Medium は週次で扱う
- **例外記録**: 修正不可の脆弱性は `SECURITY.md`（後日作成）で根拠と残留リスクを文書化

### NFR-PLT-SEC-03: コードに Secrets をコミットしない
- **仕組み**: `pre-commit` フック + `gitleaks` / `truffleHog` をオプション導入（Infrastructure Design で決定）
- **対象ファイル**: `*.env` / `*.json`（service account） / `*.pem` 等

### NFR-PLT-SEC-04: ログに PII を出さない（Functional Design に一貫）
- `mainText` は `DEBUG` のみ、`email` / FCM トークン / Integration Token は一切ログ不可
- marshaler で `string(secret)` → `"[REDACTED]"` に置換するヘルパを `shared/errs` または `platform/observability` に持つ

---

## 3. 信頼性

### NFR-PLT-REL-01: フェイルファスト
- `config.Load()` の必須欠落で **即プロセス終了**（Cloud Run は起動しない、デプロイが壊れる）
- 起動シーケンスのいずれかが失敗したら **`observability.shutdown` を defer で呼んだうえで終了**

### NFR-PLT-REL-02: Panic 復帰
- Connect Interceptor と Job Runner で `recover()` し、`errs.KindInternal` にラップして上位に返す
- `error.stack` をログ `ERROR` で 1 度だけ出力（生ログに他属性を汚染しない）

### NFR-PLT-REL-03: OTel Exporter 失敗時の耐性
- エクスポート先が落ちていても **プロセスは止めない**（BSP の default 挙動）
- stdout fallback を保証

---

## 4. 観測性（NFR-OPS-01/02 拡張）

### NFR-PLT-OBS-01: 構造化ログ
- すべてのログが JSON（slog JSON ハンドラ）
- 必須属性 9 種（`service`/`env`/`trace_id`/`span_id`/`level`/`time`/`msg`/`caller`/`request_id`）を自動付与
- 詳細は Functional Design `business-logic-model.md` §2 を参照

### NFR-PLT-OBS-02: メトリクス
- OpenTelemetry Metrics（SDK + Exporter）
- 必須共通メトリクス（`platform/observability` が自動登録）:
  - `app.startup.duration.ms`（起動時間）
  - `app.panic.count`（panic 復帰回数）
  - `app.external_api.latency.ms{endpoint,kind}`（各 SDK ラッパーで計測）
- ユニット固有メトリクスは各 Unit の NFR で追加

### NFR-PLT-OBS-03: 分散トレーシング
- OpenTelemetry Traces、W3C Trace Context 準拠
- Connect Interceptor（BFF 側）/ Pub/Sub メッセージの属性として伝播
- `platform/observability` がルート Span 管理 API を提供

### NFR-PLT-OBS-04: エクスポート先（Q5 [A]）
- `PLATFORM_OTEL_EXPORTER` 環境変数で切替:
  - `stdout` — ローカル / CI / トラブルシュート用
  - `gcp` — dev / prod（Cloud Trace + Cloud Monitoring + Cloud Logging）
- 値未指定のデフォルトは `stdout`（安全側）

---

## 5. テスト（NFR-TEST-* 拡張有効）

### NFR-PLT-TEST-01: ユニットテスト
- 対象: `shared/*` / `platform/*` の公開 API
- カバレッジ目標（Q3 [A]）:
  - 全体 **80% 以上**
  - `shared/errs` / `shared/validate` / `platform/config` は **90% 以上**
- CI で `go test -cover -coverprofile=...` + Codecov（または GitHub Actions コメント）で PR に可視化

### NFR-PLT-TEST-02: プロパティベーステスト（Q2 [A,B,D]）
PBT 拡張が有効なため以下 3 領域を **必須** 適用:

| 対象 | プロパティ例 | ライブラリ |
|---|---|---|
| `errs.Wrap` / `IsKind` / `KindOf` | `KindOf(Wrap(op, k, err)) == k`、`IsKind(Wrap(...), k) == true`、nil ラップで nil を返す | `testing/quick` または `pgregory.net/rapid` |
| proto ↔ domain 変換 | `protoToDomain(domainToProto(x)) == x`（Timestamp / Point / Filter / SafetyIncident） | 同上 |
| `validate` 境界値 | `limit` / `radius_km` / `Point.Lat/Lng` / 期間順序の境界値が必ずエラー／成功に分岐 | 同上 |

PBT ライブラリは `pgregory.net/rapid` を推奨（stateful も書ける、Go 1.21+ 対応）。

### NFR-PLT-TEST-03: ビルドタグ無し
テストは build tag 分離を使わず、`_test.go` の naming convention で分類。`integration_test.go` は `-short` でスキップできるようにする。

### NFR-PLT-TEST-04: ベンチマーク
- `platform/observability` に対して `BenchmarkLogInfo`、`BenchmarkTracerStart`、`BenchmarkMeterAdd` を用意
- 回帰検知: 前回比 +20% で PR に警告コメント（GitHub Actions + `benchstat`）

---

## 6. 保守性・拡張性

### NFR-PLT-MNT-01: 依存バージョン方針（Q1 [A] + Go 1.26）
- **Go**: **最新安定版（Go 1.26）** を利用
- 依存は **最新 patch を Dependabot で追従**、major は手動で上げる
- 主要 SDK:
  - `connectrpc.com/connect` 最新 v1
  - `go.opentelemetry.io/otel` / `otel/sdk` 最新
  - `firebase.google.com/go/v4` 最新
  - `cloud.google.com/go/pubsub` / `cloud.google.com/go/secretmanager` 最新
  - `github.com/anthropics/anthropic-sdk-go` 最新（存在しない／不安定な場合は自前 HTTP）
  - `github.com/kelseyhightower/envconfig` 最新
  - `pgregory.net/rapid`（PBT）最新
- Flutter 側は別リポだが proto 共有のため、連携は CI で整合性確認

### NFR-PLT-MNT-02: Proto 破壊的変更防止
- `buf breaking` を CI で main ブランチに対して強制
- 破壊的変更は `v2` パッケージで並列化、`v1` を残す（クライアント切替まで）

### NFR-PLT-MNT-03: Lint / フォーマット
- `gofmt -s` / `goimports` / `golangci-lint`（`revive`, `staticcheck`, `gosimple`, `errcheck`, `govet` を有効）を CI で必須
- 事前 pre-commit フック導入（Husky ライクに `lefthook` を検討、Infrastructure Design で決定）

### NFR-PLT-MNT-04: Repository パターン（NFR-EXT-01 に準拠）
U-PLT は抽象化の受け皿を提供する。各 Context の `domain` に Port I/F、`infrastructure` に Adapter を置き、`cmd/*` が DI する構造を `platform/connectserver` と `interfaces/*` で支える。

### NFR-PLT-MNT-05: ドキュメンテーション
- 各公開関数 / 型に godoc コメント
- `shared/errs` と `platform/config` は `doc.go` を用意して使い方を示す
- `README.md` の Getting Started に「Go インストール → `make setup` → `make test`」フローを記載

---

## 7. 受け入れ基準（U-PLT NFR Requirements 完了条件）

- [ ] `govulncheck` / Dependabot / GitHub Secrets / GCP Secret Manager の方針が [tech-stack-decisions.md](./tech-stack-decisions.md) に明記
- [ ] PBT 3 領域に対する sample プロパティが草案レベルで列挙されている
- [ ] カバレッジ 80% / CI 5 分以内の検証手段が CI 設計（Infrastructure Design）で引き継がれる
- [ ] OTel exporter 切替（`PLATFORM_OTEL_EXPORTER`）の env が [business-logic-model.md](../functional-design/business-logic-model.md) §4 の Config スキーマに追加される
- [ ] Go 1.26 のリリースノートを確認、サポート期限（2027-08 頃）を把握
