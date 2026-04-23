# AI-DLC 状態トラッキング

## プロジェクト情報
- **プロジェクト名**: overseas-safety-map（海外安全情報マップ）
- **プロジェクトタイプ**: グリーンフィールド
- **開始日**: 2026-04-20T17:55:00Z（アイデア変更のリセット）
- **現在のフェーズ**: CONSTRUCTION
- **現在のステージ**: U-BFF（Connect Server Unit、Sprint 3）／ Code Generation PR B (Phase 6-9) 実装完了、PR レビュー待ち

## ワークフロー圧縮方針（2026-04-22 採用）
**U-CSS 以降の Unit は Functional Design / NFR Requirements / NFR Design を「Minimal 合本版」1 ドキュメントにまとめる**。U-PLT で共通規約を確定したため、各 Unit 固有の内容のみを簡潔に記述する。Infrastructure Design / Code Generation / Build & Test は従来どおり独立して実施する。

## アイデア概要
- **データソース**: 外務省 海外安全情報オープンデータ（XML、5分毎更新、無償・商用可）
- **コア機能**: オープンデータのタイトル／本文から事故・安全事象の発生場所を抽出し、緯度経度にジオコーディングしたうえで reearth-cms に蓄積
- **クライアント**: Flutter アプリ（地図で事象を閲覧）
- **バックエンド**: reearth-cms Integration REST API（データストア、前アイデアから継続）

## ワークスペースの状態
- **既存コード**: なし
- **プログラミング言語**: Dart（Flutter 前提）／ 取り込みパイプラインの言語は要件分析で決定
- **ビルドシステム**: Flutter (pub) / パイプラインは未定
- **プロジェクト構造**: 空（AI-DLC ルールファイルのみ）
- **リバースエンジニアリングの要否**: 不要
- **ワークスペースルート**: /Users/y.soneda/projects/yuya/reearth-homework

## コード配置ルール
- **アプリケーションコード**: ワークスペースルート（aidlc-docs/ には絶対に置かない）
- **ドキュメント**: aidlc-docs/ のみ
- **構造パターン**: code-generation.md の Critical Rules を参照

## 拡張機能の有効化
| 拡張機能 | 有効化 | 決定した段階 |
|---|---|---|
| セキュリティベースライン | Yes | 要件分析 |
| プロパティベーステスト | Yes | 要件分析 |

## ステージ進捗

### 🔵 INCEPTION フェーズ
- [x] ワークスペース検出
- [ ] リバースエンジニアリング（該当なし — グリーンフィールド）
- [x] 要件分析（承認済み・PR #1 merged 2026-04-20）
- [x] ユーザーストーリー（承認済み・PR #2 merged 2026-04-20、US-01〜US-13 MVP）
- [x] ワークフロー計画（承認済み 2026-04-20）
- [x] アプリケーション設計（承認済み・PR #3 merged 2026-04-22、DDD 再構成含む）
- [x] ユニット計画（承認済み — 6 Unit、Deployable 軸、Cloud Run 統一、依存順実装）
- [x] ユニット生成（承認済み・PR #4 merged 2026-04-22）

### 🟢 CONSTRUCTION フェーズ（Unit 単位でループ）
現在: U-PLT（Sprint 0）に着手中。以降の Unit は依存順に順次進行。

#### U-PLT（進行中）
- [x] 機能設計（Functional Design — Minimal 版、承認済み・PR #5 merged）
- [x] NFR 要件（NFR Requirements — Go 1.26 採用、承認済み・PR #6 merged）
- [x] NFR 設計（NFR Design — 承認済み・PR #7 merged）
- [x] インフラ設計（Infrastructure Design — 単一 prod プロジェクト + Terraform で Cloud Run 含む全リソース管理、承認済み・PR #8 merged）
- [x] コード生成（Code Generation — PR A #9 merged、PR B #10 merged 2026-04-22）
- [x] ビルドとテスト（Build and Test — 承認済み・PR #19 merged 2026-04-22）

