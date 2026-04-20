# Application Design Plan — overseas-safety-map

## Plan Overview

本ドキュメントは AI-DLC Application Design ステージの計画書です。高レベルなコンポーネント識別・責務分割・サービス層設計・依存関係の方針を確定するための質問を含みます。**詳細なビジネスロジックは Construction フェーズの Functional Design で扱う** ため、ここではアーキテクチャ骨子に絞ります。

すべての [Answer]: に回答し、`done` と伝えてください。

---

## Step-by-step Checklist

- [ ] Q1〜Q10 の計画質問にすべて回答
- [ ] 回答の矛盾・曖昧さを AI が検証、必要なら clarification を作成
- [ ] 確定事項をもとに以下 5 つの成果物を生成:
  - [ ] `aidlc-docs/inception/application-design/components.md` — コンポーネント定義と責務
  - [ ] `aidlc-docs/inception/application-design/component-methods.md` — メソッドシグネチャ（ビジネスルール詳細は後で）
  - [ ] `aidlc-docs/inception/application-design/services.md` — サービスレイヤ定義
  - [ ] `aidlc-docs/inception/application-design/component-dependency.md` — 依存関係マトリクス・データフロー図
  - [ ] `aidlc-docs/inception/application-design/application-design.md` — 上記を統合した索引ドキュメント
- [ ] 承認後、Units Planning へ進む

---

## Context Summary

要件＋ユーザーストーリーから抽出した主要サブシステム:
1. **取り込みパイプライン**（Go）— MOFA XML → LLM 抽出 → Mapbox Geocode → CMS Item 登録
2. **CMS セットアップスクリプト**（Go）— Integration API で Project/Model/Field を作成
3. **BFF**（Go）— Flutter 向け REST API、Firebase ID Token 検証、CMS → 将来 DB 差し替え可能な repository
4. **Flutter アプリ**（iOS/Android）— 13 MVP ストーリーに対応
5. **通知配信**（クラウド側）— 新着取り込み時に対象ユーザーへ FCM 送信
6. **Firebase**（Auth / Firestore / FCM）— 認証・ユーザープロファイル・通知配信基盤

外部依存: reearth-cms SaaS / Mapbox Geocoding / LLM / MOFA オープンデータ / GitHub Actions

---

## Questions

### Question 1 — リポジトリ構成
ソースコードのリポジトリ構成はどうしますか？

A) **モノレポ**（この `reearth-homework` リポジトリに Go / Flutter / ドキュメントをすべて配置、ディレクトリで分ける）
B) **2 リポジトリ分割**（サーバー側 Go モノレポ + Flutter 専用リポジトリ）
C) **マルチリポジトリ**（pipeline / bff / setup-script / flutter-app で4リポジトリに分ける）
X) Other（[Answer]: の後ろに自由記述）

[B]: 

### Question 2 — Go モジュール構成（Q1 の枠内で）
Go 側（ingestion / bff / setup-script / 共通 domain・repository）のモジュール構成は？

A) **単一 Go モジュール + internal サブパッケージ**（`cmd/{ingestion,bff,setup}` + `internal/{domain,cms,geocode,llm}`）。最小運用で DRY。
B) **Go workspace（go.work）+ 複数モジュール**（pipeline / bff / setup / shared の独立モジュール）。境界明確・将来分離しやすいが初期オーバーヘッド。
C) **単一バイナリにサブコマンド**（`safety-map ingest | bff | setup` のように CLI でモード切替）。シンプルだがスコープ違いのフラグが混在する。
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 3 — Flutter プロジェクト構造
Flutter アプリの内部構造は？

A) **Feature-first**（`lib/features/{map,list,detail,search,nearby,favorites,notifications,crime_map,auth,about}` それぞれに ui/state/data を抱える）
B) **Layer-first**（`lib/{presentation,domain,data}` の伝統的クリーンアーキテクチャ）
C) **Feature-first × 薄いレイヤー**（Feature ごとに ui / controller / repo、共通基盤は `lib/core`）
X) Other（[Answer]: の後ろに自由記述）

[X, Clean Architecture + MVVM]: 

### Question 4 — BFF ↔ Flutter の API スタイル
BFF が Flutter に提供する API の形式は？

