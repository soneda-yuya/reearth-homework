# サービスレイヤ設計 — overseas-safety-map

DDD では **「サービス」≒ Application Service（UseCase）** を中心に記述する。各 Bounded Context の `application` レイヤに配置し、同 Context の `domain` の Port（I/F）をコンストラクタ注入で受け取って動作する。Construction フェーズの Functional Design で、各 UseCase のビジネスルール（閾値・分岐・計算）を詳細化する。

---

## サーバー側 Application Service 一覧

### S-01: `IngestUseCase`（`safetyincident.application`）

**責務**: MOFA XML フィードから安全情報を取り込み、地名抽出 → ジオコーディング → CMS 保存 → Pub/Sub 発行までを 1 処理サイクルで完結させる。

**公開インターフェイス**（抜粋）:
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

type IngestUseCase struct {
    source    safetyincident.MofaSource
    extractor safetyincident.LocationExtractor
    geocoder  safetyincident.Geocoder
    repo      safetyincident.Repository
    publisher safetyincident.EventPublisher
    clock     clock.Clock
}
func (u *IngestUseCase) Run(ctx context.Context, mode IngestMode) (IngestReport, error)
```

**依存 Port**: `MofaSource` / `LocationExtractor` / `Geocoder` / `Repository` / `EventPublisher`（すべて `safetyincident.domain`）、`shared/clock.Clock`

**協調フロー（概略）**:
1. モードに応じて `MofaSource.FetchAll` または `FetchNewArrivals`
2. 各 `MailItem` について `Repository.Exists(keyCd)` で重複判定
3. 新規のみ `LocationExtractor.Extract` → `Geocoder.Geocode` → `Repository.Upsert`
4. 保存成功したものを `EventPublisher.PublishNewArrival(NewArrivalEvent)`
5. `IngestReport` を返す

**横断**: `platform/observability` により `ingestion.run` / `ingestion.process` の 2 階層 Span を生成。

**起動元**: `internal/interfaces/job.IngestionRunner` → `cmd/ingestion/main.go`


### S-02: `DispatchOnNewArrivalUseCase`（`notification.application`）

**責務**: `NewArrivalMessage` を受け、`DispatchPolicy` で絞った購読者へ FCM 配信する。

**公開インターフェイス**:
```go
package notificationapp

type DispatchOnNewArrivalUseCase struct {
    store  notification.SubscriberStore
    sender notification.PushSender
    policy notification.DispatchPolicy
}
func (u *DispatchOnNewArrivalUseCase) Execute(ctx context.Context, msg notification.NewArrivalMessage) error
```

**依存 Port**: `SubscriberStore` / `PushSender` / `DispatchPolicy`（すべて `notification.domain`）

**協調フロー**:
1. `SubscriberStore.ListSubscribersFor(countryCode, infoType)` で購読候補を取得
2. `DispatchPolicy.ShouldDeliver` で各 Subscriber を評価し、有効な FCM トークンを集約
3. `PushSender.Send(tokens, Notification{title, keyCd, deeplink})`
4. `SendReport.InvalidTokens` を `SubscriberStore.RemoveInvalidTokens` へ流す

**Consumer 起動**: `notification.domain.NewArrivalConsumer` → 実装は `infrastructure/eventbus.PubSubConsumer`。`interfaces/job.NotifierRunner` が `consumer.Start(ctx, usecase.Execute)` を呼び Pub/Sub を購読する。

**横断**: 冪等配信の判定（`notification_logs/{keyCd}_{uid}`）の採否は Functional Design で決定。


### S-03: BFF Application Services（`safetyincident.application` と `user.application`）

BFF は Connect サーバのハンドラ群であり、**単一の "Service" ではなく複数の UseCase を組み合わせる**。

```go
// safetyincident.application
ListUseCase.Execute(ctx, ListFilter) (ListResult, error)
GetUseCase.Execute(ctx, KeyCd) (*SafetyIncident, error)
SearchUseCase.Execute(ctx, SearchQuery) (ListResult, error)
NearbyUseCase.Execute(ctx, LatLng, float64) (ListResult, error)

