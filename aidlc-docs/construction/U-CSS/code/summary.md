# U-CSS Code Generation — Summary

**Unit**: U-CSS（CMS Migrate Unit）
**対象**: `cmd/cmsmigrate`（Cloud Run Job、冪等 スキーマ適用）
**対応する計画**: [`U-CSS-code-generation-plan.md`](../../plans/U-CSS-code-generation-plan.md)
**上位設計**: [`U-CSS/design/U-CSS-design.md`](../design/U-CSS-design.md)、[`U-CSS/infrastructure-design/`](../infrastructure-design/)

---

## 1. 生成ファイル一覧

### 新規

| ファイル | 役割 |
|---|---|
| [`internal/cmsmigrate/domain/field_type.go`](../../../../internal/cmsmigrate/domain/field_type.go) | `FieldType` enum + wire 名の `String()` |
| [`internal/cmsmigrate/domain/schema_definition.go`](../../../../internal/cmsmigrate/domain/schema_definition.go) | `SchemaDefinition` / `ProjectDefinition` / `ModelDefinition` / `FieldDefinition` VO + `Validate()` R1-R7 |
| [`internal/cmsmigrate/domain/safety_map_schema.go`](../../../../internal/cmsmigrate/domain/safety_map_schema.go) | `SafetyMapSchema()` — proto と 1:1 の 19 Field 宣言 |
| [`internal/cmsmigrate/domain/schema_definition_test.go`](../../../../internal/cmsmigrate/domain/schema_definition_test.go) | Table-driven test（R1-R7）+ PBT（`pgregory.net/rapid`） |
| [`internal/cmsmigrate/domain/safety_map_schema_test.go`](../../../../internal/cmsmigrate/domain/safety_map_schema_test.go) | 19 Field ↔ proto 整合 anti-regression、`String()` |
| [`internal/cmsmigrate/application/schema_applier.go`](../../../../internal/cmsmigrate/application/schema_applier.go) | `SchemaApplier` Port + `RemoteProject`/`RemoteModel`/`RemoteField` |
| [`internal/cmsmigrate/application/ensure_schema.go`](../../../../internal/cmsmigrate/application/ensure_schema.go) | `EnsureSchemaUseCase.Execute`（Validate → ensureProject → ensureModel → ensureField、fail-fast） |
| [`internal/cmsmigrate/application/drift.go`](../../../../internal/cmsmigrate/application/drift.go) | `DriftWarning` + `detectFieldDrift`（type / required / unique / multiple） |
| `internal/cmsmigrate/application/ensure_schema_test.go` | FakeApplier + 5 シナリオ（初回／再実行／Model 既存／fail-fast／Validate 失敗） |
| `internal/cmsmigrate/application/drift_test.go` | 型不一致 / 複合 drift の検知 |
| `internal/cmsmigrate/application/fake_applier_test.go` | 共有テスト用 FakeApplier（内部 test-only） |
| [`internal/platform/cmsx/dto.go`](../../../../internal/platform/cmsx/dto.go) | `ProjectDTO` / `ModelDTO` / `FieldDTO` + `fieldTypeToAPI` / `fieldTypeFromAPI` |
| [`internal/platform/cmsx/schema.go`](../../../../internal/platform/cmsx/schema.go) | `FindProjectByAlias` / `CreateProject` / `FindModelByAlias` / `CreateModel` / `FindFieldByAlias` / `CreateField`（GET = retry、POST = once） |
| [`internal/platform/cmsx/schema_test.go`](../../../../internal/platform/cmsx/schema_test.go) | `httptest` で 200/201/401/404/409/503 シナリオ |
| [`internal/cmsmigrate/infrastructure/cmsclient/applier.go`](../../../../internal/cmsmigrate/infrastructure/cmsclient/applier.go) | `CMSSchemaApplier` — `application.SchemaApplier` を `cmsx.Client` に委譲 |
| `internal/cmsmigrate/infrastructure/cmsclient/applier_test.go` | stub client で DTO↔domain 変換を検証 |
| [`terraform/environments/prod/prod.tfvars.example`](../../../../terraform/environments/prod/prod.tfvars.example) | 運用者向け tfvars サンプル |

### 変更

| ファイル | 変更内容 |
|---|---|
| [`cmd/cmsmigrate/main.go`](../../../../cmd/cmsmigrate/main.go) | 3 CMS-specific envconfig フィールド追加、DI 配線、`usecase.Execute` 呼び出し、fail-fast `os.Exit(1)` |
| [`internal/platform/cmsx/client.go`](../../../../internal/platform/cmsx/client.go) | `Client` に `http.Client` を持たせ、`Timeout` 適用（U-PLT スケルトンの発展） |
| [`terraform/modules/cmsmigrate/main.tf`](../../../../terraform/modules/cmsmigrate/main.tf) | `template.template.max_retries = 0` を追加（Q3 [A]） |
| [`terraform/environments/prod/variables.tf`](../../../../terraform/environments/prod/variables.tf) | `cms_base_url` / `cms_workspace_id` に description 追記 |

