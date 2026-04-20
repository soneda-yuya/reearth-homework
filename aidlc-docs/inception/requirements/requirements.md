# 要件定義書 — overseas-safety-map

## 1. Intent 分析サマリー

| 項目 | 内容 |
|---|---|
| **ユーザー要求** | 外務省 海外安全情報オープンデータを取り込み、本文テキストから発生地を抽出して緯度経度に変換のうえ reearth-cms に蓄積し、Flutter アプリで地図・一覧・詳細を閲覧できるようにする。 |
| **プロジェクトタイプ** | New Project（グリーンフィールド） |
| **スコープ見積り** | Cross-system（取り込みパイプライン / reearth-cms / BFF / Flutter アプリ / Firebase） |
| **複雑さ見積り** | Moderate 〜 Complex（LLM 地名抽出＋ジオコーディング、5分毎の継続取り込み、repository パターンによるデータソース差し替え可能設計、本番運用相当の監視/ログを MVP に含める） |
| **方法論** | AI-DLC（Adaptive）／Standard Depth |

---

## 2. データソースと前提

- **提供元**: 外務省 海外安全情報オープンデータ（https://www.ezairyu.mofa.go.jp/html/opendata/）
- **形式**: XML、5分毎更新、商用・非商用とも無償
- **ライセンス**: 政府標準利用規約 第2.0 版（CC BY 4.0 互換）— 出典表記必須、加工時はその旨明示、加工情報を国作成と偽ることの禁止
- **主フィード**:
  - 初回全件: `https://www.ezairyu.mofa.go.jp/html/opendata/area/00A.xml`
  - 継続取り込み: `https://www.ezairyu.mofa.go.jp/html/opendata/area/newarrivalA.xml`
- **mail 要素の主フィールド**: `keyCd`, `infoType`, `infoName`, `leaveDate`, `title`, `lead`, `mainText`, `infoUrl`, `koukanCd`, `koukanName`, `area(cd, name)`, `country(cd, name)`
- **緯度経度フィールドは存在しない** → タイトル／本文からの地名抽出＋ジオコーディングが必要

---

## 3. システム構成（論理）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 取り込みパイプライン（Go / クラウドスケジューラ実行）                              │
│  1. MOFA XML 取得（area/00A.xml 初回 / newarrivalA.xml 継続）                   │
│  2. XML パース → mail 構造体化                                                  │
│  3. 既存 keyCd チェック（重複スキップ、追記専用）                                  │
│  4. LLM で title+mainText から発生地名抽出                                       │
│  5. Mapbox Geocoding API で緯度経度化                                           │
│  6. 失敗時は country コードのセントロイドにフォールバック                           │
│  7. reearth-cms Integration API で Item 作成（POST）                            │
└─────────────────────────────────────────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ reearth-cms SaaS（reearth.io）                                                │
│  - Workspace: 手動作成                                                        │
│  - Project / Model / Field: 初回セットアップスクリプトで Integration API 経由作成 │
│  - Item: 取り込みパイプラインが CRUD                                             │
└─────────────────────────────────────────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ BFF（Backend-for-Frontend、Go、クラウド常駐または Serverless）                     │
│  - Flutter からのリクエストを受けて CMS Integration API を叩く                    │
│  - Integration token は BFF 側に保持（アプリ配布しない）                           │
│  - データソース抽象化（repository パターン、将来 DB 直接化を可能に）                │
│  - 検索・絞り込み（areaCd / countryCd / infoType / 期間）のクエリ変換              │
└─────────────────────────────────────────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Flutter アプリ（iOS / Android）                                                │
│  - 地図画面（flutter_map / OSM タイル / ピン・クラスタ）                          │
│  - 一覧 / 詳細 / 検索 / 現在地近く / プッシュ通知                                  │
│  - Firebase Authentication でログイン、Firestore にお気に入り・通知設定保存        │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. 機能要件（Functional Requirements）

### 4.1 取り込みパイプライン

