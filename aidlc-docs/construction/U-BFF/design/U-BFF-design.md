# U-BFF Design (Minimal 合本版)

**Unit**: U-BFF（Connect Server / BFF Unit、Sprint 3、**実装順で最後のバックエンド Unit**）
**Deployable**: `cmd/bff`（Cloud Run Service、Connect over HTTP/2）
**Bounded Context**: `safetyincident`（読取系 + `crimemap` subdomain）+ `user`
**ワークフロー圧縮**: Option B（Functional Design + NFR Requirements + NFR Design 1 本に集約）

---

## 0. Design Decisions（計画回答の確定）

[`U-BFF-design-plan.md`](../../plans/U-BFF-design-plan.md) Q1-Q9 すべて **[A]** で確定。

| # | 決定事項 | 選択 | 要旨 |
|---|---------|------|------|
| Q1 | 認証方式 | **A** | 全 RPC で Firebase ID Token 必須、AuthInterceptor、Anonymous Auth 含む |
| Q2 | SafetyIncident キャッシュ | **A** | 毎回 CMS 直アクセス、キャッシュなし |
| Q3 | CrimeMap 集計 | **A** | `crimemap/application.Aggregator` で in-memory 集計、`color` サーバ計算 |
| Q4 | UserProfile 永続化 | **A** | U-NTF と同じ Firestore `users` collection 共有 |
| Q5 | FCM Token 冪等性 | **A** | `ArrayUnion`、device_id はログのみ |
| Q6 | Connect error code | **A** | `errs.KindOf` 一律マッピング + prod でメッセージマスク |
| Q7 | Observability | **A** | Span + 4 Metric + `app.bff.phase` 属性ログ |
| Q8 | テスト戦略 | **A** | 層別カバレッジ + `connecttest.Server` で handler e2e |
| Q9 | PR 分割 | **A** | 2 PR（U-ING パターン） |

---

## 1. Functional Design

### 1.1 Context — U-BFF の責務

**目的**: Flutter アプリが叩く唯一の Connect RPC サーバ。認証 (Firebase ID Token) と以下 11 RPC を提供。

| Service | RPC | 概要 |
|---|---|---|
| `SafetyIncidentService` | `ListSafetyIncidents` | 全件取得 (filter + cursor) |
| | `GetSafetyIncident` | 単一 item 取得 (by key_cd) |
| | `SearchSafetyIncidents` | キーワード検索 |
| | `ListNearby` | 地点付近の item |
| | `GetSafetyIncidentsAsGeoJSON` | GeoJSON 形式 bulk 取得 |
| `CrimeMapService` | `GetChoropleth` | 国別 count + color |
| | `GetHeatmap` | 個別 Point + weight |
| `UserProfileService` | `GetProfile` | Profile 全体取得 |
| | `ToggleFavoriteCountry` | お気に入り国切替 |
| | `UpdateNotificationPreference` | 通知設定更新 |
| | `RegisterFcmToken` | FCM token 登録 |

**ライフサイクル**: Cloud Run Service、`min=0 / max=3` スケーリング、Firebase ID Token 検証 interceptor を全 RPC 前段に配置。

**非責務**:
- Item の write（U-ING の責務、BFF は readonly）
- FCM 配信（U-NTF の責務）
- CMS スキーマ管理（U-CSS の責務）

### 1.2 Domain Model

#### 1.2.1 `safetyincident` BC（読取系）

`domain.SafetyIncident` は U-ING で定義済みの VO（19 フィールド）をそのまま使用。BFF は read-only なので追加の VO 定義は無し。

#### 1.2.2 `safetyincident/crimemap` subdomain

```go
// internal/safetyincident/crimemap/domain/

type CrimeMapFilter struct {
    LeaveFrom time.Time
    LeaveTo   time.Time
}

type CountryChoropleth struct {
    CountryCd   string
    CountryName string
    Count       int
    Color       string // hex "#rrggbb", サーバ側で count から計算
}

type HeatmapPoint struct {
    Location domain.Point
    Weight   float64 // MVP では 1.0 固定
}

type ChoroplethResult struct {
    Items []CountryChoropleth
    Total int
}

type HeatmapResult struct {
    Points          []HeatmapPoint
    ExcludedFallback int // country_centroid で除外された件数
}
```

