# Build and Test Summary — U-PLT

## Build Status

| 項目 | ツール / バージョン | ステータス |
|---|---|---|
| Go ビルド | Go 1.26.x | ✅ 成功（`go build ./...`） |
| Proto コード生成 | buf 1.47.2 / connect-go v1.18.1 / protobuf-go v1.36.5 | ✅ 成功（`buf generate`） |
| Docker イメージ | gcr.io/distroless/static-debian12:nonroot | ✅ 4 Deployable すべて成功（bff / ingestion / notifier / setup） |
| Terraform 検証 | Terraform 1.9 / google provider 6.x | ✅ `fmt -check -recursive` + `validate` 成功 |

### ビルド成果物
- Go バイナリ: ローカル `bin/{deployable}`、本番 `asia-northeast1-docker.pkg.dev/overseas-safety-map/app/{deployable}:{git-sha}`
- Proto 生成コード: `gen/go/v1/*.pb.go`（CI で常時生成）
- Docker イメージ: distroless ベース、`USER nonroot`

## Test Execution Summary

### Unit Tests
- **実行コマンド**: `go test -race -coverprofile=coverage.out ./...`
- **テスト結果**: **全 pass**（失敗ゼロ）
- **カバレッジ合計**: **67.4%**（NFR-PLT-TEST-01 目標 80% に未達）
- **ステータス**: ⚠ Pass（カバレッジのみ未達、後続 Unit で底上げ）

#### パッケージ別カバレッジ
| パッケージ | カバレッジ | 目標 | 備考 |
|---|---:|---:|---|
| `shared/clock` | 100% | — | ✓ |
| `shared/validate` | 100% | 90%+ | ✓ |
| `platform/ratelimit` | 100% | — | ✓ |
| `shared/errs` | 85.2% | 90%+ | Redact の境界値追加テストで到達可能 |
| `platform/retry` | 78.1% | — | ShouldRetry の分岐網羅で改善余地 |
| `platform/observability` | 60.9% | — | OTel 初期化の代替検証が困難 |
| `platform/connectserver` | 49.3% | — | Start/Stop ライフサイクルは統合テスト寄り |
| `platform/config` | 33.3% | 90%+ | `os.Exit` パスは構造的に unit テスト不可（envconfig 直接呼び出しで代替検証済み） |

#### PBT（pgregory.net/rapid）適用済み
- `errs.Wrap` / `IsKind` / `KindOf` のラウンドトリップ性質
- `errs.Redact` の first2 + "..." + last2 形式保持
- `validate.IntRange` / `LatLng` の境界値

### Integration Tests
- U-PLT スコープでは疎通確認のみ（他 Unit 未実装のため統合先がない）
- 確認内容: `/healthz` 200、`/readyz` 200、Docker 起動、Terraform validate
- ステータス: ✅ 疎通 OK

### Performance Tests
- U-PLT スコープでは未実施（MVP 範囲外）
- NFR-PLT-PERF-03/04（`slog.Info < 10μs`、`OTel Span < 50μs`）のベンチマークは後続 Unit で追加予定

### Security Tests
- **govulncheck**: Go stdlib + 依存パッケージに既知 CVE なし（Go 1.26.x check-latest + OTel v1.43.0 で `GO-2026-4394` 等は修正済み）
- **Dependabot**: gomod daily / GitHub Actions + Docker weekly、OpenTelemetry と Google Cloud は group で束ねる
- **Secrets 管理**: `.env` は `.gitignore`、本番は GCP Secret Manager + Cloud Run `env.value_source.secret_key_ref`
- **WIF**: 最小権限（ref `refs/heads/main` のみ）、`allowed_audiences` 指定
- ステータス: ✅ Pass

### Contract Tests
- Proto 契約は `buf breaking` で main ブランチに対して検証（現在は `continue-on-error: true`、U-APP 実装後に blocking 化予定）
- ステータス: ✅ Pass（early-stage 扱い）

### E2E Tests
- U-APP 実装前のため未実施

## CI Execution（最新）

GitHub Actions PR #10 最終 run:

| Check | Status | 時間 |
|---|:-:|:-:|
| Go (lint / vet / test / vuln) | ✅ pass | ~35s |
| Proto (buf lint / breaking) | ✅ pass | ~8s |
| Docker build × 4 | ✅ pass | 13-23s |
| terraform-validate | ✅ pass | ~11s |

## Overall Status

- **Build**: ✅ Success
- **All Tests**: ⚠ Pass（カバレッジのみ NFR 目標未達、後続 Unit で改善）
- **Ready for Next Unit (U-CSS)**: ✅ Yes

## Next Steps

1. **U-PLT 承認 → U-CSS へ遷移**（本フェーズ完了後）
2. U-CSS の Functional / NFR Req / NFR Design を **Minimal 合本版**（Workflow 圧縮 Option B）で実施
3. U-CSS Infrastructure Design → Code Generation → Build and Test
4. 以降 U-ING / U-BFF / U-NTF / U-APP を同じリズムで進める

## Deferred Items（後続 Unit で対応）

- カバレッジ 80%+ への底上げ（特に `shared/errs` を 90%+ に、`platform/retry` を 85%+ に）
- `platform/observability` / `connectserver` のベンチマーク実装
- `buf breaking` の blocking 化（U-APP 実装後）
- Firestore Security Rules のユニットテスト（Firebase emulator + U-USR / U-BFF で実施）
- 実 GCP への Bootstrap と E2E デモ（U-CSS Build and Test フェーズの受け入れ節目）
