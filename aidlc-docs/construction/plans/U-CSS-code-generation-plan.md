# U-CSS Code Generation Plan

## Overview

U-CSS（CMS Migrate Unit、Sprint 1）の Code Generation 計画。[`U-CSS/design/U-CSS-design.md`](../U-CSS/design/U-CSS-design.md) と [`U-CSS/infrastructure-design/`](../U-CSS/infrastructure-design/) に基づいて実装する。

## Goals

- `SchemaDefinition`（19 Field 宣言）を Go コードで持ち、冪等 CREATE で reearth-cms に適用する Cloud Run Job `cmsmigrate` を完成させる
- U-PLT 共通規約（slog / OTel / envconfig / errs / retry / Clock）を **踏襲する**（再発明しない）
- Code Generation 完了時点で `cmd/cmsmigrate` は **実動する**（実 CMS への疎通確認は Build and Test で手動実施）

## Non-Goals

- reearth-cms 本体のデプロイ / 管理（Q5 [A]、外部既存を利用）
- Data の CRUD（U-ING / U-BFF の責務）
- Scheduler / 自動実行（Q3 [A]、手動のみ）
- Alerting Policy（Q6 [A]、MVP では作らない）

---

## Step-by-step Checklist（計画段階、承認後に実装）

### Phase 1: Domain Layer

- [ ] 1-1. `internal/cmsmigrate/domain/field_type.go`
  - `FieldType` enum（`FieldTypeText` / `FieldTypeTextArea` / `FieldTypeURL` / `FieldTypeDate` / `FieldTypeGeometryObject` / `FieldTypeSelect`（予約））
  - `String()` メソッド（proto / REST API のシリアライズ用）
- [ ] 1-2. `internal/cmsmigrate/domain/schema_definition.go`
  - `SchemaDefinition` / `ProjectDefinition` / `ModelDefinition` / `FieldDefinition` 構造体
  - Alias 正規表現: Project `^[a-z][a-z0-9-]*$`、Field `^[a-z][a-z0-9_]*$`
  - `Validate()` — 不変条件 R1〜R7（design §1.2.1）
- [ ] 1-3. `internal/cmsmigrate/domain/safety_map_schema.go`
  - `SafetyMapSchema()` — `overseas-safety-map` project + `safety-incident` model + **19 Field**（proto と 1:1、design §1.2.2 の表）
- [ ] 1-4. `internal/cmsmigrate/domain/schema_definition_test.go`
  - Table-driven test（R1-R7 各ルールの失敗ケース 1 件ずつ）
  - PBT（`pgregory.net/rapid`）: ランダム Alias / Field 組合せで Validate() の invariant を検証
  - `SafetyMapSchema()` が Validate() を通ること
- [ ] 1-5. `internal/cmsmigrate/domain/safety_map_schema_test.go`
  - 19 Field が proto の `SafetyIncident` と名前・型で対応しているかのチェック（`key_cd` / `info_type` / ... `updated_at`）
  - Key field = `key_cd` かつ required+unique であること

### Phase 2: Application Layer

- [ ] 2-1. `internal/cmsmigrate/application/schema_applier.go`
  - `SchemaApplier` interface（`FindProject` / `CreateProject` / `FindModel` / `CreateModel` / `FindField` / `CreateField`）
  - `RemoteProject` / `RemoteModel` / `RemoteField` DTO
- [ ] 2-2. `internal/cmsmigrate/application/ensure_schema.go`
  - `EnsureSchemaUseCase` struct（applier + logger + tracer + meter）
  - `Execute(ctx, EnsureSchemaInput) (EnsureSchemaResult, error)`
  - アルゴリズム: Validate → Project ensure → for each Model ensure → for each Field ensure → Drift 集約 WARN
  - fail-fast（Q4 [A]）
  - 各ステップで OTel Span + slog + Metric Counter 更新
- [ ] 2-3. `internal/cmsmigrate/application/drift.go`
  - `DriftWarning` struct + `detectFieldDrift(got RemoteField, want FieldDefinition) *DriftWarning`
- [ ] 2-4. `internal/cmsmigrate/application/ensure_schema_test.go`
  - `FakeSchemaApplier`（in-memory map）を定義
  - シナリオ: (1) 初回 all-empty、(2) 再実行 all-no-op、(3) Model だけ既存で Field 19 作成、(4) 途中で CreateField がエラー → fail-fast
  - Result の `ProjectCreated` / `ModelsCreated` / `FieldsCreated` / `DriftWarnings` の確認