**Color 計算**:
```go
// 5 段階グラデーション (count を quintile に分割)
func colorFromCount(count, max int) string {
    switch {
    case count == 0:       return "#f0f0f0"
    case count < max*1/5:  return "#fee5d9"
    case count < max*2/5:  return "#fcae91"
    case count < max*3/5:  return "#fb6a4a"
    case count < max*4/5:  return "#de2d26"
    default:               return "#a50f15"
    }
}
```

#### 1.2.3 `user` BC

```go
// internal/user/domain/

type UserProfile struct {
    UID                    string
    FavoriteCountryCds     []string
    NotificationPreference NotificationPreference
    FCMTokens              []string
}

type NotificationPreference struct {
    Enabled          bool
    TargetCountryCds []string
    InfoTypes        []string
}

// U-NTF の user_profile.go と同じ shape だが、所有は U-BFF (= user BC の
// オーナー)。U-NTF は読み手 + ArrayRemove on fcm_tokens のみ。
```

### 1.3 Port（Outbound）

#### 1.3.1 `safetyincident` 読取 Port

```go
// internal/safetyincident/domain/read_ports.go（新規、write port は U-ING が持つ）

type SafetyIncidentReader interface {
    List(ctx, filter ListFilter) ([]SafetyIncident, error)
    Get(ctx, keyCd string) (*SafetyIncident, error)
    Search(ctx, filter SearchFilter) ([]SafetyIncident, error)
    ListNearby(ctx, center Point, radiusKm float64, limit int) ([]SafetyIncident, error)
}

type ListFilter struct {
    AreaCd    string
    CountryCd string
    InfoTypes []string
    LeaveFrom time.Time
    LeaveTo   time.Time
    Limit     int
    Cursor    string
}
```

#### 1.3.2 `user` Port

```go
// internal/user/domain/ports.go

type ProfileRepository interface {
    Get(ctx, uid string) (*UserProfile, error)
    CreateIfMissing(ctx, profile UserProfile) error
    ToggleFavoriteCountry(ctx, uid, countryCd string) error
    UpdateNotificationPreference(ctx, uid string, pref NotificationPreference) error
    RegisterFcmToken(ctx, uid, token string) error
}

type AuthVerifier interface {
    // Verify validates a Firebase ID token and returns the authenticated
    // UID. Any error returned should have Kind=KindUnauthorized so the
    // ErrorInterceptor surfaces it as connect.CodeUnauthenticated.
    Verify(ctx context.Context, idToken string) (uid string, err error)
}
```

### 1.4 Application Layer

#### 1.4.1 `safetyincident/application/` 読取系 UseCase

```go
type ListUseCase struct    { reader SafetyIncidentReader; /* observability */ }
type GetUseCase struct     { reader SafetyIncidentReader; ... }
type SearchUseCase struct  { reader SafetyIncidentReader; ... }
type NearbyUseCase struct  { reader SafetyIncidentReader; ... }
type GeoJSONUseCase struct { reader SafetyIncidentReader; ... } // List 結果を GeoJSON にエンコード
```

各 UseCase は `reader` を呼ぶだけの薄いラッパ。認証は interceptor 側で済んでいるので UseCase 内で uid を扱う必要なし（読取は uid non-aware）。

#### 1.4.2 `safetyincident/crimemap/application.Aggregator`

```go
type Aggregator struct {
    reader domain.SafetyIncidentReader
    /* observability */
}

func (a *Aggregator) Choropleth(ctx, filter CrimeMapFilter) (ChoroplethResult, error) {
    items, err := a.reader.List(ctx, ListFilter{
        LeaveFrom: filter.LeaveFrom, LeaveTo: filter.LeaveTo,
        Limit: 10000, // MVP 上限
    })
    if err != nil { return ChoroplethResult{}, err }

    counts := map[string]CountryChoropleth{}
    for _, it := range items {
        entry := counts[it.CountryCd]
        entry.CountryCd = it.CountryCd
        entry.CountryName = it.CountryName
        entry.Count++
        counts[it.CountryCd] = entry
    }

    maxCount := 0
    for _, v := range counts { if v.Count > maxCount { maxCount = v.Count } }

    out := make([]CountryChoropleth, 0, len(counts))
    for _, v := range counts {
        v.Color = colorFromCount(v.Count, maxCount)
        out = append(out, v)
    }
    return ChoroplethResult{Items: out, Total: len(items)}, nil
}

func (a *Aggregator) Heatmap(ctx, filter CrimeMapFilter) (HeatmapResult, error) {
    items, err := a.reader.List(ctx, /* same filter */)
    // centroid fallback は除外 (精度低いため heatmap に乗せない)
    var pts []HeatmapPoint
    excluded := 0
    for _, it := range items {
        if it.GeocodeSource == domain.GeocodeSourceCountryCentroid {
            excluded++
            continue
        }
        pts = append(pts, HeatmapPoint{Location: it.Geometry, Weight: 1.0})
    }
    return HeatmapResult{Points: pts, ExcludedFallback: excluded}, nil
}
```