- **FR-ING-01**: パイプラインは Go で実装する。
- **FR-ING-02**: クラウドスケジューラ（GitHub Actions または同等）により 5 分間隔で実行する。
- **FR-ING-03**: 初回実行時は `area/00A.xml`（全量）を取得して CMS を初期化する。
- **FR-ING-04**: 以降は `area/newarrivalA.xml`（新着、全量）を取得する。
- **FR-ING-05**: mail 要素を `keyCd` でユニーク識別し、既に CMS に存在すれば **スキップ**（追記専用、既存は変更しない）。
- **FR-ING-06**: 新規 mail について、`title + mainText` を LLM に渡して発生地名（文字列）を抽出する。
- **FR-ING-07**: 抽出した地名を **Mapbox Geocoding API** に投げて緯度経度（1 点）を取得する。
- **FR-ING-08**: 地名抽出またはジオコーディングに失敗した場合、`country.cd` に対応する **国セントロイド座標** にフォールバックして必ず保存する。
- **FR-ING-09**: パイプライン実行結果（成功/失敗件数、失敗理由）を構造化ログとして出力する。

### 4.2 reearth-cms セットアップ

- **FR-CMS-01**: reearth-cms は reearth.io SaaS を使用する。
- **FR-CMS-02**: Workspace は手動で作成する（Integration API 非対応のため）。
- **FR-CMS-03**: Project / Model / Field は **セットアップスクリプト**（Go、単発実行）を用意し、Integration API の `ProjectCreate` / `ModelCreate` / `FieldCreate` で自動作成する。
- **FR-CMS-04**: Integration token は手動で発行し、環境変数として各コンポーネント（パイプライン / BFF / セットアップスクリプト）に渡す。
- **FR-CMS-05**: 安全情報モデルには以下のフィールドを保存する：
  - `keyCd`（テキスト、ユニークキー相当）
  - `title` / `lead` / `mainText`（テキスト）
  - `leaveDate`（日時）
  - `infoType` / `infoName`（テキスト）
  - `koukanCd` / `koukanName`（テキスト）
  - `areaCd` / `areaName` / `countryCd` / `countryName`（テキスト）
  - `extractedLocation`（テキスト — LLM 抽出地名）
  - `geometry`（Geometry — Point）
  - `infoUrl`（URL — 外務省側ページ）
  - `ingestedAt` / `updatedAt`（日時）

### 4.3 BFF（Backend-for-Frontend）

- **FR-BFF-01**: BFF は Go で実装し、クラウドに常駐または Serverless（Cloud Run 等）として稼働する。
- **FR-BFF-02**: Integration token はサーバ側にのみ保持し、クライアント（Flutter アプリ）には漏洩させない。
- **FR-BFF-03**: Flutter アプリ向け REST API を提供する：
  - 安全情報一覧（新着順、ページング、フィルタ: `areaCd` / `countryCd` / `infoType` / `期間`）
  - 安全情報詳細（`keyCd` または内部 ID）
  - GeoJSON 形式での地図用データ（Integration API の `ItemsAsGeoJSON` をプロキシ可）
- **FR-BFF-04**: **データソース抽象化**（repository パターン）を採用し、後日 reearth-cms から直接 DB へ差し替えられる構造にする。

### 4.4 Flutter アプリ

- **FR-APP-01**: 配信ターゲットは iOS と Android の 2 プラットフォーム。
- **FR-APP-02**: 地図ライブラリは `flutter_map`（OSM 系タイル、OSS）。
- **FR-APP-03**: 以下の画面を MVP に含める：
  - 地図画面（ピン／クラスタ表示、タップで概要ポップアップ）
  - 一覧画面（新着順、タップで詳細）
  - 詳細画面（`title` / `mainText` / 発信日 / 公館 / **出典テキスト＋元記事リンク**）
  - 検索・絞り込み画面（地域 / 国 / 情報種別 / 期間）
  - 現在地近くの安全情報（端末 GPS ＋ 周辺クエリ）
  - プッシュ通知（新着通知、ユーザー設定で ON/OFF・対象国を指定可）
