# U-PLT Business Logic Model (Minimal)

U-PLT はビジネスロジックを持たない基盤 Unit のため、本ドキュメントは **4 つの契約／スキーマ** を宣言的に記述する。これらは他の Unit が依存する共通語彙となる。

---

## 1. Proto スキーマ（Connect + Pub/Sub）

### 1.1 方針（Q1 [A]）

- **ファイル配置**: `proto/v1/`（Go と Dart 双方の生成ソース）
- **命名規則**: `snake_case`（proto 標準）
- **必須表現**: `optional` を付けず、デフォルト値（空文字 / 0 / 空配列）で未設定を表現
- **ID 類**: `string`
- **日時**: `google.protobuf.Timestamp`（UTC）
- **座標**: Connect 側は独自 `Point { double lat = 1; double lng = 2; }`（WGS84）。GeoJSON 互換は BFF 応答時に変換する（REST エイリアスが必要になったら対応）
- **列挙**: `enum` の 0 値は必ず `_UNSPECIFIED`（例: `INFO_TYPE_UNSPECIFIED = 0`）
- **パッケージ**: `overseasmap.v1`
- **GoPackage option**: `option go_package = "github.com/soneda-yuya/overseas-safety-map/gen/go/v1;overseasmapv1";`

### 1.2 `proto/v1/common.proto`（共通型）

```proto
syntax = "proto3";
package overseasmap.v1;
option go_package = "github.com/soneda-yuya/overseas-safety-map/gen/go/v1;overseasmapv1";

import "google/protobuf/timestamp.proto";

message Point {
  double lat = 1;  // WGS84 latitude
  double lng = 2;  // WGS84 longitude
}

enum GeocodeSource {
  GEOCODE_SOURCE_UNSPECIFIED = 0;
  GEOCODE_SOURCE_MAPBOX = 1;
  GEOCODE_SOURCE_COUNTRY_CENTROID = 2;
}

// U-PLT では InfoType / CountryCode / AreaCode は string で扱う（MOFA コード表そのまま）。
// enum に閉じ込めない理由: MOFA 側でコードが増えた際にアプリを止めないため。
```

### 1.3 `proto/v1/safetymap.proto`（Connect サービス）

```proto
syntax = "proto3";
package overseasmap.v1;
option go_package = "github.com/soneda-yuya/overseas-safety-map/gen/go/v1;overseasmapv1";

import "google/protobuf/timestamp.proto";
import "v1/common.proto";

// ---- SafetyIncident ----
message SafetyIncident {
  string key_cd = 1;
  string info_type = 2;
  string info_name = 3;
  google.protobuf.Timestamp leave_date = 4;
  string title = 5;
  string lead = 6;
  string main_text = 7;
  string info_url = 8;
  string koukan_cd = 9;
  string koukan_name = 10;
  string area_cd = 11;
  string area_name = 12;
  string country_cd = 13;
  string country_name = 14;
  string extracted_location = 15;
  Point geometry = 16;
  GeocodeSource geocode_source = 17;
  google.protobuf.Timestamp ingested_at = 18;
  google.protobuf.Timestamp updated_at = 19;
}

message SafetyIncidentFilter {
  string area_cd = 1;
  string country_cd = 2;
  repeated string info_types = 3;
  google.protobuf.Timestamp leave_from = 4;
  google.protobuf.Timestamp leave_to = 5;
  int32 limit = 6;
  string cursor = 7;
}

message ListSafetyIncidentsRequest {
  SafetyIncidentFilter filter = 1;
}
message ListSafetyIncidentsResponse {
  repeated SafetyIncident items = 1;
  string next_cursor = 2;
  int32 total_hint = 3;
}

message GetSafetyIncidentRequest { string key_cd = 1; }
message GetSafetyIncidentResponse { SafetyIncident item = 1; }

message SearchSafetyIncidentsRequest { SafetyIncidentFilter filter = 1; }
message SearchSafetyIncidentsResponse {
  repeated SafetyIncident items = 1;
  string next_cursor = 2;
}

message ListNearbyRequest {
  Point center = 1;
  double radius_km = 2;
  int32 limit = 3;
}
message ListNearbyResponse { repeated SafetyIncident items = 1; }

message GetSafetyIncidentsAsGeoJSONRequest { SafetyIncidentFilter filter = 1; }
message GetSafetyIncidentsAsGeoJSONResponse { bytes geojson = 1; }

service SafetyIncidentService {
  rpc ListSafetyIncidents(ListSafetyIncidentsRequest) returns (ListSafetyIncidentsResponse);
  rpc GetSafetyIncident(GetSafetyIncidentRequest) returns (GetSafetyIncidentResponse);
  rpc SearchSafetyIncidents(SearchSafetyIncidentsRequest) returns (SearchSafetyIncidentsResponse);
  rpc ListNearby(ListNearbyRequest) returns (ListNearbyResponse);
  rpc GetSafetyIncidentsAsGeoJSON(GetSafetyIncidentsAsGeoJSONRequest) returns (GetSafetyIncidentsAsGeoJSONResponse);
}

// ---- CrimeMap ----
message CrimeMapFilter {
  google.protobuf.Timestamp leave_from = 1;
  google.protobuf.Timestamp leave_to = 2;
}

message CountryChoropleth {
  string country_cd = 1;
  string country_name = 2;
  int32 count = 3;
  string color = 4;  // 色スケールのヒント（サーバ側で計算）
}

message HeatmapPoint {
  Point location = 1;
  double weight = 2;
}

message GetChoroplethRequest { CrimeMapFilter filter = 1; }
message GetChoroplethResponse { repeated CountryChoropleth items = 1; int32 total = 2; }

message GetHeatmapRequest { CrimeMapFilter filter = 1; }
message GetHeatmapResponse { repeated HeatmapPoint points = 1; int32 excluded_fallback = 2; }

service CrimeMapService {
  rpc GetChoropleth(GetChoroplethRequest) returns (GetChoroplethResponse);
  rpc GetHeatmap(GetHeatmapRequest) returns (GetHeatmapResponse);
}

// ---- UserProfile ----
message NotificationPreference {
  bool enabled = 1;
  repeated string target_country_cds = 2;
  repeated string info_types = 3;
}

message UserProfile {
  string uid = 1;
  repeated string favorite_country_cds = 2;
  NotificationPreference notification_preference = 3;
  int32 fcm_token_count = 4;  // トークン本体はレスポンスに含めない
}

message GetProfileRequest {}
message GetProfileResponse { UserProfile profile = 1; }

message ToggleFavoriteCountryRequest { string country_cd = 1; }
message ToggleFavoriteCountryResponse { UserProfile profile = 1; }

message UpdateNotificationPreferenceRequest { NotificationPreference preference = 1; }
message UpdateNotificationPreferenceResponse {}

message RegisterFcmTokenRequest {
  string token = 1;
  string device_id = 2;
}
message RegisterFcmTokenResponse {}

service UserProfileService {
  rpc GetProfile(GetProfileRequest) returns (GetProfileResponse);
  rpc ToggleFavoriteCountry(ToggleFavoriteCountryRequest) returns (ToggleFavoriteCountryResponse);
  rpc UpdateNotificationPreference(UpdateNotificationPreferenceRequest) returns (UpdateNotificationPreferenceResponse);
  rpc RegisterFcmToken(RegisterFcmTokenRequest) returns (RegisterFcmTokenResponse);
}
```

