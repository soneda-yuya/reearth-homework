# Unit of Work Plan — overseas-safety-map

## Plan Overview

本ドキュメントは AI-DLC Units Planning（Units Generation の Part 1: Planning）の成果物です。Application Design で決定した **DDD × Bounded Context** 構造をベースに、Construction フェーズで **Unit 単位にループ** する際の粒度を確定します。

すべての [Answer]: に回答し、`done` と伝えてください。回答後に Part 2 で以下 3 つの成果物を生成します。

- `aidlc-docs/inception/application-design/unit-of-work.md` — Unit 定義と責務
- `aidlc-docs/inception/application-design/unit-of-work-dependency.md` — 依存マトリクス
- `aidlc-docs/inception/application-design/unit-of-work-story-map.md` — Story ↔ Unit 対応表

---

## Step-by-step Checklist

- [ ] Q1〜Q6 すべて回答
- [ ] 回答の矛盾・曖昧さを AI が検証、必要なら clarification
- [ ] 承認
- [ ] `unit-of-work.md` を生成
- [ ] `unit-of-work-dependency.md` を生成
- [ ] `unit-of-work-story-map.md` を生成
- [ ] Unit 境界と依存の妥当性を検証
- [ ] 全ストーリーの Unit 割当が漏れていないか確認

---

## Context Summary（Application Design からの確定事項）

- **Bounded Context（バックエンド）**: `safetyincident`（Core、`crimemap` を Subdomain として内包）/ `notification` / `user` / `cmsmigrate`
- **Composition Root / Deployable**: `cmd/ingestion`（取り込みパイプライン）/ `cmd/bff`（Connect サーバ）/ `cmd/notifier`（Pub/Sub サブスクライバ）/ `cmd/cmsmigrate`（CMS スキーマ冪等適用）
- **入口レイヤ**: `internal/interfaces/{rpc,job}`
- **横断基盤**: `internal/platform/*`（observability / cmsx / firebasex / pubsubx / mapboxx / config / connectserver）、`internal/shared/*`（errs / clock）
- **Flutter アプリ**: 別リポジトリ（`overseas-safety-map-app`、Clean Architecture + MVVM + Riverpod）
- **契約**: `proto/v1/*.proto`（Connect + Pub/Sub の両方）
- **MVP Story 数**: 13（US-01〜US-13）＋ Post-MVP 3

**前提となる AI 推奨案（参考）**:
- Unit は「Construction フェーズで一巡（Functional Design → NFR Req → NFR Design → Infra Design → Code Gen → Build & Test）する粒度」として扱う。
- 候補は 2 軸:
  - **軸 A (Deployable 単位)**: ingestion / bff / notifier / cmsmigrate / flutter-app の 5 Unit + 共通基盤 Platform を 1 Unit（計 6）
  - **軸 B (Bounded Context 単位)**: safetyincident / user / notification / cmsmigrate / flutter-app の 5 Unit + Platform（計 6）
  - **軸 C (Hybrid)**: BC を基本に Deployable で切り出す混合（例: `safetyincident-ingestion` と `safetyincident-read` に分ける）

---

## Questions

### Question 1 — Unit 分割の主軸
Construction フェーズで一巡する「Unit」の切り方はどれを採用しますか？

A) **Deployable 単位**（推奨）: U-ING（ingestion）/ U-BFF（bff）/ U-NTF（notifier）/ U-SET（cmsmigrate）/ U-APP（Flutter）/ U-PLT（platform & proto 共通）の 6 Unit
B) **Bounded Context 単位**: U-SI（safetyincident + crimemap）/ U-USR（user）/ U-NTC（notification）/ U-CSS（cmsmigrate）/ U-APP（Flutter）/ U-PLT の 6 Unit
C) **Hybrid**: safetyincident だけ「書き込み側（ingestion）」と「読み取り側（bff 用 application）」に分け、他は Deployable 単位に寄せる
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 2 — U-PLT（platform & proto 共通）の位置づけ
横断基盤（`internal/platform/*` / `internal/shared/*` / `proto/v1/*.proto` / `buf` 設定）を独立 Unit として最初に仕上げる運用にしますか？

A) はい、**U-PLT を Unit 0** として先に完了させ、以降の全 Unit が参照する（推奨）
B) いいえ、Unit 化せず各 Unit の作業に都度追加していく（遅延決定型）
C) U-PLT は切り出すが、proto だけは各 Unit が必要になった時に追加する
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 3 — Unit 実装順序の方針
Construction フェーズで Unit をどの順で進めますか？

A) **依存順**（推奨）: U-PLT → U-CSS（CMS スキーマ確定が前提）→ U-ING（書き込みの土台）→ U-BFF（読み取り）→ U-NTC → U-APP（最後、他のサーバ確認後）
B) **ストーリー駆動**: MVP の高優先ストーリーから着手できる Unit を優先
C) **並行**: 順序をつけず全 Unit を並行に進める
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 4 — デプロイモデル
各 Deployable をどのように配置しますか？

A) **Cloud Run 系ですべて統一**: ingestion = Cloud Run Job、bff = Cloud Run Service、notifier = Cloud Run Service（Pub/Sub push 受け）、cmsmigrate = Cloud Run Job 単発（推奨）
B) **Functions 混在**: ingestion は GitHub Actions、notifier は Cloud Functions（Pub/Sub trigger）、bff だけ Cloud Run
C) **単一コンテナに同居**: 1 プロセスで複数役割、サブコマンド切替
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 5 — チーム構成
開発体制はどれですか？（Unit の担当分けに影響します）

A) **1 人**で全 Unit を担当（本プロジェクトの想定）
B) 2 人（バックエンド／フロントエンド分担）
C) 機能別複数人
X) Other（[Answer]: の後ろに自由記述）

[A]: 

### Question 6 — Unit 粒度の微調整
Unit 毎の Construction 1 巡は **「完結した機能デモができる単位」** とします。以下の境界は MVP デモの節目として妥当ですか？（複数選択可）

A) **U-CSS 完了時**: CMS に安全情報モデルが存在し、手動で Item 作成できる
B) **U-ING 完了時**: MOFA から CMS へ取り込みが自動で動く（地名抽出・ジオコード・保存まで）
C) **U-BFF 完了時**: `curl` や `buf curl` で Connect API 経由の一覧取得・詳細・犯罪マップ集計が返る
D) **U-NTC 完了時**: Pub/Sub 経由で FCM 通知が届く（テストユーザー端末に実機検証）
E) **U-APP 完了時**: iOS/Android 実機で 13 MVP ストーリーが動作する
X) Other（[Answer]: の後ろに自由記述）

[A,B,C,E]: 

---

## 承認前の最終確認（回答後に AI が埋めます）

- Unit 分割軸: _TBD_
- Unit 数: _TBD_
- 実装順序: _TBD_
- デプロイモデル: _TBD_
- 体制: _TBD_
- デモ節目: _TBD_

回答完了後、矛盾・曖昧さがなければ 3 つの Unit 成果物を生成します。
