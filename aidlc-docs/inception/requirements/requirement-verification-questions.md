# 要件検証質問 — overseas-safety-map

以下の質問に回答して、要件の明確化にご協力ください。各質問の [Answer]: タグの後ろに選択肢のアルファベット（複数選択の場合はカンマ区切り、例: `A,C`）を記入してください。いずれも該当しない場合は最後の「Other」を選び、内容を [Answer]: の後ろに記述してください。

すべて記入後、`done` と伝えてください。

---

## データソースとピボット後の前提

把握済みの情報（回答不要）:
- データ提供元: 外務省 海外安全情報オープンデータ（XML、5分毎更新、商用・非商用問わず無償）
- フィード種別: 新着 / すべての地域 / 地域別 / 国別 / 領事メール詳細 / 海外安全HP詳細（各×全量A/通常/軽量L）
- mail 要素の主フィールド: `keyCd`（ユニークID）/ `infoType` / `title` / `lead` / `mainText` / `leaveDate` / `infoUrl` / `koukanCd` / `koukanName` / `area` / `country`
- **緯度経度フィールドは存在しない** → 本文から地名抽出してジオコーディングが必要
- クライアント: Flutter アプリ
- バックエンド: reearth-cms Integration REST API（データストア）

---

## 拡張機能（Extension）の有効化

### Question 1
セキュリティ拡張ルールを本プロジェクトで強制しますか？

A) はい — SECURITY ルールを全てブロッキング制約として強制（本番相当のアプリに推奨）
B) いいえ — SECURITY ルールをスキップ（PoC・プロトタイプ・実験用途に適切）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 2
プロパティベーステスト（PBT）ルールを本プロジェクトで強制しますか？

A) はい — PBT ルールを全てブロッキング制約として強制（ビジネスロジック・データ変換・シリアライズ・状態のあるコンポーネントに推奨）
B) 部分的 — 純粋関数とシリアライズのラウンドトリップのみ PBT ルールを強制（アルゴリズム複雑性が限定的な場合に推奨）
C) いいえ — PBT ルールをスキップ（単純な CRUD / UI のみ / 薄い統合レイヤーに適切）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

---

## 取り込みパイプライン（Ingestion）

### Question 3
最初に取り込む XML フィードはどれを想定していますか？（MVP の範囲、複数選択可）

A) 新着（`area/newarrivalA.xml` など） — 常に最新の安全情報のみ処理
B) すべての地域（`area/00A.xml`） — 全世界の有効な安全情報を一括取得
C) 地域別（`area/{areaCd}A.xml`） — 特定地域のみ（例: 大洋州、中東など）
D) 国別（`country/{countryCd}A.xml`） — 特定国のみ
E) 領事メール詳細（`mail/{keyCd}A.xml`） — 一覧取得後に個別の詳細を追加取得
X) Other（[Answer]: の後ろに自由記述）

[Bで国/地域別で検索できるようにしてください。バックグラウンドで新着を使用してデータを更新し続けます。]: 

### Question 4
XML の粒度（情報量）はどれを使いますか？

A) 全量（`*A.xml`） — mainText を含む最大情報（地名抽出に必要）
B) 通常（`*.xml`） — mainText 省略・lead までを含む
C) 軽量（`*L.xml`） — タイトルとメタ情報のみ
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 5
取り込みパイプラインの実装言語／ランタイムは何を希望しますか？

A) Dart スクリプト（Flutter アプリと言語を統一）
B) Node.js / TypeScript（reearth-cms 周辺で一般的）
C) Python（XML パーサ・ジオコーダライブラリが豊富）
D) Go（高速・シングルバイナリ配布が容易）
X) Other（[Answer]: の後ろに自由記述）

[D]: 

### Question 6
パイプラインをどのように実行しますか？

A) ローカル実行のみ（手動 or ローカル cron） — MVP・検証用途
B) 定期スケジューラ（GitHub Actions / cron 的クラウド実行）
C) サーバ常駐プロセス（5分毎ポーリング）
D) サーバーレス関数 + スケジュールトリガー（AWS Lambda + EventBridge 等）
X) Other（[Answer]: の後ろに自由記述）

