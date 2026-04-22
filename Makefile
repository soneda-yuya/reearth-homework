.PHONY: help setup deps test test-race cover vet lint vuln build build-all proto proto-lint proto-breaking clean

DEPLOYABLES := ingestion bff notifier cmsmigrate

help: ## このヘルプを表示
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

setup: ## 開発環境セットアップ
	@go version
	@go mod download
	@command -v buf >/dev/null 2>&1 || { echo "Install buf: https://buf.build/docs/installation"; exit 1; }
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Install golangci-lint: https://golangci-lint.run/usage/install/"; exit 1; }

deps: ## 依存を同期
	go mod tidy

test: ## go test を実行
	go test ./...

test-race: ## race 検出付きで実行
	go test -race ./...

cover: ## カバレッジ計測
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vet: ## go vet
	go vet ./...

lint: ## golangci-lint
	golangci-lint run

vuln: ## govulncheck
	go run golang.org/x/vuln/cmd/govulncheck ./...

build: ## 全 Deployable をビルド
	@for d in $(DEPLOYABLES); do \
		echo ">> building $$d"; \
		CGO_ENABLED=0 go build -o bin/$$d ./cmd/$$d; \
	done

build-%: ## 特定 Deployable のみビルド (例: make build-bff)
	CGO_ENABLED=0 go build -o bin/$* ./cmd/$*

proto: ## proto から Go コードを生成
	buf generate

proto-lint: ## proto lint
	buf lint

proto-breaking: ## proto 破壊的変更検出 (main との比較)
	buf breaking --against '.git#branch=main'

clean: ## ビルド成果物を削除
	rm -rf bin/ dist/ coverage.out coverage.html