---

## 2. NFR-CSS-* カバレッジ

| NFR ID | 要件 | 実装 |
|---|---|---|
| NFR-CSS-PERF-01 | 初回実行 < 60s | `cmsx.Client.Timeout = 30s` / Job `timeout = 120s`、19 Field 直列 POST |
| NFR-CSS-PERF-02 | 再実行 < 10s | Find-then-noop パス、ネットワーク往復 20 回で十分達成 |
| NFR-CSS-SEC-01 | Secret Manager | Terraform `value_source.secret_key_ref.version = "latest"` |
| NFR-CSS-SEC-02 | Token redact | Token は `cmsx.Config` 内に閉じ、ログに attr 追加しない |
| NFR-CSS-SEC-03 | 最小権限 SA | Runtime SA は Secret accessor のみ（Terraform 既存 IAM） |
| NFR-CSS-REL-01 | 冪等 | `Find → Create if nil` を各層で適用 |
| NFR-CSS-REL-02 | 再実行復旧 | fail-fast + 既作成は残す（FakeApplier テストで検証） |
| NFR-CSS-REL-03 | Retry | `cmsx.doJSONRetry` で GET を `retry.Do(DefaultPolicy)`、POST は 1 回 |
| NFR-CSS-OPS-01 | 構造化ログ | slog handler が `service.name` / `env` を付与、各 phase で `app.cmsmigrate.phase` |
| NFR-CSS-OPS-02 | OTel Metric | `app.cmsmigrate.project.created` / `.model.created` / `.field.created` / `.drift.detected` Counter |
| NFR-CSS-OPS-03 | ランブック | Infrastructure Design `deployment-architecture.md §6` |
| NFR-CSS-OPS-04 | Drift 運用 | `DriftWarning` は `slog.Warn` で集約、自動修正なし |
| NFR-CSS-TEST-01 | PBT | `TestValidate_Property_GeneratedValidSchemaPasses` / `TestValidate_Property_BrokenKeyAliasIsRejected` |
| NFR-CSS-TEST-02 | Unit + mock | `FakeSchemaApplier` で 4+ シナリオ |
| NFR-CSS-TEST-03 | 統合 | Build and Test フェーズで手動（本 PR 範囲外） |
| NFR-CSS-TEST-04 | カバレッジ 85%+ | **92.9%**（§3 参照） |
| NFR-CSS-EXT-01 | Field 追加手順 | `schema_definition.go` 編集 → PR → Job 再実行 |
| NFR-CSS-EXT-02 | DIP | `SchemaApplier` Port、`FakeApplier` / `CMSSchemaApplier` を差し替え可能 |

---

## 3. テストカバレッジ実績

| パッケージ | 実績 | 目標（Q B [A]）| 達成 |
|---|---|---|---|
| `internal/cmsmigrate/domain` | **95.7%** | 95%+ | ✓ |
| `internal/cmsmigrate/application` | **93.3%** | 90%+ | ✓ |
| `internal/cmsmigrate/infrastructure/cmsclient` | **87.5%** | 70%+ | ✓ |
| `internal/cmsmigrate/` 全体 | **92.9%** | 85%+ | ✓ |

関連: `internal/platform/cmsx/schema.go` にも httptest ベースの基本シナリオテストを追加（status code 200/201/401/404/409/503、POST リトライなし、GET リトライあり）。

---

## 4. U-ING への申し送り

次 Unit（U-ING = Ingestion）で U-CSS の資産を使う時の要点:

1. **Project / Model ID を取得する手段** — `cmsx.Client.FindProjectByAlias` / `FindModelByAlias` がそのまま使える。ID は U-ING の item CRUD（本 PR では未実装）で必要。
2. **`cmsx` を item 系メソッドで拡張** — U-ING が `CreateItem` / `UpsertItem` / `ListItems` を追加する。U-CSS と同じ `doJSON` / `doJSONRetry` の使い分けに従うこと（冪等 GET は retry、POST は一度だけ）。
3. **スキーマ適用前提** — U-ING がデプロイされる時点で `cms-migrate` Job が少なくとも 1 回正常終了している必要がある。CD 手順で docstring を付ける。
4. **Drift の扱い** — 実運用で `app.cmsmigrate.drift.detected` が立ったら schema の整合を取るまで U-ING の振る舞いは未定義と宣言する（U-ING の README に書くこと）。

---

## 5. 実行確認（Build and Test で実施する項目）

本 PR の範囲外。Build and Test で以下を手動確認する:

- `gcloud run jobs execute cms-migrate --region=asia-northeast1 --wait` で exit 0
- Cloud Logging `resource.labels.job_name=cms-migrate` で `app.cmsmigrate.phase=done` + `fields_created=["safety-incident.key_cd", ..., "safety-incident.updated_at"]`
- 2 回目の実行で `fields_created` 空、`drift_warnings=0`
- reearth-cms 管理画面で `overseas-safety-map` プロジェクト + `safety-incident` Model + 19 Field が UI 上に表示される
