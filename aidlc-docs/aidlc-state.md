# AI-DLC 状態トラッキング

## プロジェクト情報
- **プロジェクト名**: overseas-safety-map（海外安全情報マップ）
- **プロジェクトタイプ**: グリーンフィールド
- **開始日**: 2026-04-20T17:55:00Z（アイデア変更のリセット）
- **現在のフェーズ**: INCEPTION
- **現在のステージ**: アプリケーション設計（Application Design — 承認待ち）

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
- [x] アプリケーション設計（承認待ち — 成果物 5点生成済み）
- [ ] ユニット計画（EXECUTE 予定）
- [ ] ユニット生成（EXECUTE 予定）

### 🟢 CONSTRUCTION フェーズ
- [ ] 機能設計（Functional Design — EXECUTE 予定）
- [ ] NFR 要件（NFR Requirements — EXECUTE 予定）
- [ ] NFR 設計（NFR Design — EXECUTE 予定）
- [ ] インフラ設計（Infrastructure Design — EXECUTE 予定）
- [ ] コード生成（Code Generation — EXECUTE 必須）
- [ ] ビルドとテスト（Build and Test — EXECUTE 必須）

### 🟡 OPERATIONS フェーズ
- [ ] オペレーション（プレースホルダー）

## ワークフロー切替前の事前合意事項（参考）
- クライアント: Flutter アプリ
- バックエンド: reearth-cms Integration REST API（データストア）
- データソース: 外務省 海外安全情報オープンデータ（https://www.ezairyu.mofa.go.jp/html/opendata/）
- 前プロジェクト（toilet-map）から overseas-safety-map へ 2026-04-20 にピボット（audit.md 参照）
