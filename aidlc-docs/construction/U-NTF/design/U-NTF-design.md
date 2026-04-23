# U-NTF Design (Minimal 合本版)

**Unit**: U-NTF（Notifier Unit、Sprint 4）
**Deployable**: `cmd/notifier`（Cloud Run Service、Pub/Sub Push Subscription のターゲット）
**Bounded Context**: `notification`（Supporting domain）
**ワークフロー圧縮**: Option B（Functional Design + NFR Requirements + NFR Design 1 本に集約）

---

## 0. Design Decisions（計画回答の確定）

[`U-NTF-design-plan.md`](../../plans/U-NTF-design-plan.md) Q1-Q8 すべて **[A]** で確定。

| # | 決定事項 | 選択 | 要旨 |
|---|---------|------|------|
| Q1 | Pub/Sub 受信方式 | **A** | **Push Subscription**（Cloud Run Service + HTTP handler） |
| Q2 | Dedup 戦略 | **A** | **Firestore `notifier_dedup` collection + TTL 24h** |
| Q3 | 購読者解決 | **A** | `country_cd` 一致 + `enabled=true` の Firestore query + `info_types` in-memory filter |
| Q4 | FCM 配信 | **A** | **`SendMulticast`** でユーザーごとに複数 token 一括配信 |
| Q5 | 無効 token 除去 | **A** | **同一 Request 内で Firestore `ArrayRemove`** |
| Q6 | エラーハンドリング | **A** | HTTP status code 細かく使い分け（200/400/500） |
| Q7 | Observability | **A** | Span 4 + Metric 6 + `app.notifier.phase` 属性ログ |
| Q8 | テスト戦略 | **A** | 層別カバレッジ（domain 95%/app 90%/infra 70%/全体 85%）、SDK 部分は Port 抽象 + fake |

---

## 1. Functional Design

### 1.1 Context — U-NTF の責務

**目的**: U-ING が publish した `NewArrivalEvent` を受け、Firestore の購読者に FCM push 通知を配信する。無効 token はそのリクエスト内で Firestore から除去する。

**ライフサイクル**: Cloud Run Service（常駐、request-driven autoscaling、`min_instance_count=0`）。Pub/Sub Push Subscription が HTTP POST を `/pubsub/push` に送ってくるたびに起動。

**非責務**:
- 通知メッセージの内容設計（Flutter アプリ側の UI テキスト生成ではなく、`NewArrivalEvent` の data フィールドを pass-through）
- ユーザー設定の編集（U-BFF の責務）
- イベントの生成（U-ING の責務）

### 1.2 Domain Model（`internal/notification/domain`）

#### 1.2.1 VO / Entity

```go
// UserProfile は notification 視点での Firestore user ドキュメントの部分ビュー。
// U-BFF の user BC と同じ Firestore collection を読み取る (書き込みは U-BFF のみ)。
type UserProfile struct {
    UID                    string
    FCMTokens              []string
    NotificationPreference NotificationPreference
}

type NotificationPreference struct {
    Enabled           bool
    TargetCountryCds  []string // 通知を受けたい国
    InfoTypes         []string // 受信する info_type (空 = 全種別)
}

// Subscriber は「このイベントの配信対象ユーザー + 有効 token リスト」。
// DeliverUseCase が Firestore query + in-memory filter した結果。
type Subscriber struct {
    UID    string
    Tokens []string
}

// FCMMessage は 1 ユーザー向けの multicast 送信 payload。
type FCMMessage struct {
    Tokens  []string
    Title   string
    Body    string
    Data    map[string]string // key_cd / country_cd / info_type
}

// BatchResult は FCM SendMulticast の結果。
type BatchResult struct {
    SuccessCount int
    FailureCount int
    Invalid      []string // 永続的に無効 (registration-token-not-registered など) な token
    Transient    []string // 一時的に失敗 (QUOTA_EXCEEDED 等) な token
}
```

#### 1.2.2 Port（Outbound）