#### 1.4.3 `user/application/` 4 UseCase

```go
type GetProfileUseCase struct   { repo ProfileRepository }
type ToggleFavoriteUseCase      { repo ProfileRepository }
type UpdateNotificationUseCase  { repo ProfileRepository }
type RegisterFcmTokenUseCase    { repo ProfileRepository }
```

- `GetProfile` は初回時に `CreateIfMissing` を実行（新規 uid 用、MVP では空 Profile を lazy create）

### 1.5 Infrastructure

#### 1.5.1 `safetyincident/infrastructure/cms_reader.go`

```go
type CMSReader struct {
    client   *cmsx.Client
    projectAlias, modelAlias string
}

func (r *CMSReader) List(ctx, f ListFilter) ([]domain.SafetyIncident, error) {
    // cmsx.Client.ListItems(...) で filter 付き取得
    // Item → SafetyIncident 変換 (19 フィールド、U-ING の toFields の逆)
}

func (r *CMSReader) Get(ctx, keyCd string) (*SafetyIncident, error) {
    // cmsx.Client.GetItemByFieldValue (既存 API、U-CSS/U-ING で実装済み)
    // nil → KindNotFound
}

// Search / ListNearby / ... 同様
```

※ `cmsx.Client` の ListItems / クエリフィルタ機能は U-CSS の Item CRUD に読み取り系のメソッドを追加する形で実装。

#### 1.5.2 `user/infrastructure/firestore_profile_repo.go`

```go
type FirestoreProfileRepository struct {
    client     *firestore.Client
    collection string
}

func (r *FirestoreProfileRepository) Get(ctx, uid string) (*UserProfile, error) {
    doc, err := r.client.Collection(r.collection).Doc(uid).Get(ctx)
    if status.Code(err) == codes.NotFound {
        return nil, errs.Wrap(..., errs.KindNotFound, err)
    }
    // doc.DataTo(&profile) → UserProfile
}

func (r *FirestoreProfileRepository) CreateIfMissing(ctx, p UserProfile) error {
    // Firestore Create (失敗 = 既存 → ignore)
}

func (r *FirestoreProfileRepository) ToggleFavoriteCountry(ctx, uid, countryCd string) error {
    // Transaction: read → contains? remove : add → write
}

func (r *FirestoreProfileRepository) UpdateNotificationPreference(ctx, uid string, pref NotificationPreference) error {
    // Update 全置換 (map 形式)
}

func (r *FirestoreProfileRepository) RegisterFcmToken(ctx, uid, token string) error {
    // ArrayUnion (冪等)
}
```

#### 1.5.3 `user/infrastructure/firebase_auth_verifier.go`

```go
type FirebaseAuthVerifier struct {
    client *auth.Client // firebase.google.com/go/v4/auth
}

func (v *FirebaseAuthVerifier) Verify(ctx, idToken string) (string, error) {
    tok, err := v.client.VerifyIDToken(ctx, idToken)
    if err != nil {
        return "", errs.Wrap("auth.verify", errs.KindUnauthorized, err)
    }
    return tok.UID, nil
}
```

### 1.6 Interfaces Layer（`internal/interfaces/rpc/`）

#### 1.6.1 AuthInterceptor

