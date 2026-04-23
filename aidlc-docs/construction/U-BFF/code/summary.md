# U-BFF Code Generation — Summary

**Unit**: U-BFF（Connect Server / BFF Unit、Sprint 3、**実装順で最後のバックエンド Unit**）
**対象**: `cmd/bff`（Cloud Run Service、Flutter クライアント向け Connect RPC）
**対応する計画**: [`U-BFF-code-generation-plan.md`](../../plans/U-BFF-code-generation-plan.md)
**上位設計**: [`U-BFF/design/U-BFF-design.md`](../design/U-BFF-design.md)、[`U-BFF/infrastructure-design/`](../infrastructure-design/)

---

## 1. 生成ファイル一覧

### Proto / 生成物（`gen/go/v1/`、Phase 1）

| ファイル | 役割 |
|---|---|
| `common.pb.go` / `pubsub.pb.go` / `safetymap.pb.go` | 3 Service / 11 RPC の proto メッセージ |
| `overseasmapv1connect/safetymap.connect.go` | Connect handler constructors |

### Domain（Phase 2）

| パス | 役割 |
|---|---|
| `internal/safetyincident/domain/read_ports.go` | `SafetyIncidentReader` + `ListFilter` + `SearchFilter`（`List`/`Search` は `(items, nextCursor, err)`、`ListNearby` は top-N） |
| `internal/safetyincident/crimemap/domain/` | `CountryChoropleth` / `HeatmapPoint` / `Filter` / `Result` + `ColorFromCount`（5-stop Reds palette） |
| `internal/user/domain/profile.go` | `UserProfile` / `NotificationPreference` / `EmptyProfile` |
| `internal/user/domain/ports.go` | `ProfileRepository` + `AuthVerifier` |
| `internal/shared/authctx/context.go` | `WithUID` / `UIDFrom`（nil-safe、unexported key 型） |

### Application（Phase 3）

| パス | 役割 |
|---|---|
| `internal/safetyincident/application/{list,get,search,nearby,geojson}_usecase.go` | 5 read UseCase（reader の薄いラッパ、cursor 伝播） |
| `internal/safetyincident/crimemap/application/aggregator.go` | Choropleth + Heatmap（in-memory 集計、centroid fallback 除外） |
| `internal/user/application/{get_profile,toggle_favorite,update_preference,register_fcm_token}.go` | 4 user UseCase（全て idempotent、`GetProfile` は 404 で lazy create） |

### Infrastructure（Phase 4）

| パッケージ | 役割 |
|---|---|
| `internal/platform/cmsx/item_read.go` | `ListItems` / `SearchItems`（filter + cursor + limit + infoTypes、opaque cursor passthrough） |
| `internal/platform/firebasex/app.go`（拡張） | `Auth()` accessor（sync.Once、既存 Firestore / Messaging と同じパターン） |
| `internal/safetyincident/infrastructure/cms/reader.go` | `CMSReader`（List/Get/Search/ListNearby）。`ListNearby` は Haversine 距離フィルタ（Q D [A]） |
| `internal/safetyincident/infrastructure/cms/fromfields.go` | `ItemDTO.Fields → domain.SafetyIncident` 変換（toFields の逆、19 フィールド） |
| `internal/user/infrastructure/firestore/profile_repo.go` | `FirestoreProfileRepository`（document-id 直接アクセス、ArrayUnion/ArrayRemove） |
| `internal/user/infrastructure/firebaseauth/verifier.go` | `FirebaseAuthVerifier`（`VerifyIDToken` のみ、Q C [A]） |

### Interfaces（Phase 5）

| ファイル | 役割 |
|---|---|
| `internal/interfaces/rpc/auth_interceptor.go` | `NewAuthInterceptor` — Bearer 検証 + `authctx.WithUID` で uid 注入 |
| `internal/interfaces/rpc/error_interceptor.go` | `NewErrorInterceptor` — `errs.Kind` → `connect.Code` 自動変換、prod メッセージマスク |
| `internal/interfaces/rpc/conversions.go` | proto ⇄ domain 変換ヘルパ |
| `internal/interfaces/rpc/safety_incident_server.go` | SafetyIncidentService の 5 RPC |
| `internal/interfaces/rpc/crimemap_server.go` | CrimeMapService の 2 RPC |
| `internal/interfaces/rpc/user_profile_server.go` | UserProfileService の 4 RPC（`authctx.UIDFrom` で uid 取得） |