// safetyincident/crimemap.application
GetChoroplethUseCase.Execute(ctx, AggregateFilter) ([]CountryChoropleth, error)
GetHeatmapUseCase.Execute(ctx, AggregateFilter) ([]HeatmapPoint, error)

// user.application
GetProfileUseCase.Execute(ctx, Uid) (*UserProfile, error)
ToggleFavoriteCountryUseCase.Execute(ctx, Uid, CountryCode) (UserProfile, error)
UpdateNotificationPrefUseCase.Execute(ctx, Uid, NotificationPref) error
RegisterFcmTokenUseCase.Execute(ctx, Uid, FcmToken) error
```

**認証ライン**: `interfaces/rpc.AuthInterceptor` が `user.AuthVerifier` で ID Token を検証し、`VerifiedUser` を context に注入。各 Handler は context から Uid を取り出して UseCase に渡す。

**依存 Port**: 各 UseCase は **同 Context の domain Port のみ** に依存する（Context 越境は `interfaces/rpc` の組み立てで吸収）。


### S-04: `EnsureSchemaUseCase`（`cmsmigrate.application`）

**責務**: `SchemaDefinition`（宣言的定義）を reearth-cms に冪等適用する。

**公開インターフェイス**:
```go
package cmsmigrateapp

type EnsureSchemaUseCase struct {
    applier *cmsapplier.SchemaApplier
    def     cmsmigrate.SchemaDefinition
}
func (u *EnsureSchemaUseCase) Execute(ctx context.Context) error
```

**協調フロー**:
1. Project が存在しなければ `SchemaApplier.Apply` で作成（Adapter 内の低レベル呼び出し）
2. Model が存在しなければ作成
3. Field が不足していれば追加
4. 既存と一致すれば no-op

**起動元**: `internal/interfaces/job.SetupRunner` → `cmd/cmsmigrate/main.go`

---

## Flutter 側サービス（ユースケース層）

Flutter の `domain/usecases/` 配下に実装する。ViewModel から呼び出され、Repository を組み合わせる。

### US-01: `ListSafetyIncidentsUseCase`
- **入力**: `SafetyIncidentFilter?`
- **出力**: `List<SafetyIncident>`
- **依存**: `SafetyIncidentRepository`

### US-02: `GetSafetyIncidentDetailUseCase`
- **入力**: `keyCd`
- **出力**: `SafetyIncident`
- **依存**: `SafetyIncidentRepository`

### US-03: `ListNearbyUseCase`
- **入力**: `LatLng`, `radiusKm`
- **出力**: `List<SafetyIncident>`
- **依存**: `SafetyIncidentRepository` + `LocationService`（現在地取得）

### US-04: `SearchSafetyIncidentsUseCase`
- **入力**: `SafetyIncidentFilter`
- **出力**: `List<SafetyIncident>`
- **依存**: `SafetyIncidentRepository`

### US-05: `GetCrimeMapDataUseCase`
- **入力**: `DateRange?`, `ZoomLevel`
- **出力**: `CrimeMapData`（choropleth or heatmap）
- **依存**: `SafetyIncidentRepository`（Choropleth / Heatmap API）

### US-06: `ToggleFavoriteCountryUseCase`
- **入力**: `countryCode`
- **出力**: 更新後の `List<CountryCode>`
- **依存**: `UserProfileRepository`

### US-07: `UpdateNotificationPreferenceUseCase`
- **入力**: `NotificationPreference`
- **出力**: なし（成功／失敗）
- **依存**: `UserProfileRepository` + FCM トークン更新

### US-08: `SignInUseCase` / `SignUpUseCase` / `SignOutUseCase`
- **依存**: `AuthRepository`

### US-09: `ObserveAuthStateUseCase`
- **出力**: `Stream<AuthUser?>`
- **依存**: `AuthRepository`

### US-10: `HandleNotificationTapUseCase`
- **入力**: 通知ペイロード（`keyCd`）
- **出力**: 詳細画面への遷移命令（`Router` 経由）
- **依存**: `Router` + `SafetyIncidentRepository`（存在確認）

---

## サービス間オーケストレーション（シーケンス概略）

### シーケンス 1: 新着取り込み→通知配信（Context 間は Domain Event）

```
Scheduler(GitHub Actions) -> cmd/ingestion -> interfaces/job.IngestionRunner
  safetyincident.application.IngestUseCase.Run(NewArrival)
    MofaSource.FetchNewArrivals() --> [MailItem]          (infrastructure/mofa)
    for each MailItem:
      if not Repository.Exists(keyCd):                    (infrastructure/cms)
        LocationExtractor.Extract() --> locationText      (infrastructure/llm)
        Geocoder.Geocode(locationText, countryCd)
          --> (LatLng, GeocodeSource)                     (infrastructure/geocode: Mapbox→Centroid)
        Repository.Upsert(SafetyIncident)                 (infrastructure/cms)
        EventPublisher.PublishNewArrival(NewArrivalEvent) (infrastructure/eventbus → Pub/Sub)
    return IngestReport

