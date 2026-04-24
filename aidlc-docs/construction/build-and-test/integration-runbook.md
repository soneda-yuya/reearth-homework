# 全体 Build and Test — 統合ランブック

**Status**: 🟡 **Template only** — バックエンド 5 Unit + U-APP を **一気通貫で動かす E2E 手順**。各 Unit の個別 runbook で単体疎通を済ませたあと、本ドキュメントに沿って統合疎通を実施する。

**前提**:
- [`shared-infrastructure.md`](../shared-infrastructure.md) のとおり U-PLT Terraform が `apply` 済み
- GCP project `overseas-safety-map-test`（または prod）に Cloud Run / Pub/Sub / Firestore が存在
- Firebase プロジェクトが有効、Anonymous Auth Enabled、APNs Auth Key 登録済
- reearth-cms が稼働し、Integration Token が `cms-integration-token` Secret Manager にある
- Mapbox access token が `mapbox-access-token` Secret Manager にある
- Anthropic API key が `anthropic-api-key` Secret Manager にある
- Flutter 実機（iOS 1 台 + Android emulator 1 台）が用意済

---

## 1. 目的

以下の **MOFA XML → Flutter 表示 + 通知受信** の E2E パイプラインを検証する:

```
MOFA XML (外務省オープンデータ)
   ↓ U-ING Scheduler Run (5min interval, 手動 trigger 可)
   ↓ Claude extract + Mapbox / centroid geocode
reearth-cms (Integration REST API)
   ↓ U-ING publish
Pub/Sub topic: safety-incident.new-arrival
   ↓ Push Subscription
U-NTF (Cloud Run Service)
   ↓ Firestore dedup (24h TTL)
   ↓ Firestore users collection subscribers 解決
Firebase Cloud Messaging
   ↓ FCM push (多端末 Multicast)
iOS / Android 実機 (U-APP)
   ↓ 通知表示 + タップで deep link

        ─── 独立した経路 ───
Flutter app ⇄ U-BFF Connect RPC
   ↓ SafetyIncident / CrimeMap / UserProfile RPC
reearth-cms (read) + Firestore (users)
```

確認項目:
1. **U-CSS** の cmsmigrate が reearth-cms にスキーマを適用できる（idempotent CREATE）
2. **U-ING** が MOFA XML を取り込み、`safety-incident` item として CMS に書き、Pub/Sub に publish できる
3. **U-NTF** が Pub/Sub push を受け、Firestore dedup を通し、FCM 配信する
4. **U-BFF** が Firebase ID Token を検証し、11 RPC を正しく返す
5. **U-APP** が Flutter UI で地図 / 一覧 / 詳細を表示し、通知受信する
6. 各 Unit 間の **OTel trace が span_id で連鎖**している（同一 trace で CMS → Pub/Sub → FCM → Flutter RPC が追える）
7. DLQ（Pub/Sub 失敗時）が空である

---

## 2. 事前準備

### 2.1 Terraform apply（まだの場合）

```bash
cd terraform/environments/prod
terraform init
terraform apply -var="cms_base_url=https://cms.example.com"
```

確認:
- Cloud Run Service 2（bff / notifier）+ Cloud Run Job 2（cmsmigrate / ingestion）が作成
- Firestore database `(default)` + `users_notification` composite index + `notifier_dedup` TTL policy
- Pub/Sub topic `safety-incident.new-arrival` + DLQ + Push Subscription
- Artifact Registry + Secret Manager + IAM

### 2.2 イメージデプロイ

`main` ブランチへの push で GitHub Actions（ci + deploy）が:
1. buf generate / go test / flutter test
2. Docker build + Artifact Registry push
3. `terraform apply -var="<deployable>_image_tag=<git-sha>"` で Revision 更新

手動実行する場合:
```bash
make build-all
make push   # Artifact Registry に push (CI 設定前の暫定)
terraform -chdir=terraform/environments/prod apply \
  -var="bff_image_tag=<sha>" \
  -var="notifier_image_tag=<sha>" \
  -var="ingestion_image_tag=<sha>" \
  -var="cmsmigrate_image_tag=<sha>"
```

### 2.3 Flutter 実機の準備