```go
type AuthInterceptor struct {
    verifier domain.AuthVerifier
    logger   *slog.Logger
}

func (a *AuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
    return func(ctx, req) (resp, err) {
        authHeader := req.Header().Get("Authorization")
        if !strings.HasPrefix(authHeader, "Bearer ") {
            return nil, errs.Wrap("auth.missing_token", errs.KindUnauthorized, err)
        }
        idToken := strings.TrimPrefix(authHeader, "Bearer ")
        uid, err := a.verifier.Verify(ctx, idToken)
        if err != nil {
            return nil, err // already KindUnauthorized
        }
        ctx = authctx.WithUID(ctx, uid) // internal/shared/authctx/
        return next(ctx, req)
    }
}
```

#### 1.6.2 ErrorInterceptor（Q6 [A]）

```go
func NewErrorInterceptor(env string) connect.UnaryInterceptorFunc {
    maskInternal := env == "prod"
    return func(next) func(ctx, req) (resp, err) {
        resp, err := next(ctx, req)
        if err == nil { return resp, nil }

        code := toConnectCode(errs.KindOf(err))
        msg := err
        if maskInternal && (code == connect.CodeInternal || code == connect.CodeUnavailable) {
            msg = errors.New(safeMessage(code)) // "internal server error" 等
        }
        return nil, connect.NewError(code, msg)
    }
}
```

#### 1.6.3 Service Handlers

11 RPC を 3 Service に分けて実装:

```go
// interfaces/rpc/safety_incident_server.go
type SafetyIncidentServer struct {
    listUC   *application.ListUseCase
    getUC    *application.GetUseCase
    searchUC *application.SearchUseCase
    nearbyUC *application.NearbyUseCase
    geoJSONUC *application.GeoJSONUseCase
}

func (s *SafetyIncidentServer) GetSafetyIncident(
    ctx context.Context, req *connect.Request[v1.GetSafetyIncidentRequest],
) (*connect.Response[v1.GetSafetyIncidentResponse], error) {
    item, err := s.getUC.Execute(ctx, req.Msg.KeyCd)
    if err != nil { return nil, err }  // Interceptor が connect.Error に変換
    return connect.NewResponse(&v1.GetSafetyIncidentResponse{
        Item: toProto(item),
    }), nil
}
// ... 他 4 RPC

// interfaces/rpc/crimemap_server.go
type CrimeMapServer struct { aggregator *Aggregator }
// Choropleth / Heatmap

// interfaces/rpc/user_profile_server.go
type UserProfileServer struct { /* 4 UseCase */ }
// GetProfile / ToggleFavoriteCountry / UpdateNotificationPreference / RegisterFcmToken
```

### 1.7 Composition Root（`cmd/bff/main.go`）

```go
func run() error {
    cfg := loadConfig()
    ctx, cancel := signal.NotifyContext(...)
    defer cancel()

    // Observability
    shutdown, _ := observability.Setup(ctx, ...)
    defer shutdown(...)

    // Firebase App (shared by AuthVerifier + ProfileRepo)
    fbApp, _ := firebasex.NewApp(ctx, firebasex.Config{ProjectID: cfg.GCPProjectID})
    defer fbApp.Close(ctx)
    authClient, _ := fbApp.Auth(ctx) // 新規メソッド、Messaging / Firestore と同じパターン
    fsClient, _ := fbApp.Firestore(ctx)

    // CMS Client (read)
    cmsClient := cmsx.NewClient(cmsx.Config{BaseURL: ..., Token: ..., ...})
    defer cmsClient.Close(ctx)

    // Reader / Repositories
    cmsReader := safetyincident_infra.New(cmsClient, cfg.CMSProjectAlias, cfg.CMSModelAlias)
    profileRepo := user_infra.NewProfileRepo(fsClient, cfg.UsersCollection)
    authVerifier := user_infra.NewAuthVerifier(authClient)

    // UseCases
    listUC := application.NewListUseCase(cmsReader, ...)
    // ... 他の UseCase
    aggregator := crimemap_app.NewAggregator(cmsReader, ...)

    // Servers
    siServer := rpc.NewSafetyIncidentServer(listUC, getUC, searchUC, nearbyUC, geoJSONUC)
    cmServer := rpc.NewCrimeMapServer(aggregator)
    upServer := rpc.NewUserProfileServer(getProfileUC, toggleUC, updateUC, registerFcmUC)

    // Connect mux
    interceptors := connect.WithInterceptors(
        observability.TraceInterceptor(tracer),
        observability.MetricInterceptor(meter),
        rpc.NewErrorInterceptor(cfg.Env),
        rpc.NewAuthInterceptor(authVerifier, logger),
    )
    mux := http.NewServeMux()
    mux.Handle(safetymapv1connect.NewSafetyIncidentServiceHandler(siServer, interceptors))
    mux.Handle(safetymapv1connect.NewCrimeMapServiceHandler(cmServer, interceptors))
    mux.Handle(safetymapv1connect.NewUserProfileServiceHandler(upServer, interceptors))
    mux.HandleFunc("/healthz", healthHandler)

    // HTTP/2 server
    srv := connectserver.New(connectserver.Config{Port: cfg.Port}, mux, logger)
    return srv.Serve(ctx)
}
```

