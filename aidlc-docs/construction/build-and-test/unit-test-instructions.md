# Unit Test Execution — U-PLT

## 実行

### 全テスト
```bash
go test ./...             # 基本
go test -race ./...       # race 検出付き (CI と同じ)
make cover                # coverage プロファイル取得 + 末尾に合計 %
```

### パッケージ単位
```bash
go test ./internal/shared/errs
go test ./internal/platform/config
```

### PBT（プロパティベーステスト）のみ
pgregory.net/rapid を使った PBT は `TestProp_*` という命名規則で揃えています:
```bash
go test -run 'TestProp_' ./...
go test -run 'TestProp_' -rapid.steps=1000 ./...   # 実行回数を増やす (デフォルト 100)
```

### ベンチマーク
（NFR-PLT-PERF-03/04 で定義、U-PLT 範囲では未実装、後続 Unit で追加予定）
```bash
go test -bench=. -benchmem ./internal/platform/observability
```

## 期待結果

| パッケージ | カバレッジ目標 | 現状 |
|---|---:|---:|
| `shared/errs` | 90%+ | 85.2%（NFR target へ追い込み中） |
| `shared/validate` | 90%+ | 100% ✓ |
| `platform/config` | 90%+ | 33%（`os.Exit` パスのため構造的に上がらない、envconfig 直接呼び出しで代替検証） |
| `shared/clock` | — | 100% |
| `platform/ratelimit` | — | 100% |
| `platform/retry` | — | 78% |
| `platform/observability` | — | 61% |
| `platform/connectserver` | — | 49% |
| **合計** | 80%+ | **67.4%** |

MVP の U-PLT 時点では合計 80% に未達。以下の理由で許容しつつ、後続 Unit で底上げ:
- `platform/config` の `os.Exit` パスは構造的にテスト不可（別プロセスでの統合的検証が必要、コスト高）
- `platform/observability` / `connectserver` の OTel 初期化・HTTP サーバ起動は Build and Test スコープで統合テストとして扱う
- `platform/retry` のバックオフ時間依存分岐は PBT 化の余地あり

## レビューポイント（失敗時）

1. `go test ./...` の失敗テスト名を確認
2. 該当パッケージの `_test.go` を読んで期待値を理解
3. 実装修正 → 再度 `go test ./...` が緑になるまで繰り返す
4. coverage 退行がないか `make cover` で確認

## CI との整合
CI ジョブ `Go (lint / vet / test / vuln)` が以下を PR / main push で実行し、どれかが失敗すればマージブロック:
- `gofmt -s -d . || exit 1`
- `go vet ./...`
- `golangci-lint run ./...`（v1.64.8 を Go 1.26 ツールチェーンでビルドして使用）
- `go test -race -coverprofile=coverage.out ./...`
- `govulncheck ./...`
- `go tool cover -func=coverage.out | tail -1` で合計カバレッジを summary 出力
