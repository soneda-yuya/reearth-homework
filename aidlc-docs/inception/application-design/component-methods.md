# コンポーネントメソッド — overseas-safety-map

各コンポーネントの **公開インターフェイス** とメソッドシグネチャを DDD の **Bounded Context × Layered Architecture** に沿って記載する。詳細なビジネスルール（分岐条件・境界値・計算式など）は Construction フェーズの Functional Design で定義する。

**命名規則**:
- Port（I/F）は各 Context の `domain` パッケージに置く
- Adapter（I/F 実装）は各 Context の `infrastructure/{技術名}` パッケージに置く
- Application Service（UseCase）は各 Context の `application` パッケージに置く
- Connect ハンドラは `internal/interfaces/rpc`、Job ランナーは `internal/interfaces/job`

---

## 🟩 Bounded Context: `safetyincident`（Core）

### ドメイン型（`internal/safetyincident/domain`）

```go
package safetyincident

type KeyCd string            // MOFA の keyCd（ユニークID）
type CountryCode string      // 例: "0049"=ドイツ
type AreaCode string         // 例: "42"=ヨーロッパ
type InfoType string         // 例: "R10"=領事メール（一般）
type KoukanCode string

type LatLng struct {
    Lat float64
    Lng float64
}

type GeocodeSource int
const (
    GeocodeSourceUnknown GeocodeSource = iota
    GeocodeSourceMapbox
    GeocodeSourceCountryCentroid  // フォールバック
)

// Entity: MOFA から取得した生データ
type MailItem struct {
    KeyCd       KeyCd
    InfoType    InfoType
    InfoName    string
    LeaveDate   time.Time
    Title       string
    Lead        string
    MainText    string
    InfoUrl     string
    KoukanCd    KoukanCode
    KoukanName  string
    AreaCd      AreaCode
    AreaName    string
    CountryCd   CountryCode
    CountryName string
}

// Aggregate Root: 地名抽出・ジオコーディング済みの安全情報
type SafetyIncident struct {
    MailItem
    ExtractedLocation string
    Geometry          LatLng
    GeocodeSource     GeocodeSource
    IngestedAt        time.Time
    UpdatedAt         time.Time
}

// VO: 検索/一覧フィルタ
type ListFilter struct {
    AreaCd    *AreaCode
    CountryCd *CountryCode
    InfoTypes []InfoType
    LeaveFrom *time.Time
    LeaveTo   *time.Time
    Near      *NearQuery
    Limit     int
    Cursor    string
}

type NearQuery struct {
    Center   LatLng
    RadiusKm float64
}

type ListResult struct {
    Items      []SafetyIncident
    NextCursor string
    TotalHint  int
}

// Domain Event
type NewArrivalEvent struct {
    KeyCd       KeyCd
    CountryCode CountryCode
    AreaCode    AreaCode
    InfoType    InfoType
    Title       string
    LeaveDate   time.Time
    OccurredAt  time.Time
}
```

### Port（`internal/safetyincident/domain`）

```go
package safetyincident

// C-01: MOFA 取得ポート
type MofaSource interface {
    FetchAll(ctx context.Context) ([]MailItem, error)
    FetchNewArrivals(ctx context.Context) ([]MailItem, error)
    Parse(xml []byte) ([]MailItem, error)
}

// C-02: LLM 地名抽出ポート
type LocationExtractionResult struct {
    LocationText string
    Confidence   float64
    RawResponse  string
}
type LocationExtractor interface {
    Extract(ctx context.Context, title, mainText string) (LocationExtractionResult, error)
}

// C-03: ジオコーダポート
type CountryHint struct {
    CountryCode CountryCode
    CountryName string
}
type GeocodeResult struct {
    Location LatLng
    Source   GeocodeSource
}
type Geocoder interface {
    Geocode(ctx context.Context, query string, hint CountryHint) (GeocodeResult, error)
}
var ErrGeocodeNotFound = errors.New("safetyincident: geocode not found")

// C-04: Repository ポート（読み書き統合）
type Repository interface {
    Get(ctx context.Context, keyCd KeyCd) (*SafetyIncident, error)
    Exists(ctx context.Context, keyCd KeyCd) (bool, error)
    List(ctx context.Context, f ListFilter) (ListResult, error)
    Upsert(ctx context.Context, incident SafetyIncident) error
    Delete(ctx context.Context, keyCd KeyCd) error
}
var ErrNotFound = errors.New("safetyincident: not found")

// C-06(publish): Domain Event の発行ポート
type EventPublisher interface {
    PublishNewArrival(ctx context.Context, evt NewArrivalEvent) error
}
```

