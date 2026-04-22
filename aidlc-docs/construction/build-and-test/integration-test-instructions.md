# Integration Test Instructions — U-PLT

## スコープ

U-PLT は基盤 Unit であり、**他 Unit と統合するものが現時点で存在しない**（U-CSS / U-ING / U-BFF / U-NTF / U-APP は未実装）。このため U-PLT 単体での統合テストは **疎通テスト** レベルに限定します。本格的な統合は各後続 Unit で以下のシナリオをカバー:

| Unit | 統合シナリオ |
|---|---|
| U-CSS | `cmd/cmsmigrate` → reearth-cms Integration API で Project/Model/Field を作成 |
| U-ING | MOFA XML 取得 → Claude → Mapbox → CMS 保存 → Pub/Sub publish |
| U-BFF | Flutter → Connect RPC → Firebase Auth 検証 → CMS / Firestore 読取 |
| U-NTF | Pub/Sub push → Firestore 読取 → FCM 配信 |
| U-APP | Flutter 実機 → BFF → 全 13 MVP ストーリー動作 |

## U-PLT 時点で実施可能な最小疎通確認

### 1. 各 cmd/* の起動ログ確認（ローカル）

空の main でも `config.Load` と `observability.Setup` が通ることを確認:

```bash
# BFF (HTTP サーバ起動)
make build-bff
PLATFORM_SERVICE_NAME=bff \
PLATFORM_ENV=dev \
PLATFORM_GCP_PROJECT_ID=overseas-safety-map \
BFF_PORT=8080 \
./bin/bff &

curl -s http://localhost:8080/healthz     # "ok"
curl -s http://localhost:8080/readyz      # {"status":"ready","probers":[]}
kill %1
```

```bash
# Ingestion / Setup / Notifier は同様に、それぞれの env を設定して起動確認
```

### 2. Proto コード生成の往復テスト

```bash
make proto-lint           # buf lint
make proto                # buf generate
git diff gen/             # 生成ファイルが期待形状か目視
make test                 # 生成コードが import されてもビルドが通る
```

### 3. Docker イメージの起動確認

```bash
docker build --build-arg DEPLOYABLE=bff -t bff:dev .
docker run --rm -p 8080:8080 \
  -e PLATFORM_SERVICE_NAME=bff \
  -e PLATFORM_ENV=dev \
  -e PLATFORM_GCP_PROJECT_ID=overseas-safety-map \
  bff:dev &

curl -s http://localhost:8080/healthz
docker rm -f $(docker ps -q --filter ancestor=bff:dev)
```

### 4. Terraform の plan チェック（ローカル、ADC 認証）

GCP プロジェクトがすでに bootstrap 済みの場合のみ:

```bash
cd terraform/environments/prod
terraform init
terraform plan \
  -var="project_number=$(gcloud projects describe overseas-safety-map --format='value(projectNumber)')" \
  -var="cms_base_url=https://<cms-url>" \
  -var="cms_workspace_id=<workspace-id>"
```

GCP 未 bootstrap の場合は `terraform validate` のみで U-PLT としては十分（CI の `validate` ジョブが自動実行）。

## 期待結果

- `/healthz` が 200 を返す
- `/readyz` が 200 を返す（Prober 未登録なので空配列）
- `buf generate` の diff が proto 定義の変更に対応している
- Docker コンテナが distroless 環境で正常に起動（`USER nonroot:nonroot` でも権限不足にならない）
- `terraform validate` が `Success!`（ローカル / CI 両方）

## 将来の統合テスト（U-CSS 以降で追加）

- `testcontainers` ライクな e2e（docker-compose でローカル CMS + Pub/Sub emulator + Firebase emulator を起動）
- U-BFF 実装後に `buf curl` で全 RPC の smoke テスト
- U-APP 実装後に iOS/Android 実機で flutter_test + integration_test

## CI との整合
CI では `Docker build` ジョブが 4 Deployable × matrix で `docker build`（push なし）を成功させており、distroless ベースイメージでのビルド互換性は常時検証されています。
