# U-CSS Build and Test — Runbook

**Status**: 🟡 **Template only** — 実 CMS への疎通確認は未実施。reearth-cms インスタンスと Integration Token が用意でき次第、本ランブックに沿って実行し、結果を §6 に記録する。

**前提**: U-CSS Code Generation（PR #25 merged 2026-04-23）が main に取り込まれていること。

---

## 1. 目的

`cmd/cmsmigrate` が **実 reearth-cms に対して** 以下を満たすことを確認する:

1. Project / Model / 19 Field が想定通り Create される（初回実行）
2. 2 回目以降の実行が完全 no-op になる（冪等性、NFR-CSS-REL-01）
3. Cloud Logging / OTel に期待した属性付きで構造化ログが出る
4. **設計時に仮定した REST エンドポイント形状が正しい**ことの答え合わせ（U-CSS Design Q A [A]、未確認の領域）

未一致が出た場合は `internal/platform/cmsx/dto.go` / `schema.go` を修正し、本ランブックに「ズレた点と修正内容」を記録する。

---

## 2. 事前準備（実行者が用意するもの）

### 2.1 reearth-cms インスタンス

| 項目 | 例 | 取得方法 |
|---|---|---|
| Base URL | `https://cms.example.com` | CMS 管理者 / SaaS hosted URL |
| Workspace ID | `wkp_XXXXXXXX` | CMS 管理画面 → Workspaces |
| Integration Token | `it_XXXXXXXX` | CMS 管理画面 → Integration → 新規発行 |

> ⚠️ **本番 Workspace で実行しないこと**。U-CSS は冪等 CREATE のみで破壊操作は無いが、`overseas-safety-map` という名前の Project を新規作成するため、衝突や他用途との混在を避けるため **専用の test workspace** を用意することを強く推奨する。
>
> Token に必要な権限: `Project Create` / `Model Create` / `Field Create`（読み取り権限も必須、Find 系のため）。

### 2.2 ローカル環境

- Go 1.26+
- gcloud CLI（OTel exporter を `gcp` にする場合のみ。`stdout` なら不要）
- リポジトリの最新 main をチェックアウト

```bash
git checkout main && git pull --ff-only
make build-cmsmigrate     # bin/cmsmigrate を生成
```

### 2.3 環境変数

`.env`（リポジトリの `.gitignore` 済み）を作成:

```bash
# Platform 共通
PLATFORM_SERVICE_NAME=cmsmigrate
PLATFORM_ENV=dev
PLATFORM_GCP_PROJECT_ID=overseas-safety-map
PLATFORM_LOG_LEVEL=DEBUG       # 検証時は DEBUG で attr を全部見たい
PLATFORM_OTEL_EXPORTER=stdout  # 実 GCP へ送らない

# cmsmigrate 固有
CMSMIGRATE_CMS_BASE_URL=https://cms.example.com
CMSMIGRATE_CMS_WORKSPACE_ID=wkp_XXXXXXXX
CMSMIGRATE_CMS_INTEGRATION_TOKEN=it_XXXXXXXX
```

---

## 3. 実行手順

### 3.1 初回実行（全 Create）

```bash
set -a; source .env; set +a
./bin/cmsmigrate 2>&1 | tee /tmp/cmsmigrate.run1.log
```

**期待されるログ（抜粋）**:

```json
{"time":"...","level":"INFO","msg":"cmsmigrate starting","app.cmsmigrate.phase":"start","cms.base_url":"https://cms.example.com",...}
{"time":"...","level":"INFO","msg":"project created","app.cmsmigrate.phase":"create-project","project.alias":"overseas-safety-map"}
{"time":"...","level":"INFO","msg":"model created","app.cmsmigrate.phase":"create-model","model.alias":"safety-incident"}
{"time":"...","level":"INFO","msg":"cmsmigrate finished","app.cmsmigrate.phase":"done","project_created":true,"models_created":["safety-incident"],"fields_created":["safety-incident.key_cd","safety-incident.info_type",..."safety-incident.updated_at"],"drift_warnings":0}
```

**確認項目**:

- [ ] exit code = 0
- [ ] `project_created` = true
- [ ] `models_created` = `["safety-incident"]`
- [ ] `fields_created` の長さ = **19**
- [ ] 19 Field の alias が proto と一致（key_cd / info_type / info_name / leave_date / title / lead / main_text / info_url / koukan_cd / koukan_name / area_cd / area_name / country_cd / country_name / extracted_location / geometry / geocode_source / ingested_at / updated_at）
- [ ] `drift_warnings` = 0
- [ ] reearth-cms 管理画面で Project / Model / 19 Field が UI に表示される

### 3.2 2 回目実行（no-op、冪等性確認）

```bash
./bin/cmsmigrate 2>&1 | tee /tmp/cmsmigrate.run2.log
```

**期待**:

```json
{"time":"...","level":"INFO","msg":"project exists","app.cmsmigrate.phase":"find-project",...}
{"time":"...","level":"INFO","msg":"model exists","app.cmsmigrate.phase":"find-model",...}
{"time":"...","level":"INFO","msg":"cmsmigrate finished","project_created":false,"models_created":[],"fields_created":[],"drift_warnings":0}
```

**確認項目**:

- [ ] exit code = 0
- [ ] `project_created` = false
- [ ] `models_created` = `[]`
- [ ] `fields_created` = `[]`
- [ ] `drift_warnings` = 0
- [ ] 実 CMS 側で重複 Project / Model / Field が **作られていない**（管理画面で確認）

### 3.3 Drift 検出の確認（任意）

実 CMS 側で 1 つの Field の `required` フラグを手動で変更し、3 回目を実行:

```bash
./bin/cmsmigrate 2>&1 | tee /tmp/cmsmigrate.run3.log
```

**期待**: `drift_warnings` > 0、`level=WARN` の `schema drift detected (no auto-apply)` ログが出る。**自動修正されない**ことが重要（Q2 [A]）。

---

## 4. トラブルシューティング

### 4.1 `KindUnauthorized`（HTTP 401 / 403）

**症状**: `cmsx.auth: HTTP 401: ...` が ERROR ログに出て exit 1。

**原因と対処**:
- Token が無効 / 期限切れ → CMS 管理画面で再発行
- Token に必要な権限が不足 → Project / Model / Field の Create 権限を付与
- `Authorization: Bearer ` ヘッダ形式の不一致 → CMS 側ドキュメントを確認、必要なら `cmsx/schema.go` の `req.Header.Set("Authorization", ...)` を修正

### 4.2 `KindNotFound`（HTTP 404）

**症状**: `cmsx.not_found: HTTP 404: ...` が出て exit 1。

**原因と対処**:
- `CMSMIGRATE_CMS_WORKSPACE_ID` が間違っている → CMS 管理画面で確認
- API パスの仮定がズレている → 設計では `/api/workspaces/{ws}/projects` 等を仮定。実 API と異なる場合は `cmsx/schema.go` の `c.url(...)` 引数を修正
- Project は作成されているのに Model 作成で 404 → `/api/projects/{projectID}/models` のパス仮定がズレ。実 API を確認

### 4.3 JSON 形状不一致（`json: cannot unmarshal ...`）

**症状**: `cmsx.decode` エラーで exit 1。

**原因と対処**:
- レスポンスの top-level キーが想定と違う（`items` ではなく `projects` など） → `cmsx/dto.go` の構造体や `cmsx/schema.go` の `var out struct{ Items []ProjectDTO ...}` を修正
- Field の wire 名が違う（`text` ではなく `textShort` など） → `cmsx/dto.go` の `fieldTypeToAPI` / `fieldTypeFromAPI` の文字列を修正

### 4.4 リクエスト Body がはじかれる（HTTP 400）

**症状**: 200/201 を返さず 400 で `cmsx.unexpected: HTTP 400: ...`。

**原因と対処**:
- POST body のキー名が違う（`alias` ではなく `key` など） → `cmsx/dto.go` の `createProjectBody` / `createModelBody` / `createFieldBody` の json tag を修正
- 必須フィールドが欠けている → エラー本文を読み、必要な追加フィールドを `domain.SafetyMapSchema()` または body 構造体に追加

### 4.5 Field 作成途中で失敗

**症状**: 5 個目の Field で `KindExternal` などで exit 1。

**対処**: ログで `field.alias` を特定 → 原因対処 → **同じコマンドで再実行**。冪等性により既に成功した 4 個は再作成されず、失敗した Field から再開する。

---

## 5. Production 反映手順

ローカルでの確認が成功したら、本番環境（GCP `overseas-safety-map` プロジェクト）への反映:

### 5.1 Token を Secret Manager に投入

```bash
echo -n "<本番用 Token>" | gcloud secrets versions add cms-integration-token \
  --data-file=- --project=overseas-safety-map
```

### 5.2 Terraform / image を反映

通常の `deploy.yml` フロー（main merge → CI が自動で `terraform apply` + image push）。U-CSS の差分は既に main に取り込まれているので追加作業なし。

### 5.3 Job 実行

```bash
gcloud run jobs execute cms-migrate \
  --region=asia-northeast1 \
  --project=overseas-safety-map \
  --wait
```

`--wait` で完了まで block、stdout に終了ステータス。

### 5.4 確認

Cloud Logging（`resource.labels.job_name=cms-migrate`）で:
- `app.cmsmigrate.phase=done`
- `project_created`, `models_created`, `fields_created` 属性
- `drift_warnings=0`

---

## 6. 実行記録

> 実 CMS で実行する都度、ここに追記する。少なくとも初回の Production 反映は記録すること。

### 6.1 [日付未定] ローカル初回実行（test workspace）

**実行者**: TBD
**Workspace**: TBD
**結果**: TBD
**API 仕様の差分（あれば）**: TBD

### 6.2 [日付未定] Production 初回実行

**実行者**: TBD
**結果**: TBD

---

## 7. 関連ドキュメント

- [`U-CSS/design/U-CSS-design.md`](../design/U-CSS-design.md) — Functional + NFR Req + NFR Design 合本
- [`U-CSS/infrastructure-design/`](../infrastructure-design/) — Cloud Run Job / IAM / Secret 設計
- [`U-CSS/code/summary.md`](../code/summary.md) — Code Generation 成果物一覧
- [`construction/build-and-test/`](../../build-and-test/) — U-PLT 全体ビルド & テスト