- **FR-APP-04**: UI 言語は日本語＋英語（MOFA データ本文は日本語のまま表示）。
- **FR-APP-05**: 認証は **Firebase Authentication** で必須（**全画面ログイン必須**）。初回起動時にログイン画面を表示し、未ログインではアプリを利用不可とする。お気に入り国・通知設定は **Firestore** に保存する。セッション維持・自動トークン更新・ログアウト導線も提供する。
- **FR-APP-06**: アプリからのデータ取得は **BFF 経由のみ**。Integration token は端末に配布しない。BFF は Firebase ID Token を検証したうえでリクエストを受け付ける。
- **FR-APP-07**: **出典表記**：
  - 「情報について」または「設定」メニュー内に出典ページを設ける。
  - 各安全情報詳細画面にも「出典：外務省 海外安全情報オープンデータ」とリンク、および「本アプリでは LLM／ジオコーダで位置情報を加工しています」旨を明示する。
- **FR-APP-08**: **犯罪マップ画面**（独立メニュー項目）を MVP に含める：
  - 犯罪・事件関連の `infoType` コードのみで抽出したアイテム集合を GIS 可視化する。
  - 初期ズーム（広域）では **国別カロプレス**（件数を色濃淡で表現）、詳細ズームでは **ヒートマップ**（密度表現）へ自動で切替える。
  - 期間フィルタ（直近7日 / 30日 / 90日 / 全期間）を UI で切替可能。
  - フォールバック座標（国セントロイド）で保存されたアイテムは、**ヒートマップには寄与させず、カロプレス集計にのみ含める**（誤解を避けるため）。
  - 凡例・色スケール・合計件数を常時表示し、特定国タップで US-06 の該当国絞り込み一覧に遷移可能にする。
  - 「犯罪」判定に用いる `infoType` コード集合の最終確定は **設計フェーズで決定**する（前提: MOFA `infotype.xlsx` コード表より事件・犯罪に該当するタイプを選定）。

---

## 5. 非機能要件（Non-Functional Requirements）

### 5.1 パフォーマンス
- **NFR-PERF-01**: CMS に蓄積されるアイテム数の目安は〜500 件（MVP）。将来的な拡張も見越すが、スケーリング設計は後続フェーズとする。
- **NFR-PERF-02**: パイプラインの 1 実行は 5 分以内に完了する（次回実行と重ならない）。
- **NFR-PERF-03**: BFF の一覧 API はサーバ側キャッシュを検討する（CMS 呼び出しレート抑制）。

### 5.2 セキュリティ（Security Baseline 拡張: **有効**）
- **NFR-SEC-01**: `reearth-cms` Integration token、Mapbox API キー、LLM API キー、Firebase Admin キーは環境変数／シークレットマネージャで管理し、コミットしない。
- **NFR-SEC-02**: BFF は Integration token を端末配布せず、API 認可は Firebase ID Token を BFF 側で検証する方式とする。
- **NFR-SEC-03**: Flutter アプリとの通信は HTTPS のみ。
- **NFR-SEC-04**: Firestore のセキュリティルールはユーザーごとの読み書き権限を適切に設定する（自身のお気に入り・通知設定のみアクセス可）。
- **NFR-SEC-05**: 依存ライブラリは CI で脆弱性スキャンを行う（Go modules / pub / npm のいずれも対象）。

### 5.3 テスト（Property-Based Testing 拡張: **有効**）
- **NFR-TEST-01**: テスト戦略は **ユニットテスト＋ウィジェットテスト＋結合テスト**。
- **NFR-TEST-02**: 地名抽出・ジオコーディング・XML パーサ・repository 実装等の純粋関数／シリアライズ層には **PBT（プロパティベーステスト）** を適用する。
- **NFR-TEST-03**: Flutter アプリはウィジェットテストおよびゴールデンパスの結合テストを CI で実行する。
- **NFR-TEST-04**: カバレッジ閾値は MVP 時点では強制しないが、追加ユニットから下回らないレギュレッションチェックを入れる。

### 5.4 運用・観測性
- **NFR-OPS-01**: 取り込みパイプラインと BFF は **構造化ログ**（JSON）を出力し、クラウドロギングに集約する。
- **NFR-OPS-02**: 監視項目: パイプライン実行成功率、ジオコーディング成功率、CMS API エラー率、BFF レイテンシ／エラー率。
- **NFR-OPS-03**: 異常検知時は **アラート**（メール/Slack 等）を発報する。
- **NFR-OPS-04**: スケーリングは 500 件規模のため MVP では静的リソースで十分。設計上は水平スケール可能な構成（ステートレス BFF 等）を採用しておく。

