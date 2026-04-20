# サービスレイヤ設計 — overseas-safety-map

「サービス」は **コンポーネントをオーケストレーションし、1つのユースケースを完結させる振る舞い単位** として定義する。Construction フェーズの Functional Design で、各サービスのビジネスルール（閾値・分岐・計算）を詳細化する。

---

## サーバー側サービス一覧

### S-01: `IngestionService`（取り込みオーケストレーション）

**責務**: MOFA XML フィードから安全情報を取り込み、地名抽出 → ジオコーディング → CMS 保存 → Pub/Sub 発行までを1トランザクションで完結させる。

**公開インターフェイス**:
```go
package ingestion

type Mode int
const (
    ModeInitial Mode = iota  // /area/00A.xml
    ModeNewArrival           // /area/newarrivalA.xml
)

type Service interface {
    Run(ctx context.Context, mode Mode) (RunReport, error)
}

type RunReport struct {
    Fetched      int
    Skipped      int  // 既存 keyCd で未変更
    Created      int
    GeocodeMiss  int  // フォールバックに頼った件数
    LLMMiss      int  // 地名抽出で "" が返った件数
    Errors       []error
    Elapsed      time.Duration
}
```

**依存**: C-01 MofaXmlClient / C-02 LocationExtractor / C-03 Geocoder / C-04 SafetyIncidentRepository / C-06 Publisher / C-13 Observability

**協調フロー（概略、詳細は Functional Design で決定）**:
1. モードに応じて `mofa.Client.FetchAll` または `FetchNewArrivals`
2. 各 MailItem について `repo.Exists(keyCd)` で重複判定
3. 新規のみ `extractor.Extract` → `geocoder.Geocode` → `repo.Upsert`
4. 保存成功したものを `publisher.PublishNewArrival`
5. 集計レポートを返す

**横断**: OpenTelemetry Span を `ingestion.run` で 1 本、各 MailItem 処理は `ingestion.process` 子 Span で囲む。


### S-02: `NotifierService`（通知配信オーケストレーション）

**責務**: Pub/Sub から受け取った新着メッセージを対象ユーザーに FCM 配信する。

**公開インターフェイス**:
```go
package notifier

type Service interface {
    // Start は Pub/Sub Subscriber を起動しブロッキング。
    Start(ctx context.Context) error
}
```

**依存**: C-06 Subscriber / C-07 UserStore + FcmSender / C-13 Observability

**協調フロー**:
1. `Subscriber.Start` で NewArrivalMessage を受信
2. `UserStore.ListSubscribersFor(countryCd, infoType)` で対象 UserProfile 列を取得
3. 各 UserProfile の `NotificationPreference` を評価し、有効な FCM トークンを集める
4. `FcmSender.Send` で配信
5. `SendReport.InvalidTokens` を `UserStore` から除去

**横断**: リトライ指数バックオフ、失敗時は Pub/Sub の自動再配信に委ねる（冪等性担保のため `keyCd + uid` の配信記録を Firestore に保存するかは Functional Design で決定）。


### S-03: `BffApiService`（Flutter 向け API）

**責務**: Connect サーバーとして Flutter からのリクエストを受け、認証後に repository / aggregator を経由してデータを返す。

**公開インターフェイス**: `proto/v1/safetymap.proto` に定義（[component-methods.md](./component-methods.md) 参照）

**依存**: C-04 Repository / C-07 AuthVerifier + UserStore / C-08 CrimeMapAggregator / C-13 Observability

**協調フロー**（例: ListSafetyIncidents）:
1. Connect Interceptor が `Authorization: Bearer <idToken>` から ID Token を抽出
2. `AuthVerifier.Verify` で Uid を取得、`context.Context` に注入
3. ハンドラは認証済み `ctx` と リクエストで `repo.List` を呼ぶ
4. DTO 変換して Connect Response を返す

**横断**: 各 RPC は OTel Span、slog は Request ID（Connect 側で自動付与）と Uid を attr に含める。


### S-04: `CmsSetupService`（CMS 初期化）

**責務**: CMS 上に Project / Model / Field が存在しなければ作成する。冪等実行。

**公開インターフェイス**:
```go
package setup

type Runner interface {
    EnsureSchema(ctx context.Context) error
}
```

**依存**: C-05 ReearthCmsClient / C-13 Observability

**協調フロー**:
1. 環境変数で受け取った WorkspaceID / Integration Token を初期化
2. Project が存在しなければ作成
3. `SafetyIncident` Model が存在しなければ作成
4. Model の Field 定義（FR-CMS-05 の一覧）が揃っていなければ追加
5. Result をログ出力

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

### シーケンス 1: 新着取り込み→通知配信

```
Scheduler(GitHub Actions) -> IngestionService.Run(NewArrival)
  mofa.FetchNewArrivals() --> [MailItem]
  for each MailItem:
    if not repo.Exists(keyCd):
      llm.Extract() --> location
      geocoder.Geocode(location, country) --> (LatLng, Source)
      repo.Upsert(SafetyIncident)
      pub.PublishNewArrival(msg)  // Pub/Sub へ
  return RunReport

Pub/Sub -> NotifierService.Handle(msg)
  userStore.ListSubscribersFor(countryCd, infoType) --> [UserProfile]
  filter by NotificationPreference
  tokens = collect FCM tokens
  fcm.Send(tokens, Notification{keyCd, title})
  userStore.RemoveInvalidTokens(SendReport.InvalidTokens)
```

### シーケンス 2: Flutter アプリから一覧取得

```
Flutter(MapViewModel) -> Connect client -> BffService.ListSafetyIncidents
  Interceptor: verify Firebase ID Token --> VerifiedUser
  repo.List(filter) --> ListResult
  map DTO
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
      repo.get(keyCd) via BFF
  View renders
```

---

## トランザクション・一貫性の方針

- **ingestion の 1 件処理は "Create → Publish" の擬似トランザクション**: CMS 保存に成功したあと Pub/Sub publish が失敗した場合、次回実行時に `repo.Exists(keyCd)` で保存済みと判定されるため Publish が漏れる。対策は以下のいずれかを Functional Design で選択:
  - (a) 保存時に `published=false` カラムを立て、Publish 成功で `true` に更新、次回起動時に未 Publish を再送
  - (b) Outbox テーブル（別 Model）に Publish 予定を書き、Outbox Worker が拾って Publish
  - (c) 軽量運用: CMS 保存 → Publish の順序でベストエフォート、重複は冪等通知で許容
- **通知配信の冪等**: 同一 (keyCd, uid) への重複配信を避けるため、Firestore 上に `notification_logs/{keyCd}_{uid}` を保存する案を Functional Design で検討。
- **Flutter 側の状態一貫性**: Connect の単一 RPC = 単一 UI 遷移。複数 RPC が必要な場面は UseCase 層でコンポジションする。
