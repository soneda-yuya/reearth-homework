# U-PLT Code Generation Summary

AI-DLC Code Generation フェーズ Part 2（Generation）の生成成果物の一覧と確認手順。

## PR 分割

| PR | Phases | 主成果物 | 状態 |
|---|---|---|---|
| PR #9 | Phase 1–7（Go コード + proto + Dockerfile） | `internal/*` / `cmd/*` / `proto/*` / `buf.yaml` / `go.mod` | merged (`6f82bc8`) |
| 本 PR | Phase 8–10（Terraform + CI/CD + README） | `terraform/*.tf` / `.github/workflows/*.yml` / `.github/dependabot.yml` / `README.md` | — |

## ファイル一覧（本 PR: Phase 8–10）

### Phase 8: Terraform（16 ファイル）

| ファイル | 内容 |
|---|---|
| `terraform/versions.tf` | Terraform / provider バージョン固定、GCS backend |
| `terraform/main.tf` | google / google-beta provider |
| `terraform/variables.tf` | project_id / region / image_tag / CMS 設定 |
| `terraform/outputs.tf` | 重要 URL / WIF provider / CI SA email |
| `terraform/apis.tf` | 16 API 有効化 |
| `terraform/artifact_registry.tf` | Docker レジストリ `app` |
| `terraform/secret_manager.tf` | 3 Secret + 5 IAM binding |
| `terraform/pubsub.tf` | Topic + Subscription + DLQ + invoker IAM |
| `terraform/service_accounts.tf` | 5 SA |
| `terraform/wif.tf` | Workload Identity Federation |
| `terraform/iam.tf` | project-scoped IAM bindings |
| `terraform/cloud_run_bff.tf` | Service (public) + env + secret + probes |
| `terraform/cloud_run_ingestion.tf` | Job + env + secrets |
| `terraform/cloud_run_notifier.tf` | Service (internal) + env + probes |
| `terraform/cloud_run_setup.tf` | Job + env + secrets |
| `terraform/cloud_scheduler.tf` | 5 分毎の ingestion 起動 |
| `terraform/firestore.tf` | Native mode database |
| `terraform/README.md` | 使い方 / Bootstrap 手順 |

### Phase 9: CI/CD（5 ファイル）

| ファイル | 内容 |
|---|---|
| `.github/actions/setup-go/action.yml` | 再利用可能な composite action（Go + buf + govulncheck） |
| `.github/workflows/ci.yml` | PR/push で lint / vet / test / buf / docker build |
| `.github/workflows/deploy.yml` | main push で docker build + push + terraform apply |
| `.github/workflows/terraform-plan.yml` | terraform/ の PR で plan を PR コメント化 |
| `.github/workflows/setup-go.yml` | composite action のスモークテスト（手動実行） |
| `.github/dependabot.yml` | gomod daily / github-actions weekly / docker weekly |

### Phase 10: Documentation

| ファイル | 内容 |
|---|---|
| `README.md` | プロジェクト概要 / Getting Started / Architecture / Deployment |
| 本ファイル | Code Generation サマリー |

## 確認手順

### ローカル確認（PR A で検証済み）

```bash
make setup
make test            # 全 pass
make lint            # clean
make vuln            # clean
```

### CI 確認（本 PR）

- `ci.yml`: PR 作成で自動起動
- `terraform-plan.yml`: terraform/ 配下の変更時に PR へ plan コメント
- main merge 後に `deploy.yml` が docker push + terraform apply

### GitHub Secrets に登録が必要な値（本 PR 後に設定）

- `GCP_WIF_PROVIDER` — `projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/overseas-safety-map-pool/providers/github-provider`
- `GCP_PROJECT_NUMBER` — `gcloud projects describe overseas-safety-map --format='value(projectNumber)'`
- `CMS_BASE_URL` — reearth-cms インスタンスの URL
- `CMS_WORKSPACE_ID` — 手動作成した CMS ワークスペース ID

### GCP 側のマニュアル操作

Terraform 管理外のブートストラップ:

1. GCP プロジェクト作成 + billing 有効化
2. `gsutil mb -l asia-northeast1 gs://overseas-safety-map-tfstate`
3. Bucket versioning 有効化
4. Terraform 初回 apply 後、Secret の実値を手動投入
5. Firebase コンソールで Auth / FCM を有効化

## NFR カバレッジ

| NFR | 実装場所 |
|---|---|
| NFR-PLT-SEC-01（Secrets） | `terraform/secret_manager.tf` + Cloud Run `env.value_source.secret_key_ref` |
| NFR-PLT-SEC-02（脆弱性スキャン） | `.github/workflows/ci.yml` govulncheck + `.github/dependabot.yml` |
| NFR-PLT-SEC-03（Secrets 非コミット） | `.gitignore` で .env 除外 + `.env.example` のみコミット |
| NFR-PLT-OBS-01〜04 | `internal/platform/observability/*` + Cloud Run env `PLATFORM_OTEL_EXPORTER=gcp` |
| NFR-PLT-REL-01〜03 | `internal/platform/config` の fail-fast + `observability.RecoverInterceptor` / `WrapJobRun` |
| NFR-PLT-TEST-01〜04 | `internal/**/*_test.go`（rapid PBT を 3 領域に適用） |
| NFR-PLT-MNT-02（Proto 互換性） | `ci.yml` の `buf breaking` ステップ |

## 次の Unit

U-PLT の Build & Test フェーズ（`aidlc-docs/construction/U-PLT/build-and-test/`）の後、U-CSS（CMS Setup）から後続 Unit を順次実装します。各 Unit は **Functional / NFR Req / NFR Design を合本化した Minimal 版** で進める方針（ワークフロー圧縮 Option B）。
