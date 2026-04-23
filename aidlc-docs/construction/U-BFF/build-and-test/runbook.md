# U-BFF Build and Test — Runbook

**Status**: 🟡 **Template only** — 実 reearth-cms / Firebase Auth / Firestore 疎通は未実施。Flutter アプリ（別レポ `overseas-safety-map-app`）で Firebase Anonymous Auth の ID Token が発行可能になり、かつ U-CSS と U-ING が稼働している環境で、本ランブックに沿って実行し、結果を §6 に記録する。

**前提**:
- U-BFF Code Generation PR A (#49) と PR B (#50) が main に取り込まれていること
- U-PLT shared infrastructure が `terraform apply` 済み（Artifact Registry / Secret Manager / Firestore database / BFF Cloud Run Service / IAM）
- **U-CSS** が適用済み（`cmd/cmsmigrate` で reearth-cms 側の Project / Model / Field が揃っている）
- **U-ING の Build and Test** で CMS に少なくとも 1 件の `safety-incident` item が書き込み済み（読み取り対象として必要）
- Firebase プロジェクトが有効化済み、Anonymous Auth が有効になっている
- reearth-cms の Integration Token が Secret Manager `cms-integration-token` に入っている

---

## 1. 目的

`cmd/bff` が **Flutter → AuthInterceptor → UseCase → CMSReader / FirestoreProfileRepo → proto 応答** のパイプラインを正しく動かすことを確認する:

1. Firebase ID Token 検証が `AuthInterceptor` で正しく動作する（valid / missing / expired / invalid）
2. `/healthz` と `/readyz` が Cloud Run の liveness / readiness probe に応答する
3. **`SafetyIncidentService`** 5 RPC が CMS の実データを返す（List / Get / Search / ListNearby / GeoJSON）
4. `CrimeMapService.GetChoropleth` が in-memory 集計 + 5 段 Reds palette で colour を付ける
5. `CrimeMapService.GetHeatmap` が **country centroid fallback** を除外する
6. **`UserProfileService`** 4 RPC が Firestore `users/{uid}` に idempotent に書き込む（ArrayUnion / ArrayRemove）
7. 初回 `GetProfile` 時に empty profile が **lazy create** される
8. `ErrorInterceptor` が `errs.Kind` を `connect.Code` に変換、prod では Internal / Unavailable をマスク
9. CMS の next-page cursor が opaque に pass-through される（List/Search ページング）
10. `ListNearby` の Haversine 距離フィルタが正しい（500 km 内の件数を返す）
11. SIGTERM で graceful shutdown が in-flight RPC を drain

---

## 2. 事前準備（実行者が用意するもの）

### 2.1 Firebase / GCP / CMS セットアップ

| 項目 | 取得方法 |
|---|---|
| Firebase プロジェクト | GCP `overseas-safety-map-test` に対して Firebase を有効化し、**Anonymous Authentication** をオンに |
| Firebase Admin SDK 認証 | **ADC** を使用（Cloud Run Runtime SA または `gcloud auth application-default login`） |
| Flutter アプリ（任意） | U-APP 実装まで、ID Token は Firebase CLI / Firebase REST API / テスト用 Web ページで取得可 |
| reearth-cms | U-CSS / U-ING で schema + データが揃っている workspace |
| CMS Integration Token | Secret Manager `cms-integration-token` に投入済 |

> ⚠️ **本番 CMS / 本番 Firebase プロジェクトで実行しない**。テスト専用プロジェクトを用意する。

### 2.2 Firebase ID Token の取得

Flutter アプリがまだ無い前提で ID Token をコマンドラインで発行する一例:

```bash
# Firebase REST API で匿名ユーザを作成
FIREBASE_API_KEY="<Firebase プロジェクトの Web API key>"
RESP=$(curl -sX POST \
  "https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=${FIREBASE_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"returnSecureToken":true}')

ID_TOKEN=$(echo "$RESP" | jq -r .idToken)
UID=$(echo "$RESP" | jq -r .localId)
echo "uid=$UID id_token=<取得成功>"
```

ID Token の有効期限は 1 時間。テスト中に切れたら再度発行する。

### 2.3 テストデータの投入

**CMS**: U-ING で少なくとも数十件の `safety-incident` item が書き込まれていること（複数国が含まれると Choropleth が検証しやすい）。

**Firestore `users/{uid}`**: 空で OK — 初回 `GetProfile` で自動作成される。

### 2.4 ローカル環境（オプション）

```bash
git checkout main && git pull --ff-only
make build-bff                       # bin/bff を生成
gcloud auth application-default login  # ADC 認証
```

### 2.5 環境変数

`.env`（`.gitignore` 済み）:

```bash
# Platform 共通
PLATFORM_SERVICE_NAME=bff
PLATFORM_ENV=dev
PLATFORM_GCP_PROJECT_ID=overseas-safety-map-test
PLATFORM_LOG_LEVEL=DEBUG
PLATFORM_OTEL_EXPORTER=stdout

# U-BFF 必須
BFF_CMS_BASE_URL=https://cms.example.com
BFF_CMS_WORKSPACE_ID=<workspace alias or id>
BFF_CMS_INTEGRATION_TOKEN=<Secret Manager 値を直接貼る、または secret-cli で展開>

# 任意（envconfig default で吸収、上書き時のみ指定）
# BFF_PORT=8080
# BFF_CMS_PROJECT_ALIAS=overseas-safety-map
# BFF_CMS_MODEL_ALIAS=safety-incident
# BFF_USERS_COLLECTION=users
# BFF_SHUTDOWN_GRACE_SECONDS=10
```

---

## 3. 実行手順

### 3.1 ローカル実行（起動確認）

```bash
set -a; source .env; set +a
./bin/bff 2>&1 | tee /tmp/bff.log
```

**期待されるログ**:

```json
{"level":"INFO","msg":"bff starting","app.bff.phase":"start","port":8080}
{"level":"INFO","msg":"bff ready","app.bff.phase":"ready","cms.project.id":"...","cms.model.id":"..."}
```

**確認項目**:
- [ ] `cms.project.id` / `cms.model.id` が stdout に載っている（startup で CMS Model 解決成功）
- [ ] `/healthz` が HTTP 200 を返す
- [ ] `/readyz` が HTTP 200 を返し JSON body の `status: "ready"` と probers リストが載っている

### 3.2 AuthInterceptor 確認

```bash
# 3.2.1 missing Bearer → Unauthenticated
curl -sX POST http://localhost:8080/overseasmap.v1.UserProfileService/GetProfile \
  -H "Content-Type: application/json" -d '{}' -w "\nHTTP %{http_code}\n"

# 3.2.2 invalid Bearer → Unauthenticated
curl -sX POST http://localhost:8080/overseasmap.v1.UserProfileService/GetProfile \
  -H "Authorization: Bearer not-a-real-token" \
  -H "Content-Type: application/json" -d '{}' -w "\nHTTP %{http_code}\n"

# 3.2.3 valid Bearer → 200
curl -sX POST http://localhost:8080/overseasmap.v1.UserProfileService/GetProfile \
  -H "Authorization: Bearer ${ID_TOKEN}" \
  -H "Content-Type: application/json" -d '{}' -w "\nHTTP %{http_code}\n"
```

**確認項目**:
- [ ] 3.2.1 は HTTP 401 相当（`{"code":"unauthenticated","message":"..."}`）
- [ ] 3.2.2 も HTTP 401 相当、Firebase 側のエラー文字列がログに出る（prod なら mask）
- [ ] 3.2.3 は HTTP 200 でレスポンス body にプロファイル JSON

### 3.3 GetProfile の lazy create 確認

3.2.3 のあと:

```bash
# Firestore に直接 doc があるか確認
gcloud firestore export --collection-ids=users \
  --project=overseas-safety-map-test gs://<bucket>/users-dump
# または Console の Firestore → users → {uid} を確認
```

**確認項目**:
- [ ] `users/{uid}` が存在する（initial fields: `fcm_tokens=[]`, `favorite_country_cds=[]`, `notification_preference.enabled=false`）
- [ ] 2 回目の `GetProfile` で同じ doc が返る（新規作成されない）

### 3.4 UserProfileService の残り 3 RPC

```bash
# ToggleFavoriteCountry (JP を追加)
curl -sX POST http://localhost:8080/overseasmap.v1.UserProfileService/ToggleFavoriteCountry \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d '{"countryCd":"JP"}'

# UpdateNotificationPreference (enabled + JP + danger)
curl -sX POST http://localhost:8080/overseasmap.v1.UserProfileService/UpdateNotificationPreference \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d '{"preference":{"enabled":true,"targetCountryCds":["JP"],"infoTypes":["danger"]}}'

# RegisterFcmToken
curl -sX POST http://localhost:8080/overseasmap.v1.UserProfileService/RegisterFcmToken \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d '{"token":"fake_fcm_token_for_test","deviceId":"local-dev"}'
```

**確認項目**:
- [ ] Firestore `users/{uid}` の `favorite_country_cds` に `JP` が入る（2 回呼ぶと消える = Toggle）
- [ ] `notification_preference.enabled=true` + `target_country_cds=["JP"]` + `info_types=["danger"]`
- [ ] `fcm_tokens` に `fake_fcm_token_for_test` が追加（同じ token を 2 回登録しても重複しない = ArrayUnion 冪等）

### 3.5 SafetyIncidentService 確認

```bash
# List (国 = JP、上位 5 件)
curl -sX POST http://localhost:8080/overseasmap.v1.SafetyIncidentService/ListSafetyIncidents \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d '{"filter":{"countryCd":"JP","limit":5}}'

# Get (既知の key_cd)
KNOWN_KEY_CD="<U-ING が書いた任意の key_cd>"
curl -sX POST http://localhost:8080/overseasmap.v1.SafetyIncidentService/GetSafetyIncident \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d "{\"keyCd\":\"${KNOWN_KEY_CD}\"}"

# Search (keyword = "earthquake")
curl -sX POST http://localhost:8080/overseasmap.v1.SafetyIncidentService/SearchSafetyIncidents \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d '{"filter":{"limit":5},"query":"earthquake"}'

# ListNearby (東京駅 500 km)
curl -sX POST http://localhost:8080/overseasmap.v1.SafetyIncidentService/ListNearby \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d '{"center":{"lat":35.68,"lng":139.76},"radiusKm":500,"limit":10}'

# GeoJSON
curl -sX POST http://localhost:8080/overseasmap.v1.SafetyIncidentService/GetSafetyIncidentsAsGeoJSON \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d '{"filter":{"limit":50}}' | jq '.geojson | @base64d | fromjson'
```

**確認項目**:
- [ ] List: items の `geocodeSource` が `GEOCODE_SOURCE_MAPBOX` か `COUNTRY_CENTROID`
- [ ] List レスポンスの `nextCursor` が CMS から返った値と一致（opaque pass-through）
- [ ] Get: 存在する key_cd は 200、存在しない key_cd は **CodeNotFound** (`"code":"not_found"`)
- [ ] Search: keyword 検索のヒット数が妥当
- [ ] ListNearby: Tokyo 付近の item だけ返る、件数が `limit` 以下
- [ ] GeoJSON: `type=FeatureCollection`、`features[].geometry.coordinates` は `[lng, lat]` 順

### 3.6 CrimeMapService 確認

```bash
# Choropleth
curl -sX POST http://localhost:8080/overseasmap.v1.CrimeMapService/GetChoropleth \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d '{"filter":{"leaveFrom":"2026-01-01T00:00:00Z","leaveTo":"2026-12-31T23:59:59Z"}}'

# Heatmap
curl -sX POST http://localhost:8080/overseasmap.v1.CrimeMapService/GetHeatmap \
  -H "Authorization: Bearer ${ID_TOKEN}" -H "Content-Type: application/json" \
  -d '{"filter":{}}'
```

**確認項目**:
- [ ] Choropleth: 国ごとに 1 entry、`count` が reasonable、`color` が `#rrggbb`
- [ ] 最も `count` が大きい国は `color=#a50f15`（palette 最上位）
- [ ] Heatmap: `points[]` に Mapbox で geocode された点のみ載る、country centroid fallback は `excludedFallback` にカウントされる

### 3.7 ErrorInterceptor 確認（prod mask）

`PLATFORM_ENV=prod` で再起動してから invalid token を送る:

```bash
PLATFORM_ENV=prod ./bin/bff &
curl -sX POST http://localhost:8080/overseasmap.v1.UserProfileService/GetProfile \
  -H "Authorization: Bearer bad" -H "Content-Type: application/json" -d '{}'
```

**確認項目**:
- [ ] message が `"internal server error"` (Internal マスク) ではなく元の Firebase エラーメッセージで露出していない（Unauthenticated はマスク対象外なのでそのまま）
- [ ] 無理やり Internal を起こすには CMS を落とすなど（§4.3 参照）

### 3.8 SIGTERM drain

bff 実行中に別シェルで進行中の RPC を送りながら Ctrl+C:

- [ ] 受信中の RPC は HTTP 200 で応答（drain される）
- [ ] ログに `bff stopped cleanly` が出る
- [ ] `BFF_SHUTDOWN_GRACE_SECONDS` (default 10) を超えると `connectserver` が WARN を出してから落ちる

### 3.9 Production 反映手順

ローカル疎通確認 OK なら:

1. `git tag -a u-bff-v1 -m "initial bff release" && git push --tags`
2. Cloud Build / GitHub Actions が Artifact Registry に image push
3. `terraform -chdir=terraform/environments/prod apply -var bff_image_tag=<new-sha>`
4. Cloud Run Service URL を取得:
   ```bash
   gcloud run services describe bff --region=asia-northeast1 --format='value(status.url)'
   ```
5. 上記 URL に対して 3.2-3.8 を再実行
6. Cloud Logging (`resource.labels.service_name=bff`) で `app.bff.phase=ready` を確認
7. Flutter アプリ（U-APP）の `BFF_BASE_URL` を設定

---

## 4. トラブルシューティング

### 4.1 `CMS project alias "..." not found — deploy U-CSS first`

**症状**: 起動時に `bff.startup` エラー。

**原因と対処**:
- U-CSS（`cmd/cmsmigrate`）を実行して schema 投入していない → `bin/cmsmigrate` を先に流す
- `BFF_CMS_PROJECT_ALIAS` / `BFF_CMS_MODEL_ALIAS` が env と一致していない

### 4.2 `firebasex.new_app` / `firebasex.auth` の external error

**症状**: 起動時または初回 RPC で `could not find default credentials` 等。

**原因と対処**:
- ローカル: `gcloud auth application-default login` を実行
- Cloud Run: Runtime SA（`bff-runtime`）に `roles/datastore.user` が付与されているか確認
- Firebase プロジェクトと GCP プロジェクトの `PLATFORM_GCP_PROJECT_ID` が不一致

### 4.3 CMS 側 5xx で `SafetyIncidentService` が CodeUnavailable を返す

**症状**: RPC が `{"code":"unavailable"}` を返す。

**原因と対処**:
- CMS 側の障害 → Status Dashboard で確認
- Integration Token 期限切れ → Secret Manager を更新して `terraform apply` で Cloud Run revision 差し替え
- retry policy は `cmsx` 側に実装済み（exponential backoff）なので、一時的なら自動復帰する

### 4.4 `GetProfile` が CodeNotFound を返してしまう

**症状**: 初回の `GetProfile` で lazy create が動かず、404 が返る。

**原因と対処**:
- Firestore Rules がブロックしている（runtime SA に `roles/datastore.user` 付与を再確認）
- `UsersCollection` env が間違っている（default は `users`）

### 4.5 `ListNearby` が 0 件を返す

**症状**: 中心から半径 500 km を指定しても 0 件。

**原因と対処**:
- CMS に `geometry` を持つ item が不足 → U-ING を走らせて geocode 済みの item を追加
- U-ING の Geocoder が country centroid fallback しか返していない → centroid は Haversine 距離的には国の中心に集まるので半径がかなり小さいと漏れる。`500` km を `2000` km に広げて再確認

### 4.6 `UpdateNotificationPreference` が CodeNotFound

**症状**: 直前に `GetProfile` していれば doc は存在するはずなのに NotFound。

**原因と対処**:
- `GetProfile` を経由せずに `UpdateNotificationPreference` を直叩きしている → 各 write UseCase は `CreateIfMissing` を呼ぶので lazy create は動く、`GetProfile` 不要。それでも NotFound なら Firestore Rules / IAM を確認

---

## 5. 観測ポイント

運用時に必ず見る Metric / ログ:

| 観測対象 | 見る場所 | 期待値 |
|---|---|---|
| `app.bff.request_duration` (p95) | Cloud Monitoring Histogram | **< 500ms**（NFR-BFF-PERF-01）、`GetChoropleth` は < 1s |
| `app.bff.phase` | Cloud Logging | `ready` / `auth` / `cms_read` / `firestore` / `aggregate` / `response` の分布 |
| HTTP status 5xx 率 | Cloud Monitoring | < 1% |
| `app.bff.auth_failure` | Counter | 急増時は Flutter 側の Firebase 設定不整合 |
| `app.bff.cms.request_count{status=5xx}` | Counter | < 1% |
| `/readyz` probers | Cloud Run Revision logs | 全 prober が OK |
| Cloud Run instance count | Cloud Monitoring | 平常時 0、日中 1-2 |

---

## 6. 実行記録

> 実パイプラインで実行する都度、ここに追記する。

### 6.1 [日付未定] ローカル初回実行（test project + test Firebase）

**実行者**: TBD
**Firebase project**: TBD
**CMS workspace / model**: TBD
**AuthInterceptor 動作確認**: TBD（valid / missing / invalid token それぞれのコード）
**GetProfile lazy create 動作確認**: TBD（Firestore に doc が新規作成されたこと）
**SafetyIncidentService 5 RPC 応答確認**: TBD
**CrimeMapService choropleth / heatmap 応答確認**: TBD
**ErrorInterceptor prod mask 動作確認**: TBD
**cursor pass-through 確認**: TBD

### 6.2 [日付未定] Production 初回接続（Flutter → BFF → CMS / Firestore 疎通）

**実行者**: TBD
**Cloud Run URL**: TBD
**Flutter build**: TBD（debug / release）
**初回 GetProfile レスポンス時間 (cold start)**: TBD
**2 回目 GetProfile レスポンス時間 (warm)**: TBD
**Choropleth レスポンス時間 (warm)**: TBD
**観測ダッシュボード確認**: TBD

### 6.3 [日付未定] Production 継続運用開始

**実行者**: TBD
**一週間の RPC 数 / RPC**: TBD
**平均 p95 request_duration**: TBD
**auth_failure 発生頻度**: TBD
**instance_count 分布**: TBD（min=0 の妥当性確認）

---

## 7. 関連ドキュメント

- [`U-BFF/design/U-BFF-design.md`](../design/U-BFF-design.md) — Functional + NFR Req + NFR Design 合本
- [`U-BFF/infrastructure-design/`](../infrastructure-design/) — Cloud Run / IAM / Firestore 現状維持の根拠
- [`U-BFF/code/summary.md`](../code/summary.md) — Code Generation 成果物一覧
- [`U-CSS/build-and-test/runbook.md`](../../U-CSS/build-and-test/runbook.md) — CMS schema が前提
- [`U-ING/build-and-test/runbook.md`](../../U-ING/build-and-test/runbook.md) — CMS データ流し込みが前提
- [`construction/shared-infrastructure.md`](../../shared-infrastructure.md) — Firebase / Firestore / IAM bootstrap