A) **REST + JSON**（`net/http` + `chi` or `echo`、OpenAPI で仕様管理）
B) **GraphQL**（`gqlgen` 等、スキーマ駆動）
C) **Connect（gRPC 互換）**（スキーマ駆動、strong typing、Flutter 側は `connectrpc`）
X) Other（[Answer]: の後ろに自由記述）

[C]: 

### Question 5 — Go HTTP フレームワーク／ルーター
Q4 が REST または必要に応じて Connect 以外を選ぶ場合、Go 側で使うフレームワーク／ルータは？

A) 標準 `net/http` + `github.com/go-chi/chi/v5`（軽量、依存少）
B) `github.com/labstack/echo/v4`（機能豊富、バッテリー同梱）
C) `github.com/gin-gonic/gin`（高速、人気）
D) Q4 で GraphQL / Connect を選んだため不要
X) Other（[Answer]: の後ろに自由記述）

[D]: 

### Question 6 — Flutter 状態管理
Flutter 側の状態管理ライブラリは？

A) **Riverpod**（近年の標準、`riverpod_generator` で DI もカバー）
B) **BLoC / flutter_bloc**（イベント駆動、規約厳格）
C) **Provider**（シンプル、小規模向け）
D) **GetX**（ルーティング・DI 含め一式）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 7 — 通知配信の仕組み
「取り込みパイプラインが新着を CMS に入れた後、対象ユーザーに FCM プッシュを送る」動作の仕組みは？

A) **パイプラインが直接 FCM Admin SDK を呼ぶ**（ingestion 内で Firestore からユーザー購読情報を取得し、そのまま配信）
B) **Pub/Sub 経由の分離**（ingestion が新着を Pub/Sub へ publish → 通知用 Cloud Function が購読者を解決して FCM 配信）
C) **Firestore トリガー**（ingestion が通知キュー用 Firestore コレクションに書き込み → Cloud Function が onCreate で配信）
D) **Cron 型通知**（通知専用の定期関数が未配信分をまとめて配信、低遅延要件なし）
X) Other（[Answer]: の後ろに自由記述）

[B]: 

### Question 8 — LLM プロバイダ（地名抽出用）
地名抽出で使う LLM プロバイダの第一候補は？

A) **Anthropic Claude**（Haiku クラスを想定、高速・低コスト）
B) **OpenAI**（GPT-4o-mini クラス）
C) **Google Gemini**
D) **ローカル LLM / セルフホスト**（Ollama 等）
E) プロバイダ抽象化のみして設計時点では未確定とする（インターフェイスだけ先に）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 9 — データアクセスの抽象化深さ
Q19（Integration API → 将来 DB 差替）を実現する repository 抽象化の範囲は？

A) **最小範囲**: BFF のみ repository 抽象化、ingestion は CMS Integration API を直呼び
B) **両方**: BFF と ingestion の両方で repository インターフェイスを挟む（保存先の一貫差し替え）
C) **BFF のみインターフェイス + ingestion は専用 "Writer" 抽象**（読み・書きで抽象が異なる運用）
X) Other（[Answer]: の後ろに自由記述）

[B]: 

### Question 10 — エラーハンドリング・観測性の方針
Go 側の横断的ルールを選んでください（複数選択可）。

A) **エラーは `fmt.Errorf("context: %w", err)` でラップ、`errors.Is/As` で判定**（sentinel + typed error）
B) **構造化ログは `log/slog`**（標準ライブラリ、JSON ハンドラ）
C) **メトリクスは OpenTelemetry** で出力、バックエンドは後日（Cloud Run / GCP Monitoring を想定）
D) **トレーシング**（OpenTelemetry で pipeline / BFF / LLM / Geocode / CMS の呼び出しチェーンを記録）
E) 上記すべてを採用（NFR-OPS に即した包括的採用）
X) Other（[Answer]: の後ろに自由記述）

[A,B,C,D]: 

---

## 承認前の最終確認（回答後に AI が埋めます）

- リポジトリ構成: _TBD_
- Go モジュール構成: _TBD_
- Flutter プロジェクト構造: _TBD_
- API スタイル / フレームワーク: _TBD_
- Flutter 状態管理: _TBD_
- 通知配信の仕組み: _TBD_
- LLM プロバイダ: _TBD_
- 抽象化深さ: _TBD_
- 横断ルール: _TBD_

回答完了後、矛盾・曖昧さがなければ 5 つの設計成果物を生成します。