[B]: 

### Question 7
既存データとの差分取り込みはどう扱いますか？

A) `keyCd` 単位で重複排除し、既存と異なれば更新、無ければ新規作成
B) `keyCd` 新規のみ追加、既存は一切触らない（追記専用）
C) 毎回全件洗い替え（削除→再投入） — 件数が少ない場合の簡易運用
X) Other（[Answer]: の後ろに自由記述）

[B]: 

---

## 地名抽出とジオコーディング

### Question 8
本文から事故発生地点を抽出する主たるアプローチはどれを希望しますか？

A) LLM（Claude / OpenAI 等）で title + mainText から発生地名を抽出し、別途ジオコーダで緯度経度に変換
B) ルールベース（正規表現 + 地名辞書）で抽出し、別途ジオコーダで緯度経度に変換
C) 抽出は行わず、XML の `country` の国コードを国の代表座標（セントロイド）にマップする簡易方式
D) LLM に地名抽出＋緯度経度推定を一括で依頼（ジオコーダ呼び出しなし）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 9
ジオコーディングに使う外部サービスはどれを希望しますか？（Q8 で A/B を選んだ場合）

A) Nominatim（OpenStreetMap、無料・利用規約に注意）
B) Google Maps Geocoding API（高精度・有料・APIキー必要）
C) Mapbox Geocoding API（有料・APIキー必要）
D) Gaia / GeoNames 等の無料 API
E) ジオコーディングしない（Q8-D または Q8-C を選択）
X) Other（[Answer]: の後ろに自由記述）

[C]: 

### Question 10
1件の安全情報から抽出する座標は何点を想定しますか？

A) 1点のみ（最も代表的な発生地、見つからなければ国セントロイドにフォールバック）
B) 複数点（本文に複数地名がある場合はすべて保存し、地図上に複数ピンとして表示）
C) 国レベルのみ（県市区町村は保存しない）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 11
抽出・ジオコーディングに失敗した安全情報はどうしますか？

A) CMS には保存しない（スキップ、ログのみ残す）
B) 座標 null で保存し、地図には表示せずリストのみで閲覧できるようにする
C) 国セントロイドにフォールバックして必ず保存する
X) Other（[Answer]: の後ろに自由記述）

[C]: 

---

## reearth-cms のスキーマと連携

### Question 12
前回同様、reearth-cms の Workspace / Project / Model はあなたが CMS UI で手動作成し、Integration token を渡していただく運用で進めますか？

A) はい — Workspace / Project / Model / Field まで手動作成、パイプラインは Integration token 経由で Item の CRUD のみ行う
B) Workspace / Project / Model まで手動、Field は Integration API で自動作成（可能なら）
C) できる限り自動化したい — 手動作業を最小化する手順・スクリプトを用意してほしい
X) Other（[Answer]: の後ろに自由記述）

[X, Workspace, Projectのみ手動]: 

### Question 13
CMS の安全情報モデルに保存するフィールドはどれですか？（複数選択可、MVP 範囲）

A) keyCd（ユニークID）
B) title / lead / mainText（テキスト）
C) leaveDate（発信日時）
D) infoType / infoName（情報種別）
E) koukanCd / koukanName（発信公館）
F) areaCd / areaName / countryCd / countryName（地域・国）
G) extractedLocation（抽出した地名文字列）
H) geometry（緯度経度 — Point または GeoJSON）
I) infoUrl（外務省側の詳細ページURL）
J) ingestedAt / updatedAt（取り込みメタ）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 14
reearth-cms のインスタンスはどれを使いますか？

A) 既存の社内ホスト型 reearth-cms（前回と同じ）
B) 自前でローカルに docker-compose 等で立てた reearth-cms
C) reearth.io が提供する SaaS
X) Other（[Answer]: の後ろに自由記述）

[C]: 

---

## Flutter アプリ（クライアント）

### Question 15
Flutter アプリの配信ターゲットはどれですか？（複数選択可）