#### U-CSS（進行中、Sprint 1）
- [x] Minimal 合本版（Functional + NFR Req + NFR Design）— PR #21 merged 2026-04-22
- [x] インフラ設計 計画（PR #22 merged、Q1-Q6 全 A）
- [x] インフラ設計（Infrastructure Design）— PR #23 merged 2026-04-22
- [x] コード生成 計画（Q A-F 全 A、PR #24 merged 2026-04-22）
- [x] コード生成（Code Generation Phase 1-7、Copilot 5 round 対応含む）— PR #25 merged 2026-04-23
- [x] ビルドとテスト（Build and Test runbook template）— PR #27 merged 2026-04-23、実 CMS 疎通は運用フェーズで追記

#### U-ING（進行中、Sprint 2）
- [x] Minimal 合本版 計画（PR #28 merged 2026-04-23、Q1-Q9 全 A）
- [x] Minimal 合本版 design 本編（PR #29 merged 2026-04-23）
- [x] インフラ設計 計画（PR #30 merged 2026-04-23、Q1-Q5 全 A）
- [x] インフラ設計 本編（PR #31 merged 2026-04-23）
- [x] コード生成 計画（PR #32 merged 2026-04-23、Phase 1-11 + Q A-F 全 A）
- [x] コード生成 PR A（Phase 1-7、PR #33 merged 2026-04-23、Copilot 2 round 対応含む）
- [x] コード生成 PR B（Phase 8-11、PR #34 merged 2026-04-23、Copilot 3 round 対応含む）
- [x] ビルドとテスト（Build and Test runbook template、PR #35 merged 2026-04-23、実 API 疎通は運用フェーズで実施）

#### U-NTF（進行中、Sprint 4）
- [x] Minimal 合本版 計画（PR #36 merged 2026-04-23、Q1-Q8 全 A）
- [x] Minimal 合本版 本編（PR #37 merged 2026-04-23）
- [x] インフラ設計 計画（PR #38 merged 2026-04-23、Q1-Q4 全 A）
- [x] インフラ設計 本編（PR #39 merged 2026-04-23）
- [x] コード生成 計画（PR #40 merged 2026-04-23、Phase 1-9 + Q A-C 全 A）
- [x] コード生成 実装（PR #41 merged 2026-04-23、Copilot 1 round 対応含む）
- [x] ビルドとテスト（Build and Test runbook template、PR #42 merged 2026-04-23、実 API 疎通は運用フェーズで実施）

#### U-BFF（進行中、Sprint 3 / 実装順で最後）
- [x] Minimal 合本版 計画（PR #43 merged 2026-04-23、Q1-Q9 全 A）
- [x] Minimal 合本版 本編（Functional + NFR Req + NFR Design）— PR #44 merged 2026-04-23（Copilot 対応は PR #45 内）
- [x] インフラ設計 計画（PR #45 merged 2026-04-23、Q1-Q6 全 A）
- [x] インフラ設計 本編（Infrastructure Design：deployment-architecture + terraform-plan）— PR #46 merged 2026-04-23
- [x] コード生成 計画（PR #47 merged 2026-04-23、Phase 1-9 + Q A-F 全 A）
- [x] コード生成 PR A（Phase 1-5: proto + domain + application + infrastructure + interfaces）— PR #49 merged 2026-04-23（Copilot 3 round 対応含む）
- [ ] コード生成 PR B（Phase 6-9: Composition Root + Terraform + Docs + CI）— 実装完了、PR レビュー待ち
- [ ] ビルドとテスト（Build and Test）

#### U-CSS / U-ING / U-BFF / U-NTF / U-APP
- [ ] 各 Unit を同じ 6 サブステージでループ

### 🟡 OPERATIONS フェーズ
- [ ] オペレーション（プレースホルダー）

## ワークフロー切替前の事前合意事項（参考）
- クライアント: Flutter アプリ
- バックエンド: reearth-cms Integration REST API（データストア）
- データソース: 外務省 海外安全情報オープンデータ（https://www.ezairyu.mofa.go.jp/html/opendata/）
- 前プロジェクト（toilet-map）から overseas-safety-map へ 2026-04-20 にピボット（audit.md 参照）
