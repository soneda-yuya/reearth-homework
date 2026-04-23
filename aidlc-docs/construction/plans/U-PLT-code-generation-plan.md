# U-PLT Code Generation Plan

## Unit Context

- **Unit**: U-PLT（Platform & Proto 基盤）
- **Sprint**: 0
- **依存**: なし（基盤 Unit）
- **下流**: U-CSS / U-ING / U-BFF / U-NTF / U-APP すべてが本 Unit に依存
- **関連 Story**: 13 MVP Story の **間接的前提**（直接 Primary な Story はなし）
- **所在**: すべて **リポジトリルート**（aidlc-docs/ 以外）

---

## Generation Strategy

U-PLT のコード量は多いため、**2 PR に分割** して提出する:

| PR | 内容 | 規模 |
|---|---|---|
| **PR A: Go コード + Proto + Dockerfile + Makefile** | `shared/` / `platform/` / `proto/` / `cmd/*/main.go` / `Dockerfile` / `Makefile` / `go.mod` | 大（~20 ファイル） |
| **PR B: Terraform + GitHub Actions + README** | `terraform/*.tf` / `.github/workflows/*.yml` / `.github/dependabot.yml` / `README.md` | 中（~20 ファイル） |

PR A を先にマージ → PR B を cherry で追加。どちらもレビューしやすいサイズに収まる。

---

## Step-by-step Execution Plan

**ルール**: 各 Step は 1 回の作業単位。すべて **リポジトリルート** に書き出す（`aidlc-docs/` は要約のみ）。

### Phase 1: プロジェクト初期化

- [ ] **Step 1**: `go.mod` を初期化（`module github.com/soneda-yuya/overseas-safety-map`、`go 1.26`）、`tools.go` を作成（buf/connect-go/protoc-gen-go のビルド時依存を宣言）
- [ ] **Step 2**: ディレクトリ構造作成（`cmd/{ingestion,bff,notifier,setup}`、`internal/platform/{config,observability,retry,ratelimit,connectserver,cmsx,firebasex,pubsubx,mapboxx}`、`internal/shared/{errs,clock,validate}`、`proto/v1`、`gen/go/v1`、`terraform`、`.github/workflows`）
- [ ] **Step 3**: `.gitignore` / `.dockerignore` / `.env.example` / `Makefile` の雛形

### Phase 2: Shared パッケージ（依存ゼロ）

- [ ] **Step 4**: `internal/shared/errs/errors.go` — `AppError`, `Kind`（7 値）, `Wrap`, `IsKind`, `KindOf`, `Redact`
- [ ] **Step 5**: `internal/shared/errs/errors_test.go` — unit + `rapid` PBT（ラウンドトリップ）
- [ ] **Step 6**: `internal/shared/clock/clock.go` — `Clock` I/F, `SystemClock`, `FixedClock`
- [ ] **Step 7**: `internal/shared/clock/clock_test.go`
- [ ] **Step 8**: `internal/shared/validate/validate.go` — `NonEmpty`, `IntRange`, `Float64Range`, `DurationOrder`, `LatLng`
- [ ] **Step 9**: `internal/shared/validate/validate_test.go` — unit + `rapid` PBT（境界値）

### Phase 3: Platform パッケージ

- [ ] **Step 10**: `internal/platform/config/config.go` — `Config`（共通部分）+ `MustLoad[T]` ジェネリック関数
- [ ] **Step 11**: `internal/platform/config/config_test.go` — 必須欠落で panic を検証
- [ ] **Step 12**: `internal/platform/observability/setup.go` — `Setup`, `Logger`, `Tracer`, `Meter`, `With`（stdout / gcp exporter 切替）
- [ ] **Step 13**: `internal/platform/observability/recovery.go` — `RecoverInterceptor`（Connect）
- [ ] **Step 14**: `internal/platform/observability/observability_test.go`
- [ ] **Step 15**: `internal/platform/retry/retry.go` — `Policy`, `Do`, `ShouldRetry`
- [ ] **Step 16**: `internal/platform/retry/retry_test.go`
- [ ] **Step 17**: `internal/platform/ratelimit/ratelimit.go` — `Limiter`
- [ ] **Step 18**: `internal/platform/ratelimit/ratelimit_test.go`
- [ ] **Step 19**: `internal/platform/connectserver/server.go` — `Server`, `Start`, `Stop`（Graceful shutdown）
- [ ] **Step 20**: `internal/platform/connectserver/readiness.go` — `Prober`, `HealthzHandler`, `ReadyzHandler`
- [ ] **Step 21**: `internal/platform/connectserver/server_test.go`

