# Story ↔ Unit 対応表 — overseas-safety-map

## 凡例
- **Primary（◎）**: 当該 Story の体験を成立させる **主担当** Unit（UI / UX の所在地）。
- **Contributing（○）**: データ供給・API 提供・契約定義など、Story の成立に **必要な貢献** をする Unit。
- **Supporting（△）**: 間接的な前提を提供する Unit（基盤・スキーマ）。

## 表: Story × Unit

| Story ID | タイトル | U-PLT | U-CSS | U-ING | U-BFF | U-NTF | U-APP |
|---|---|:-:|:-:|:-:|:-:|:-:|:-:|
| US-01 | 初回オンボーディング・新規登録 | △ | — | — | ○ | — | ◎ |
| US-02 | ログイン維持・再認証 | △ | — | — | ○ | — | ◎ |
| US-03 | 地図で安全情報を概観する | △ | △ | ○ | ○ | — | ◎ |
| US-04 | 一覧で新着順に確認する | △ | △ | ○ | ○ | — | ◎ |
| US-05 | 詳細を読む（出典表記含む） | △ | △ | ○ | ○ | — | ◎ |
| US-06 | 条件で絞り込む | △ | △ | ○ | ○ | — | ◎ |
| US-07 | 現在地周辺を確認する | △ | △ | ○ | ○ | — | ◎ |
| US-08 | お気に入り国を登録する | △ | — | — | ○ | — | ◎ |
| US-09 | 通知設定（国・情報種別） | △ | — | — | ○ | ○ (購読者情報の登録先) | ◎ |
| US-10 | プッシュ通知 → 詳細への遷移 | △ | △ | ○ (trigger event) | ○ (詳細 API) | ◎ | ◎ |
| US-11 | 出典・利用規約ページ | △ | — | — | — | — | ◎ |
| US-12 | オフライン・エラー時の挙動 | △ | — | — | ○ | — | ◎ |
| US-13 | 犯罪多発エリアを GIS で俯瞰 | △ | △ | ○ | ○ (集計 API) | — | ◎ |

### 集計

| Unit | Primary Story 数 | Contributing Story 数 | Supporting Story 数 |
|---|:-:|:-:|:-:|
| U-PLT | 0 | 0 | 13 |
| U-CSS | 0 | 0 | 8（データ永続化を伴う Story） |
| U-ING | 0 | 7 | 0 |
| U-BFF | 0 | 12 | 0 |
| U-NTF | 1（US-10） | 1（US-09 の購読者情報） | 0 |
| U-APP | 12 | 1（US-10 は NTF と合同） | 0 |

---

## Story ごとの Unit 寄与詳細

### US-01: 初回オンボーディング・新規登録
- **U-APP**: ログイン／新規登録画面、利用規約・出典表記ページ遷移、Firebase Auth 連携
- **U-BFF**: （直接寄与は小さいが）`AuthInterceptor` が認証後の API 呼び出しを許可するため、認証後の最初の画面ロードで必要
- **U-PLT**: Firebase SDK factory、proto 契約

### US-02: ログイン維持・再認証
- **U-APP**: Firebase Auth のトークン永続化と自動再取得、Connect Interceptor の ID Token 付与
- **U-BFF**: 401 応答と Token 検証ロジック

### US-03: 地図で安全情報を概観する
- **U-APP**: 地図画面 / ピン描画 / クラスタ / ポップアップ / フォールバック座標の注記
- **U-BFF**: `ListSafetyIncidents` / `GetSafetyIncidentsAsGeoJSON` RPC
- **U-ING**: 表示対象のデータを日々供給
- **U-CSS**: Model と geometry フィールドの存在

### US-04: 一覧で新着順に確認する
- **U-APP**: 一覧画面、新着順ソート、ページング、アクセシビリティ対応
- **U-BFF**: `ListSafetyIncidents` RPC（`leaveDate` 降順）
- **U-ING**: データ供給
- **U-CSS**: Model に `leaveDate` フィールド

### US-05: 安全情報の詳細を読む（出典表記含む）
- **U-APP**: 詳細画面、本文、出典テキスト、`infoUrl` リンク、フォールバック注記、加工ポリシーツールチップ
- **U-BFF**: `GetSafetyIncident` RPC
- **U-ING**: `mainText` / `extractedLocation` / `geocodeSource` を保持した Item を作成
- **U-CSS**: Model にそれらのフィールド