### Adapter（`internal/safetyincident/infrastructure/*`）

```go
// infrastructure/mofa: HTTP + XML
package mofa

type HttpSource struct{ /* unexported: httpClient, urls */ }
func NewHttpSource(cfg Config) *HttpSource
// implements safetyincident.MofaSource

// infrastructure/llm: Claude
package llm

type ClaudeExtractor struct{ /* unexported */ }
func NewClaudeExtractor(cfg ClaudeConfig) *ClaudeExtractor
// implements safetyincident.LocationExtractor

// infrastructure/geocode: Mapbox + Centroid + Chain
package geocode

type MapboxGeocoder struct{ /* unexported */ }
func NewMapboxGeocoder(cfg MapboxConfig) *MapboxGeocoder

type CountryCentroidFallback struct{ /* unexported */ }
func NewCountryCentroidFallback() *CountryCentroidFallback

func Chain(primary safetyincident.Geocoder, fallback safetyincident.Geocoder) safetyincident.Geocoder

// infrastructure/cms: Repository 実装
package cms

type Repository struct{ /* unexported: client *cmsx.Client, modelID string */ }
func NewRepository(client *cmsx.Client, cfg RepositoryConfig) *Repository
// implements safetyincident.Repository

// infrastructure/eventbus: EventPublisher 実装
package eventbus

type PubSubPublisher struct{ /* unexported: topic *pubsub.Topic */ }
func NewPubSubPublisher(client *pubsubx.Client, topic string) *PubSubPublisher
// implements safetyincident.EventPublisher
```

### Application（`internal/safetyincident/application`）

```go
package safetyincidentapp

type IngestMode int
const (
    IngestModeInitial IngestMode = iota
    IngestModeNewArrival
)

type IngestReport struct {
    Fetched     int
    Skipped     int
    Created     int
    GeocodeMiss int
    LLMMiss     int
    Errors      []error
    Elapsed     time.Duration
}

// IngestUseCase: 取り込みオーケストレーション
type IngestUseCase struct {
    source    safetyincident.MofaSource
    extractor safetyincident.LocationExtractor
    geocoder  safetyincident.Geocoder
    repo      safetyincident.Repository
    publisher safetyincident.EventPublisher
    clock     clock.Clock
}
func NewIngestUseCase(...) *IngestUseCase
func (u *IngestUseCase) Run(ctx context.Context, mode IngestMode) (IngestReport, error)

// ListUseCase / GetUseCase / SearchUseCase / NearbyUseCase: BFF 読み取り
type ListUseCase struct{ repo safetyincident.Repository }
func (u *ListUseCase) Execute(ctx context.Context, f safetyincident.ListFilter) (safetyincident.ListResult, error)

type GetUseCase struct{ repo safetyincident.Repository }
func (u *GetUseCase) Execute(ctx context.Context, keyCd safetyincident.KeyCd) (*safetyincident.SafetyIncident, error)

type SearchUseCase struct{ repo safetyincident.Repository }
func (u *SearchUseCase) Execute(ctx context.Context, q SearchQuery) (safetyincident.ListResult, error)

type NearbyUseCase struct{ repo safetyincident.Repository }
func (u *NearbyUseCase) Execute(ctx context.Context, center safetyincident.LatLng, radiusKm float64) (safetyincident.ListResult, error)
```

---

## 🟨 Subdomain: `safetyincident/crimemap`

### ドメイン型・Port（`internal/safetyincident/crimemap/domain`）