---

## 2. NFR Requirements（U-BFF 固有）

U-PLT の NFR を継承。以下は U-BFF 固有値。

### 2.1 性能

- **NFR-BFF-PERF-01**: 全 RPC の **p95 < 500ms**（認証 + CMS 読取 + レスポンス）
- **NFR-BFF-PERF-02**: `GetChoropleth` / `GetHeatmap` は MVP 規模（~1,000 item）で **p95 < 1s**
- **NFR-BFF-PERF-03**: Cold start **< 3s**（Firebase Admin SDK + cmsx + Firestore client 初期化）

### 2.2 セキュリティ

- **NFR-BFF-SEC-01**: 全 RPC で Firebase ID Token 検証（Q1 [A]）
- **NFR-BFF-SEC-02**: Token の失敗パターン（missing / invalid / expired）を区別した Metric
- **NFR-BFF-SEC-03**: Firestore `users/{uid}` の **Security Rules** で `request.auth.uid == userId` のみ許可（Infrastructure Design で apply）
- **NFR-BFF-SEC-04**: 内部エラーメッセージを prod で隠蔽（Q6 [A]）

### 2.3 信頼性

- **NFR-BFF-REL-01**: CMS が応答不能 → `CodeUnavailable` を返す（Flutter 側 retry）
- **NFR-BFF-REL-02**: Firestore が応答不能 → 同上
- **NFR-BFF-REL-03**: RPC 単位 idempotent（`ToggleFavoriteCountry` は transaction、`RegisterFcmToken` は `ArrayUnion`、`UpdateNotificationPreference` は絶対値 update で）

### 2.4 運用 / 可観測性

- **NFR-BFF-OPS-01**: 必須 slog 属性: `service.name=bff`, `env`, `trace_id`, `span_id`, `app.bff.phase`, `service` (Connect service name), `method` (RPC name), `uid`
- **NFR-BFF-OPS-02**: 4 Metric（§Q7）
- **NFR-BFF-OPS-03**: `/healthz` で 200 応答（probe 用）

### 2.5 テスト / 品質

カバレッジ目標（Q8 [A]）:
| 層 | 目標 |
|---|---|
| `safetyincident/crimemap/domain`, `user/domain` | 95%+ |
| `safetyincident/application`（読取）+ `crimemap/application` + `user/application` | 90%+ |
| `safetyincident/infrastructure/cms_reader` + `user/infrastructure/*` | 70%+ |
| `interfaces/rpc/*` | 80%+ |
| `interfaces/rpc/auth_interceptor` + `error_interceptor` | 90%+ |
| 全体 | 85%+ |

### 2.6 拡張性

- **NFR-BFF-EXT-01**: RPC 追加は `proto/v1/` に定義 → `buf generate` → Server struct に method 追加、で完結
- **NFR-BFF-EXT-02**: Cache 層の後付け（decorator pattern で UseCase に挟む）が容易

---

## 3. NFR Design Patterns（U-BFF 固有）

### 3.1 Interceptor Chain パターン（Q1 + Q6 + Q7 [A]）

```
request
  ├→ TraceInterceptor (OTel span 開始)
  ├→ MetricInterceptor (duration / status 記録)
  ├→ ErrorInterceptor (errs → Connect error 変換)
  ├→ AuthInterceptor (Firebase ID Token 検証、uid を ctx に格納)
  └→ Handler (UseCase 呼び出し)
```

**順序の意図**:
- Trace/Metric は最外層で、認証失敗も含めた全リクエストを観測
- ErrorInterceptor は handler から上に伝わる error を変換 → Metric が connect.Error を見られる
- Auth は最後、なぜなら handler が uid を必要とするため