```go
// Dedup は Q2 [A] の重複排除ストア。
type Dedup interface {
    // CheckAndMark は key_cd がまだ処理されていなければマーク (TTL 24h) し、
    // alreadySeen=false を返す。既に存在するなら alreadySeen=true を返す
    // (マークは触らない)。transactional に動くこと。
    CheckAndMark(ctx context.Context, keyCd string) (alreadySeen bool, err error)
}

// UserRepository は Firestore user コレクションの読み取り + 無効 token 除去。
type UserRepository interface {
    FindSubscribers(ctx context.Context, countryCd, infoType string) ([]UserProfile, error)
    RemoveInvalidTokens(ctx context.Context, uid string, tokens []string) error
}

// FCMClient は Firebase Admin SDK の薄いラッパ。SendMulticast のみ使う。
type FCMClient interface {
    SendMulticast(ctx context.Context, msg FCMMessage) (BatchResult, error)
}

// EventDecoder は Pub/Sub push envelope から NewArrivalEvent を取り出す。
// domain から infrastructure (pub/sub envelope parser) への DIP 境界。
type EventDecoder interface {
    Decode(body []byte) (NewArrivalEvent, error)
}

// NewArrivalEvent は U-ING が publish するイベントの domain 表現。
// proto / JSON shape は infrastructure 側で解釈、domain はフィールドのみ知る。
type NewArrivalEvent struct {
    KeyCd     string
    CountryCd string
    InfoType  string
    Title     string
    Lead      string
    LeaveDate time.Time
}
```

### 1.3 Application Layer（`internal/notification/application`）

#### 1.3.1 `DeliverNotificationUseCase`

```go
type DeliverNotificationUseCase struct {
    dedup    domain.Dedup
    users    domain.UserRepository
    fcm      domain.FCMClient
    logger   *slog.Logger
    tracer   trace.Tracer
    meter    metric.Meter
    // counters
    receivedCounter        metric.Int64Counter
    dedupedCounter         metric.Int64Counter
    recipientsHistogram    metric.Int64Histogram
    fcmSentCounter         metric.Int64Counter
    tokenInvalidatedCounter metric.Int64Counter
}

type DeliverInput struct {
    Event domain.NewArrivalEvent
}

type DeliverOutcome int
const (
    OutcomeDelivered      DeliverOutcome = iota // 通常配信完了
    OutcomeDeduped                              // dedup hit (200 OK、早期 return)
    OutcomeNoSubscribers                        // 購読者 0 (200 OK)
)

type DeliverResult struct {
    Outcome              DeliverOutcome
    RecipientsCount      int
    FCMSuccessTokens     int
    FCMFailedTokens      int
    InvalidTokensRemoved int
}

func (u *DeliverNotificationUseCase) Execute(ctx, DeliverInput) (DeliverResult, error)
```

**アルゴリズム**:

```
1. receivedCounter.Add(1, attrs: country_cd, info_type)
2. alreadySeen = dedup.CheckAndMark(event.KeyCd)
   if err → return err (500 に変換)
   if alreadySeen → return {Outcome: Deduped} (200)
3. users = userRepository.FindSubscribers(event.CountryCd, event.InfoType)
   if err → return err (500)
   if len(users) == 0 → return {Outcome: NoSubscribers} (200)
4. recipientsHistogram.Record(len(users))
5. 並列 (限定度 5) で各ユーザーに SendMulticast:
   - msg = buildFCMMessage(event, user.FCMTokens)
   - result = fcm.SendMulticast(ctx, msg)
   - if err → 部分失敗、WARN ログのみ (transient は FCMClient 実装側で retry 済み)
   - result.SuccessCount / FailureCount をカウント
   - result.Invalid が非空 → users.RemoveInvalidTokens(user.UID, result.Invalid)
     - ArrayRemove はノンブロッキング (次回 Run で効く程度で十分)
     - tokenInvalidatedCounter.Add(len(result.Invalid))
6. return {Outcome: Delivered, RecipientsCount, ...}
```

**エラーハンドリング**（Q6 [A]）:

| シナリオ | Execute が返すもの | HTTP handler が返す status |
|---|---|---|
| 正常配信 | `{Delivered}, nil` | 200 |
| Dedup hit | `{Deduped}, nil` | 200 |
| 購読者 0 | `{NoSubscribers}, nil` | 200 |
| FCM 部分失敗 | `{Delivered, FailedTokens > 0}, nil` | 200 (本流は成功) |
| Firestore dedup 失敗 (transient) | `{}, err(KindExternal)` | 500 |
| Firestore users query 失敗 | `{}, err(KindExternal)` | 500 |

**Observability**:
- Span: `notifier.Deliver` (root) → `dedup.CheckAndMark` / `users.FindSubscribers` / `fcm.SendMulticast` (per user) / `users.RemoveInvalidTokens` (per user when needed)
- Metric: §1.3.1 の 6 個
- Log: `app.notifier.phase` 属性 (`receive` / `dedup` / `resolve` / `send` / `cleanup` / `done`)

### 1.4 Interfaces Layer — HTTP Handler（`internal/interfaces/job/notifier_runner.go`）

```go
// NotifierHandler は Pub/Sub Push endpoint 本体。
type NotifierHandler struct {
    decoder  domain.EventDecoder
    usecase  *application.DeliverNotificationUseCase
    logger   *slog.Logger
}

// ServeHTTP は POST /pubsub/push の handler。
// Q6 [A] の status code 戦略を実装する。
func (h *NotifierHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "read body", http.StatusInternalServerError)
        return
    }
    defer r.Body.Close()

    event, err := h.decoder.Decode(body)
    if err != nil {
        // Malformed payload は即 DLQ (400)
        h.logger.ErrorContext(ctx, "malformed push payload",
            "app.notifier.phase", "receive",
            "err", err,
        )
        http.Error(w, "malformed payload", http.StatusBadRequest)
        return
    }

    result, err := h.usecase.Execute(ctx, application.DeliverInput{Event: event})
    if err != nil {
        // Transient error → Pub/Sub に retry させる (500)
        h.logger.ErrorContext(ctx, "deliver failed",
            "app.notifier.phase", "failed",
            "key_cd", event.KeyCd,
            "err", err,
        )
        http.Error(w, "deliver failed", http.StatusInternalServerError)
        return
    }

    // 成功 / Dedup hit / 購読者 0 は全て 200
    h.logger.InfoContext(ctx, "notifier finished",
        "app.notifier.phase", "done",
        "outcome", result.Outcome.String(),
        "key_cd", event.KeyCd,
        "recipients", result.RecipientsCount,
        "fcm_success", result.FCMSuccessTokens,
        "fcm_failed", result.FCMFailedTokens,
        "invalidated", result.InvalidTokensRemoved,
    )
    w.WriteHeader(http.StatusOK)
}
```

### 1.5 Infrastructure Adapters（`internal/notification/infrastructure`）

#### 1.5.1 `dedup/firestore.go`

```go
type FirestoreDedup struct {
    client *firestore.Client
    col    *firestore.CollectionRef // notifier_dedup
    ttl    time.Duration            // 24h
}

// CheckAndMark は transaction で既存チェック + 無ければ書き込み。
// Firestore の TTL は expireAt フィールドに基づく自動削除 (Terraform で
// TTL policy を col に適用)。
func (d *FirestoreDedup) CheckAndMark(ctx context.Context, keyCd string) (bool, error) {
    ref := d.col.Doc(keyCd)
    alreadySeen := false
    err := d.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
        doc, err := tx.Get(ref)
        if err == nil && doc.Exists() {
            alreadySeen = true
            return nil
        }
        if status.Code(err) != codes.NotFound {
            return err
        }
        return tx.Set(ref, map[string]interface{}{
            "expireAt": time.Now().Add(d.ttl),
        })
    })
    return alreadySeen, err
}
```

Terraform:
```
# firestore TTL policy (modules/shared/firestore.tf 追加)
resource "google_firestore_field" "notifier_dedup_ttl" {
  project    = var.project_id
  database   = "(default)"
  collection = "notifier_dedup"
  field      = "expireAt"
  ttl_config {}
}
```