### Phase 4: SDK Factory（薄ラッパー）

- [ ] **Step 22**: `internal/platform/cmsx/client.go` — reearth-cms HTTP クライアント stub（`NewClient`, `Close`, `Prober`、Integration API は Adapter 段階で詳細）
- [ ] **Step 23**: `internal/platform/firebasex/app.go` — Firebase Admin SDK 薄ラッパー（`App`, `NewApp`）
- [ ] **Step 24**: `internal/platform/pubsubx/client.go` — Pub/Sub 薄ラッパー
- [ ] **Step 25**: `internal/platform/mapboxx/client.go` — Mapbox Geocoding stub

### Phase 5: Proto 定義 + コード生成

- [ ] **Step 26**: `proto/v1/common.proto`
- [ ] **Step 27**: `proto/v1/safetymap.proto`
- [ ] **Step 28**: `proto/v1/pubsub.proto`
- [ ] **Step 29**: `buf.yaml` / `buf.gen.yaml` / `buf.lock`
- [ ] **Step 30**: `gen/go/v1/` にコード生成（`buf generate` を Make ターゲットに）※ 実際の生成ファイルはコミットするが本 Plan ではスキップ（CI で再生成）

### Phase 6: cmd 雛形

- [ ] **Step 31**: `cmd/ingestion/main.go` — 起動シーケンス雛形（TODO コメントで U-ING 実装指示）
- [ ] **Step 32**: `cmd/bff/main.go` — 同上（U-BFF）
- [ ] **Step 33**: `cmd/notifier/main.go` — 同上（U-NTF）
- [ ] **Step 34**: `cmd/setup/main.go` — 同上（U-CSS）

### Phase 7: ビルド成果物

- [ ] **Step 35**: `Dockerfile`（マルチステージ、ARG DEPLOYABLE）+ `.dockerignore`
- [ ] **Step 36**: `Makefile` 充実版（`test`, `lint`, `vet`, `vuln`, `build`, `proto`, `tf-plan`, `tf-apply`）

**ここまでで PR A 提出 →レビュー → マージ**

### Phase 8: Terraform（13 ファイル）

- [ ] **Step 37**: `terraform/main.tf` + `terraform/versions.tf` + `terraform/variables.tf` + `terraform/outputs.tf`
- [ ] **Step 38**: `terraform/apis.tf` — `google_project_service` 群
- [ ] **Step 39**: `terraform/artifact_registry.tf`
- [ ] **Step 40**: `terraform/secret_manager.tf` — 4 Secret 定義（値は未設定）
- [ ] **Step 41**: `terraform/pubsub.tf` — topic + subscription + DLQ
- [ ] **Step 42**: `terraform/service_accounts.tf` — 5 SA（ci-deployer + 4 runtime）
- [ ] **Step 43**: `terraform/wif.tf` — Workload Identity Pool / Provider
- [ ] **Step 44**: `terraform/iam.tf` — project-level bindings
- [ ] **Step 45**: `terraform/cloud_run_bff.tf` — BFF Service（env + value_source）
- [ ] **Step 46**: `terraform/cloud_run_ingestion.tf` — Ingestion Job
- [ ] **Step 47**: `terraform/cloud_run_notifier.tf` — Notifier Service + Push Subscription
- [ ] **Step 48**: `terraform/cloud_run_setup.tf` — Setup Job
- [ ] **Step 49**: `terraform/cloud_scheduler.tf` — ingestion 5 分毎
- [ ] **Step 50**: `terraform/firestore.tf` — Native mode 初期化
- [ ] **Step 51**: `terraform/README.md` — 使い方

### Phase 9: CI/CD

- [ ] **Step 52**: `.github/workflows/setup-go.yml`（reusable — Go 1.26 + buf + govulncheck）
- [ ] **Step 53**: `.github/workflows/ci.yml` — PR / main push で静的チェック + test + build
- [ ] **Step 54**: `.github/workflows/deploy.yml` — main push（CI 成功後）に docker push + terraform apply
- [ ] **Step 55**: `.github/workflows/terraform-plan.yml` — terraform/ PR で plan コメント
- [ ] **Step 56**: `.github/dependabot.yml`