```go
package crimemap

type AggregateFilter struct {
    LeaveFrom *time.Time
    LeaveTo   *time.Time
}

type CountryChoropleth struct {
    CountryCode safetyincident.CountryCode
    CountryName string
    Count       int
    Color       string
}

type HeatmapPoint struct {
    Location safetyincident.LatLng
    Weight   float64
}

// Policy: 何を「犯罪」とみなすかのドメインポリシー
type InfoTypePolicy interface {
    IsCrime(it safetyincident.InfoType) bool
    CrimeInfoTypes() []safetyincident.InfoType
}

// Domain Service
type Aggregator interface {
    Choropleth(ctx context.Context, f AggregateFilter) ([]CountryChoropleth, error)
    Heatmap(ctx context.Context, f AggregateFilter) ([]HeatmapPoint, error)  // フォールバック座標は除外
}
```

### Adapter（`internal/safetyincident/crimemap/infrastructure`）

```go
package crimemapinfra

type RepositoryAggregator struct {
    repo   safetyincident.Repository
    policy crimemap.InfoTypePolicy
}
func NewRepositoryAggregator(repo safetyincident.Repository, policy crimemap.InfoTypePolicy) *RepositoryAggregator
// implements crimemap.Aggregator
```

### Application（`internal/safetyincident/crimemap/application`）

```go
package crimemapapp

type GetChoroplethUseCase struct{ agg crimemap.Aggregator }
func (u *GetChoroplethUseCase) Execute(ctx context.Context, f crimemap.AggregateFilter) ([]crimemap.CountryChoropleth, error)

type GetHeatmapUseCase struct{ agg crimemap.Aggregator }
func (u *GetHeatmapUseCase) Execute(ctx context.Context, f crimemap.AggregateFilter) ([]crimemap.HeatmapPoint, error)
```

---

## 🟦 Bounded Context: `user`（Supporting）

### ドメイン型・Port（`internal/user/domain`）

```go
package user

type Uid string

type FavoriteCountry struct {
    CountryCode safetyincident.CountryCode  // ※ shared VO として参照
}

type NotificationPref struct {
    Enabled         bool
    TargetCountries []safetyincident.CountryCode
    InfoTypes       []safetyincident.InfoType
}

type FcmToken struct {
    Value      string
    DeviceID   string
    RegisteredAt time.Time
}

// Aggregate Root
type UserProfile struct {
    Uid              Uid
    FavoriteCountries []FavoriteCountry
    NotificationPref NotificationPref
    FcmTokens        []FcmToken
}

type VerifiedUser struct {
    Uid    Uid
    Email  string
    Claims map[string]any
}

// Port: Firebase Auth 検証
type AuthVerifier interface {
    Verify(ctx context.Context, idToken string) (*VerifiedUser, error)
}
var ErrInvalidToken = errors.New("user: invalid token")

// Port: プロファイル永続化
type ProfileRepository interface {
    Get(ctx context.Context, uid Uid) (*UserProfile, error)
    Upsert(ctx context.Context, profile UserProfile) error
    ToggleFavorite(ctx context.Context, uid Uid, countryCd safetyincident.CountryCode) (UserProfile, error)
    UpdateNotificationPref(ctx context.Context, uid Uid, pref NotificationPref) error
    AddFcmToken(ctx context.Context, uid Uid, token FcmToken) error
    RemoveFcmTokens(ctx context.Context, uid Uid, tokens []string) error
}
```

> **Context 境界上の注意**: `user.domain` が `safetyincident.CountryCode` / `InfoType` を利用しているのは、これらが MOFA 由来の **識別子コード** で実質的にアプリ全体の共有語彙であるため（Shared Kernel 相当）。必要に応じて将来は `shared/codes` に分離する。

### Adapter（`internal/user/infrastructure/*`）

```go
// infrastructure/firebaseauth
package firebaseauth

type Verifier struct{ /* unexported: *auth.Client */ }
func NewVerifier(app *firebasex.App) *Verifier
// implements user.AuthVerifier

// infrastructure/firestore
package firestoreadapter

type ProfileRepository struct{ /* unexported: *firestore.Client */ }
func NewProfileRepository(app *firebasex.App) *ProfileRepository
// implements user.ProfileRepository
```

### Application（`internal/user/application`）

