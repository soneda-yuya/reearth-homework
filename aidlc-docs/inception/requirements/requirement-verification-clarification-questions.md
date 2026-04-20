# 要件検証 — Clarification Questions（overseas-safety-map）

回答いただいた中で、矛盾または曖昧な点がありました。以下の追加質問にお答えください。各質問の [Answer]: の後ろに選択肢のアルファベット（複数選択の場合はカンマ区切り）を記入してください。完了後 `done` と伝えてください。

---

## 検出した矛盾・曖昧さ

### 矛盾 1: Q13（保存フィールド） vs Q16（画面）／Q8〜Q11（位置情報）
Q13 で「A（keyCd のみ）」のみ選ばれていますが、Q16 で地図・一覧・詳細・検索すべてを MVP に含める方針です。地図表示には H（geometry）、一覧・詳細には B（title/lead/mainText）、絞り込みには F（area/country）、発信日順ソートには C（leaveDate）が **不可欠** です。A のみでは機能が成立しません。意図を確認させてください。

### 曖昧さ 1: Q3（取り込みフィード）
「Bで国/地域別で検索できるようにしてください。バックグラウンドで新着を使用してデータを更新し続けます。」は複合指示のため、運用フローを確定させたいです。

### 曖昧さ 2: Q12（CMS 手動セットアップ範囲）
「X, Workspace, Projectのみ手動」と回答いただきましたが、**reearth-cms Integration REST API には Model / Schema / Field を作成するエンドポイントが存在しません**（前回の要件分析時にも判明）。パイプラインから Model/Field を自動作成することは技術的に不可能なため、補完手段を選んでください。

### 曖昧さ 3: Q18（ユーザー登録） × Q19（BFF 経由）
ユーザー登録ありの場合、ユーザー情報（アカウント・お気に入り・通知設定）はどこに保存しますか。reearth-cms はコンテンツ管理用途であり、ユーザー管理には不向きです。

### 曖昧さ 4: Q25（MOFA 出典表記）
MOFA 利用規約で **出典記載が必須** と確認しました（「出典：外務省 海外安全情報オープンデータ（URL）」、編集・加工時はその旨も明示）。この前提で方針を確定させてください。

### 情報提供: Q21（本番運用相当） × Q22（〜500 件）
Q21 で本番運用相当（監視・ログ・アラート・スケーリング）を選択、Q22 で 〜500 件と非常に小規模です。500 件規模であればスケーリング要件はほぼ不要ですが、監視・ログ・アラートは学習目的としては妥当です。意図を確認するため Clarify 5 として再確認します。

---

## Clarification 1（Q13 の再確認）
CMS に保存するフィールドを改めて選び直してください（複数選択可）。

A) keyCd（ユニークID）【必須】
B) title / lead / mainText（テキスト）
C) leaveDate（発信日時）
D) infoType / infoName（情報種別）
E) koukanCd / koukanName（発信公館）
F) areaCd / areaName / countryCd / countryName（地域・国）
G) extractedLocation（抽出した地名文字列）
H) geometry（緯度経度 — Point または GeoJSON）【地図表示に必須】
I) infoUrl（外務省側の詳細ページURL）【出典表記のため必須相当】
J) ingestedAt / updatedAt（取り込みメタ）
K) 全部（A〜J すべて）
X) Other（[Answer]: の後ろに自由記述）

[K]: 

---

## Clarification 2（Q3 の運用フロー）
取り込みフィードの実運用フローを1つ選んでください。

A) **初回 + 継続運用**: 初回は `area/00A.xml`（全件）で CMS を初期化、以降は `area/newarrivalA.xml`（5分毎の新着）を取り込む。Flutter アプリ側の検索・絞り込みは CMS クエリで `areaCd/countryCd` を使う。
B) **継続運用のみ**: 新着 `newarrivalA.xml` のみ取り込む（過去データは蓄積されない）。検索・絞り込みは同様に Flutter 側で。
C) **国・地域別ごと取り込み**: Flutter 側で国/地域を選んだときに、その都度対応する XML（例: `country/{cd}A.xml`）を取得して表示（CMS に保存せず、BFF が MOFA → GeoJSON 変換のみ担当）。
X) Other（[Answer]: の後ろに自由記述）

[A]: 

---

## Clarification 3（Q12 の補完手段）
Integration API では Model/Field を作成できないため、どう進めますか。

A) Workspace / Project / Model / Field まですべて CMS UI で手動作成し、AI が「どのフィールドを何型で作るか」の手順書（スクリーンショット付き等）を用意する。
B) reearth-cms の内部 GraphQL Admin API があればそれをスクリプトで叩き、Model / Field を自動作成する（調査と実装コスト発生）。
C) Workspace / Project のみ手動、Model / Field は「コマンド一発で作れるスクリプト」を用意（実体は B 相当、Workspace と Project ID だけ環境変数で渡す形）。
X) Other（[Answer]: の後ろに自由記述）

[X,https://github.com/reearth/reearth-cms/blob/main/server/schemas/integration/integration.yml 定義に存在しませんか？]: 

---

## Clarification 4（Q18・Q19 のユーザー管理）
ユーザー情報の保存先を選んでください。

A) Firebase Authentication + Firestore（BFF が薄く前段に立つ、あるいは直接アプリから）
B) Auth0 + BFF 側 DB（PostgreSQL / SQLite など）
C) Supabase（認証＋DB をマネージド一括）
D) BFF 側に自前の認証・DB を実装（フルスタック開発）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

---

## Clarification 5（Q25 の出典表記の実装範囲）
MOFA 規約で出典表記は必須です。実装する表示範囲を選んでください。

A) **推奨**: アプリ内の「情報について」「設定」等のメニューに出典ページを設け、各安全情報詳細画面にも出典テキスト＋元記事へのリンクを表示。
B) アプリ起動時のスプラッシュ／メニューに一度だけ出典表記。各詳細画面にはリンクのみ。
C) 各詳細画面に出典テキスト＋元記事へのリンクのみ（集約ページは作らない）。
X) Other（[Answer]: の後ろに自由記述）

[A]: 

---

## Clarification 6（Q21 × Q22 のスコープ再確認）
本番運用相当（監視・ログ・アラート・スケーリング）× 〜500 件 の組み合わせについて、意図はどれですか。

A) 学習目的でも本番と同等の運用体験を積みたい（監視・ログ・アラートは導入するが、スケーリングは 500 件規模で十分と認識している）
B) 取り込み件数はまず 500 件から始め、将来的には件数を増やす想定。最初から本番運用レベルで組みたい。
C) 500 件という件数は MVP の目安であり、本番運用は将来の話。MVP は Q21 を「B: MVP + CI/CD と基本的な自動テスト」に変更したい。
X) Other（[Answer]: の後ろに自由記述）

[X,MVPで作るが将来を見越しています]: 
