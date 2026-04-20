# User Stories 実施判定 — overseas-safety-map

## Request Analysis
- **Original Request**: 外務省 海外安全情報オープンデータを取り込み、LLM 抽出 + ジオコーディングで緯度経度化して reearth-cms に蓄積し、Flutter アプリで閲覧する。
- **User Impact**: Direct（一般旅行者・登録ユーザーがアプリを直接操作する）
- **Complexity Level**: Complex（複数サブシステム＋LLM＋ジオコーディング＋通知＋認証＋マップ）
- **Stakeholders**:
  - エンドユーザー（一般閲覧ユーザー／登録ユーザー／管理・運用担当）
  - 外務省（データ提供元、出典表記などの規約遵守）
  - 開発者本人（AI-DLC を通じた学習・実装）

## Assessment Criteria Met
- [x] **High Priority — New User Features**: Flutter アプリに 6 つの新規画面（地図／一覧／詳細／検索／現在地近く／通知設定）が実装される。
- [x] **High Priority — Multi-Persona Systems**: 認証前の一般閲覧ユーザーと、認証後のお気に入り／通知を設定する登録ユーザーが明確に異なる。
- [x] **High Priority — Customer-Facing APIs**: BFF が外部クライアント（Flutter）に対する公開 API を提供する。
- [x] **High Priority — Complex Business Logic**: LLM 地名抽出→ジオコーディング→国セントロイドフォールバックの分岐、通知配信、差分取り込みなど、実装検討事項が多い。
- [x] **Medium Priority — Integration Work**: MOFA XML・reearth-cms・Firebase・Mapbox など複数サービス統合。
- [x] **Medium Priority — Security Enhancements**: Firebase Auth、Integration token の BFF 側保持など、ユーザー／権限に関わる意思決定が必要。

## 予想される効果
- アプリ画面ごとに「誰が」「何を」「なぜ」する行為なのかが明文化され、設計フェーズで議論が発散しない。
- 受け入れ基準（Acceptance Criteria）が結合・E2E テストのゴールデンパスに直結する（NFR-TEST-01/03 と整合）。
- ジオコーディング失敗・通知 ON/OFF・オフライン閲覧など、境界条件のストーリー化により非機能の抜け漏れを検知できる。
- Firebase 周りのユーザー導線（認証・設定保存）が明確化され、画面遷移図・Firestore スキーマ設計に直結する。

## Decision
**Execute User Stories**: **Yes**

**Reasoning**:
- High Priority 基準に 4 項目、Medium Priority に 2 項目が該当しており、スキップ条件（Pure Refactoring / Isolated Bug Fix / Infrastructure Only / Developer Tooling / Documentation）には当てはまらない。
- 画面数・ペルソナ数・外部統合数のいずれも多く、ストーリーを経由して受け入れ基準を先に固めるほうが後工程の手戻りを抑えられる。

## Expected Outcomes
- `personas.md`: 2〜3 種類のユーザーペルソナ（一般閲覧者 / 登録ユーザー / 運用担当）
- `stories.md`: INVEST 準拠のユーザーストーリー + 各ストーリーに受け入れ基準（Given/When/Then or Checklist）
- ストーリーと要件（FR-ID）のトレーサビリティマトリクス
- 非機能的関心事（ログ出典表記・通知 ON/OFF・フォールバック挙動など）がストーリー内受け入れ基準として明文化される