### Composition Root（Phase 6）

| ファイル | 役割 |
|---|---|
| `cmd/bff/main.go` | `run()` pattern + defer graceful shutdown。`cmsx.Client` + `firebasex.App` + Reader/Repo/Verifier + UseCase + Server の配線、Interceptor Chain（Recover → Error → Auth） |

### Terraform（Phase 7、変更なし）

U-BFF Infrastructure Design の結論どおり **Terraform コード変更ゼロ**。U-PLT で `modules/bff/` が、U-NTF で `modules/shared/firestore.tf` が既に整備済み。

---

## 2. env 一覧（`cmd/bff/main.go` の bffConfig）

| env | 供給元 | default | 用途 |
|---|---|---|---|
| `BFF_PORT` | Terraform | `8080` | HTTP listen port |
| `BFF_CMS_BASE_URL` | Terraform | — | reearth-cms URL |
| `BFF_CMS_WORKSPACE_ID` | Terraform | — | CMS workspace id |
| `BFF_CMS_INTEGRATION_TOKEN` | Terraform (secret_key_ref) | — | CMS Integration Token |
| `BFF_CMS_PROJECT_ALIAS` | envconfig default | `overseas-safety-map` | CMS project alias |
| `BFF_CMS_MODEL_ALIAS` | envconfig default | `safety-incident` | CMS model alias |
| `BFF_CMS_KEY_FIELD` | envconfig default | `key_cd` | unique 識別 field |
| `BFF_USERS_COLLECTION` | envconfig default | `users` | Firestore collection |
| `BFF_SHUTDOWN_GRACE_SECONDS` | envconfig default | `10` | graceful shutdown timeout |
| `PLATFORM_*` | Terraform | — | U-PLT 共通設定（ServiceName / Env / GCPProjectID / OTelExporter / LogLevel） |

---

## 3. テストカバレッジ

| 層 | 目標 | 実測 | 備考 |
|---|---|---|---|
| `safetyincident/domain` | 95% | 既存（U-ING） | 既存 struct のみ利用 |
| `safetyincident/crimemap/domain` | 95% | 100% | `color.go` 含め PBT 済み |
| `user/domain` | 95% | 100% | `EmptyProfile` |
| `shared/authctx` | 95% | 100% | nil-ctx / empty uid / WithUID-after-WithUID |
| `safetyincident/application` | 90% | 92.1% | 5 UseCase + fake reader |
| `safetyincident/crimemap/application` | 90% | 100% | Aggregator（choropleth/heatmap） |
| `user/application` | 90% | 86.7% | 4 UseCase + in-memory fake |
| `platform/cmsx` | 70% | 既存＋新規 80%+ | httptest.Server で ListItems / SearchItems |
| `safetyincident/infrastructure/cms` | 70% | 80.1% | stub ReaderClient |
| `user/infrastructure/firebaseauth` | 70% | 100% | stub TokenVerifier |
| `user/infrastructure/firestore` | 70% | 4.5%（コンストラクタのみ） | live SDK 依存、Build and Test で実機疎通（U-NTF userrepo と同方針） |
| `interfaces/rpc` | 80% | 82.9% | `httptest.Server` + Connect client で e2e |

---

## 4. 起動方法（ローカル）

```sh
export PLATFORM_SERVICE_NAME=bff
export PLATFORM_ENV=dev
export PLATFORM_GCP_PROJECT_ID=<your-gcp-project>
export PLATFORM_OTEL_EXPORTER=none
export PLATFORM_LOG_LEVEL=debug
export BFF_PORT=8080
export BFF_CMS_BASE_URL=https://cms.example.com
export BFF_CMS_WORKSPACE_ID=<workspace>
export BFF_CMS_INTEGRATION_TOKEN=<token>
# Firebase: gcloud auth application-default login が事前に必要
go run ./cmd/bff
```

---

## 5. 疎通確認（Build and Test で実施）

```sh
# 1) Firebase Anonymous Auth で ID Token を取得（Flutter Debug ビルド or REST API）
ID_TOKEN="$(firebase auth:sign-in-anonymous --json | jq -r .idToken)"

# 2) Connect RPC を curl
curl -H "Authorization: Bearer ${ID_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{}' \
     http://localhost:8080/overseasmap.v1.UserProfileService/GetProfile
```

期待レスポンス: `{"profile": {"uid": "...", "fcmTokenCount": 0}}`（初回は lazy create）。