Pub/Sub -> cmd/notifier -> interfaces/job.NotifierRunner
  notification.domain.NewArrivalConsumer.Start()          (infrastructure/eventbus)
  on message:
    notification.application.DispatchOnNewArrivalUseCase.Execute(msg)
      SubscriberStore.ListSubscribersFor(countryCd, infoType)  (infrastructure/firestore)
      DispatchPolicy.ShouldDeliver(sub, ...)                    (domain)
      collect FCM tokens
      PushSender.Send(tokens, Notification{keyCd, title})       (infrastructure/fcm)
      SubscriberStore.RemoveInvalidTokens(SendReport.InvalidTokens)
```

### シーケンス 2: Flutter アプリから一覧取得

```
Flutter(MapViewModel) -> Connect client -> cmd/bff -> interfaces/rpc.AuthInterceptor
  user.AuthVerifier.Verify(idToken) --> VerifiedUser       (user.infrastructure/firebaseauth)
  inject VerifiedUser into context
  interfaces/rpc.SafetyIncidentHandler.ListSafetyIncidents
    safetyincident.application.ListUseCase.Execute(ctx, ListFilter)
      Repository.List(filter) --> ListResult               (safetyincident.infrastructure/cms)
    map domain → proto DTO
    return ListSafetyIncidentsResponse
Flutter(MapViewModel) <- AsyncValue<MapState>
View re-renders
```

### シーケンス 3: 通知タップで詳細画面へ

```
FCM -> Flutter(FCM handler) -> Router.push("/detail/{keyCd}")
  if not authenticated: Router.push("/login?redirect=/detail/{keyCd}")
  DetailViewModel.build(keyCd)
    GetSafetyIncidentDetailUseCase.execute(keyCd)
      SafetyIncidentRepository.get(keyCd)  (Flutter data layer → Connect → BFF → safetyincident.application.GetUseCase → Repository)
  View renders（出典表記・フォールバック注記を含む）
```

---

## トランザクション・一貫性の方針

- **ingestion の 1 件処理は "Create → Publish" の擬似トランザクション**: CMS 保存に成功したあと Pub/Sub publish が失敗した場合、次回実行時に `Repository.Exists(keyCd)` で保存済みと判定されるため Publish が漏れる。対策は以下のいずれかを Functional Design で選択:
  - (a) 保存時に `published=false` フィールドを立て、Publish 成功で `true` に更新、次回起動時に未 Publish を再送
  - (b) Outbox パターン — `safetyincident.domain` に `OutboxRepository` Port を追加、`infrastructure/cms` or 別ストアで実装、Outbox Worker が拾って Publish
  - (c) 軽量運用: CMS 保存 → Publish の順序でベストエフォート、重複は冪等通知で許容
- **通知配信の冪等**: 同一 `(keyCd, uid)` への重複配信を避けるため、`notification.domain` に `DeliveryLedger` Port を追加し、Firestore で `notification_logs/{keyCd}_{uid}` として実装する案を Functional Design で検討。
- **Flutter 側の状態一貫性**: Connect の単一 RPC = 単一 UI 遷移。複数 RPC が必要な場面は Flutter の UseCase 層でコンポジションする。