#### 1.5.2 `userrepo/firestore.go`

```go
type FirestoreUserRepository struct {
    client *firestore.Client
    col    *firestore.CollectionRef // users
}

func (r *FirestoreUserRepository) FindSubscribers(ctx context.Context, countryCd, infoType string) ([]UserProfile, error) {
    it := r.col.
        Where("notification_preference.enabled", "==", true).
        Where("notification_preference.target_country_cds", "array-contains", countryCd).
        Documents(ctx)

    var out []UserProfile
    for {
        doc, err := it.Next()
        if err == iterator.Done { break }
        if err != nil { return nil, err }

        var u UserProfile
        if err := doc.DataTo(&u); err != nil { return nil, err }
        u.UID = doc.Ref.ID

        // In-memory info_type filter
        if len(u.NotificationPreference.InfoTypes) > 0 &&
           !slices.Contains(u.NotificationPreference.InfoTypes, infoType) {
            continue
        }
        if len(u.FCMTokens) == 0 {
            continue // 対象ユーザーだが端末 token 無し
        }
        out = append(out, u)
    }
    return out, nil
}

func (r *FirestoreUserRepository) RemoveInvalidTokens(ctx, uid string, tokens []string) error {
    _, err := r.col.Doc(uid).Update(ctx, []firestore.Update{{
        Path:  "fcm_tokens",
        Value: firestore.ArrayRemove(tokens...),
    }})
    return err
}
```

Firestore インデックス:
```
// modules/shared/firestore.tf
resource "google_firestore_index" "users_notification" {
  project    = var.project_id
  database   = "(default)"
  collection = "users"
  fields {
    field_path = "notification_preference.enabled"
    order      = "ASCENDING"
  }
  fields {
    field_path   = "notification_preference.target_country_cds"
    array_config = "CONTAINS"
  }
}
```

#### 1.5.3 `fcm/firebase.go`

```go
type FirebaseFCM struct {
    client *messaging.Client  // firebase.google.com/go/v4/messaging
}

func (f *FirebaseFCM) SendMulticast(ctx context.Context, msg domain.FCMMessage) (domain.BatchResult, error) {
    res, err := f.client.SendEachForMulticast(ctx, &messaging.MulticastMessage{
        Tokens:       msg.Tokens,
        Notification: &messaging.Notification{Title: msg.Title, Body: msg.Body},
        Data:         msg.Data,
    })
    if err != nil {
        return domain.BatchResult{}, errs.Wrap("fcm.send_multicast", errs.KindExternal, err)
    }

    out := domain.BatchResult{
        SuccessCount: res.SuccessCount,
        FailureCount: res.FailureCount,
    }
    for i, r := range res.Responses {
        if r.Error == nil { continue }
        if messaging.IsRegistrationTokenNotRegistered(r.Error) ||
           messaging.IsInvalidArgument(r.Error) {
            out.Invalid = append(out.Invalid, msg.Tokens[i])
        } else {
            out.Transient = append(out.Transient, msg.Tokens[i])
        }
    }
    return out, nil
}
```

#### 1.5.4 `eventdecoder/pubsub_envelope.go`

```go
// Pub/Sub Push envelope:
// {
//   "message": { "data": "<base64>", "attributes": {...}, "messageId": "..." },
//   "subscription": "projects/.../subscriptions/..."
// }

type PubSubEnvelopeDecoder struct{}

func (PubSubEnvelopeDecoder) Decode(body []byte) (domain.NewArrivalEvent, error) {
    var env struct {
        Message struct {
            Data       string            `json:"data"`
            Attributes map[string]string `json:"attributes"`
        } `json:"message"`
    }
    if err := json.Unmarshal(body, &env); err != nil {
        return domain.NewArrivalEvent{}, errs.Wrap("decoder.envelope", errs.KindInvalidInput, err)
    }
    data, err := base64.StdEncoding.DecodeString(env.Message.Data)
    if err != nil {
        return domain.NewArrivalEvent{}, errs.Wrap("decoder.base64", errs.KindInvalidInput, err)
    }
    var inner struct {
        KeyCd     string `json:"key_cd"`
        CountryCd string `json:"country_cd"`
        InfoType  string `json:"info_type"`
        // ... title/lead/leave_date
    }
    if err := json.Unmarshal(data, &inner); err != nil {
        return domain.NewArrivalEvent{}, errs.Wrap("decoder.inner_json", errs.KindInvalidInput, err)
    }
    return domain.NewArrivalEvent{
        KeyCd: inner.KeyCd, CountryCd: inner.CountryCd, InfoType: inner.InfoType,
        // ...
    }, nil
}
```

