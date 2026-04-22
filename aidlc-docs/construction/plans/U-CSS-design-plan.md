# U-CSS Design Plan (Minimal 合本版)

## Overview

U-CSS（CMS Setup Unit、Sprint 1）は **reearth-cms の Project / Model / Field を冪等に適用する** 一回限りの Cloud Run Job です。Workflow 圧縮方針 Option B に従い、**Functional Design + NFR Requirements + NFR Design を 1 ドキュメント** にまとめます。

## Context（確定済み）

- **Bounded Context**: `cmsmigrate`（DDD application / infrastructure レイヤあり）
- **Deployable**: `cmd/cmsmigrate`（Cloud Run Job、手動実行、冪等）
- **責務**: `SchemaDefinition`（宣言的スキーマ）を CMS に適用し、足りなければ作成、あれば何もしない
- **依存**: reearth-cms Integration REST API（`CreateProject` / `ListModels` / `CreateModel` / `CreateField` など）
- **関連 Story**: 直接 Primary なし（U-CSS は全 Story の前提インフラ）
- **関連 NFR 根拠**: NFR-SEC-01（Integration token は Secret Manager）、NFR-OPS-01（構造化ログ）、NFR-EXT-01（Repository の受け皿）

U-PLT で確定済みの共通規約（slog + OTel / envconfig + Secret Manager / `errs.Wrap` / retry / rate limit / Clock / terraform module 構成 / CI / Dockerfile）は **全てそのまま踏襲** します。U-CSS では **Unit 固有の決定事項だけ** を本ドキュメントで確定させてください。

---

## Step-by-step Checklist

- [ ] Q1〜Q6 すべて回答
- [ ] 矛盾・曖昧さの検証、必要なら clarification
- [ ] 成果物を生成:
  - [ ] `construction/U-CSS/design/U-CSS-design.md` — Functional + NFR Req + NFR Design の合本
- [ ] 承認後、U-CSS Infrastructure Design へ進む

---

## Questions

### Question 1 — SchemaDefinition の対象範囲

`cmsmigrate.SchemaDefinition` で Terraform 的に管理するのはどこまで？

A) **推奨**: **Project + Model + Field のフル宣言**（`SafetyIncident` Model と `keyCd` / `title` / `mainText` / `leaveDate` / `infoType` / `infoName` / `koukanCd` / `koukanName` / `areaCd` / `areaName` / `countryCd` / `countryName` / `extractedLocation` / `geometry` / `geocodeSource` / `infoUrl` / `ingestedAt` / `updatedAt` の全 18 フィールド）。フィールド追加/変更も cmsmigrate Job の再実行で反映される。
B) **Project + Model のみ宣言、Field は手動管理**（運用簡素だが schema drift 検知なし）
C) Field まで宣言するが、変更検知は CREATE のみで UPDATE/DELETE は行わない（drift 上書きしない、safe default）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 2 — 既存リソースとの差分適用

Project / Model / Field が既に存在する場合の挙動:

A) **推奨**: **冪等 CREATE**。各リソースに対し GET / LIST で存在確認 → 無ければ POST、あれば何もしない（no-op）。type の変更など既存定義との差分は **ログに warning 出すだけ**、自動上書きしない（安全側）。
B) 強制上書き（Delete → Create で完全に宣言と一致させる）。drift は強制リセット。データ損失リスクあり。
C) 差分を検出したら **エラーで終了**。人間が手動介入して解決する。
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 3 — 実行モードとオペレーション

`cmd/cmsmigrate` はどう実行しますか？

A) **推奨**: 手動実行のみ（`gcloud run jobs execute cms-migrate --region=asia-northeast1`）。CI/CD からの自動トリガーは行わない。スキーマ変更がある時だけ手動で実行。
B) main ブランチへの `terraform/modules/cmsmigrate/` 変更時に deploy.yml の後続として自動実行
C) 毎回の deploy.yml で必ず実行（冪等なので安全だがコストが少し）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 4 — エラーハンドリング / 部分失敗

Field を 18 個作る途中でエラーが出たら？

A) **推奨**: **即時終了（fail fast）**。既に作成済みのものはそのまま、未作成のものは次回実行で補完（冪等性により自然にリカバリ）。exit code 1 で Cloud Run Job が失敗扱い、ログに失敗した Field 名を残す。
B) ベストエフォート続行（他 Field の作成を試みて最後にまとめて報告）
C) ロールバック（作成済みを Delete で戻す）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 5 — テスト戦略

`cmsmigrate` の test は？

A) **推奨**:
  - `SchemaDefinition` のバリデーション（フィールド名重複・型整合）を unit test で PBT 含めカバー
  - `SchemaApplier` は `cms.Client` インターフェイスに対するモック実装でユニットテスト（ListProjects が空なら CreateProject を呼ぶ、等）
  - **実 CMS への統合テストは Build and Test フェーズで手動実施**（Cloud Run Job を実行してログ確認）
B) A + `httptest` で reearth-cms API を mock した統合テストも追加
C) A のみ（統合テスト省略、実環境で初回実行時に確認）
X) Other（[Answer]: の後ろに自由記述）

[A]: A

### Question 6 — Field 定義の保守

18 フィールドの定義が将来変わることは想定しますか？

A) **推奨**: **Go コードで宣言**（`cmsmigrate/domain/schema_definition.go` にべた書きの `SchemaDefinition{...}`）。変更は PR レビュー + cmsmigrate Job 再実行で反映。Flutter 側 proto と同期が取れるので推奨。
B) 外部 YAML / JSON ファイルから読み込み（`schema.yaml`、`go:embed` で埋め込み）
C) 環境変数で override 可能にする
X) Other（[Answer]: の後ろに自由記述）

[A]: A

---

## 承認前の最終確認（回答確定）

- SchemaDefinition 対象範囲: **Q1 [A]** — Project + Model + Field のフル宣言。`SafetyIncident` Model は proto と整合する **19 フィールド**（`keyCd` / `infoType` / `infoName` / `leaveDate` / `title` / `lead` / `mainText` / `infoUrl` / `koukanCd` / `koukanName` / `areaCd` / `areaName` / `countryCd` / `countryName` / `extractedLocation` / `geometry` / `geocodeSource` / `ingestedAt` / `updatedAt`）。
  - ※ 計画段階では「18 フィールド」と表記していたが、proto `safetymap.proto` を再確認し実体は **19 フィールド**（`lead` を含む）。合本 design では 19 で確定。
- 差分適用方針: **Q2 [A]** — 冪等 CREATE（GET/LIST → 無ければ POST、あれば no-op）、drift は warning log のみ。
- 実行モード: **Q3 [A]** — 手動実行のみ（`gcloud run jobs execute cms-migrate --region=asia-northeast1`）、CI/CD からの自動起動なし。
- 部分失敗時挙動: **Q4 [A]** — fail-fast（最初のエラーで exit code 1、既作成は残して次回実行で補完）。
- テスト戦略: **Q5 [A]** — `SchemaDefinition` の unit + PBT、`SchemaApplier` は `cms.Client` mock でユニット、実 CMS 統合テストは Build and Test で手動。
- Field 定義の保守方法: **Q6 [A]** — Go コードでべた書き宣言（`internal/cmsmigrate/domain/schema_definition.go`）、変更は PR + Job 再実行。

回答確定済み。`U-CSS-design.md`（合本版）を生成する。