### 5.5 拡張性・保守性
- **NFR-EXT-01**: BFF のデータソース層は **repository パターン** で抽象化し、`reearth-cms` から直接 DB（PostgreSQL 等）への差し替えを **コード変更箇所を局所化** できるようにする。
- **NFR-EXT-02**: LLM / ジオコーダ / 地図タイルプロバイダも **インターフェイスで抽象化** し、差し替えを容易にする。

### 5.6 ライセンス・コンプライアンス
- **NFR-LIC-01**: MOFA 利用規約に従い、アプリ内に出典表記を実装する（FR-APP-07）。
- **NFR-LIC-02**: MOFA 原文を加工・変形して表示する場合、その旨を UI 上で明示する（位置情報は LLM／ジオコーダで加工している）。
- **NFR-LIC-03**: OSM タイル、Mapbox、LLM、Firebase 各サービスの利用規約にも従う。

---

## 6. デプロイ・配布

- **DEP-01**: パイプライン・BFF・セットアップスクリプトはクラウドにデプロイする（GitHub Actions / Cloud Run / Cloud Functions 等、設計フェーズで確定）。
- **DEP-02**: Flutter アプリは TestFlight（iOS）／Play Console Internal Testing（Android）に配信する。
- **DEP-03**: CI/CD（GitHub Actions）でテスト → ビルド → デプロイまでの最小パイプラインを MVP に含める。

---

## 7. 技術スタックまとめ

| レイヤ | 採用技術 |
|---|---|
| 取り込みパイプライン | Go + クラウドスケジューラ |
| 地名抽出 | LLM（Claude / OpenAI 等、設計で確定） |
| ジオコーディング | Mapbox Geocoding API |
| データストア | reearth-cms（reearth.io SaaS）+ Integration REST API |
| BFF | Go（Cloud Run / Functions / コンテナ常駐） |
| Flutter 地図 | `flutter_map`（OSM タイル） |
| 認証・ユーザーデータ | Firebase Authentication / Firestore |
| 多言語 | 日本語・英語 |
| テスト | ユニット＋ウィジェット＋結合＋PBT |
| CI/CD | GitHub Actions |

---

## 8. スコープ（MVP）

- MVP の対象機能: 上記 FR すべて（地図・一覧・詳細・検索・現在地・プッシュ通知）。
- MVP の対象件数: 〜500 件。
- 将来の拡張（MVP 外）: 全世界数万件への対応、reearth-cms → DB 直接化、多言語機械翻訳、Web・デスクトップ展開、ユーザー投稿などは本 MVP 範囲外。ただし設計時にこれらを見越した抽象化を行う。

---

## 9. 主要な仮定・未決事項

- **AS-01**: LLM の選定（Claude / OpenAI / ローカル LLM）は設計フェーズで決定する。
- **AS-02**: BFF のホスティング先（Cloud Run / Cloud Functions / Render / Fly.io 等）は設計フェーズで決定する。
- **AS-03**: プッシュ通知のプロバイダ（Firebase Cloud Messaging を基本線とする）。
- **AS-04**: スケジューラの具体的実装（GitHub Actions を基本線、取り込み頻度 5 分制約に従う代替を検討）。
- **AS-05**: アプリ公開（ストア申請）は本 MVP 範囲外とし、TestFlight／Internal Testing のみを対象とする。

---

## 10. 拡張機能コンプライアンスサマリー

| 拡張 | 状態 | 本段階での適合性 | 根拠 |
|---|---|---|---|
| セキュリティベースライン | 有効 | 要件で NFR-SEC-01〜05 として網羅。詳細設計・実装で強制。 | Q1 回答 `A`（有効化） |
| プロパティベーステスト | 有効 | NFR-TEST-02 で地名抽出・ジオコーディング・XML パーサ・repository などに適用方針を定義。 | Q2 回答 `A`（有効化） |