### 1.4 `proto/v1/pubsub.proto`（Domain Event）

```proto
syntax = "proto3";
package overseasmap.v1;
option go_package = "github.com/soneda-yuya/overseas-safety-map/gen/go/v1;overseasmapv1";

import "google/protobuf/timestamp.proto";

message NewArrivalEvent {
  string key_cd = 1;
  string country_cd = 2;
  string area_cd = 3;
  string info_type = 4;
  string title = 5;
  google.protobuf.Timestamp leave_date = 6;
  google.protobuf.Timestamp occurred_at = 7;  // Event 発行時刻（ingestion 側）
}
```

---

## 2. ログスキーマ（Q2 [A]）

### 2.1 共通属性（全サービスで必須）

| 属性 | 型 | 由来 | 例 |
|---|---|---|---|
| `time` | RFC3339Nano | slog 標準 | `2026-04-22T11:05:23.456Z` |
| `level` | string | slog 標準（`DEBUG`/`INFO`/`WARN`/`ERROR`） | `INFO` |
| `msg` | string | slog 標準 | `incident upserted` |
| `caller` | string | `file:line` | `ingest_usecase.go:87` |
| `service` | string | `SERVICE_NAME` env | `ingestion` / `bff` / `notifier` / `cmsmigrate` |
| `env` | string | `ENV` env | `dev` / `prod` |
| `trace_id` | string | OpenTelemetry（自動付与） | `4bf92f3577b34da6a3ce929d0e0e4736` |
| `span_id` | string | OpenTelemetry（自動付与） | `00f067aa0ba902b7` |
| `request_id` | string | BFF のみ: Connect Interceptor で自動付与 | uuid v4 |

### 2.2 ドメイン属性（処理に応じて flat で追加）

| 属性 | 型 | いつ付与 | 備考 |
|---|---|---|---|
| `key_cd` | string | safety_incident を扱う処理 | `slog.String("key_cd", ...)` を手動で |
| `uid` | string | user context を扱う処理（BFF / notifier） | PII に準ずる — dev 環境のみフル、prod ではハッシュ値 |
| `country_cd` | string | safetyincident / notification で対象国が定まった時 | — |
| `info_type` | string | ingestion / notification | — |
| `geocode_source` | string | ingestion の geocode 完了時 | `mapbox` / `country_centroid` |
| `deployment_id` | string | Cloud Run リビジョン | 起動時に 1 度セット |

### 2.3 エラー属性（`error` Kind 時）

| 属性 | 型 | 必須 |
|---|---|---|
| `error.kind` | string | ✓（`errs.Kind` の名前） |
| `error.message` | string | ✓ |
| `error.cause` | string | オプション（`errors.Unwrap()` チェーンの要約） |
| `error.stack` | string | panic recover 時のみ |