- [ ] 2-5. `internal/cmsmigrate/application/drift_test.go`
  - 型不一致 / required 不一致 / unique 不一致のドリフト検知テスト

### Phase 3: Infrastructure Adapter（HTTP Client）

- [ ] 3-1. `internal/platform/cmsx/schema.go`（U-PLT `client.go` を拡張）
  - 低レベル HTTP メソッド追加: `FindProject(ctx, alias) (*ProjectDTO, error)`、`CreateProject(...)`、同様に Model / Field
  - 認証: `Authorization: Bearer ${Token}`
  - エラー変換: 404 → `(nil, nil)`、401/403 → `errs.Wrap(..., KindUnauthenticated, err)`、409 → `KindConflict`、5xx/429 → `retry.Do` で再試行、その他 → `KindInternal`
  - 各呼び出しに OTel Span（`cms.FindProject` 等）と `http.method` / `http.status_code` 属性
- [ ] 3-2. `internal/platform/cmsx/dto.go`
  - `ProjectDTO` / `ModelDTO` / `FieldDTO` + `fieldTypeFromAPI` / `fieldTypeToAPI`（`FieldType` ↔ reearth-cms API 文字列）
- [ ] 3-3. `internal/platform/cmsx/schema_test.go`
  - `httptest.NewServer` でモック API を立てて主要パスをテスト
  - 200 / 201 / 404 / 401 / 409 / 500 の各レスポンスに対する挙動
  - Retry 動作（5xx → 3 回 → 最終 fail、あるいは途中成功）
- [ ] 3-4. `internal/cmsmigrate/infrastructure/cmsclient/applier.go`
  - `CMSSchemaApplier` — `application.SchemaApplier` 実装（`cmsx.Client` への delegate）
  - DTO ↔ `application.RemoteXxx` の変換
- [ ] 3-5. `internal/cmsmigrate/infrastructure/cmsclient/applier_test.go`
  - `cmsx.Client` を interface 化するか httptest ベースで wire して end-to-end round-trip（1 シナリオ: 全 no-op）
  - (NFR-CSS-TEST-03 によれば任意。最小 1 本でも可)

### Phase 4: Composition Root

- [ ] 4-1. `cmd/cmsmigrate/main.go` を拡張
  - `cmsmigrateConfig` に `CMSBaseURL` / `CMSWorkspaceID` / `CMSIntegrationToken` 追加（envconfig タグ、`required:"true"`）
  - `observability.Setup` → `cmsx.NewClient` → `cmsclient.New` → `application.NewEnsureSchemaUseCase` → `domain.SafetyMapSchema()` → `Execute`
  - fail-fast: error 時 `os.Exit(1)`、成功時は INFO ログで result を出力
  - Shutdown: OTel flush + context cancel
- [ ] 4-2. `cmd/cmsmigrate/main_test.go`（任意、最小）
  - config parse の smoke test（必須 env 欠落で MustLoad が panic すること）

### Phase 5: Terraform

- [ ] 5-1. `terraform/modules/cmsmigrate/main.tf` に `max_retries = 0` を追加
- [ ] 5-2. `terraform/environments/prod/variables.tf` の `cms_base_url` / `cms_workspace_id` に `description` を追加
- [ ] 5-3. `terraform/environments/prod/prod.tfvars.example` を新規作成
- [ ] 5-4. `terraform fmt` / `terraform init -backend=false` / `terraform validate` を通す（ローカルで）

### Phase 6: Docs

- [ ] 6-1. `aidlc-docs/construction/U-CSS/code/summary.md` を新規作成
  - 生成したファイル一覧（新規 / 変更）
  - 各 NFR-CSS-* の実装カバレッジ
  - テストカバレッジ（`go test -coverprofile`）の数値
  - 次 Unit（U-ING）への申し送り事項
- [ ] 6-2. `README.md` に `cmsmigrate` セクションを追記（ローカル実行手順 + 必須 env）
- [ ] 6-3. `aidlc-docs/aidlc-state.md` を更新（Code Generation 完了）

### Phase 7: CI / Verification

- [ ] 7-1. `go test ./... -race` 全緑
- [ ] 7-2. `go vet ./...` / `gofmt -s -d .` / `golangci-lint run` 全緑
- [ ] 7-3. `govulncheck ./...` 全緑
- [ ] 7-4. `buf lint` / `buf breaking`（変更なしだが回す）
- [ ] 7-5. Docker build `cmsmigrate` 緑
- [ ] 7-6. カバレッジ `internal/cmsmigrate/` > 85%（NFR-CSS-TEST-04）

