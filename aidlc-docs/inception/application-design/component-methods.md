# コンポーネントメソッド — overseas-safety-map

各コンポーネントの **公開インターフェイス** とメソッドシグネチャを記載する。詳細なビジネスルール（分岐条件・境界値・計算式など）は Construction フェーズの Functional Design で定義する。

---

## 共通ドメイン型（`internal/domain`）

```go
package domain

type KeyCd string           // MOFA の keyCd（ユニークID）
type CountryCode string     // MOFA 国コード（例: "0049"=ドイツ、"0091"=インド）
type AreaCode string        // MOFA 地域コード（例: "42"=ヨーロッパ）
type InfoType string        // 例: "R10"=領事メール（一般）
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
    Area        AreaRef   // {Code, Name}
    Country     CountryRef
}

type SafetyIncident struct {
    MailItem
    ExtractedLocation string       // LLM 抽出地名（空の場合もある）
    Geometry          LatLng
    GeocodeSource     GeocodeSource
    IngestedAt        time.Time
    UpdatedAt         time.Time
}

type UserProfile struct {
    Uid              string
    FavoriteCountries []CountryCode
    Notification     NotificationPreference
}

type NotificationPreference struct {
    Enabled        bool
    TargetCountries []CountryCode  // 空 = 全件対象
    InfoTypes       []InfoType
    FcmTokens       []string       // 端末複数
}
```

---

## C-01: `mofa.Client`

```go
package mofa

type Client interface {
    // FetchAll は /area/00A.xml を取得してパース済み MailItem のスライスを返す（初回初期化用）。
    FetchAll(ctx context.Context) ([]domain.MailItem, error)

    // FetchNewArrivals は /area/newarrivalA.xml を取得してパース済み MailItem のスライスを返す（継続運用用）。
    FetchNewArrivals(ctx context.Context) ([]domain.MailItem, error)

    // Parse は XML バイト列を MailItem 列に変換する（テスト・再利用用）。
    Parse(xml []byte) ([]domain.MailItem, error)
}
```

## C-02: `llm.LocationExtractor`

```go
package llm

type LocationExtractionResult struct {
    LocationText string  // 抽出された地名文字列（"ベルリン市内" 等）
    Confidence   float64 // 0.0〜1.0、実装依存
    RawResponse  string  // デバッグ用
}

type LocationExtractor interface {
    // Extract は title + mainText を入力に、代表地名を1件返す（見つからなければ LocationText="" で返す）。
    Extract(ctx context.Context, title, mainText string) (LocationExtractionResult, error)
}

// ClaudeExtractor は Anthropic Claude Haiku クラス向け実装。
// API キー・モデル名・タイムアウトを構造体フィールドで受け取る。
type ClaudeExtractor struct{ /* unexported fields */ }
func NewClaudeExtractor(cfg ClaudeConfig) *ClaudeExtractor
```

## C-03: `geocode.Geocoder`

```go
package geocode

type Result struct {
    Location domain.LatLng
    Source   domain.GeocodeSource
}

type Geocoder interface {
    // Geocode は地名文字列 + 補助情報（国コード）から緯度経度を返す。
    // 取れなければ ErrNotFound を返す（フォールバック側へ回す判断は Chain が行う）。
    Geocode(ctx context.Context, query string, hint CountryHint) (Result, error)
}

type CountryHint struct {
    CountryCode domain.CountryCode
    CountryName string
}

// MapboxGeocoder: primary 実装
type MapboxGeocoder struct{ /* unexported */ }
func NewMapboxGeocoder(cfg MapboxConfig) *MapboxGeocoder

// CountryCentroidFallback: 国コード → 代表座標への静的マップ参照
type CountryCentroidFallback struct{ /* unexported */ }
func NewCountryCentroidFallback() *CountryCentroidFallback

// Chain は primary → fallback の順に試す Geocoder 合成。
func Chain(primary Geocoder, fallback Geocoder) Geocoder

var ErrNotFound = errors.New("geocode: not found")
```

## C-04: `repository.SafetyIncidentRepository`

