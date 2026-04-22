# U-CSS Design (Minimal 合本版)

**Unit**: U-CSS（CMS Migrate Unit、Sprint 1）
**Deployable**: `cmd/cmsmigrate`（Cloud Run Job、手動実行、冪等）
**Bounded Context**: `cmsmigrate`（DDD: domain / application / infrastructure）
**ワークフロー圧縮**: Option B（Functional Design + NFR Requirements + NFR Design を 1 本に集約）

---

## 0. Design Decisions（計画回答の確定）

U-CSS 設計計画（[`plans/U-CSS-design-plan.md`](../../plans/U-CSS-design-plan.md)）の Q1–Q6 すべて **[A]** で確定。

| # | 決定事項 | 選択 | 要旨 |
|---|---------|------|------|
| Q1 | SchemaDefinition 対象範囲 | **A** | Project + Model + Field の **フル宣言**。`SafetyIncident` = 19 フィールド |
| Q2 | 既存リソースとの差分適用 | **A** | **冪等 CREATE**。drift は `WARN` ログのみ、自動上書きしない |
| Q3 | 実行モード | **A** | **手動実行のみ**（`gcloud run jobs execute`）。CI/CD 起動なし |
| Q4 | 部分失敗時の挙動 | **A** | **fail-fast**。exit 1、未作成は次回実行で補完（冪等） |
| Q5 | テスト戦略 | **A** | Unit + PBT（`SchemaDefinition`） + `cms.Client` モック（`SchemaApplier`）、実 CMS は Build & Test で手動 |
| Q6 | Field 定義の保守 | **A** | **Go コードで宣言**（`schema_definition.go`）。変更は PR + Job 再実行 |