---

## 設計上の要判断事項（計画段階で確定したい）

### Question A — reearth-cms Integration API の実エンドポイント形状

Design では以下を仮定:
```
GET /api/workspaces/{workspaceID}/projects?alias={alias}
POST /api/workspaces/{workspaceID}/projects
GET /api/projects/{projectID}/models/{modelAlias}
POST /api/projects/{projectID}/models
GET /api/models/{modelID}/fields/{fieldAlias}
POST /api/models/{modelID}/fields
```

実際の reearth-cms Integration REST API 仕様:
- 情報源: [https://github.com/reearth/reearth-cms](https://github.com/reearth/reearth-cms) の server/schemas または swagger
- MVP 時点で **実サーバに繋いでいない**（Build and Test で接続確認予定）

**A1 の推奨**: 仕様の調査は Code Generation の前に **限定的な範囲** で実施（Project/Model/Field の Create / List / Get のみ）。不明点があれば **PBT / モックで済ませる範囲** は進め、実 API の誤りは Build and Test で修正する。

A) **推奨**: 調査は最小限（Web で確認できる範囲）。不明点は仮定のまま進めて Build and Test で実 API に合わせて修正
B) 調査に時間をかけて Code Generation 前に API 仕様を固める（Build and Test での手戻り最小化、ただし時間がかかる）
C) 実 API に依存しない **抽象 Applier** として書き、具体的なエンドポイント変換はプラガブルにする（完璧だが過剰設計気味）

[A]:

### Question B — テストのカバレッジ目標

NFR-CSS-TEST-04 は「cmsmigrate/ 配下で 85%+」。

A) **推奨**: **domain 95%+ / application 90%+ / infrastructure 70%+**（HTTP 系は httptest が重いので緩める）。全体で 85%+ を狙う
B) 全パッケージで一律 85%+
C) 数値目標にこだわらず「主要パスとエラーパスを網羅」で qualitative に

[A]:

### Question C — PR 分割

U-PLT では Code Generation を 2 PR（PR A: Go / PR B: Terraform+CI）に分けた。U-CSS は小さいので:

A) **推奨**: **1 PR にまとめる**（Go + Terraform + Docs 全部、total 推定 +600〜800 行）。レビュー 1 回で済む
B) 2 PR（Go / Terraform+Docs）
C) 3 PR（domain+app / infrastructure+cmd / terraform+docs）

[A]:

### Question D — `SafetyMapSchema()` の初期 `description` をどう書くか

reearth-cms 側の Project / Model / Field に `description` フィールドがある前提。

A) **推奨**: MVP では最小限（Project/Model は短い日本語文、Field は空文字）。ドリフト検知対象外で run コスト低い
B) 全 Field に詳細な説明文を付ける（CMS 管理画面で運用者に親切、記述が増える）
C) 全部空文字（Go 側の記述最小化）

[A]:

### Question E — Logger / Tracer / Meter の取り回し

U-PLT の `observability.Logger(ctx)` / `observability.Tracer(ctx)` / `observability.Meter(ctx)` を使う前提。

`EnsureSchemaUseCase` / `CMSSchemaApplier` への注入方針:

A) **推奨**: Constructor Injection（`New(applier, logger, tracer, meter)`）。test で差し替えやすい
B) Context 取得（`observability.Logger(ctx)` を呼び出し側で都度取得）
C) A のうち logger のみ注入、tracer / meter は ctx 取得

[A]:

### Question F — Retry 戦略の source of truth

`platform/retry.Do` は U-PLT で実装済み。

A) **推奨**: `cmsx.Client` 内部で 5xx / 429 に対して `retry.Do(ctx, 3, 200*time.Millisecond, ...)` を適用。application 層は transient error を知らなくて良い（NFR-CSS-REL-03）
B) application 層（`EnsureSchemaUseCase`）で retry を適用
C) 両方の層で retry（二重、非採用）

[A]:

---

## 承認前の最終確認（回答後に AI が埋めます）

- API 仕様の調査深度: _TBD_
- テストカバレッジ目標: _TBD_
- PR 分割: _TBD_
- SafetyMapSchema の description: _TBD_
- Observability 注入方針: _TBD_
- Retry の配置: _TBD_

回答完了後、矛盾・曖昧さがなければ **Phase 1〜7 を順次実装** → PR 作成。
