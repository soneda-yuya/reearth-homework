# U-PLT Tech Stack Decisions

U-PLT で採用する Go 言語・ツール・ライブラリ・CI・観測性スタックを列挙する。バージョン追従方針は Q1 [A] に基づき「Go 最新安定版 + 依存は最新 patch を Dependabot で追従、major は手動」。

---

## 1. 言語・ランタイム

| 項目 | 選定 | 根拠 |
|---|---|---|
| 言語 | **Go 1.26**（最新安定、ユーザー指定） | Q1 回答（Go 1.26 を利用）。`log/slog`（1.21+）と `for range int`（1.22+）を活用 |
| Go module mode | Module（`go.mod`、workspace 非使用） | Q2 [A] 単一モジュール |
| module path | `github.com/soneda-yuya/overseas-safety-map` | 現リポジトリに合わせる |

## 2. Proto / IDL

| 項目 | 選定 | バージョン方針 |
|---|---|---|
| IDL | proto3 | — |
| スキーマ管理 | `buf`（bufbuild/buf CLI） | CI で最新安定を使用 |
| Go 生成プラグイン | `buf.build/gen/go` の `connectrpc/connect-go` / `protocolbuffers/go`（ビルド時取得） | — |
| Dart 生成プラグイン | 別リポ（`overseas-safety-map-app`）で管理。Go と同じ `.proto` から生成 | Flutter 側 CI で取得 |
| 破壊的変更検知 | `buf breaking --against main` を CI 必須 | — |
| Lint | `buf lint` を CI 必須 | デフォルトルールセット |

## 3. Connect RPC（BFF）

| 項目 | 選定 | 備考 |
|---|---|---|
| サーバ | `connectrpc.com/connect` 最新 | HTTP/1 と HTTP/2 の双方で動作 |
| クライアント（テスト用） | `connectrpc.com/connect` + `connectrpc.com/validate`（任意） | Flutter 側は Dart の Connect client |
| ルーティング | `connect-go` が提供する標準 Handler を `net/http` mux に登録（`platform/connectserver`） | 追加ルータ不要 |

## 4. ログ / メトリクス / トレース

| 項目 | 選定 | 根拠 |
|---|---|---|
| ログ | `log/slog`（Go 標準、JSON Handler） | Q2 [A]、標準化、依存ゼロ |
| メトリクス・トレース SDK | `go.opentelemetry.io/otel` / `otel/sdk` 最新 | NFR-OPS-02 / 03 |
| Exporter（ローカル・CI） | `otel/exporters/stdout/stdouttrace` + `stdoutmetric` | Q5 [A] |
| Exporter（dev / prod） | GCP 系: `cloud.google.com/go/trace`、`cloud.google.com/go/monitoring`、`cloud.google.com/go/logging`（または OTel → Cloud Trace/Monitoring 自動エクスポート） | Q5 [A] |
| 切替 | `PLATFORM_OTEL_EXPORTER=stdout\|gcp` env | Config スキーマに追加 |
| ログ相関 | `trace_id` / `span_id` を slog attr に自動注入（OTel の Bridge は使わない） | NFR-PLT-OBS-01 |

## 5. 設定管理

| 項目 | 選定 | 備考 |
|---|---|---|
| 環境変数ローダ | `github.com/kelseyhightower/envconfig` 最新 | `envconfig:"FOO" required:"true" default:"..."` タグで静的検証 |
| バリデーション | `config.Load` 内で型変換・必須検証、失敗時 `log.Fatalf` | NFR-PLT-REL-01 |
| Secrets | **GCP Secret Manager**（`cloud.google.com/go/secretmanager`） | 本番・dev |
| 開発用 | `.env`（`.gitignore`）+ `.env.example`（コミット） | ローカル開発のみ |
| CI 用 | GitHub Secrets（`${{ secrets.XXX }}` で注入） | — |

## 6. Firebase / Cloud SDK

| 項目 | 選定 | 用途 |
|---|---|---|
| Firebase Admin | `firebase.google.com/go/v4` 最新 | Auth ID Token 検証、FCM 送信、Firestore 読み書き |
| Firestore 直接 | Firebase 経由で取得する `firestore.Client` | user / notification Context から利用 |
| Cloud Pub/Sub | `cloud.google.com/go/pubsub` 最新 | ingestion → notifier 連携 |
| Cloud Secret Manager | `cloud.google.com/go/secretmanager` 最新 | Secrets 取得 |
| Google Auth | ADC（Application Default Credentials） | Cloud Run で自動、ローカルは `gcloud auth application-default login` |

## 7. 外部 API クライアント

| 項目 | 選定 | 備考 |
|---|---|---|
| Anthropic Claude | `github.com/anthropics/anthropic-sdk-go` 最新（存在する場合） | 存在しない・不安定なら `net/http` + 自前 JSON クライアント（`platform/mapboxx` と同パターン） |
| Mapbox Geocoding | 自前 `net/http` クライアント | 公式 Go SDK が無いため、`platform/mapboxx` でラップ |
| reearth-cms Integration API | 自前 `net/http` クライアント | OpenAPI（`integration.yml`）から `oapi-codegen` で生成も検討可（Infrastructure Design で決定） |
| MOFA XML | `net/http` + `encoding/xml` | 標準ライブラリで十分 |