```go
package userapp

type GetProfileUseCase struct{ repo user.ProfileRepository }
type ToggleFavoriteCountryUseCase struct{ repo user.ProfileRepository }
type UpdateNotificationPrefUseCase struct{ repo user.ProfileRepository }
type RegisterFcmTokenUseCase struct{ repo user.ProfileRepository }
// 各 UseCase は Execute(ctx, ...) メソッドを持つ
```

---

## 🟦 Bounded Context: `notification`（Supporting）

### ドメイン型・Port（`internal/notification/domain`）

```go
package notification

type Subscriber struct {
    Uid       string  // user.Uid と同値だが、Context 独立性のため string で保持
    FcmTokens []string
    Prefs     Preferences
}

type Preferences struct {
    Enabled         bool
    TargetCountries []string
    InfoTypes       []string
}

type Notification struct {
    Title   string
    Body    string
    Payload map[string]string  // 例: {"keyCd": "...", "deeplink": "/detail/<keyCd>"}
}

type SendReport struct {
    SuccessCount  int
    FailureCount  int
    InvalidTokens []string
}

// Domain Service: 配信対象の判定
type DispatchPolicy interface {
    ShouldDeliver(sub Subscriber, countryCode, infoType string) bool
}

// Port: 購読者の取得
type SubscriberStore interface {
    ListSubscribersFor(ctx context.Context, countryCode, infoType string) ([]Subscriber, error)
    RemoveInvalidTokens(ctx context.Context, invalid []string) error
}

// Port: プッシュ送信
type PushSender interface {
    Send(ctx context.Context, tokens []string, notif Notification) (SendReport, error)
}

// Port: 他コンテキスト発のイベント受信
type NewArrivalMessage struct {
    KeyCd       string
    CountryCode string
    AreaCode    string
    InfoType    string
    Title       string
    LeaveDate   time.Time
}
type NewArrivalConsumer interface {
    Start(ctx context.Context, handler func(context.Context, NewArrivalMessage) error) error
}
```

### Adapter（`internal/notification/infrastructure/*`）

```go
// infrastructure/firestore: SubscriberStore 実装
// user Context とは独立して、同一 Firestore コレクションを「購読者ビュー」として読む
package firestoreadapter

type SubscriberStore struct{ /* unexported */ }
func NewSubscriberStore(app *firebasex.App) *SubscriberStore
// implements notification.SubscriberStore

// infrastructure/fcm
package fcmadapter

type PushSender struct{ /* unexported */ }
func NewPushSender(app *firebasex.App) *PushSender
// implements notification.PushSender

// infrastructure/eventbus
package eventbus

type PubSubConsumer struct{ /* unexported: *pubsub.Subscription */ }
func NewPubSubConsumer(client *pubsubx.Client, subscription string) *PubSubConsumer
// implements notification.NewArrivalConsumer
```

### Application（`internal/notification/application`）

```go
package notificationapp

type DispatchOnNewArrivalUseCase struct {
    store   notification.SubscriberStore
    sender  notification.PushSender
    policy  notification.DispatchPolicy
}
func (u *DispatchOnNewArrivalUseCase) Execute(ctx context.Context, msg notification.NewArrivalMessage) error
```

---

## 🟦 Bounded Context: `cmsmigrate`（Supporting）

### ドメイン（`internal/cmsmigrate/domain`）

```go
package cmsmigrate

// 宣言的スキーマ定義（どの Model にどの Field が必要か）
type FieldDefinition struct {
    Key       string
    Type      FieldType
    Required  bool
    Multiple  bool
}

type FieldType string
const (
    FieldTypeText        FieldType = "text"
    FieldTypeTextArea    FieldType = "textarea"
    FieldTypeDateTime    FieldType = "dateTime"
    FieldTypeURL         FieldType = "url"
    FieldTypeGeometry    FieldType = "geometry"
)

type ModelDefinition struct {
    Key    string
    Name   string
    Fields []FieldDefinition
}

type SchemaDefinition struct {
    ProjectKey  string
    ProjectName string
    Models      []ModelDefinition
}
```

### Adapter（`internal/cmsmigrate/infrastructure/cms`）

```go
package cmsapplier

type SchemaApplier struct{ /* unexported: *cmsx.Client, workspaceID */ }
func NewSchemaApplier(client *cmsx.Client, workspaceID string) *SchemaApplier
// Apply は SchemaDefinition を reearth-cms に冪等に適用する
func (a *SchemaApplier) Apply(ctx context.Context, def cmsmigrate.SchemaDefinition) error
```