```go
package repository

type ListFilter struct {
    AreaCd      *domain.AreaCode
    CountryCd   *domain.CountryCode
    InfoTypes   []domain.InfoType
    LeaveFrom   *time.Time
    LeaveTo     *time.Time
    Near        *NearQuery   // 現在地検索用
    Limit       int
    Cursor      string       // ページングトークン
}

type NearQuery struct {
    Center    domain.LatLng
    RadiusKm  float64
}

type ListResult struct {
    Items      []domain.SafetyIncident
    NextCursor string
    TotalHint  int  // 参考値（厳密でなくてよい）
}

type SafetyIncidentRepository interface {
    Get(ctx context.Context, keyCd domain.KeyCd) (*domain.SafetyIncident, error)
    Exists(ctx context.Context, keyCd domain.KeyCd) (bool, error)
    List(ctx context.Context, f ListFilter) (ListResult, error)
    Upsert(ctx context.Context, incident domain.SafetyIncident) error  // 追記専用、ただし冪等性のため Upsert 名称
    Delete(ctx context.Context, keyCd domain.KeyCd) error              // 管理用途、MVP では通常利用しない
}

// CMSRepository: reearth-cms Integration API 経由の実装
type CMSRepository struct{ /* unexported */ }
func NewCMSRepository(cms cms.Client, cfg CMSRepositoryConfig) *CMSRepository

var ErrNotFound = errors.New("repository: not found")
```

## C-05: `cms.Client`（reearth-cms Integration API の薄いラッパー）

```go
package cms

type Client interface {
    // Item CRUD
    CreateItem(ctx context.Context, modelID string, fields map[string]any) (ItemID, error)
    UpdateItem(ctx context.Context, itemID ItemID, fields map[string]any) error
    GetItem(ctx context.Context, itemID ItemID) (Item, error)
    ListItems(ctx context.Context, modelID string, q ListQuery) (ItemPage, error)
    ItemsAsGeoJSON(ctx context.Context, modelID string, q ListQuery) (GeoJSON, error)
    DeleteItem(ctx context.Context, itemID ItemID) error

    // Schema CRUD（setup から呼ばれる）
    CreateProject(ctx context.Context, wsID WorkspaceID, spec ProjectSpec) (ProjectID, error)
    CreateModel(ctx context.Context, wsID WorkspaceID, projectID ProjectID, spec ModelSpec) (ModelID, error)
    CreateField(ctx context.Context, wsID WorkspaceID, projectID ProjectID, schemaID SchemaID, spec FieldSpec) (FieldID, error)
}
```

## C-06: `pubsub.Publisher` / `pubsub.Subscriber`

```go
package pubsub

type NewArrivalMessage struct {
    KeyCd       domain.KeyCd
    CountryCode domain.CountryCode
    AreaCode    domain.AreaCode
    InfoType    domain.InfoType
    Title       string
    LeaveDate   time.Time
}

type Publisher interface {
    PublishNewArrival(ctx context.Context, msg NewArrivalMessage) error
}

type Handler func(ctx context.Context, msg NewArrivalMessage) error

type Subscriber interface {
    // Start はブロッキング。ctx キャンセルで終了。
    Start(ctx context.Context, handler Handler) error
}
```

## C-07: `firebase.*`

```go
package firebase

type AuthVerifier interface {
    // Verify は ID Token を検証し、UID とカスタムクレームを返す。
    Verify(ctx context.Context, idToken string) (*VerifiedUser, error)
}

type VerifiedUser struct {
    Uid    string
    Email  string
    Claims map[string]any
}

type UserStore interface {
    Get(ctx context.Context, uid string) (*domain.UserProfile, error)
    UpsertProfile(ctx context.Context, profile domain.UserProfile) error
    ListSubscribersFor(ctx context.Context, countryCd domain.CountryCode, infoType domain.InfoType) ([]domain.UserProfile, error)
}

type FcmSender interface {
    // Send は端末トークン配列に対して単一通知を配信する。結果（成功/失敗件数）は error と別の Report で返す。
    Send(ctx context.Context, tokens []string, notif Notification) (SendReport, error)
}

type Notification struct {
    Title   string
    Body    string
    Payload map[string]string  // 例: {"keyCd": "...", "deeplink": "/detail/<keyCd>"}
}

type SendReport struct {
    SuccessCount int
    FailureCount int
    InvalidTokens []string  // Firestore 側から除去するためのリスト
}
```

