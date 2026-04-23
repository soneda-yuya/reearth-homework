# U-NTF Build and Test — Runbook

**Status**: 🟡 **Template only** — 実 Firestore / Firebase Cloud Messaging / Pub/Sub Push 疎通は未実施。Firebase プロジェクトに iOS/Android 実機が登録でき次第、本ランブックに沿って実行し、結果を §6 に記録する。

**前提**:
- U-NTF Code Generation（PR #41 merged 2026-04-23）が main に取り込まれていること
- U-PLT shared infrastructure が `terraform apply` 済み（Firestore database + Pub/Sub topic/subscription + DLQ + IAM）
- **U-ING の Build and Test** も並行して（または先行して）実行できる状態（U-NTF は U-ING が Pub/Sub に publish するイベントを受信するため）
- Firebase プロジェクトが有効化済み、少なくとも 1 つの iOS / Android アプリが Firebase Console に登録済み

---

## 1. 目的

`cmd/notifier` が **Pub/Sub Push → Firestore Dedup → 購読者解決 → FCM 配信 → 無効 token 除去** のパイプラインを正しく動かすことを確認する:

1. Pub/Sub Push endpoint (`/pubsub/push`) が正しい HTTP status code を返す（Q6 [A]: 200 / 400 / 500）
2. **Firestore Dedup** が 24h TTL で機能する（同じ `key_cd` を 2 回 push しても 1 回しか FCM が叩かれない）
3. Firestore **複合インデックス** でユーザー検索が高速動作（`enabled` + `target_country_cds` array-contains）
4. `info_types` の in-memory filter が正しく動作（指定あり時のみ filter、空は pass-through）
5. FCM **`SendEachForMulticast`** で複数 token 一括配信が成功
6. 無効 token（`IsUnregistered` / `IsInvalidArgument` / `IsSenderIDMismatch`）が同一 Request 内で Firestore から `ArrayRemove` される
7. MOFA XML 構造の検証と同じく、**Pub/Sub push envelope の仮定が実 Pub/Sub と一致**
8. SIGTERM で graceful shutdown が in-flight request を drain

---

## 2. 事前準備（実行者が用意するもの）

### 2.1 Firebase / GCP セットアップ