> ※ 計画では「18 フィールド」と表記したが、proto（[`proto/v1/safetymap.proto`](../../../../proto/v1/safetymap.proto#L11-L31)）の再確認で実体は **19 フィールド**（`lead` を含む）。本 design は 19 で確定。

---

## 1. Functional Design

### 1.1 Context — U-CSS の責務

**目的**: reearth-cms の Project / Model / Field を宣言的に管理し、U-ING / U-BFF が依存する **SafetyIncident Model とそのスキーマ** を CMS 側に揃える。

**ライフサイクル**: 一回限り（初期デプロイ） + スキーマ変更時の再実行。U-CSS Job が実行されていない限り、U-ING / U-BFF は正常に動作しない（前提インフラ）。

**非責務**:
- データの CRUD（U-ING / U-BFF の領分）
- 定期実行 / 自動再実行（Scheduler 不使用 — Q3 [A]）
- スキーマの UPDATE / DELETE（Q1/Q2 [A] — 冪等 CREATE のみ、破壊的操作なし）

### 1.2 Domain Model（`internal/cmsmigrate/domain`）

#### 1.2.1 `SchemaDefinition`（Aggregate Root、VO 相当）

```go
// internal/cmsmigrate/domain/schema_definition.go
package domain

type SchemaDefinition struct {
    Project ProjectDefinition
    Models  []ModelDefinition
}

type ProjectDefinition struct {
    Alias       string // e.g. "overseas-safety-map"
    Name        string // 表示名
    Description string
}

type ModelDefinition struct {
    Alias       string // e.g. "safety-incident"
    Name        string
    Description string
    KeyFieldAlias string        // 一意キーとして扱うフィールド（= "key_cd"）
    Fields        []FieldDefinition
}

type FieldDefinition struct {
    Alias       string     // e.g. "key_cd"
    Name        string     // 表示名
    Type        FieldType  // enum
    Required    bool
    Unique      bool
    Multiple    bool
    Description string
}

type FieldType int

const (
    FieldTypeUnspecified FieldType = iota
    FieldTypeText         // string（短文）
    FieldTypeTextArea     // string（長文）
    FieldTypeURL
    FieldTypeDate         // RFC3339 / Timestamp
    FieldTypeSelect       // 将来用（U-CSS では使わない）
    FieldTypeGeometryObject // GeoJSON ライク
)
```

**不変条件（`SchemaDefinition.Validate`）**:

| Rule | 説明 |
|------|------|
| R1 | `Project.Alias` が非空、`^[a-z][a-z0-9-]*$` マッチ |
| R2 | `Models` が 1 件以上 |
| R3 | 各 `Model.Alias` が一意（重複禁止） |
| R4 | 各 `Model.Fields` が 1 件以上 |
| R5 | `Field.Alias` が Model 内で一意、`^[a-z][a-z0-9_]*$`（snake_case）マッチ |
| R6 | `Field.Type != FieldTypeUnspecified` |
| R7 | `Model.KeyFieldAlias` が `Fields` に存在、かつ `Required = true` かつ `Unique = true` |

検証失敗は `errs.Wrap("...", errs.KindInvalidArgument, err)` で返却。

#### 1.2.2 `SafetyIncident` Model 宣言（19 フィールド、proto 整合）

| # | Alias (snake_case) | Type | Required | Unique | 用途 |
|---|---|---|---|---|---|
| 1 | `key_cd` | Text | ✓ | ✓ | MOFA 一意キー（upsert 用） |
| 2 | `info_type` | Text | ✓ | | 情報種別（危険・スポット等） |
| 3 | `info_name` | Text | | | 情報種別 表示名 |
| 4 | `leave_date` | Date | ✓ | | 発出日時（UTC） |
| 5 | `title` | Text | ✓ | | タイトル |
| 6 | `lead` | TextArea | | | リード文 |
| 7 | `main_text` | TextArea | | | 本文（PII 扱い注意） |
| 8 | `info_url` | URL | | | 元記事 URL |
| 9 | `koukan_cd` | Text | | | 公館コード |
| 10 | `koukan_name` | Text | | | 公館名 |
| 11 | `area_cd` | Text | | | 地域コード |
| 12 | `area_name` | Text | | | 地域名 |
| 13 | `country_cd` | Text | ✓ | | 国コード |
| 14 | `country_name` | Text | | | 国名 |
| 15 | `extracted_location` | Text | | | LLM 抽出地名（原文） |
| 16 | `geometry` | GeometryObject | | | WGS84 Point（Mapbox or 国重心） |
| 17 | `geocode_source` | Text | | | `mapbox` / `country_centroid` |
| 18 | `ingested_at` | Date | ✓ | | 取込時刻 |
| 19 | `updated_at` | Date | ✓ | | 最終更新時刻 |

**Key**: `key_cd`（MOFA の XML に含まれる一意 ID、Required + Unique）。U-ING は `key_cd` で upsert する。

### 1.3 Application Layer（`internal/cmsmigrate/application`）

#### 1.3.1 `EnsureSchemaUseCase`

```go
package application

type EnsureSchemaUseCase struct {
    applier SchemaApplier // port
    logger  *slog.Logger
}

type EnsureSchemaInput struct {
    Definition domain.SchemaDefinition
}

type EnsureSchemaResult struct {
    ProjectCreated bool
    ModelsCreated  []string
    FieldsCreated  []string
    DriftWarnings  []DriftWarning
}

type DriftWarning struct {
    Resource string // "Model:safety-incident" / "Field:safety-incident.title"
    Reason   string // "type mismatch: want Text got TextArea"
}

func (u *EnsureSchemaUseCase) Execute(ctx context.Context, in EnsureSchemaInput) (EnsureSchemaResult, error)
```

**アルゴリズム**（Q2 [A] 冪等 CREATE + Q4 [A] fail-fast）:

```
1. Validate(Definition) — R1..R7。失敗 → return error（exit 1）
2. Project 確保:
   p, err := applier.FindProject(ctx, Definition.Project.Alias)
   if p == nil:   applier.CreateProject(...) → ProjectCreated = true
   else:          (no-op)
   err != nil → return err（fail-fast）
3. 各 Model について:
   m, err := applier.FindModel(ctx, projectID, Model.Alias)
   if m == nil:   applier.CreateModel(...) → ModelsCreated += Alias
   else:          既存フィールドと宣言を照合 → 差分は DriftWarnings に追加（warning のみ、stop しない）
   err != nil → return err
4. 各 Model の Field について:
   f, err := applier.FindField(ctx, modelID, Field.Alias)
   if f == nil:   applier.CreateField(...) → FieldsCreated += "<model>.<alias>"
   else if 差分: DriftWarnings に追加
   err != nil → return err（即 fail-fast、未作成 Field は次回実行に任せる）
5. DriftWarnings を slog.Warn でまとめて出力
6. return (result, nil)
```

**Observability**:
- 各ステップの前後で `observability.Tracer` で Span（`cmsmigrate.EnsureSchema`、`cmsmigrate.CreateField` 等）
- `app.cmsmigrate.project.created`、`app.cmsmigrate.model.created`、`app.cmsmigrate.field.created`、`app.cmsmigrate.drift.detected` のカウンタ Metric

#### 1.3.2 `SchemaApplier` Port（Hexagonal の Outbound Port）

```go
package application

type SchemaApplier interface {
    FindProject(ctx context.Context, alias string) (*RemoteProject, error)
    CreateProject(ctx context.Context, def domain.ProjectDefinition) (*RemoteProject, error)

    FindModel(ctx context.Context, projectID, alias string) (*RemoteModel, error)
    CreateModel(ctx context.Context, projectID string, def domain.ModelDefinition) (*RemoteModel, error)

    FindField(ctx context.Context, modelID, alias string) (*RemoteField, error)
    CreateField(ctx context.Context, modelID string, def domain.FieldDefinition) (*RemoteField, error)
}

type RemoteProject struct { ID, Alias, Name string }
type RemoteModel   struct { ID, Alias string; Fields []RemoteField }
type RemoteField   struct { ID, Alias string; Type domain.FieldType; Required, Unique, Multiple bool }
```

### 1.4 Infrastructure Adapter（`internal/cmsmigrate/infrastructure/cmsclient`）

`SchemaApplier` を reearth-cms Integration REST API で実装する `CMSSchemaApplier`。基盤 HTTP クライアントは `internal/platform/cmsx.Client`（U-PLT スケルトン）を U-CSS で **拡張**（Project/Model/Field 系メソッドを追加）する。

```go
// internal/cmsmigrate/infrastructure/cmsclient/applier.go
type CMSSchemaApplier struct {
    client *cmsx.Client
    logger *slog.Logger
}

func New(client *cmsx.Client, logger *slog.Logger) *CMSSchemaApplier { ... }

// application.SchemaApplier 実装
func (a *CMSSchemaApplier) FindProject(ctx context.Context, alias string) (*application.RemoteProject, error) { ... }
// ...
```

**HTTP マッピング**（reearth-cms Integration API の Project/Model/Field エンドポイント想定、URL は Build & Test で実サーバに対して確定）:
- `GET /api/workspaces/{workspaceID}/projects?alias={alias}` → Find
- `POST /api/workspaces/{workspaceID}/projects` → Create
- `GET /api/projects/{projectID}/models/{modelAlias}` → Find
- `POST /api/projects/{projectID}/models` → Create
- `GET /api/models/{modelID}/fields/{fieldAlias}` → Find
- `POST /api/models/{modelID}/fields` → Create

**共通**: `Authorization: Bearer {CMSMIGRATE_CMS_INTEGRATION_TOKEN}`（Secret Manager 経由、env value_source）。

**エラー変換**:
- 404 → nil（"not found" を `FindXxx` の戻り値 nil で表現、err は nil）
- 401/403 → `errs.Wrap("cms: unauthorized", errs.KindUnauthenticated, err)` → fail-fast
- 409 → `errs.Wrap("cms: conflict", errs.KindConflict, err)` → fail-fast（別プロセスと競合）
- 429 / 5xx → U-PLT の `platform/retry` で指数バックオフ（`retry.Do`、max 3 回）
- その他 → `errs.KindInternal` で Wrap

**timeout**: 各リクエスト 30s（`cmsx.Config.Timeout`）。全体 Job としては Cloud Run Job `task.timeout = 600s` で上限。

### 1.5 Composition Root — `cmd/cmsmigrate/main.go`（拡張計画）

現状（U-PLT スケルトン）から以下を追加する:

```go
type cmsmigrateConfig struct {
    config.Common
    CMSBaseURL            string `envconfig:"CMS_BASE_URL" required:"true"`
    CMSWorkspaceID        string `envconfig:"CMS_WORKSPACE_ID" required:"true"`
    CMSIntegrationToken   string `envconfig:"CMS_INTEGRATION_TOKEN" required:"true"` // Secret
}

func main() {
    // ...既存の config / observability / signal セットアップ...

    client := cmsx.NewClient(cmsx.Config{
        BaseURL:     cfg.CMSBaseURL,
        WorkspaceID: cfg.CMSWorkspaceID,
        Token:       cfg.CMSIntegrationToken,
        Timeout:     30 * time.Second,
    })
    defer client.Close(ctx)

    applier := cmsclient.New(client, logger)
    usecase := application.NewEnsureSchemaUseCase(applier, logger)

    def := domain.SafetyMapSchema() // Q6 [A]: Go コードで宣言
    result, err := usecase.Execute(ctx, application.EnsureSchemaInput{Definition: def})
    if err != nil {
        logger.Error("ensure schema failed", "err", err)
        os.Exit(1) // Q4 [A] fail-fast
    }
    logger.Info("ensure schema done",
        "project_created", result.ProjectCreated,
        "models_created", result.ModelsCreated,
        "fields_created", result.FieldsCreated,
        "drift_warnings", len(result.DriftWarnings),
    )
}
```

`domain.SafetyMapSchema()` が §1.2.2 の 19 フィールドを返すファクトリ（`schema_definition.go` に定数的にべた書き、Q6 [A]）。

### 1.6 Sequence（初回実行 / 再実行）

**初回実行（全て空）**:
```
main → EnsureSchemaUseCase
  → FindProject("overseas-safety-map") → nil
  → CreateProject(...)                 → ok
  → FindModel(projectID, "safety-incident") → nil
  → CreateModel(...)                   → ok
  → FindField × 19 → 全部 nil
  → CreateField × 19                   → 順次 ok
  → no drift, result returned
```

**再実行（全て揃っている、no-op）**:
```
main → EnsureSchemaUseCase
  → FindProject → exists, no-op
  → FindModel   → exists
  → FindField × 19 → 全 exists、Field 毎に差分チェック
    - 一致 → no-op
    - 不一致 → DriftWarnings に追加（Q2 [A]）
  → 0 creation, drift_warnings=N
```

**再実行（Field を 1 個追加したあと）**:
```
→ Project / Model / 既存 18 Field は no-op
→ 新しい 19 個目の Field だけ CreateField → ok
→ result.FieldsCreated = ["safety-incident.new_field"]
```

---

## 2. NFR Requirements（U-CSS 固有）

U-PLT の NFR 要件（[`U-PLT/nfr-requirements/nfr-requirements.md`](../../U-PLT/nfr-requirements/nfr-requirements.md)）を **前提として継承**。以下は U-CSS 固有値。

### 2.1 性能

- **NFR-CSS-PERF-01**: 初回実行（19 Field 新規作成）の Job 完了時間 **< 60 秒**（reearth-cms のレスポンスが 500ms/req 想定、順次 + 余裕分）
- **NFR-CSS-PERF-02**: 再実行（全 no-op）の Job 完了時間 **< 10 秒**（Find のみで CREATE なし）
- **測定**: Cloud Run Job 実行ログの `duration_ms`、`observability.Tracer` の Root Span

### 2.2 セキュリティ

- **NFR-CSS-SEC-01**: `CMS_INTEGRATION_TOKEN` は GCP Secret Manager に保管し、Cloud Run Job 定義の `env.value_source.secret_key_ref` で注入（U-PLT NFR-PLT-SEC-01 を踏襲）。ローカル開発は `.env`（`.gitignore` 登録）
- **NFR-CSS-SEC-02**: Token が万一ログに出ないこと — `slog` の Marshaler で `Token` を `[REDACTED]` 置換。Handler レベルの attr filter も導入
- **NFR-CSS-SEC-03**: Runtime SA (`cmsmigrate-runtime`) は **Secret 読み取り + OTel 送信のみ** の最小権限。CMS 側への認可は Bearer Token のみで Google IAM には依存しない

### 2.3 信頼性 / 冪等性

- **NFR-CSS-REL-01**: 同一 `SchemaDefinition` で N 回実行しても CMS 側に重複リソースが生まれないこと（冪等、Q2 [A]）。PBT で Find→Create→Find→Create 2 回呼びの同値性を保証
- **NFR-CSS-REL-02**: 途中失敗からの復旧は「再実行」で完結すること（Q4 [A]、既作成は残して次回補完）。ロールバック機構は持たない
- **NFR-CSS-REL-03**: reearth-cms の一時的 429 / 5xx は `platform/retry.Do`（max 3、exponential backoff 200ms→400ms→800ms、jitter ±25%）で吸収。恒久障害は fail-fast

### 2.4 運用 / 可観測性

- **NFR-CSS-OPS-01**: 構造化 JSON ログ（slog）で以下属性を必須: `service.name=cmsmigrate`、`env=prod`、`trace_id`、`span_id`、`app.cmsmigrate.phase`（= `validate` / `find-project` / `create-model` 等）
- **NFR-CSS-OPS-02**: OTel Metric（Counter）: `app.cmsmigrate.project.created`、`app.cmsmigrate.model.created`、`app.cmsmigrate.field.created`、`app.cmsmigrate.drift.detected`、`app.cmsmigrate.run.failure`
- **NFR-CSS-OPS-03**: 実行手順は [`construction/U-CSS/design/U-CSS-design.md`](./U-CSS-design.md) §4.1 を運用ランブックとしてそのまま利用
- **NFR-CSS-OPS-04**: Drift 警告が検出されたら、運用者は手動で reearth-cms 側を修正するか、`SchemaDefinition` を合わせるかを選ぶ（自動解決しない、Q2 [A]）

### 2.5 テスト / 品質

- **NFR-CSS-TEST-01**: `SchemaDefinition.Validate` は unit + PBT（`rapid` で Alias 生成、R1–R7 の各ルール逸脱を反証）でカバー率 **90%+**
- **NFR-CSS-TEST-02**: `EnsureSchemaUseCase` は `SchemaApplier` の fake 実装（in-memory）で unit test。初回 / 再実行 / drift / fail-fast の 4 シナリオを網羅（Q5 [A]）
- **NFR-CSS-TEST-03**: `CMSSchemaApplier` は `httptest.Server` でのラウンドトリップ test を 1 本（`PackageLevel`）用意してみる **のは Build & Test フェーズ任意**（Q5 [A] ベース路線は mock で unit、統合は Build & Test 手動）
- **NFR-CSS-TEST-04**: Go カバレッジ `cmsmigrate/` 配下で **85%+**。`govulncheck` / `golangci-lint` は U-PLT CI にそのまま乗る

### 2.6 拡張性

- **NFR-CSS-EXT-01**: 新しい Field を追加する手順は「`schema_definition.go` に追記 → PR レビュー → Job 再実行」の 3 ステップで完結（Q6 [A]）
- **NFR-CSS-EXT-02**: 将来 `SchemaApplier` の別実装（e.g. CMS 以外の Firestore 直書き）に差し替えるのが容易であること — `SchemaApplier` Port を `application` 層に置いて DIP を担保

---

## 3. NFR Design Patterns（U-CSS 固有）

### 3.1 Idempotent CREATE パターン（Q2 [A]）

**問題**: 同じ `SchemaDefinition` で再実行しても、既存リソースを重複作成しない。

**解法**: 「Read-then-Write（Find → CREATE）」を各リソース単位で行う。

```go
func ensureField(ctx context.Context, a SchemaApplier, modelID string, def FieldDefinition) (*RemoteField, bool, error) {
    existing, err := a.FindField(ctx, modelID, def.Alias)
    if err != nil { return nil, false, err }
    if existing != nil {
        if driftDetected(existing, def) {
            // 記録のみ、overwrite しない
            return existing, false, nil // created = false
        }
        return existing, false, nil
    }
    created, err := a.CreateField(ctx, modelID, def)
    if err != nil { return nil, false, err }
    return created, true, nil
}
```

**性質**:
- **Safety**: 既存データを壊さない（UPDATE/DELETE なし）
- **Liveness**: N 回実行で収束（新規分だけ差分適用）
- **Race condition**: 2 並列実行時は `POST` で 409 が返り得る → `errs.KindConflict` に分類して fail-fast（Q3 [A] 手動実行なので実質発生しない）

### 3.2 Fail-Fast パターン（Q4 [A]）

**問題**: 19 Field の作成途中で 5 個目でエラー。どう振る舞うか？

**解法**: 即 return → exit 1。既作成 4 個はそのまま残し、次回実行で `FindField` が ok を返すので自然に 5 個目から補完される（冪等 + fail-fast の合わせ技）。

**実装**:
```go
for _, field := range model.Fields {
    if _, created, err := ensureField(ctx, a, modelID, field); err != nil {
        // ログに失敗した Field 名を残す
        logger.Error("create field failed",
            "model.alias", model.Alias,
            "field.alias", field.Alias,
            "err", err,
        )
        return result, err // Use Case 即終了
    } else if created {
        result.FieldsCreated = append(result.FieldsCreated, model.Alias+"."+field.Alias)
    }
}
```

**非採用案**:
- **ベストエフォート続行**: 複数エラーを集約できるが、後続の Field 作成が前提（存在する Field に依存する）のケースを壊し得る → 採用せず
- **ロールバック**: CMS 側に DELETE API を呼ぶのはデータ削除の副作用が重い + 途中まで作った Field を消す意味が薄い（次回補完される）→ 採用せず

### 3.3 SchemaDefinition-as-Code パターン（Q6 [A]）

**問題**: Field 定義の真実（Source of Truth）はどこに置くか？

**解法**: Go コード（`schema_definition.go`）のみを真実とし、PR レビュー + Job 再実行のパイプラインに乗せる。

```go
// internal/cmsmigrate/domain/schema_definition.go
package domain

func SafetyMapSchema() SchemaDefinition {
    return SchemaDefinition{
        Project: ProjectDefinition{
            Alias:       "overseas-safety-map",
            Name:        "Overseas Safety Map",
            Description: "外務省 海外安全情報を地図で可視化するための CMS プロジェクト",
        },
        Models: []ModelDefinition{safetyIncidentModel()},
    }
}

func safetyIncidentModel() ModelDefinition {
    return ModelDefinition{
        Alias:         "safety-incident",
        Name:          "SafetyIncident",
        KeyFieldAlias: "key_cd",
        Fields: []FieldDefinition{
            {Alias: "key_cd",    Name: "Key CD",    Type: FieldTypeText,    Required: true, Unique: true},
            {Alias: "info_type", Name: "Info Type", Type: FieldTypeText,    Required: true},
            // ...残り 17 フィールド（§1.2.2 の表と 1:1）
            {Alias: "updated_at", Name: "Updated At", Type: FieldTypeDate,  Required: true},
        },
    }
}
```

**非採用案の却下理由**:
- **外部 YAML / JSON**: proto との整合性を型で担保できない、`go:embed` 追加分のコストに見合わない
- **環境変数 override**: 宣言的設計を壊す、運用ミスで prod 破壊のリスク

### 3.4 Drift 警告パターン（Q2 [A]）

**問題**: 既存 Field の型が宣言と食い違っていたらどうする？

**解法**: `DriftWarning{}` に記録、最後にまとめて `slog.Warn` 出力。**自動修正せず、運用者判断に委ねる**。

```go
type DriftWarning struct {
    Resource string // "Field:safety-incident.title"
    Reason   string // "type mismatch: want=Text got=TextArea"
}

// 集約ログ
if len(result.DriftWarnings) > 0 {
    attrs := make([]any, 0, len(result.DriftWarnings)*2)
    for i, w := range result.DriftWarnings {
        attrs = append(attrs, "drift."+itoa(i)+".resource", w.Resource,
                              "drift."+itoa(i)+".reason", w.Reason)
    }
    logger.Warn("schema drift detected (no auto-apply)", attrs...)
}
```

Cloud Logging で `severity=WARNING` かつ `app.cmsmigrate.drift.detected` カウンタが `>0` のアラートを（将来的に）張る。

### 3.5 Port / Adapter 分離（Hexagonal）

```
┌──────────────────── cmd/cmsmigrate/main.go ──────────────────────┐
│  config + observability + signal                                  │
│                         │                                         │
│                         ▼                                         │
│            EnsureSchemaUseCase ──── SchemaApplier (Port)          │
│                                          │                        │
└──────────────────────────────────────────┼────────────────────────┘
                                           │
                          ┌────────────────┴────────────────┐
                          │                                 │
                          ▼                                 ▼
               CMSSchemaApplier                    InMemoryFakeApplier
               (infrastructure/cmsclient)          (test 用)
                          │
                          ▼
                  cmsx.Client (HTTP)
                          │
                          ▼
                 reearth-cms Integration REST
```

- **application** 層は外部依存ゼロ（`SchemaApplier` Port、`domain` VO のみ）
- **infrastructure/cmsclient** は `cmsx.Client` + HTTP 経由
- **test** は `InMemoryFakeApplier`（map 保持）で Application Use Case をユニット検証（Q5 [A]）

---

## 4. 運用ランブック

### 4.1 初回デプロイ / スキーマ変更時の手順

1. `schema_definition.go` を編集（新フィールド追加など）→ PR → レビュー → main merge
2. `deploy.yml`（通常の CD）が main 更新で起動 → `cmd/cmsmigrate` のイメージも自動ビルド + Cloud Run Job 定義が Terraform で更新される
3. **手動実行**:
   ```bash
   gcloud run jobs execute cms-migrate \
     --region=asia-northeast1 \
     --project=overseas-safety-map \
     --wait
   ```
4. 実行ログ（Cloud Logging、`resource.labels.job_name=cms-migrate`）で `project_created` / `models_created` / `fields_created` を確認
5. Drift 警告が出ていないか `app.cmsmigrate.drift.detected` カウンタで確認

### 4.2 失敗した場合

1. ログで失敗した `field.alias` を特定
2. 原因（権限 / 型不整合 / reearth-cms 側の障害）を切り分け
3. 修正後、同じ手順で再実行（冪等 + fail-fast により既作成は保持、失敗した箇所から再開）

### 4.3 Drift 検出時の選択肢

- **宣言を現状に合わせる**: `schema_definition.go` を編集して PR → 次回 Job 再実行で warning が消える
- **CMS 側を宣言に合わせる**: reearth-cms の UI / API で手動修正（U-CSS Job は UPDATE しないため）

---

## 5. 次フェーズ（Infrastructure Design）での未決事項

本 design で **決めない** もの（U-CSS Infrastructure Design へ持ち越し）:

- Cloud Run Job の具体的な `cpu` / `memory` / `task.timeout` / `max_retries` の数値（現状 U-PLT 既定の 1 CPU / 512Mi / 600s を使う想定）
- Terraform `google_cloud_run_v2_job` の env 定義と Secret バインド（U-PLT の既存雛形を踏襲）
- Artifact Registry のビルド / push 手順（既に U-PLT で ci.yml / deploy.yml 整備済み、cmsmigrate も matrix に含まれるため追加作業は最小）
- IAM — `cmsmigrate-runtime` SA に必要な追加ロール（現状は Secret 読み取り + OTel 送信のみ、追加不要見込み）

---

## 6. トレーサビリティ

| 上位要件 | U-CSS 対応 |
|---|---|
| NFR-SEC-01 (Secret 管理) | NFR-CSS-SEC-01/02/03 |
| NFR-OPS-01 (構造化ログ) | NFR-CSS-OPS-01/02 |
| NFR-EXT-01 (拡張性 / DIP) | NFR-CSS-EXT-01/02、§3.5 Port/Adapter |
| NFR-REL (冪等性 / 再実行性) | NFR-CSS-REL-01/02/03、§3.1 Idempotent CREATE |
| REQ-DATA-01 (CMS データストア) | §1.2.2 SafetyIncident Model = 19 フィールド |
| Unit Plan: U-CSS 責務 | §1.1 Context + §1.3 Application |

---

## 7. 承認プロセス

- **本ドキュメントの承認**: ユーザーレビュー → LGTM で次ステップへ
- **次ステップ**: U-CSS Infrastructure Design（`construction/U-CSS/infrastructure-design/`）
- U-PLT で整備済みの Terraform module（`terraform/modules/cmsmigrate/`）があるため、Infrastructure Design は薄く済む見込み（IAM / 環境変数 / Secret バインドの確認が中心）