A) iOS（実機・シミュレータ）
B) Android（実機・エミュレータ）
C) Web（Flutter Web）
D) macOS デスクトップ
X) Other（[Answer]: の後ろに自由記述）

[A,B]: 

### Question 16
アプリの主要画面として MVP に含めたいものはどれですか？（複数選択可）

A) 地図画面（ピン or クラスタで安全事象を可視化）
B) 一覧画面（新着順リスト、タップで詳細）
C) 詳細画面（title / mainText / infoUrl へのリンク）
D) 検索・絞り込み（地域/国/情報種別/期間）
E) 現在地の近くの安全情報
F) プッシュ通知（新着の安全情報）
X) Other（[Answer]: の後ろに自由記述）

[A,B,C,D,E,F]: 

### Question 17
地図ライブラリは何を使いますか？

A) flutter_map（MapLibre / OpenStreetMap タイル、OSS）
B) google_maps_flutter（Google Maps、APIキー必要）
C) mapbox_gl（Mapbox、APIキー必要）
D) reearth の地図コンポーネント（埋め込み可能なら）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 18
アプリの認証はどうしますか？

A) 認証なし（閲覧専用の公開アプリ）
B) 匿名認証のみ（利用統計のため端末IDは生成）
C) ユーザー登録あり（お気に入り国・通知設定のため）
X) Other（[Answer]: の後ろに自由記述）

[C]: 

### Question 19
アプリが CMS からデータを取得する経路はどれですか？

A) Integration REST API を直接アプリから叩く（Integration token を端末に配布）
B) 薄い BFF（Backend-for-Frontend）を1本立て、アプリはそこ経由でアクセス（token はサーバ側に保持）
C) 静的ファイル配信（パイプラインが GeoJSON を生成して CDN 配信、アプリはそれを読むだけ）
X) Other（[Answer]: の後ろに自由記述）

[B,後でCMS APIをDBに差し替えできるように]: 

### Question 20
UI の言語はどうしますか？

A) 日本語のみ
B) 日本語＋英語（MOFA データが日本語主体のため両対応）
C) 多言語（日・英・その他の機械翻訳対応）
X) Other（[Answer]: の後ろに自由記述）

[B]: 

---

## スコープ・非機能・運用

### Question 21
本プロジェクトの実装スコープはどこまでを想定していますか？

A) MVP（地図でピンを表示・一覧・詳細まで、手動で動作確認）
B) MVP + CI/CD と基本的な自動テスト
C) 本番運用相当（監視・ログ・アラート・スケーリング）
X) Other（[Answer]: の後ろに自由記述）

[C]: 

### Question 22
想定する同時蓄積件数（CMS 上のアイテム数）の目安はどの程度ですか？

A) 〜500 件（特定地域・国のみ）
B) 〜5,000 件（全世界の直近数ヶ月）
C) 〜50,000 件（履歴を含む全件保持）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 23
テスト戦略はどうしますか？

A) 自動テストなし（型チェック＋手動確認のみ） — MVP 向け最小
B) ユニットテストのみ（地名抽出・ジオコーディング・XML パーサ等のロジック）
C) ユニット＋ウィジェット／結合テスト（Flutter アプリも含む）
X) Other（[Answer]: の後ろに自由記述）

[C]: 

### Question 24
アプリ・パイプラインのデプロイ先はどこですか？

A) ローカル起動のみ（デプロイしない） — 検証・デモ用途
B) パイプラインのみクラウドへ、アプリは手元ビルド
C) 両方クラウドへ（アプリは TestFlight / Play Console 等へ配布）
X) Other（[Answer]: の後ろに自由記述）

[C]: 

### Question 25
外務省オープンデータの利用規約・クレジット表記をアプリ内に表示する必要がありますか？

A) はい — 利用規約ページやクレジット表示を UI 上に必ず用意する
B) いいえ — MVP では不要（あとで追加）
X) Other（[Answer]: の後ろに自由記述）

[X,表示させるルールはありますか]: 