### 1.6 Composition Root — `cmd/notifier/main.go`

```go
type notifierConfig struct {
    config.Common
    Port                     string `envconfig:"NOTIFIER_PORT" default:"8080"`
    PubSubSubscription       string `envconfig:"NOTIFIER_PUBSUB_SUBSCRIPTION" required:"true"`
    FirestoreDedupCollection string `envconfig:"NOTIFIER_DEDUP_COLLECTION" default:"notifier_dedup"`
    FirestoreDedupTTLHours   int    `envconfig:"NOTIFIER_DEDUP_TTL_HOURS" default:"24"`
    FirestoreUsersCollection string `envconfig:"NOTIFIER_USERS_COLLECTION" default:"users"`
    FCMConcurrency           int    `envconfig:"NOTIFIER_FCM_CONCURRENCY" default:"5"`
    ShutdownGraceSeconds     int    `envconfig:"NOTIFIER_SHUTDOWN_GRACE_SECONDS" default:"10"`
}

func main() { if err := run(); err != nil { slog.Error("notifier failed", "err", err); os.Exit(1) } }

func run() error {
    // config / observability setup / signal ctx
    // Firebase Admin SDK init (app = firebase.NewApp(ctx))
    // firestore.NewClient(ctx, projectID)
    // messaging.NewClient(app) で FCM client
    // decoder / dedup / userrepo / fcm / usecase / handler を組み立て
    // http.Server を起動、signal で graceful shutdown
    return nil
}
```

### 1.7 Sequence

```
Pub/Sub (U-ING が publish)
   ↓ Push
POST https://notifier-xxx.run.app/pubsub/push
   ↓
NotifierHandler.ServeHTTP
   ↓
decoder.Decode(envelope)        ← parse fail → 400
   ↓
usecase.Execute(event)
   ↓
dedup.CheckAndMark(key_cd)      ← transient fail → 500
   ↓ (alreadySeen=false)
userRepo.FindSubscribers(country, infoType)  ← fail → 500
   ↓ (len == 0)  → 200 (NoSubscribers)
   ↓ (len > 0)
並列 (concurrency=5) per user:
  fcm.SendMulticast(user, event)
    ↓ (result.Invalid 非空)
  userRepo.RemoveInvalidTokens(uid, invalid)
   ↓
200 OK (Delivered)
```

---

## 2. NFR Requirements（U-NTF 固有）

U-PLT の NFR を継承。以下は U-NTF 固有値。

### 2.1 性能

- **NFR-NTF-PERF-01**: Pub/Sub push 1 request の処理時間 **p95 < 3 秒**（dedup 1 Firestore tx ~100ms + users query ~200ms + 並列 FCM 500ms〜1s + token cleanup ~100ms）
- **NFR-NTF-PERF-02**: Cloud Run の `ack_deadline = 60s` に余裕を持って収まる（100+ 購読者の大規模 event でも並列度 5 で 2-3 秒）
- **NFR-NTF-PERF-03**: Cold start 時間 **< 2 秒**（Firestore / FCM client 初期化を含む）

### 2.2 セキュリティ

- **NFR-NTF-SEC-01**: Cloud Run `INGRESS_TRAFFIC_ALL` だが IAM で **Pub/Sub service agent のみ** `run.invoker`、その他は 401/403
- **NFR-NTF-SEC-02**: Firebase Admin SDK credentials は **ADC**（Cloud Run の Runtime SA）経由、Secret Manager 不要
- **NFR-NTF-SEC-03**: Runtime SA（`notifier-runtime`）の最小権限:
  - `roles/datastore.user`（Firestore R/W）
  - `roles/firebase.messaging` は不要（FCM は Firebase Admin SDK + Firestore 経由でメッセージを送る、ADC で十分）
  - Pub/Sub は **不要**（receiver なのでロール不要、Service Agent 側が push する）