### Application（`internal/cmsmigrate/application`）

```go
package cmsmigrateapp

type EnsureSchemaUseCase struct {
    applier *cmsapplier.SchemaApplier
    def     cmsmigrate.SchemaDefinition  // 静的定義
}
func (u *EnsureSchemaUseCase) Execute(ctx context.Context) error
```

---

## 🎯 Interface レイヤ（`internal/interfaces`）

### RPC ハンドラ（`internal/interfaces/rpc`）

```go
package rpc

// SafetyIncidentHandler は各 UseCase を組み合わせて Connect Request を処理
type SafetyIncidentHandler struct {
    list   *safetyincidentapp.ListUseCase
    get    *safetyincidentapp.GetUseCase
    search *safetyincidentapp.SearchUseCase
    nearby *safetyincidentapp.NearbyUseCase
}
func (h *SafetyIncidentHandler) ListSafetyIncidents(ctx context.Context, req *connect.Request[v1.ListSafetyIncidentsRequest]) (*connect.Response[v1.ListSafetyIncidentsResponse], error)
// 他の RPC も同様

type CrimeMapHandler struct {
    choropleth *crimemapapp.GetChoroplethUseCase
    heatmap    *crimemapapp.GetHeatmapUseCase
}

type UserSettingHandler struct {
    getProfile       *userapp.GetProfileUseCase
    toggleFavorite   *userapp.ToggleFavoriteCountryUseCase
    updateNotifPref  *userapp.UpdateNotificationPrefUseCase
    registerFcmToken *userapp.RegisterFcmTokenUseCase
}

// AuthInterceptor: 全 RPC に適用、VerifiedUser を context に注入
type AuthInterceptor struct{ verifier user.AuthVerifier }
func (i *AuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc
```

### Job ランナー（`internal/interfaces/job`）

```go
package job

type IngestionRunner struct {
    usecase *safetyincidentapp.IngestUseCase
    mode    safetyincidentapp.IngestMode
}
func (r *IngestionRunner) Run(ctx context.Context) error

type NotifierRunner struct {
    consumer notification.NewArrivalConsumer
    usecase  *notificationapp.DispatchOnNewArrivalUseCase
}
func (r *NotifierRunner) Run(ctx context.Context) error

type SetupRunner struct {
    usecase *cmsmigrateapp.EnsureSchemaUseCase
}
func (r *SetupRunner) Run(ctx context.Context) error
```

---

## 📦 Platform（`internal/platform/*`）

```go
// platform/observability
package observability

type Config struct {
    ServiceName string  // "ingestion" | "bff" | "notifier" | "cmsmigrate"
    Env         string
}
func Setup(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error)
func Logger(ctx context.Context) *slog.Logger
func Tracer(ctx context.Context) trace.Tracer
func Meter(ctx context.Context) metric.Meter

// platform/cmsx: reearth-cms Integration REST API の低レベルクライアント
package cmsx

type Client interface {
    CreateItem(ctx context.Context, modelID string, fields map[string]any) (ItemID, error)
    UpdateItem(ctx context.Context, itemID ItemID, fields map[string]any) error
    GetItem(ctx context.Context, itemID ItemID) (Item, error)
    ListItems(ctx context.Context, modelID string, q ListQuery) (ItemPage, error)
    ItemsAsGeoJSON(ctx context.Context, modelID string, q ListQuery) (GeoJSON, error)
    DeleteItem(ctx context.Context, itemID ItemID) error

    CreateProject(ctx context.Context, wsID WorkspaceID, spec ProjectSpec) (ProjectID, error)
    CreateModel(ctx context.Context, wsID WorkspaceID, projectID ProjectID, spec ModelSpec) (ModelID, error)
    CreateField(ctx context.Context, wsID WorkspaceID, projectID ProjectID, schemaID SchemaID, spec FieldSpec) (FieldID, error)
}

// platform/pubsubx
package pubsubx

type Client struct{ /* GCP Pub/Sub client wrapper */ }
func NewClient(ctx context.Context, projectID string) (*Client, error)

// platform/firebasex
package firebasex

type App struct{ /* Firebase App wrapper (Auth / Firestore / FCM) */ }
func NewApp(ctx context.Context, cfg Config) (*App, error)

// platform/config
package config

type Config struct { /* fields from env */ }
func Load() (*Config, error)

// platform/connectserver
package connectserver

type Server struct{ /* http + connect mux */ }
func New(handlers []ConnectHandler, interceptors []connect.Interceptor) *Server
```