### 2.4 ログレベル指針

- `DEBUG`: ローカル開発用、prod ではマスク（`LOG_LEVEL=INFO` デフォルト）
- `INFO`: 正常系の節目（起動・ジョブ開始・1件処理・配信成功）
- `WARN`: リトライ可能 / 無効トークン検知 / フォールバック発動
- `ERROR`: 処理中断を伴う異常、アラート対象

### 2.5 禁止事項

- **nested group は使わない**（`slog.Group(...)` 非推奨）。flat フラット属性で出す。
- **PII（メールアドレス・生トークン）は `INFO` 以上でログしない**。`uid` のみ許可。
- **OpenTelemetry Logs SDK との二重出力は禁止**（slog → stdout → Cloud Logging 経由で trace 相関のみ使う）。

---

## 3. エラー分類（Q3 [A]）

### 3.1 `shared/errs` の Kind 列挙

```go
package errs

type Kind string

const (
    KindUnknown          Kind = "unknown"
    KindNotFound         Kind = "not_found"
    KindInvalidInput     Kind = "invalid_input"
    KindUnauthorized     Kind = "unauthorized"      // 認証失敗
    KindPermissionDenied Kind = "permission_denied" // 認可失敗
    KindExternal         Kind = "external"          // 外部 API 起因
    KindConflict         Kind = "conflict"
    KindInternal         Kind = "internal"
)
```

### 3.2 Connect ステータスコードへのマッピング

BFF の RPC ハンドラで `errs.Kind` → Connect `CodeX` を変換する。

| errs.Kind | Connect Code |
|---|---|
| `NotFound` | `CodeNotFound` |
| `InvalidInput` | `CodeInvalidArgument` |
| `Unauthorized` | `CodeUnauthenticated` |
| `PermissionDenied` | `CodePermissionDenied` |
| `External` | `CodeUnavailable` |
| `Conflict` | `CodeAlreadyExists` |
| `Internal` / `Unknown` / 他 | `CodeInternal` |

### 3.3 Kind は `string`

JSON ログとレスポンス DTO で直接表示できるよう `string` 定数。`int iota` にすると列挙値がログで読みにくい。

---

## 4. Config スキーマ（Q4 [A]）

### 4.1 環境変数の命名規約

- **プレフィックス** = `{DEPLOYABLE}_`（`INGESTION_` / `BFF_` / `NOTIFIER_` / `SETUP_`）+ 共通 `PLATFORM_`
- **大文字 + SNAKE_CASE**
- **Secrets は環境変数では扱わない** — Secret Manager 参照のリンク／ID のみを env に置き、SDK 経由で取得（Infrastructure Design で詳細化）

### 4.2 共通環境変数（全 Deployable）

| 環境変数 | 型 | 必須 | 既定値 | 説明 |
|---|---|:-:|---|---|
| `PLATFORM_SERVICE_NAME` | string | ✓ | — | `ingestion` / `bff` / `notifier` / `cmsmigrate` |
| `PLATFORM_ENV` | string | ✓ | — | `dev` / `prod` |
| `PLATFORM_LOG_LEVEL` | string |  | `INFO` | `DEBUG` / `INFO` / `WARN` / `ERROR` |
| `PLATFORM_GCP_PROJECT_ID` | string | ✓ | — | Cloud Project ID |

### 4.3 Deployable 別（抜粋、詳細は各 Unit の Infrastructure Design）

| 変数 | Unit |
|---|---|
| `INGESTION_MOFA_BASE_URL` | U-ING |
| `INGESTION_PUBSUB_TOPIC` | U-ING |
| `INGESTION_MAPBOX_SECRET_NAME` | U-ING |
| `INGESTION_CLAUDE_SECRET_NAME` | U-ING |
| `INGESTION_CMS_BASE_URL` | U-ING / U-BFF / U-SET |
| `INGESTION_CMS_WORKSPACE_ID` | U-ING / U-BFF / U-SET |
| `INGESTION_CMS_TOKEN_SECRET_NAME` | U-ING / U-BFF / U-SET |
| `BFF_PORT` | U-BFF |
| `BFF_FIREBASE_PROJECT_ID` | U-BFF |
| `NOTIFIER_PUBSUB_SUBSCRIPTION` | U-NTF |
| `NOTIFIER_FCM_PROJECT_ID` | U-NTF |

### 4.4 Config 読み込み

- ライブラリ: `github.com/kelseyhightower/envconfig`（標準的・薄い）
- 起動時に `config.Load()` 呼び出し → 必須欠落は `log.Fatal`（panic）
- 型対応: `string` / `int` / `bool` / `time.Duration` / `url.URL` / `[]string`（カンマ区切り）

---

## 受け入れ条件（Sign-off）

- [x] Proto 命名・型方針が Q1 [A] で確定
- [x] ログスキーマが Q2 [A] で確定
- [x] エラー分類 7 Kind が Q3 [A] で確定
- [x] Config 命名規約が Q4 [A] で確定
