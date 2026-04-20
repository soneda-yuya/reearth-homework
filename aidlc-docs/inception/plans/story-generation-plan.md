# Story Generation Plan — overseas-safety-map

## Plan Overview

本ドキュメントは AI-DLC User Stories フェーズ Part 1（Planning）の成果物です。ユーザーストーリー作成方針を決めるための質問を含みます。すべての [Answer]: に回答し、`done` と伝えてください。Part 2（Generation）で `personas.md` と `stories.md` を生成します。

## Step-by-step Checklist

- [ ] 質問（下記 Q1〜Q8）にすべて回答
- [ ] 回答の矛盾・曖昧さを AI が検証
- [ ] 必要なら clarification 質問ファイルを作成・再回答
- [ ] 計画内容（ペルソナ数・ブレイクダウン方式・ストーリー形式・受け入れ基準形式）をユーザーが承認
- [ ] `aidlc-docs/inception/user-stories/personas.md` を生成（INVEST 準拠の前提として）
- [ ] `aidlc-docs/inception/user-stories/stories.md` を生成
- [ ] 各ストーリーに受け入れ基準と要件トレーサビリティ（FR-ID 参照）を付与
- [ ] INVEST 基準（Independent / Negotiable / Valuable / Estimable / Small / Testable）適合性を確認
- [ ] 完了メッセージを提示し承認を仰ぐ

---

## Context Summary（要件から抽出）

- **対象プラットフォーム**: Flutter iOS / Android アプリ、BFF（Go）、取り込みパイプライン（Go）、reearth-cms（SaaS）、Firebase（Auth / Firestore / FCM）
- **主要画面**: 地図 / 一覧 / 詳細 / 検索・絞り込み / 現在地近く / 通知設定（計6画面）
- **主要機能**: MOFA XML 取り込み・LLM 地名抽出・Mapbox Geocoding・国セントロイドフォールバック・プッシュ通知・ユーザー別お気に入り / 通知設定
- **スコープ**: MVP（〜500 件）、設計レベルで本番運用を見越す
- **出典表記**: アプリ内メニュー＋各詳細画面で必須

---

## 計画質問（Questions）

### Question 1 — ペルソナ構成
どのペルソナを定義したいですか？（複数選択可）

A) 一般閲覧ユーザー（認証不要、地図・一覧・詳細・検索のみ利用）
B) 登録ユーザー（Firebase 認証後、お気に入り国・通知設定を利用）
C) 運用担当（開発者自身、取り込みパイプラインの監視・再実行・CMS セットアップを行う）
D) データ提供者視点（外務省＝間接的ステークホルダーとして出典表記・ライセンス遵守の観点をストーリーに含める）
X) Other（[Answer]: の後ろに自由記述）

[B,認証をmustにしたいです]: 

### Question 2 — ストーリーのブレイクダウン方式
どのようにストーリーを分割したいですか？（1つ選択）

A) User Journey-Based（アプリ起動〜地図閲覧〜通知受信など、利用者の流れに沿ってストーリー化）
B) Feature-Based（画面・機能単位、地図機能・一覧機能・検索機能などで分ける）
C) Persona-Based（ペルソナごとにストーリー集を分け、A/B/C ごとに章を作る）
D) Hybrid（Persona × Feature）— ペルソナごとに章を作り、その中で機能別にストーリーを並べる
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 3 — ストーリーの粒度
1ストーリーの見積りイメージはどれですか？（INVEST の "Small" の基準）

A) とても細かい（1ストーリー = 半日〜1日の実装、受け入れ基準3〜5個）
B) 中粒度（1ストーリー = 2〜3日、受け入れ基準5〜10個）— MVP 規模で扱いやすい
C) 粗い（1ストーリー = 1週間以上、エピック相当も含む）
X) Other（[Answer]: の後ろに自由記述）

[B]: 

### Question 4 — ストーリー記述フォーマット
どの書式を採用しますか？（1つ選択）

A) **クラシック**: `As a <persona>, I want <capability>, so that <benefit>.`
B) **Job Story**: `When <situation>, I want to <motivation>, so I can <outcome>.`
C) **両方併用**（A を基本とし、必要に応じて B を採用）
X) Other（[Answer]: の後ろに自由記述）

[C]: 

### Question 5 — 受け入れ基準の記述形式
受け入れ基準（Acceptance Criteria）の書き方はどれにしますか？

A) **Given / When / Then** 形式（BDD / Gherkin 風、テストに直結しやすい）
B) **チェックリスト** 形式（成立条件を箇条書きで列挙）
C) 両方併用（重要ストーリーは GWT、補助ストーリーはチェックリスト）
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 6 — 非機能・横断関心事のストーリー化
以下の横断関心事を「ストーリー化する」「受け入れ基準に盛り込むだけ」「要件書どまり」のいずれで扱いますか？ 該当するものをストーリー化したい場合のみ選んでください（複数選択可、何も選ばなければ各機能ストーリーの受け入れ基準に含める）。

A) 出典表記（MOFA 利用規約）
B) プッシュ通知の配信・ON/OFF
C) オフライン時の挙動（地図キャッシュ・エラー表示）
D) 認証失敗・エラー時のユーザー体験
E) ジオコーディング失敗時の国セントロイドフォールバックの UX（「おおよその位置」注記など）
F) アクセシビリティ（スクリーンリーダー・色覚配慮）
X) Other（[Answer]: の後ろに自由記述）

[Answer]: 

### Question 7 — トレーサビリティ
ストーリーから要件（FR-ID）への追跡性はどう扱いますか？

A) 各ストーリーに「関連要件: FR-APP-01, FR-BFF-03」のように明記する
B) 別途トレーサビリティマトリクス（表）を `stories.md` 末尾に付ける
C) 両方（ストーリー内明記＋末尾マトリクス）
X) Other（[Answer]: の後ろに自由記述）

[Answer]: 

### Question 8 — 優先度・リリース区分の付与
各ストーリーに優先度／リリース区分を付けますか？

A) MVP / Post-MVP の2区分を付ける（MVP 完了判定に使用）
B) Must / Should / Could / Won't（MoSCoW）4区分
C) 優先度は付けず、すべて MVP 相当として扱う（要件の通り）
X) Other（[Answer]: の後ろに自由記述）

[Answer]: 

---

## 承認前の最終確認（回答後に AI が埋めます）

- ペルソナ数: _TBD_
- ブレイクダウン方式: _TBD_
- ストーリー数の目安: _TBD_
- ストーリー形式 / 受け入れ基準形式: _TBD_
- 横断関心事の扱い: _TBD_
- トレーサビリティ / 優先度の扱い: _TBD_

回答完了後、矛盾・曖昧さがなければ `personas.md` と `stories.md` の生成に進みます。