---

## 🤝 Shared Kernel（`internal/shared/*`）

```go
// shared/errs
package errs

type Kind int
const (
    KindNotFound Kind = iota
    KindInvalidInput
    KindUnauthorized
    KindExternal
    KindInternal
)

type AppError struct{ Kind Kind; Op string; Err error }
func (e *AppError) Error() string
func (e *AppError) Unwrap() error
func Wrap(op string, kind Kind, err error) error

// shared/clock
package clock

type Clock interface {
    Now() time.Time
}
type System struct{}
func (System) Now() time.Time { return time.Now() }
```

---

## Proto（Connect）

```proto
syntax = "proto3";
package overseasmap.v1;

service SafetyIncidentService {
  rpc ListSafetyIncidents(ListSafetyIncidentsRequest) returns (ListSafetyIncidentsResponse);
  rpc GetSafetyIncident(GetSafetyIncidentRequest) returns (GetSafetyIncidentResponse);
  rpc SearchSafetyIncidents(SearchSafetyIncidentsRequest) returns (SearchSafetyIncidentsResponse);
  rpc ListNearby(ListNearbyRequest) returns (ListNearbyResponse);
  rpc GetSafetyIncidentsAsGeoJSON(GetSafetyIncidentsAsGeoJSONRequest) returns (GetSafetyIncidentsAsGeoJSONResponse);
}

service CrimeMapService {
  rpc GetChoropleth(GetChoroplethRequest) returns (GetChoroplethResponse);
  rpc GetHeatmap(GetHeatmapRequest) returns (GetHeatmapResponse);
}

service UserProfileService {
  rpc GetProfile(GetProfileRequest) returns (GetProfileResponse);
  rpc ToggleFavoriteCountry(ToggleFavoriteCountryRequest) returns (ToggleFavoriteCountryResponse);
  rpc UpdateNotificationPreference(UpdateNotificationPreferenceRequest) returns (UpdateNotificationPreferenceResponse);
  rpc RegisterFcmToken(RegisterFcmTokenRequest) returns (RegisterFcmTokenResponse);
}
```

メッセージ型は Construction フェーズで詳細化する。

---

## Flutter 側の主要メソッド（C-20 / C-21 / C-22）

> バックエンドの DDD 化に伴う変更は**なし**（要求範囲外）。

詳細は Dart ファイルの実装フェーズで決定するが、MVVM の各 ViewModel が公開するメソッドの骨子を定義する。

```dart
// C-20 domain
abstract class SafetyIncidentRepository {
  Future<List<SafetyIncident>> list(SafetyIncidentFilter filter);
  Future<SafetyIncident> get(String keyCd);
  Future<List<SafetyIncident>> listNearby(LatLng center, double radiusKm);
  Future<List<CountryChoropleth>> getChoropleth(DateRange? range);
  Future<List<HeatmapPoint>> getHeatmap(DateRange? range);
}

abstract class UserProfileRepository {
  Future<UserProfile> getMe();
  Future<void> toggleFavoriteCountry(String countryCode);
  Future<void> updateNotificationPreference(NotificationPreference pref);
  Future<void> registerFcmToken(String token);
}

abstract class AuthRepository {
  Stream<AuthUser?> authStateChanges();
  Future<AuthUser> signInWithEmail(String email, String password);
  Future<AuthUser> signUpWithEmail(String email, String password);
  Future<void> signOut();
  Future<String> getIdToken();
}

// C-22 presentation (MVVM)
class MapViewModel extends AsyncNotifier<MapState> {
  @override
  Future<MapState> build();
  Future<void> refresh();
  Future<void> onPinTapped(String keyCd);
  Future<void> applyFilter(SafetyIncidentFilter filter);
}
```
