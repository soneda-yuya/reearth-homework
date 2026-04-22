# Build Instructions — U-PLT

## Prerequisites
- Go 1.26.x（`go version` で確認、CI は `actions/setup-go@v5` + `check-latest: true` で自動追従）
- [buf](https://buf.build/docs/installation) CLI（proto コード生成・lint・breaking 検出）
- Docker（マルチステージビルドで各 Deployable のイメージを作る）
- Terraform 1.9+（インフラ管理、ローカル apply 時）
- gcloud CLI（デプロイ時の ADC および Secret 投入時）

### 環境変数
`.env` を作るのは **ローカル実行時のみ**。本番は Cloud Run の `env.value_source.secret_key_ref` で注入される。`.env.example` からコピーして必要値を記入:

```bash
cp .env.example .env
# 編集して PLATFORM_SERVICE_NAME / PLATFORM_ENV / GCP_PROJECT_ID などを埋める
```

## ローカルビルド

### Go コード
```bash
make setup       # go version 確認 + go mod download
make test        # go test ./... (race 検出付きは make test-race)
make vet         # go vet ./...
make vuln        # govulncheck ./...
make build       # 4 Deployable を bin/ に出力
```

### 個別 Deployable
```bash
make build-bff
make build-ingestion
make build-notifier
make build-setup
```

### Proto コード生成
```bash
make proto-lint       # buf lint
make proto            # buf generate -> gen/go/v1/
make proto-breaking   # main ブランチとの breaking 検出
```

### Docker イメージ
```bash
docker build --build-arg DEPLOYABLE=bff -t bff:dev .
```

### Terraform
```bash
cd terraform/environments/prod
terraform init
terraform validate
terraform plan -var="project_number=..." -var="cms_base_url=..." -var="cms_workspace_id=..."
```

## CI ビルド（GitHub Actions）

`.github/workflows/ci.yml` が以下を実行（PR および main push で自動）:

1. **Go ジョブ** — `gofmt -s` 差分 / `go vet` / `golangci-lint` (go install v1.64.8、Go 1.26 ツールチェーンでビルド) / `go test -race -coverprofile=coverage.out ./...` / `govulncheck ./...`
2. **Proto ジョブ** — `buf lint` / `buf breaking`（後者は `continue-on-error: true`、早期 rename を許容）
3. **Docker ジョブ** — 4 Deployable を matrix でビルド（push なし、検証のみ、GHA cache で 2 回目以降高速化）

main push のみで `.github/workflows/deploy.yml` が走り:
4. 4 Deployable を Artifact Registry に push
5. `terraform apply -var='*_image_tag=<git-sha>'` で Cloud Run に展開

PR `terraform/` 配下変更時は `terraform-validate.yml` が `fmt -check -recursive` + `init -backend=false` + `validate` を実行。

## ビルド成果物

| 成果物 | 出力先 |
|---|---|
| Go バイナリ | `bin/{deployable}`（ローカル）、Artifact Registry `asia-northeast1-docker.pkg.dev/overseas-safety-map/app/{deployable}:{tag}`（CI） |
| 生成 proto コード | `gen/go/v1/*.pb.go`（CI で `buf generate`、未 commit でも動作） |
| Docker イメージ | `{deployable}:{tag}`（`gcr.io/distroless/static-debian12:nonroot` ベース） |

## トラブルシューティング

### `go build` が go.mod の `go 1.26` で失敗
ローカル Go が 1.25 以下の場合。`brew upgrade go` またはゴルフポイントインストール。

### `buf generate` が失敗
`buf.yaml` で `lint.except: [PACKAGE_DIRECTORY_MATCH]` を指定しているか確認。`proto/v1/*.proto` の `go_package` オプションが `github.com/soneda-yuya/reearth-homework/gen/go/v1;overseasmapv1` と整合しているか。

### `terraform validate` が変数未定義で失敗
`terraform init -backend=false` で初回のみ `-backend=false` を付ける（tfstate bucket 未作成の場合）。変数は `-var` もしくは `terraform.tfvars` で渡す。

### Docker build で DEPLOYABLE エラー
`docker build --build-arg DEPLOYABLE=...` を忘れている。`test -n "${DEPLOYABLE}"` でガードしている。
