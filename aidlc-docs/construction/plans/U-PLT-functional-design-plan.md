# U-PLT Functional Design Plan (Minimal)

## Overview

U-PLT は純粋な基盤 Unit（`internal/platform/*` / `internal/shared/*` / `proto/v1/*.proto` / `buf` 設定）です。通常の Functional Design は「ビジネスロジックの詳細化」が主眼ですが、U-PLT には該当ロジックが無いため **Minimal 版** として 4 項目だけ詳細化します:

1. **Proto メッセージのフィールド定義**（Connect + Pub/Sub）
2. **ログスキーマ**（slog 属性名・レベル規約）
3. **エラー分類**（Kind 列挙・`%w` ラップ規約）
4. **Config スキーマ**（環境変数のプレフィックス・型・必須／任意）

それぞれ詳細なビジネスルールは存在しないので、**「どの選択肢にするか」を確定する** だけです。AI が強い推奨案を出すので、同意できれば A を選んでください。

すべての [Answer]: に回答し、`done` と伝えてください。

---

## Step-by-step Checklist

- [ ] Q1〜Q4 すべて回答
- [ ] 回答に矛盾・曖昧さがないか検証
- [ ] 成果物 3 点を生成:
  - [ ] `construction/U-PLT/functional-design/business-logic-model.md`（proto schema + log schema + error taxonomy の宣言的定義）
  - [ ] `construction/U-PLT/functional-design/business-rules.md`（バリデーション・ラップ規約・ログ出力ルール）
  - [ ] `construction/U-PLT/functional-design/domain-entities.md`（proto 型と Go 型のマッピング、Config 構造体）
- [ ] 承認後、NFR Requirements（U-PLT）へ進む

---

## Context Summary

- **所属 Unit**: U-PLT（Sprint 0、Platform & Proto 基盤）
- **関連 Application Design**:
  - [components.md](../../inception/application-design/components.md)（C-05 cmsx / C-13 observability / platform/*）
  - [component-methods.md](../../inception/application-design/component-methods.md)（各 Platform パッケージのシグネチャ）
  - [application-design.md](../../inception/application-design/application-design.md) §6（ディレクトリ構造）
- **関連 NFR**（要件書より）:
  - NFR-SEC-01（シークレット管理）
  - NFR-OPS-01/02（構造化ログ、監視項目）
  - NFR-TEST-02（PBT 対象にシリアライズ／ラウンドトリップを含む）
- **関連ストーリー**: なし（基盤 Unit、全 Story の間接的前提）

---

## Questions

### Question 1 — Proto メッセージのフィールド命名と型方針

Connect 用の `proto/v1/safetymap.proto` と Pub/Sub 用の `proto/v1/pubsub.proto` で採用するフィールド命名・型方針。

A) **推奨**: snake_case（proto 標準）/ 必須フィールドは `optional` を付けずデフォルト値で表現、ID 類は string、日時は `google.protobuf.Timestamp`、座標は Connect 側のみ `Point`（`double lat = 1; double lng = 2;`）、緯度経度の単位はメートル法 WGS84、列挙は `enum`（`INFO_TYPE_UNSPECIFIED = 0` を必ず含む）
B) 日時も string（RFC3339）で持つ（Dart 側の扱いやすさ優先）
C) Connect でも `Point` を GeoJSON 準拠に寄せる（`geometry: { type: "Point", coordinates: [lng, lat] }` を JSON 互換で）
X) Other（[Answer]: の後ろに自由記述）

[Answer]: 

### Question 2 — ログスキーマ（slog 属性）

構造化ログで **常に出力する属性** と **Unit 毎に追加する属性** を確定します。`log/slog` の JSON ハンドラ前提。

A) **推奨**: 共通属性 = `service` / `env` / `trace_id` / `span_id` / `level` / `time` / `msg` / `caller` / `request_id`（Connect 側）。ドメイン固有属性 = `key_cd` / `uid` / `country_cd` / `info_type` / `geocode_source` など、発生する処理で適宜追加（`slog.Group("safetyincident", ...)` のネストは不使用、トップレベルに flat で出す）。エラー時は `error.kind` / `error.message` / `error.stack`（stack は panic 復帰時のみ）
B) 共通属性のみ（ドメイン属性は各 Unit の設計で追加）
C) OpenTelemetry Logs の SDK を使って自動付与（slog と二重運用は避ける）
X) Other（[Answer]: の後ろに自由記述）

[Answer]: 

### Question 3 — エラー分類（`shared/errs`）

アプリ全体で使う Error Kind と `%w` ラップ規約。

A) **推奨**: Kind = `NotFound` / `InvalidInput` / `Unauthorized` / `PermissionDenied` / `External`（外部 API 起因）/ `Conflict` / `Internal` の 7 種。`errs.Wrap(op, kind, err)` でラップ、`errs.IsKind(err, KindX)` で判定。gRPC/Connect の Status マッピングは BFF 側で Kind → Code を変換（`NotFound→not_found`, `Unauthorized→unauthenticated`, 他は `internal`）。Kind の Go 表現は `int` iota ではなく `string`（JSON ログ出力のしやすさ）
B) Kind は付けず `errors.Is/As` + sentinel errors のみで運用（シンプル）
C) `cockroachdb/errors` を使って stack trace 付き（サードパーティ依存を許容）
X) Other（[Answer]: の後ろに自由記述）

[Answer]: 

### Question 4 — Config スキーマと環境変数命名

各 Deployable の環境変数の命名規約と読み込み方。

A) **推奨**: プレフィックス = `{DEPLOYABLE}_`（`INGESTION_` / `BFF_` / `NOTIFIER_` / `SETUP_`）+ 共通 `PLATFORM_`。構造体 tag（`envconfig:"FOO"`）で読み込み、`config.Load()` は起動時に必須項目を検証して欠落時は panic。型は `string` / `int` / `time.Duration` / `url.URL` / `[]string`（カンマ区切り）。Secrets は環境変数ではなく Secret Manager から取得（Infrastructure Design で詳細）。
B) 12-factor 準拠で **単一プレフィックスなし**（すべて `UPPER_SNAKE` の個別キー、Deployable ごとの衝突は名前で避ける）
C) YAML ファイル + 環境変数オーバーライド
X) Other（[Answer]: の後ろに自由記述）

[Answer]: 

---

## 承認前の最終確認（回答後に AI が埋めます）

- Proto 命名／型方針: _TBD_
- ログスキーマ: _TBD_
- エラー分類: _TBD_
- Config 規約: _TBD_

回答完了後、矛盾・曖昧さがなければ 3 成果物を生成します。