| 項目 | 取得方法 |
|---|---|
| Firebase プロジェクト | GCP `overseas-safety-map-test` に対して Firebase を有効化（Firebase Console） |
| Firebase Admin SDK 認証 | **ADC** を使用（Cloud Run Runtime SA または `gcloud auth application-default login`） |
| 実機 / エミュレータ | iOS 実機（推奨）+ Android エミュレータ 1 台以上、Flutter アプリに FCM Token 発行機構が必要（**U-APP でアプリ実装するまで Flutter 側は代用ツールで検証**） |
| テスト用 FCM Token | [Firebase Console の Cloud Messaging Test](https://firebase.google.com/docs/cloud-messaging/test-messages) で任意の token に送れることを確認 |
| Pub/Sub Topic / Subscription | Terraform で作成済み（U-PLT + U-NTF Infra Design で apply） |

> ⚠️ **本番 Firestore / 本番 FCM プロジェクトで実行しない**。テスト専用プロジェクトを用意する。

### 2.2 Firestore Index の READY 確認

U-NTF Infra Design で `google_firestore_index "users_notification"` を apply 後、**インデックス構築に数分〜数十分**かかる。実行前に必ず確認:

```bash
gcloud firestore indexes composite list \
  --project=overseas-safety-map-test \
  --format='table(collectionId,fields.fieldPath,state)'
# state が READY であることを確認
```

### 2.3 テストデータの投入

Firestore `users` コレクションに少なくとも 2 件:

```bash
# Firestore Admin SDK / Console 経由、または gcloud alpha firestore import
# ユーザー A: JP + danger 受信、token 2 つ（1 つは valid、1 つは dead）
# ユーザー B: JP + 空 info_types（全種別）、token 1 つ
```

サンプル JSON:
```json
{
  "fcm_tokens": ["<real_token_A1>", "<dead_token_A2>"],
  "notification_preference": {
    "enabled": true,
    "target_country_cds": ["JP"],
    "info_types": ["danger"]
  }
}
```

### 2.4 ローカル環境（オプション）

ローカル実行を行う場合:

```bash
git checkout main && git pull --ff-only
make build-notifier            # bin/notifier を生成
gcloud auth application-default login  # ADC 認証
```

### 2.5 環境変数

`.env`（`.gitignore` 済み）:

```bash
# Platform 共通
PLATFORM_SERVICE_NAME=notifier
PLATFORM_ENV=dev
PLATFORM_GCP_PROJECT_ID=overseas-safety-map-test
PLATFORM_LOG_LEVEL=DEBUG
PLATFORM_OTEL_EXPORTER=stdout

# U-NTF 必須
NOTIFIER_PUBSUB_SUBSCRIPTION=notifier-safety-incident-new-arrival

# 任意（envconfig default が存在、上書き時のみ指定）
# NOTIFIER_DEDUP_TTL_HOURS=24
# NOTIFIER_FCM_CONCURRENCY=5
```

---

## 3. 実行手順

### 3.1 ローカル実行（単体疎通確認）

```bash
set -a; source .env; set +a
./bin/notifier 2>&1 | tee /tmp/notifier.log
```

別ターミナルから synthetic Pub/Sub envelope を POST して動作確認:

```bash
# NewArrivalEvent の JSON を base64 エンコード
INNER='{"key_cd":"test-001","country_cd":"JP","info_type":"danger","title":"テスト通知","lead":"これはテスト配信です","leave_date":"2026-04-23T12:00:00Z","geometry":{"lat":35.6,"lng":139.7}}'
BASE64_DATA=$(printf '%s' "$INNER" | base64)

# envelope POST
curl -sX POST http://localhost:8080/pubsub/push \
  -H "Content-Type: application/json" \
  -d "{\"message\":{\"data\":\"$BASE64_DATA\",\"attributes\":{\"key_cd\":\"test-001\"}},\"subscription\":\"local-test\"}" \
  -w "\nHTTP %{http_code}\n"
```

**期待されるログ**:

```json
{"level":"INFO","msg":"notifier starting","app.notifier.phase":"start","port":"8080",...}
{"level":"INFO","msg":"http server ready","app.notifier.phase":"ready","addr":":8080"}
{"level":"INFO","msg":"delivered","app.notifier.phase":"done","key_cd":"test-001","recipients":2,"fcm_success":2,"fcm_failed":1,"invalidated":1}
```

**確認項目**:

- [ ] HTTP 200 レスポンス
- [ ] ユーザー A が **danger** 種別を target にしてるので通知受信
- [ ] ユーザー B も受信（空 info_types = 全種別）
- [ ] `recipients=2`（2 ユーザー）、`fcm_success=2`（有効 token × 2）、`invalidated=1`（ユーザー A の dead token）
- [ ] Firestore `notifier_dedup/test-001` に `expireAt` + `createdAt` フィールド付きで doc 作成
- [ ] Firestore `users/{userA}.fcm_tokens` から `dead_token_A2` が削除されている
- [ ] iOS / Android 実機で通知が表示される（title + body）

### 3.2 Dedup 確認（2 回目実行）

```bash
curl -sX POST http://localhost:8080/pubsub/push \
  -H "Content-Type: application/json" \
  -d "{\"message\":{\"data\":\"$BASE64_DATA\"}}" \
  -w "\nHTTP %{http_code}\n"
```

**期待**:

- [ ] HTTP 200
- [ ] ログに `dedup hit` が出る
- [ ] `recipients=0`、FCM は叩かれない（DEBUG ログで `fcm.SendMulticast` span が無い）
- [ ] 実機に通知が **届かない**（重複通知ゼロ）

### 3.3 情報種別フィルタの確認

`info_type=spot` で push:

```bash
INNER='{"key_cd":"test-002","country_cd":"JP","info_type":"spot","title":"スポット情報",...}'
BASE64_DATA=$(printf '%s' "$INNER" | base64)
curl -sX POST http://localhost:8080/pubsub/push -H "Content-Type: application/json" \
  -d "{\"message\":{\"data\":\"$BASE64_DATA\"}}"
```

**期待**:

- [ ] HTTP 200
- [ ] ユーザー A (`info_types=["danger"]`) は **skip**、ユーザー B (空) は受信
- [ ] `recipients=1`、`fcm_success=1`

### 3.4 エラーケース

#### Malformed envelope → 400

```bash
curl -sX POST http://localhost:8080/pubsub/push \
  -H "Content-Type: application/json" \
  -d "not-json" \
  -w "\nHTTP %{http_code}\n"
```

- [ ] HTTP 400
- [ ] ログに `malformed push payload`

#### 大きすぎる body → 500（MaxBytesReader で制限）

```bash
# 2 MiB の dummy body
dd if=/dev/zero bs=1048576 count=2 2>/dev/null | base64 | \
  curl -sX POST http://localhost:8080/pubsub/push -H "Content-Type: application/json" --data-binary @- \
  -w "\nHTTP %{http_code}\n"
```

- [ ] HTTP 500
- [ ] ログに `read push body failed` + MaxBytesReader エラー

#### SIGTERM drain

notifier 実行中に別シェルで POST → すぐ Ctrl+C:

- [ ] 受信中の request は HTTP 200 で応答（drain される）
- [ ] ログに `shutdown signal received` + `notifier stopped cleanly`
- [ ] 10s を超えると `http server shutdown timed out` が WARN で出る

### 3.5 Production 反映手順

ローカル疎通確認 OK なら:

1. **実 Pub/Sub topic に publish**:
   ```bash
   # U-ING が動いていればそちらが publish する、またはマニュアル:
   gcloud pubsub topics publish safety-incident.new-arrival \
     --project=overseas-safety-map-test \
     --message="$INNER" \
     --attribute="key_cd=test-001,country_cd=JP,info_type=danger"
   ```
2. Cloud Logging (`resource.labels.service_name=notifier`) で `app.notifier.phase=done` + 各属性を確認
3. Cloud Monitoring で以下 Metric を確認:
   - `app.notifier.received`（count 増）
   - `app.notifier.duration`（p95 < 3s）
   - `app.notifier.fcm.sent{status=success}`
4. 実機 / エミュレータに通知が届くこと
5. DLQ が空であること（`gcloud pubsub topics list-subscriptions safety-incident.new-arrival.dlq`）

---

## 4. トラブルシューティング

### 4.1 Firestore `FAILED_PRECONDITION: The query requires an index`

**症状**: `users.FindSubscribers` でこのエラー。

**原因と対処**:
- Terraform apply 後 index 構築がまだ完了していない → 数分〜数十分待つ、Console で state=READY を確認
- Index 定義が間違っている → `modules/shared/firestore.tf` の `google_firestore_index "users_notification"` の field path を確認

### 4.2 FCM `senderid-mismatch` / `invalid-argument`

**症状**: 全 token が `Invalid` 扱いされ除去される。

**原因と対処**:
- Firebase プロジェクトと Cloud Run Runtime SA が別プロジェクトに属している
- アプリの `GoogleService-Info.plist` / `google-services.json` と Firebase プロジェクトが不一致
- テスト用 token を実本番 Firebase で取得した場合など

### 4.3 Pub/Sub push が届かない

**症状**: Pub/Sub に publish してもnotifier ログに `received` が出ない。

**原因と対処**:
- Cloud Run Service の URL と subscription の `push_endpoint` が一致しているか確認
- Pub/Sub service agent (`service-{project_number}@gcp-sa-pubsub.iam.gserviceaccount.com`) に `iam.serviceAccountTokenCreator` on runtime SA が付与されているか（Terraform で設定済み）
- runtime SA に `roles/run.invoker` が付与されているか（OIDC token subject）

### 4.4 Dedup が効かない

**症状**: 同じ `key_cd` を再 push したのに毎回 FCM 配信される。

**原因と対処**:
- Firestore TTL policy (`notifier_dedup.expireAt`) が設定されていない → `gcloud firestore fields describe expireAt --collection=notifier_dedup` で確認
- notifier が dedup チェックを skip している → ログで `dedup` phase を確認、RunTransaction が error で dedup 機能していない可能性

### 4.5 Firebase 認証エラー

**症状**: `firebasex.new_app` で `google: could not find default credentials`

**原因と対処**:
- ローカル: `gcloud auth application-default login` を実行
- Cloud Run: Runtime SA（`notifier-runtime`）に `roles/firebase.admin` 相当の権限があるか確認

---

## 5. 観測ポイント

運用時に必ず見る Metric / ログ:

| 観測対象 | 見る場所 | 期待値 |
|---|---|---|
| `app.notifier.received` | Cloud Monitoring | Pub/Sub publish 数とほぼ一致 |
| `app.notifier.deduped` | Cloud Monitoring | 新着率次第、0〜10% 程度 |
| `app.notifier.duration` (p95) | Histogram | **< 3s**（NFR-NTF-PERF-01） |
| `app.notifier.fcm.sent{status=failure}` | Counter | 全体の < 5%（一時的なネットワーク障害） |
| `app.notifier.fcm.token_invalidated` | Counter | 月数件〜数十件（アプリ再インストールに比例） |
| HTTP 5xx 率 | Cloud Monitoring | < 1%（Pub/Sub retry で吸収される） |
| DLQ 残件数 | Pub/Sub | **常に 0 であるのが健全** |

---

## 6. 実行記録

> 実パイプラインで実行する都度、ここに追記する。

### 6.1 [日付未定] ローカル初回実行（test workspace + test Firebase）

**実行者**: TBD
**Firebase project**: TBD
**Pub/Sub envelope 構造の答え合わせ結果**: TBD（Pub/Sub push envelope の実際の JSON 形状）
**Dedup 動作確認**: TBD（2 回目 push で 200 OK + no FCM）
**Info types filter 動作確認**: TBD
**無効 token 除去確認**: TBD（Firestore `users.fcm_tokens` から ArrayRemove）
**実機通知受信確認**: TBD

### 6.2 [日付未定] Production 初回接続（U-ING → U-NTF 疎通）

**実行者**: TBD
**U-ING publish 数 / 時間**: TBD
**U-NTF received 数**: TBD
**配信成功率**: TBD
**DLQ 到達件数**: TBD

### 6.3 [日付未定] Production 継続運用開始

**実行者**: TBD
**一週間の `fcm.sent{status=success}` 合計**: TBD
**Dedup 率 (`deduped / received`)**: TBD
**Token 無効化率**: TBD

---

## 7. 関連ドキュメント

- [`U-NTF/design/U-NTF-design.md`](../design/U-NTF-design.md) — Functional + NFR Req + NFR Design 合本
- [`U-NTF/infrastructure-design/`](../infrastructure-design/) — Cloud Run Service / Subscription / IAM / Firestore 設計
- [`U-NTF/code/summary.md`](../code/summary.md) — Code Generation 成果物一覧 + U-APP 申し送り
- [`U-ING/build-and-test/runbook.md`](../../U-ING/build-and-test/runbook.md) — U-ING の runbook（本 Unit の前段）
- [`construction/shared-infrastructure.md`](../../shared-infrastructure.md) — Firebase プロジェクト設定 / Firestore database / IAM bootstrap