### 2.3 信頼性 / 冪等性

- **NFR-NTF-REL-01**: 重複メッセージで通知が 2 回届かないこと（Firestore dedup、Q2 [A]）
- **NFR-NTF-REL-02**: transient error は Pub/Sub が retry（subscription の `retry_policy`、max backoff 600s）→ 最終的に DLQ
- **NFR-NTF-REL-03**: FCM 部分失敗（一部 token invalid）は Run 全体としては成功扱い、**失敗 token は次回 Run 前に Firestore から除去済み**

### 2.4 運用 / 可観測性

- **NFR-NTF-OPS-01**: 必須 slog 属性: `service.name=notifier`, `env`, `trace_id`, `span_id`, `app.notifier.phase`, `key_cd`（対象イベント）
- **NFR-NTF-OPS-02**: OTel Metric 6 種（§1.3.1）
- **NFR-NTF-OPS-03**: HTTP status code 別メトリック `app.notifier.http.response_total{status=xxx, reason=...}`（Cloud Monitoring アラート用）
- **NFR-NTF-OPS-04**: DLQ topic の監視（DLQ にメッセージが入ったら運用アラート）

### 2.5 テスト / 品質

- **NFR-NTF-TEST-01**: Domain VO の unit test（UserProfile / BatchResult / Subscriber）→ **95%+**
- **NFR-NTF-TEST-02**: `DeliverNotificationUseCase` を fake 実装 5 種（Dedup / UserRepo / FCM / Decoder）で **5 シナリオ**（初回配信 / dedup hit / 購読者 0 / 部分失敗 / transient error）→ **90%+**
- **NFR-NTF-TEST-03**: HTTP handler は `httptest.Server` で status code 別パス検証（200×3, 400, 500）→ **70%+**
- **NFR-NTF-TEST-04**: 実 Firestore / 実 FCM の疎通は Build and Test で手動、事前の unit / application レイヤは SDK 非依存

### 2.6 拡張性

- **NFR-NTF-EXT-01**: FCM 以外の配信チャネル（メール / Slack）は `FCMClient` と同型の `Notifier` port を追加すれば対応可能
- **NFR-NTF-EXT-02**: Dedup ストアの差し替え（Redis 等）は `Dedup` port 経由で無痛

---

## 3. NFR Design Patterns（U-NTF 固有）

### 3.1 Transactional Dedup パターン（Q2 [A]）

Firestore transaction で「get + create if absent」をアトミックに実行。2 並行 request が同じ `key_cd` を同時処理しても、Firestore 側で serialize されるので **片方が必ず `alreadySeen=true` を受け取る**。

```
t1: tx.Get(key_cd)           → NotFound
t2:                           tx.Get(key_cd) → NotFound
t1: tx.Set(key_cd, expireAt)  → commit OK
t2:                           tx.Set(key_cd, ...) → retry → tx.Get → exists → alreadySeen=true
```

**性質**:
- Safety: 重複配信ゼロ（TTL 24h 内）
- Liveness: transient failure は Firestore SDK が retry

### 3.2 HTTP Status Code 戦略（Q6 [A]）

U-CSS / U-ING にはないパターン。Pub/Sub Push との契約を status code で表現:

| 戦略 | 値 |
|---|---|
| 成功 / 既処理 / 購読者ゼロ | 200（ACK） |
| malformed payload | 400（即 DLQ） |
| transient error | 500（retry → DLQ） |

Pub/Sub の意図と HTTP semantics を揃えることで、app コードは「status code = 結果」でシンプルに書ける。

### 3.3 Skip-on-invalid-token パターン（Q5 [A]）