### 3.2 Reader Port + in-memory Aggregator パターン（Q3 [A]）

- `SafetyIncidentReader` (domain) が CMS 読み取りの唯一の抽象
- `crimemap/application.Aggregator` が reader を使って in-memory で集計
- → **crimemap が独立した testable module** になる（fake reader で UnitTest）
- 将来 BigQuery 集計に切り替えるときは reader 実装を差替え

### 3.3 Firestore 共有 collection パターン（Q4 [A]）

```
Firestore users/{uid}:
  オーナー: U-BFF (user BC)
  読み手:   U-NTF (notification BC)
```

- U-BFF: Create / Update / ArrayUnion / ArrayRemove
- U-NTF: Read (purchase query) + ArrayRemove on fcm_tokens のみ
- スキーマ変更時は両 Unit の proto `UserProfile` / `NotificationPreference` + struct tag を同期

### 3.4 Error → Connect Code 自動変換（Q6 [A]）

- Handler は `errs.AppError` を return するだけ
- ErrorInterceptor が `errs.KindOf` → `connect.Code` に変換
- Prod では `KindInternal` / `KindExternal` のメッセージをマスク

### 3.5 冪等 Profile 書き込みパターン（Q5 + Q4 [A]）

- `RegisterFcmToken`: `ArrayUnion(token)` で冪等
- `ToggleFavoriteCountry`: transaction で「あれば削除 / なければ追加」（race-free）
- `UpdateNotificationPreference`: map update で絶対値上書き（冪等）
- `GetProfile`: 不在なら lazy create（NotFound error でなく空 Profile を返す）

---

## 4. 運用ランブック（簡略、詳細は Build and Test で）

### 4.1 Flutter からの接続確認

```bash
# Anonymous Auth で取得した ID Token を渡す
curl -X POST https://bff-xxx.run.app/overseasmap.v1.SafetyIncidentService/ListSafetyIncidents \
  -H "Authorization: Bearer $ID_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"filter":{"limit":10}}'
```

### 4.2 認証失敗の調査

- Cloud Logging で `app.bff.auth.failures` metric + `app.bff.phase=auth` log で reason を確認
  - `missing_token`: Flutter が Authorization header を付けていない
  - `invalid_token`: signature fail、Firebase プロジェクト不一致
  - `expired`: Token の有効期限切れ (Firebase は 1 時間で refresh)

### 4.3 CrimeMap の応答遅い

- `app.bff.rpc.duration{service=CrimeMapService}` p95 を監視
- CMS への List 呼び出しが重い → 期間を短く絞る、または pagination 実装

### 4.4 UserProfile が壊れた

- Firestore `users/{uid}` を直接確認
- Security Rules で read 制限、operator は Firebase Console で直接編集

---

## 5. 次フェーズ（Infrastructure Design）で決めること

本 design で決めない:
- Cloud Run Service の `min_instance_count` / `max_instance_count` の最終値
- Firestore Security Rules の YAML（`firestore.rules`）
- Runtime SA の IAM 最終確認（`datastore.user` / `firebase.sdkAdminServiceAgent` 相当 / `secretmanager.secretAccessor`）
- `firebasex.Auth(ctx)` ヘルパーの追加（既存の Firestore / Messaging と同じ pattern）
- env 詳細（U-ING/U-NTF と同じ方針で tuning は envconfig default）

---

## 6. トレーサビリティ

| 上位要件 | U-BFF 対応 |
|---|---|
| US-01〜US-13 (MVP Stories) | §1.1 11 RPC の実装 |
| NFR-SEC-04 (Firestore Security Rules) | §2.2 NFR-BFF-SEC-03 + Infrastructure で rules.yaml |
| NFR-OPS-*（構造化ログ、Metric） | §2.4 NFR-BFF-OPS-01/02 |
| NFR-PERF (Connect RPC レイテンシ) | §2.1 NFR-BFF-PERF-01/02/03 |
| NFR-REL (冪等性、Fail-open / Fail-close) | §2.3 NFR-BFF-REL-01/02/03 |

---

## 7. 承認プロセス

- 本ドキュメントの承認 → U-BFF Infrastructure Design
- Infra 後 → Code Gen 計画 + 実装 (PR A / PR B の 2 PR)
- Code Gen 後 → Build and Test runbook
