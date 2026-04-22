# U-PLT NFR Requirements Plan (Minimal)

## Overview

U-PLT の非機能要件を確定します。U-PLT は基盤 Unit のため、スケーラビリティ／可用性は対象外（ライブラリ化された SDK は自律スケールしない）。代わりに **ビルド時間・カバレッジ・PBT 対象・依存管理・観測バックエンド** を軸に 5 項目で絞ります。

既存要件書の NFR を前提とし（Security Baseline / PBT 拡張は有効）、U-PLT に固有の値だけ確定します。

---

## Step-by-step Checklist

- [ ] Q1〜Q5 すべて回答
- [ ] 矛盾・曖昧さの検証
- [ ] 成果物 2 点を生成:
  - [ ] `construction/U-PLT/nfr-requirements/nfr-requirements.md`
  - [ ] `construction/U-PLT/nfr-requirements/tech-stack-decisions.md`
- [ ] 承認後、U-PLT NFR Design へ進む

---

## Context Summary

- **所属 Unit**: U-PLT（Sprint 0、Platform & Proto 基盤）
- **関連要件**: NFR-SEC-01（Secrets）/ NFR-SEC-05（依存脆弱性スキャン）/ NFR-OPS-01〜02（構造化ログ・メトリクス）/ NFR-TEST-01〜04（テスト戦略・PBT）
- **Application Design 確定済み技術**: Go / Connect / slog / OpenTelemetry / envconfig / buf / proto v3 / Firebase Admin / Mapbox / Anthropic Claude

---

## Questions

### Question 1 — Go / buf / 主要依存のバージョン方針

U-PLT の `go.mod` / ツール類のバージョン管理方針は？

A) **推奨**: Go は **最新安定版の直前メジャー**（現在 1.24 系を使うイメージ）、依存は **常に最新 patch** を Dependabot で追従、major は手動で上げる。buf CLI と各 SDK（firebase-admin-go v4、connect-go v1、opentelemetry-go 最新、anthropic-sdk-go 最新）も同方針。
B) 厳密に固定（minor / patch も PR で明示的に上げる）
C) まだ決めない（Infrastructure Design で確定）
X) Other（[Answer]: の後ろに自由記述）

[A,go1.26があります]: 

### Question 2 — PBT 適用範囲（U-PLT 内）

U-PLT 内で PBT（property-based testing）を適用する対象はどれですか？（複数選択可）

A) **errs.Wrap / IsKind / KindOf のラウンドトリップ性質**（`KindOf(Wrap(op, k, err)) == k` など）
B) **proto ↔ domain コンバータ**（Timestamp / Point / Filter 等を生成 → 変換 → 逆変換したときの同値性）
C) **Config 読み込み**（envconfig の `string → 型` のラウンドトリップ、必須欠落の panic）
D) **validate の境界値**（`limit` / `radius_km` / `Point.Lat/Lng` / 期間順序 の境界付近をランダム生成）
E) U-PLT では PBT を適用しない（他 Unit にて対応）
X) Other（[Answer]: の後ろに自由記述）

**推奨**: A, B, D（C は panic を伴うため通常ユニットで十分）

[A,B,D]: 

### Question 3 — テストカバレッジと CI 時間の目標

U-PLT のテストカバレッジと CI 実行時間の目標値は？

A) **推奨**: カバレッジ **80% 以上**（`shared/errs` / `shared/validate` / `platform/config` は 90%+）。CI 時間は **5 分以内**（`go test ./... + buf lint + buf breaking + govulncheck`）。
B) カバレッジ 70% 以上、CI 時間は特に制約なし
C) カバレッジ強制なし（緑で通れば OK）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 4 — 依存脆弱性スキャンと Secrets 取り扱い

NFR-SEC-05（脆弱性スキャン）と NFR-SEC-01（Secrets）の具体化方針は？

A) **推奨**: `govulncheck` を CI で必須化（Critical/High を PR 落とし条件）＋ **Dependabot** で daily 更新 PR。Secrets は **GitHub Secrets** に格納、本番は **GCP Secret Manager** から起動時に取得。`.env.example` のみコミット、`.env` は `.gitignore`。
B) `govulncheck` のみ、Dependabot は使わない
C) `trivy` / `snyk` / `grype` 等を使う（Infrastructure Design で詳細決定）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 5 — 観測性バックエンド

OpenTelemetry の送信先（exporter）はどこを想定しますか？

A) **推奨**: ローカル / CI = **stdout exporter**、本番 / dev = **GCP Cloud Trace + Cloud Monitoring + Cloud Logging**。env で切替（`PLATFORM_OTEL_EXPORTER=stdout|gcp`）
B) ローカルから本番まで stdout に統一（後で Fluentbit/Promtail 等で収集）
C) Honeycomb / Datadog / New Relic 等の SaaS
D) 観測性は MVP ではログのみ、OTel は未導入（Functional Design の決定を一部撤回）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

---

## 承認前の最終確認（回答後に AI が埋めます）

- 依存バージョン方針: _TBD_
- PBT 対象: _TBD_
- カバレッジ / CI 時間目標: _TBD_
- 脆弱性スキャン / Secrets: _TBD_
- OTel exporter: _TBD_

回答完了後、矛盾・曖昧さがなければ `nfr-requirements.md` と `tech-stack-decisions.md` の 2 成果物を生成します。