FCM `BatchResponse` の `Responses[i].Error` を見て invalid 判定 → **同じ request 内で Firestore `ArrayRemove`**。累積させないことでストレージ / 配信コストの両方を抑える。

### 3.4 Port / Adapter 分離（Hexagonal）

```
┌─────────── cmd/notifier/main.go ─────────────────────┐
│   HTTP server + NotifierHandler                       │
│       ↓                                               │
│   DeliverNotificationUseCase                          │
│       ↓ (Ports)                                       │
│   Dedup / UserRepository / FCMClient / EventDecoder   │
└───────┬──────────┬──────────┬─────────┬──────────────┘
        ↓          ↓          ↓         ↓
  Firestore   Firestore   Firebase   PubSub
  (dedup)     (users)     Admin SDK   envelope
                          (FCM)       (JSON)
```

Application 層は外部 SDK に直接依存しない。各 infrastructure は interface 経由で plug-in。

### 3.5 並列度制御 + 無効 token 即時除去

```go
// Application 層で errgroup + semaphore (concurrency=5)
var eg errgroup.Group
eg.SetLimit(u.concurrency)
for _, user := range users {
    user := user
    eg.Go(func() error {
        result, err := u.fcm.SendMulticast(ctx, buildFCMMessage(event, user))
        // ... 個別失敗は log + metric のみ、error は return しない
        if len(result.Invalid) > 0 {
            _ = u.users.RemoveInvalidTokens(ctx, user.UID, result.Invalid)
        }
        return nil
    })
}
_ = eg.Wait()
```

並列度 5 は LLM / Mapbox と同じ conventional value（NFR-ING-PERF と整合）。FCM のレート制限は per-project で 3.6M msg/min なので app 側 rate limit は不要。

---

## 4. 運用ランブック（簡略、詳細は Build and Test で）

### 4.1 通常運用

Pub/Sub が自動 push。運用者の操作不要。

### 4.2 障害時

1. Cloud Logging で `severity=ERROR` / `severity=WARN` を確認
2. `app.notifier.phase` で失敗段階を特定
3. **DLQ topic（`safety-incident.new-arrival.dlq`）にメッセージが溜まっているか** 確認
4. DLQ の中身を目視で確認、必要なら手動 replay（`gcloud pubsub subscriptions pull notifier-dlq-sub`）

### 4.3 Firestore TTL 設定（初回のみ）

Terraform 初回 apply 時に `google_firestore_field` で `expireAt` の TTL policy を作成。設定後、対応 collection のドキュメントは自動で削除される（最大 24 時間の遅延あり）。

### 4.4 通知コピーの変更

`cmd/notifier/main.go` 内の `buildFCMMessage(event)` 関数で title/body を生成。変更は PR + デプロイで反映。

---

## 5. 次フェーズ（Infrastructure Design）で決めること

本 design で決めない:

- Cloud Run Service の `min_instance_count` / `max_instance_count` の調整（雛形は 0/2）
- `scaling.max_concurrent_requests_per_instance`（雛形は default = 80）
- Pub/Sub Subscription の `ack_deadline_seconds` / `retry_policy` / `dead_letter_policy` の最終値
- Firestore TTL Terraform resource の追加
- Firestore インデックス（`users` の複合 index）の追加
- Runtime SA に付与する roles の最終確認

---

## 6. トレーサビリティ

| 上位要件 | U-NTF 対応 |
|---|---|
| US-11/12/13（通知受信） | §1.3 DeliverNotificationUseCase |
| NFR-FUN-05（配信信頼性） | §2.3 REL、§3.1 Transactional Dedup |
| NFR-SEC-*（ADC / 最小権限） | §2.2 SEC-01-03 |
| NFR-OPS-*（構造化ログ、Metric） | §2.4 OPS-01-04 |
| NFR-REL-*（at-least-once dedup、DLQ） | §3.1, §3.2 HTTP status |

---

## 7. 承認プロセス

- **本ドキュメントの承認**: ユーザーレビュー → LGTM で次へ
- **次ステップ**: U-NTF Infrastructure Design（Firestore TTL / インデックス / Cloud Run / Subscription 最終値）