別レポ [`overseas-safety-map-app`](https://github.com/soneda-yuya/overseas-safety-map-app) の [`U-APP/build-and-test/runbook.md`](https://github.com/soneda-yuya/overseas-safety-map-app/blob/main/aidlc-docs/construction/U-APP/build-and-test/runbook.md) §2 を完了しておく:
- `flutterfire configure` で `firebase_options.dart` を実値に
- APNs Auth Key を Firebase Console にアップロード
- `BFF_BASE_URL` を `$(gcloud run services describe bff ...)` で取得

---

## 3. 実行手順（順序重要）

### 3.1 U-CSS: CMS スキーマ適用

```bash
gcloud run jobs execute cms-migrate \
  --region=asia-northeast1 \
  --project=overseas-safety-map-test \
  --wait
```

- [ ] 終了コード 0
- [ ] Cloud Logging に `cmsmigrate stopped cleanly` + 作成された project / model / field 数
- [ ] CMS UI で `safety-incident` モデルに 19 field が存在（U-CSS domain の schema_definition.go と一致）

詳細は [`U-CSS/build-and-test/runbook.md`](../U-CSS/build-and-test/runbook.md) 参照。

### 3.2 U-ING: 取り込みパイプライン

```bash
gcloud scheduler jobs run ingestion-spot \
  --location=asia-northeast1 \
  --project=overseas-safety-map-test
```

- [ ] Cloud Run Job が成功（Cloud Logging）
- [ ] ログに `app.ingest.phase=done` + `processed=N` + `published=N` + `failed=0`
- [ ] reearth-cms に `spot_info` の item が追加されている
- [ ] Pub/Sub topic `safety-incident.new-arrival` にメッセージが publish されている（`gcloud pubsub topics list-subscriptions` → dead letter 有無確認）

詳細は [`U-ING/build-and-test/runbook.md`](../U-ING/build-and-test/runbook.md) 参照。

### 3.3 U-NTF: 通知配信

U-ING の publish で自動的に Pub/Sub push が走り、notifier が動く:

- [ ] Cloud Run Service `notifier` のログに `app.notifier.phase=done` + `recipients=N` + `fcm_success=N` + `fcm_failed=0`
- [ ] Firestore `notifier_dedup/{key_cd}` に doc 作成（TTL 24h）
- [ ] iOS / Android 実機の通知センターに push 通知が表示
  - ⚠ 先に U-APP を起動して FCM token を Firestore に登録しておく（§3.5 参照）

詳細は [`U-NTF/build-and-test/runbook.md`](../U-NTF/build-and-test/runbook.md) 参照。

### 3.4 U-BFF: Connect RPC 疎通

```bash
# Flutter アプリから ID Token を取得できる前提で RPC 叩く
# （もしくは Firebase REST API / Postman）
curl -X POST "$BFF_URL/overseasmap.v1.SafetyIncidentService/ListSafetyIncidents" \
  -H "Authorization: Bearer $ID_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"filter":{"limit":10}}'
```

- [ ] 認証: Bearer なしで呼ぶと `unauthenticated` エラー
- [ ] List: §3.2 で投入した item が返る（count > 0）
- [ ] GetChoropleth: `count` + `color` を持つ `CountryChoropleth` が返る
- [ ] GetProfile: 初回呼び出しで lazy create、Firestore `users/{uid}` に doc 作成
- [ ] UpdateNotificationPreference → Firestore に反映

詳細は [`U-BFF/build-and-test/runbook.md`](../U-BFF/build-and-test/runbook.md) 参照。

### 3.5 U-APP: Flutter アプリ E2E

別レポの runbook §3 を実施:

1. Android emulator で起動（`flutter run -d emulator-5554 --dart-define=BFF_BASE_URL="$BFF_BASE_URL"`）
2. SplashScreen → 匿名サインイン → 地図タブ遷移
3. 地図タブ: OSM tile + heatmap 描画
4. 一覧タブ → 詳細画面 → 戻る
5. 設定タブ: Switch 操作で Firestore `users/{uid}.notification_preference.enabled` が更新
6. iOS 実機で同手順 + FCM token が Firestore `users/{uid}.fcm_tokens` に登録
7. §3.2 で U-ING を再実行 → §3.3 の notifier 経由で通知受信
8. 通知タップで `/incidents/detail/<keyCd>` に deep link 着地

### 3.6 OTel trace 連鎖の確認

1. Cloud Trace で `app.ingest.phase=fetch` で始まる trace を開く
2. 同 trace 内に以下 span が存在することを確認:
   - U-ING: `cms.upsert_item` + `pubsub.publish`
   - U-NTF: `pubsub.receive` + `firestore.dedup.check` + `fcm.send_multicast`
   - U-BFF: `rpc.list_safety_incidents` + `cms.list_items`（※ U-APP タップ後）
3. span 間のリンク（`link` or `parentSpanId`）が設定されていること

---

## 4. トラブルシューティング（共通）

### 4.1 U-ING で Pub/Sub publish が失敗

- `ingestion-runtime` SA に `roles/pubsub.publisher` 付与を確認（U-ING Infra Design）
- Publisher 側の Cloud Logging で `publish failed: PermissionDenied`

### 4.2 U-NTF で DLQ が増える

- `gcloud pubsub subscriptions pull notifier-dlq-sub --limit=10 --auto-ack` で原因確認
- `notifier` 側のエラー集約（Cloud Logging `severity=ERROR`）

### 4.3 U-BFF が CodeUnavailable 頻発

- CMS 側の障害（ステータスページ確認）
- Integration Token 期限切れ → Secret Manager 更新 + Cloud Run Revision 更新

### 4.4 Flutter アプリで通知が届かない

- Firebase プロジェクトと GCP プロジェクトが同じか（U-NTF が他 project の FCM を叩いている）
- `users/{uid}.fcm_tokens` が空 → §3.5 で起動しているか
- iOS: APNs Auth Key が有効か、Apple Developer で certificate が revoke されていないか
- Android 13+: `POST_NOTIFICATIONS` 権限が拒否されていないか

### 4.5 各 Unit runbook の個別トラブル

- [`U-CSS`](../U-CSS/build-and-test/runbook.md)
- [`U-ING`](../U-ING/build-and-test/runbook.md)
- [`U-NTF`](../U-NTF/build-and-test/runbook.md)
- [`U-BFF`](../U-BFF/build-and-test/runbook.md)
- [`U-APP` (別レポ)](https://github.com/soneda-yuya/overseas-safety-map-app/blob/main/aidlc-docs/construction/U-APP/build-and-test/runbook.md)

---

## 5. 観測ポイント（統合ビュー）

| 観測対象 | Unit | 期待値 |
|---|---|---|
| MOFA 取得件数 / Run | U-ING | 平時 20-50 件 |
| CMS write 成功率 | U-ING | 95%+（LLM 失敗 / Mapbox 失敗は country_centroid で継続） |
| Pub/Sub publish 数 | U-ING → U-NTF | CMS write 成功数と一致 |
| Dedup hit 率 | U-NTF | 0-10%（新着中心の運用） |
| FCM delivery 成功率 | U-NTF | 95%+（無効 token は ArrayRemove） |
| BFF RPC p95 | U-BFF | 500ms 以内（Choropleth は 1s） |
| U-APP cold start | U-APP | 3s 以内 |
| DLQ 残件数 | 統合 | 常時 0 |

---

## 6. 実行記録

### 6.1 [日付未定] test 環境での統合疎通

**実行者**: TBD
**GCP project**: TBD
**Firebase project**: TBD
**§3.1 U-CSS**: TBD
**§3.2 U-ING 1 Run の処理件数**: TBD（processed / published / failed）
**§3.3 U-NTF 受信 + FCM 配信**: TBD
**§3.4 U-BFF 11 RPC 応答**: TBD
**§3.5 U-APP Flutter 疎通**: TBD
**§3.6 OTel trace 連鎖**: TBD

### 6.2 [日付未定] prod 環境での初回統合疎通

**実行者**: TBD
**BFF URL (prod)**: TBD
**U-ING 24h 稼働での processed 累計**: TBD
**U-NTF 24h 稼働での delivery 成功率**: TBD
**U-APP Flutter ストア公開状況 (TestFlight / Internal Testing)**: TBD

### 6.3 [日付未定] 本番リリース後の観測

**実行者**: TBD
**Cloud Logging severity=ERROR の件数 (1 週間)**: TBD
**DAU（推定）**: TBD
**通知配信成功率 (1 週間)**: TBD

---

## 7. 非スコープ

- **多地域 / DR**: 単一リージョン (`asia-northeast1`)、Firestore は multi-region で自動対応
- **Web 版 Flutter の E2E**: U-APP Design で non-goal、将来対応
- **Alerting Policy / SLO monitoring**: MVP では Cloud Monitoring ダッシュボード目視、Operations フェーズで自動 alert 化
- **Load test / stress test**: NFR Design で想定規模 (DAU 100 / RPC 10 qps) は余裕、必要時点で k6 or Locust で追加
- **Crashlytics / Analytics**: プライバシー検討後に判断

---

## 8. 関連ドキュメント

### 親プロジェクト
- [`build-instructions.md`](./build-instructions.md) — Go / Docker / Terraform ビルド手順
- [`unit-test-instructions.md`](./unit-test-instructions.md) — Go unit test 実行手順
- [`integration-test-instructions.md`](./integration-test-instructions.md) — Unit 間結合テスト手順
- [`build-and-test-summary.md`](./build-and-test-summary.md) — U-PLT 時点の要約
- [`../shared-infrastructure.md`](../shared-infrastructure.md) — GCP shared resource 定義

### 各 Unit runbook
- [U-CSS](../U-CSS/build-and-test/runbook.md)
- [U-ING](../U-ING/build-and-test/runbook.md)
- [U-NTF](../U-NTF/build-and-test/runbook.md)
- [U-BFF](../U-BFF/build-and-test/runbook.md)
- [U-APP (別レポ)](https://github.com/soneda-yuya/overseas-safety-map-app/blob/main/aidlc-docs/construction/U-APP/build-and-test/runbook.md)