## 8. テスト

| 項目 | 選定 | 備考 |
|---|---|---|
| 標準テスト | `testing` | — |
| PBT | `pgregory.net/rapid` 最新 | Q2 推奨、stateful テストも可能 |
| テーブル駆動 | 標準 `t.Run(name, ...)` | — |
| モック | 手書きフェイク実装を基本。自動生成が必要な場合は `go.uber.org/mock/mockgen` | — |
| カバレッジ | `go test -cover -coverprofile=coverage.out` + `go tool cover` | CI で Codecov / GitHub Actions コメント |
| ベンチマーク | `testing.B` + `benchstat`（`golang.org/x/perf/cmd/benchstat`） | PR で前回比可視化 |

## 9. Lint / フォーマット / 静的解析

| 項目 | 選定 | 備考 |
|---|---|---|
| フォーマッタ | `gofmt -s` / `goimports` | CI で diff 検査 |
| Linter | `golangci-lint` 最新 | 有効ルール: `errcheck` / `govet` / `staticcheck` / `gosimple` / `ineffassign` / `revive` / `unused` / `gocritic`（warn） |
| Vuln scan | `govulncheck`（`golang.org/x/vuln/cmd/govulncheck`） | Q4 [A]、CI 必須 |
| Pre-commit | `lefthook`（`github.com/evilmartians/lefthook`） | Infrastructure Design で詳細 |

## 10. CI / CD（U-PLT 時点で決める方針）

| 項目 | 選定 | 備考 |
|---|---|---|
| プラットフォーム | GitHub Actions | Unit 計画で確定 |
| ランナー | `ubuntu-latest` | macOS は Flutter 側で使用 |
| キャッシュ | `actions/cache` で `$GOMODCACHE` と `~/.cache/go-build` | NFR-PLT-PERF-01 / 02 |
| ワークフローファイル | `.github/workflows/ci.yml`（PR / push トリガ） | 他 Unit でも再利用可能な composite action を `ci/` 配下に作る |
| 実行内容 | `go mod download` → `buf lint` → `buf breaking` → `gofmt check` → `golangci-lint` → `go test -race -cover` → `govulncheck` | 順次 |

## 11. 依存管理 / 更新

| 項目 | 選定 | 備考 |
|---|---|---|
| Go modules 更新 | **Dependabot**（`.github/dependabot.yml` で gomod + github-actions を daily スキャン） | Q4 [A] |
| Vuln 対応 SLA | Critical 即日、High 3 日以内、Medium 週次 | NFR-PLT-SEC-02 |
| Major バージョン | 手動で上げる（breaking change 検証） | Q1 [A] |

## 12. ドキュメント

| 項目 | 選定 | 備考 |
|---|---|---|
| README | `README.md` に Getting Started / Architecture Overview / Unit 一覧（aidlc-docs へのリンク） | — |
| godoc | 全公開 API | pkg.go.dev で自動生成可能な形式 |
| 設計 | `aidlc-docs/` の AIDLC 成果物 | — |

---

## 13. 不採用の選択肢と理由

| 候補 | 不採用理由 |
|---|---|
| `github.com/sirupsen/logrus` / `go.uber.org/zap` | Go 1.21+ の `log/slog` で十分、依存削減 |
| `github.com/spf13/viper` | envconfig の方が軽量、YAML/TOML 不要 |
| `github.com/labstack/echo` / `github.com/gin-gonic/gin` | Connect-go の `net/http` ベースで完結 |
| `github.com/golang/mock` | アーカイブ済、`go.uber.org/mock` が後継 |
| `trivy` / `snyk`（OSS）をスキャナとして単独採用 | `govulncheck` で Go 固有脆弱性をカバー、Dependabot で依存更新、不足時に追加検討 |
| OpenTelemetry Logs SDK | slog と二重運用になるため不採用、trace_id のみ attr 注入で相関する |

---

## 14. サマリー

| レイヤ | スタック |
|---|---|
| 言語 | Go 1.26 |
| RPC | Connect（proto3 + buf） |
| ログ | slog（JSON） |
| メトリクス / トレース | OpenTelemetry（stdout / GCP 切替） |
| Config | envconfig + GCP Secret Manager |
| SDK | Firebase Admin v4 / Cloud Pub/Sub / Cloud Secret Manager / Mapbox (自前 HTTP) / Anthropic Claude |
| テスト | testing + rapid（PBT） |
| Lint | golangci-lint + gofmt + buf lint |
| CI | GitHub Actions（ubuntu-latest） |
| 依存管理 | Dependabot（daily） + govulncheck（Critical/High ブロック） |