## C-08: `crimemap.Aggregator`

```go
package crimemap

type AggregateFilter struct {
    LeaveFrom *time.Time
    LeaveTo   *time.Time
}

type CountryChoropleth struct {
    CountryCode domain.CountryCode
    CountryName string
    Count       int
    Color       string  // 色スケールのヒント（設計時決定）
}

type HeatmapPoint struct {
    Location domain.LatLng
    Weight   float64
}

type Aggregator interface {
    // 国別カロプレス（フォールバック座標も含めた件数ベース集計）
    Choropleth(ctx context.Context, f AggregateFilter) ([]CountryChoropleth, error)

    // ヒートマップ用ポイント（フォールバック座標のアイテムは除外 — FR-APP-08）
    Heatmap(ctx context.Context, f AggregateFilter) ([]HeatmapPoint, error)
}
```

## C-10: BFF の Connect サービス（`proto/v1/safetymap.proto`）

```proto
syntax = "proto3";
package overseasmap.v1;

service SafetyIncidentService {
  rpc ListSafetyIncidents(ListSafetyIncidentsRequest) returns (ListSafetyIncidentsResponse);
  rpc GetSafetyIncident(GetSafetyIncidentRequest) returns (GetSafetyIncidentResponse);
  rpc SearchSafetyIncidents(SearchSafetyIncidentsRequest) returns (SearchSafetyIncidentsResponse);
  rpc ListNearby(ListNearbyRequest) returns (ListNearbyResponse);
  rpc GetSafetyIncidentsAsGeoJSON(GeoJSONRequest) returns (GeoJSONResponse);
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

メッセージ型は Construction フェーズで詳細化する。Connect サーバー本体は `internal/bff` に実装する。

```go
package bff

type SafetyIncidentServer struct {
    repo     repository.SafetyIncidentRepository
    // Connect interceptor 経由で認証情報が context に入る
}

func (s *SafetyIncidentServer) ListSafetyIncidents(ctx context.Context, req *connect.Request[ListSafetyIncidentsRequest]) (*connect.Response[ListSafetyIncidentsResponse], error)
// 他サービスも同様
```

## C-11: `setup.Runner`

```go
package setup

type Runner interface {
    // EnsureSchema は Project / Model / Field が無ければ作成し、あれば何もしない（冪等）。
    EnsureSchema(ctx context.Context) error
}
```

## C-12: `notifier.Dispatcher`

```go
package notifier

type Dispatcher interface {
    // Handle は単一 NewArrivalMessage を処理する：
    //   1. Firestore から該当国＋該当情報種別を購読しているユーザーを取得
    //   2. ユーザーごとに FCM トークン配列を集約
    //   3. FCM 送信
    //   4. InvalidTokens を Firestore から除去
    Handle(ctx context.Context, msg pubsub.NewArrivalMessage) error
}
```

## C-13: `observability.*`

```go
package observability

type Config struct {
    ServiceName string  // "ingestion" | "bff" | "notifier" | "setup"
    Env         string  // "dev" | "prod"
}

func Setup(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error)

func Logger(ctx context.Context) *slog.Logger
func Tracer(ctx context.Context) trace.Tracer
func Meter(ctx context.Context) metric.Meter

// 共通属性を付与するための中間関数（request-id、user-id、keyCd 等）
func WithKeyCd(ctx context.Context, keyCd domain.KeyCd) context.Context
```

---

## Flutter 側の主要メソッド（C-20 / C-21 / C-22）

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
  Future<String> getIdToken();  // Connect インターセプタが利用
}

// C-22 presentation (MVVM)
// 例: map feature
class MapViewModel extends AsyncNotifier<MapState> {
  @override
  Future<MapState> build();
  Future<void> refresh();
  Future<void> onPinTapped(String keyCd);
  Future<void> applyFilter(SafetyIncidentFilter filter);
}
```

ViewModel は domain の UseCase を呼び出し、UI には `AsyncValue<State>` を公開する標準パターンを採用する（Riverpod の `AsyncNotifier`）。