### Phase 10: ドキュメント

- [ ] **Step 57**: `README.md`（Getting Started / Architecture / Commands / Deployment）
- [ ] **Step 58**: `aidlc-docs/construction/U-PLT/code/summary.md` — Code Generation サマリー（生成ファイル一覧）

**ここまでで PR B 提出 → レビュー → マージ → U-PLT Build and Test へ**

---

## Story Traceability

U-PLT は基盤 Unit のため直接 Primary Story を持たない。ただし下記の NFR を実装することで全 Story を支える:

| NFR | 実装 Step |
|---|---|
| NFR-PLT-SEC-01（Secrets） | Step 40（Secret Manager 定義） + Step 45〜48（Cloud Run value_source） |
| NFR-PLT-SEC-02（脆弱性スキャン） | Step 53（govulncheck in CI） + Step 56（Dependabot） |
| NFR-PLT-OBS-01〜04（観測性） | Step 12〜14（slog + OTel）|
| NFR-PLT-REL-01〜03（信頼性） | Step 13（Recovery）+ Step 19（Graceful shutdown） |
| NFR-PLT-TEST-01〜04（テスト） | Step 5/7/9/11/14/16/18/21（単体 + PBT） |
| NFR-EXT-01/02（抽象化） | Step 22〜25（SDK Factory を I/F で提供）|

---

## Expected Outcomes

PR A マージ後:
- `go build ./...` が成功（空実装 + TODO のみでも OK）
- `go test ./... -race -cover` が緑、カバレッジ 80%+
- `buf lint && buf breaking` が成功
- `golangci-lint run` がクリーン
- `docker build` が 4 Deployable すべて成功

PR B マージ後:
- `terraform plan` がローカルで実行できる（clean plan）
- Workload Identity Federation 設定が入る
- CI ワークフローが GitHub で見える状態になる
- GCP プロジェクトへの apply は手動（初回 Bootstrap）

---

## Risk & Mitigations

| リスク | 影響 | 軽減策 |
|---|---|---|
| 依存ライブラリの最新版でビルドエラー | 高 | CI で検出、ローカルでも `go build` を何度も走らせて確認 |
| `buf generate` のプロトコンパイル失敗 | 中 | `tools.go` で バージョン固定、Makefile で明示化 |
| Secret Manager のリソース名が Cloud Run 起動時に解決できない | 中 | Terraform で依存関係を明示（`depends_on`） |
| Mapbox / Anthropic Go SDK が存在しない／不安定 | 低 | `net/http` + `encoding/json` で自前実装（Adapter 段階で詳細化） |
| Terraform apply で既存リソース衝突 | 中 | initial import は別途手順書（後日）、MVP は新規プロジェクト前提 |

---

## Test Strategy

- **ユニットテスト**: 各 `_test.go` で PBT 含め実装
- **カバレッジ目標**: 全体 80%+、`shared/errs` / `shared/validate` / `platform/config` は 90%+
- **ベンチマーク**: `platform/observability` に `BenchmarkLogInfo` / `BenchmarkTracerStart` / `BenchmarkMeterAdd`
- **統合テスト**: U-PLT 単体では未実施（他 Unit 実装時に成立）
- **Build & Test フェーズ**: CI で Full 実行、結果を `aidlc-docs/construction/U-PLT/build-and-test/` に記録

---

## Plan Summary

- **総ステップ数**: 58
- **Phase 数**: 10（プロジェクト初期化 / Shared / Platform / SDK Factory / Proto / cmd / ビルド成果物 / Terraform / CI/CD / ドキュメント）
- **PR 分割**: 2 PR（Go コード PR A → Terraform+CI PR B）
- **story coverage**: 直接 Primary なし、全 NFR-PLT-* を実装で担保
- **推定作業量**: 大（初期 Go プロジェクト + IaC + CI/CD 一式）

---

## 承認依頼

本計画書で Code Generation（Part 2: Generation）へ進んで良いか、ご確認ください。

以下を明確にご判断いただければ進めます:

1. **58 ステップを 2 PR 分割**で進める方針で OK か（あるいは 1 PR に統合したいか、もっと細かく分けたいか）
2. **Phase 2〜7 を PR A、Phase 8〜10 を PR B** の分け方で違和感ないか
3. 特定 Phase を **スキップまたは延期**したい場合はその指定

承認いただけましたら Part 2（Generation）に入り、Step 1 から順次実行します。