### US-06: 条件で絞り込む
- **U-APP**: 絞り込み UI、条件永続化、一覧と地図への反映
- **U-BFF**: `SearchSafetyIncidents` RPC（areaCd / countryCd / infoType / 期間）
- **U-ING**: データ供給
- **U-CSS**: インデックス可能なフィールド

### US-07: 現在地周辺を確認する
- **U-APP**: OS 位置情報パーミッション、現在地センタリング、タイムアウト UX
- **U-BFF**: `ListNearby` RPC（緯度経度 + 半径 km）
- **U-ING**: データ供給
- **U-CSS**: geometry フィールド

### US-08: お気に入り国を登録する
- **U-APP**: 詳細 / 一覧でのアイコン、お気に入り画面、Firestore 読み書き（Connect 経由 or 直接）
- **U-BFF**: `ToggleFavoriteCountry` / `GetProfile` RPC（Q19 [B] BFF 経由の方針）
- **U-PLT**: Firebase SDK factory

### US-09: 通知設定（国・情報種別）
- **U-APP**: 通知設定画面、OS 通知パーミッション、FCM トークン取得、Firestore 同期
- **U-BFF**: `UpdateNotificationPreference` / `RegisterFcmToken` RPC
- **U-NTF**: ここで登録された購読情報を後続の配信で参照する（Subscriber として）

### US-10: プッシュ通知 → 詳細への遷移
- **U-NTF**: **主担当**。Pub/Sub 受信、購読者解決、FCM 送信、無効トークン除去
- **U-APP**: 通知ペイロードを受けた Router 遷移、未認証時のリダイレクト保持、既に削除された Item のフォールバック画面
- **U-ING**: `NewArrivalEvent` 発行（trigger）
- **U-BFF**: `GetSafetyIncident` RPC（詳細表示のため）

### US-11: 出典・利用規約ページ
- **U-APP**: 「情報について」ページ、MOFA 出典表記、CC BY 4.0 互換表示、加工ポリシー

### US-12: オフライン・エラー時の挙動
- **U-APP**: オフラインバナー / キャッシュ表示 / エラー境界 / 24時間古いキャッシュ警告
- **U-BFF**: 5xx / 401 応答ルール（Token 再取得との連携）

### US-13: 犯罪多発エリアを GIS で俯瞰
- **U-APP**: 「犯罪マップ」メニュー、カロプレス ⇄ ヒートマップ切替、期間フィルタ、凡例、国タップで絞り込み遷移
- **U-BFF**: `CrimeMap.GetChoropleth` / `GetHeatmap` RPC（犯罪 infoType 集計、フォールバック除外ロジック）
- **U-ING**: データ供給
- **U-CSS**: infoType / geometry / geocodeSource フィールド

---

## ストーリー未割当のチェック

全 13 MVP ストーリーが必ず 1 つ以上の Unit に **Primary または Contributing** として割当済み:

| Story | 割当 Unit |
|---|---|
| US-01 | U-APP (Primary), U-BFF (Contrib) |
| US-02 | U-APP (Primary), U-BFF (Contrib) |
| US-03 | U-APP (Primary), U-BFF / U-ING (Contrib) |
| US-04 | U-APP (Primary), U-BFF / U-ING (Contrib) |
| US-05 | U-APP (Primary), U-BFF / U-ING (Contrib) |
| US-06 | U-APP (Primary), U-BFF / U-ING (Contrib) |
| US-07 | U-APP (Primary), U-BFF / U-ING (Contrib) |
| US-08 | U-APP (Primary), U-BFF (Contrib) |
| US-09 | U-APP (Primary), U-BFF / U-NTF (Contrib) |
| US-10 | U-NTF (Primary), U-APP (Primary), U-BFF / U-ING (Contrib) |
| US-11 | U-APP (Primary) |
| US-12 | U-APP (Primary), U-BFF (Contrib) |
| US-13 | U-APP (Primary), U-BFF / U-ING (Contrib) |

**未割当ストーリーなし** ✓

## Post-MVP ストーリーへの参考

| Post-MVP Story | 想定 Unit |
|---|---|
| US-P01 多言語翻訳 | U-BFF（サーバ翻訳）or U-APP（オンデバイス）— どちらを取るかで追加 |
| US-P02 Web / デスクトップ | U-APP（Flutter Web）or 新規 Unit（Next.js 等）— 技術選定次第 |
| US-P03 履歴アーカイブ | U-ING（保持期間変更）+ U-BFF（検索範囲拡大）+ U-CSS（インデックス）+ U-APP（UI） |
