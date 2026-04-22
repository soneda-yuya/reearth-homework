# U-PLT NFR Design Patterns

U-PLT で採用する **共通パターン** 一式。ここで決めたものは **他 Unit すべてが踏襲する共通規約** となる。

---

## 1. Resilience — パニック復帰 + グレースフルシャットダウン（Q1 [A]）

### 1.1 Panic Recovery Middleware パターン

**適用箇所**: `interfaces/rpc`（Connect Interceptor）と `interfaces/job`（Runner）の両方。

**仕組み**:
```go
// platform/observability/recovery.go
func RecoverInterceptor() connect.UnaryInterceptorFunc {
    return func(next connect.UnaryFunc) connect.UnaryFunc {
        return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
            defer func() {
                if r := recover(); r != nil {
                    stack := debug.Stack()
                    observability.Logger(ctx).Error("panic recovered",
                        "error.kind", errs.KindInternal,
                        "error.message", fmt.Sprintf("%v", r),
                        "error.stack", string(stack),
                    )
                    meter := observability.Meter(ctx)
                    cnt, _ := meter.Int64Counter("app.panic.count")
                    cnt.Add(ctx, 1)
                    err = errs.Wrap("panic", errs.KindInternal, fmt.Errorf("%v", r))
                }
            }()
            return next(ctx, req)
        }
    }
}
```

Job Runner は同じパターンで `ctx := context.Background()` をラップしたループ内に置く。

### 1.2 Graceful Shutdown パターン

**起動フロー（`cmd/*/main.go` の雛形）**:

```go
func main() {
    cfg := loadConfig()  // 必須欠落で panic
    shutdown, err := observability.Setup(context.Background(), observability.Config{...})
    if err != nil { log.Fatal(err) }

    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    server := buildServer(cfg)  // 依存注入
    errCh := make(chan error, 1)
    go func() { errCh <- server.Start(ctx) }()

    select {
    case <-ctx.Done():
        // SIGTERM / SIGINT — Graceful shutdown
        slog.Info("shutdown initiated")
        shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancelShutdown()
        _ = server.Stop(shutdownCtx)   // 進行中リクエスト完了待ち
        _ = shutdown(shutdownCtx)       // OTel flush + close
        os.Exit(0)
    case err := <-errCh:
        slog.Error("server error", "err", err)
        _ = shutdown(context.Background())
        os.Exit(1)
    }
}
```

- **タイムアウト: 10 秒**（Cloud Run の `--timeout` と整合、SIGTERM → SIGKILL の最大猶予と同じ）
- 進行中リクエストが 10 秒以内に終わらない場合は強制終了（Cloud Run 側）

**パターン名**: *Context Cancellation + Drain*

---

## 2. Resilience — Retry / Backoff（Q2 [A]）

### 2.1 Exponential Backoff with Jitter

**共通ヘルパ**: `platform/retry/retry.go`

```go
package retry

type Policy struct {
    MaxAttempts int           // 3
    Initial     time.Duration // 500ms
    Max         time.Duration // 8s
    Multiplier  float64       // 2.0
    Jitter      float64       // 0.25（±25%）
}

var DefaultPolicy = Policy{
    MaxAttempts: 3, Initial: 500*time.Millisecond,
    Max: 8*time.Second, Multiplier: 2.0, Jitter: 0.25,
}

// Do は op を policy に従って再試行する。
// errs.Kind が KindExternal または KindInternal の transient なものだけ再試行。
// context キャンセル / ctx.Err() で即停止。
func Do(ctx context.Context, policy Policy, op func(ctx context.Context) error) error { ... }

// ShouldRetry はデフォルトの再試行判定（HTTP 5xx / 429 / ネットワークエラー）
func ShouldRetry(err error) bool { ... }
```

### 2.2 再試行ポリシーマトリクス

| Adapter | メソッド | 再試行 | 備考 |
|---|---|:-:|---|
| Mapbox Geocoding | `GET /geocoding/v6/...` | ✓ | 冪等、`5xx` / `429` / タイムアウト |
| Claude | `POST /v1/messages` | ✓ | 冪等扱い（同じ入力で複数回呼んでもコストが増えるだけ、結果はほぼ同じ） |
| CMS `ListItems` / `GetItem` | GET | ✓ | — |
| CMS `CreateItem` / `UpdateItem` | POST / PATCH | ✗ | 冪等キーなし。重複を避けるため再試行しない |
| Firestore `Get` / `List` | 冪等 | ✓ | Firebase SDK 自身も再試行するため二重は避ける。SDK 任せ |
| FCM `Send` | POST | ✓ | FCM API が冪等キー（`message.name`）を扱うため |
| Pub/Sub Publisher | gRPC | ✓ | SDK 内蔵、`retry.Do` は使わない |
| Pub/Sub Subscriber | — | N/A | SDK の nack/ack 挙動に従う |

### 2.3 再試行の観測性

- 各再試行で `slog.Warn("retrying", "attempt", n, "delay_ms", d, "endpoint", ep)` を記録
- メトリクス: `app.retry.count{endpoint, result=success|fail}`

---

## 3. Health Check パターン（Q3 [A]）

### 3.1 2-tier Health Check

**`platform/connectserver` が自動付与する 2 エンドポイント**:

| パス | 目的 | 実装 |
|---|---|---|
| `GET /healthz` | liveness（プロセスが生きている） | 常に `200 OK\nok\n`、外部依存のチェックなし |
| `GET /readyz` | readiness（依存先疎通 OK） | 下記 Prober を順に呼び、すべて OK なら 200、どれか失敗なら 503 + 失敗先のサマリー |

### 3.2 Readiness Prober インターフェイス

```go
// platform/connectserver/readiness.go
type Prober interface {
    Name() string
    Probe(ctx context.Context) error  // 1 秒以内に返す
}

type ReadinessHandler struct {
    probers []Prober
    timeout time.Duration  // 3 秒（全 prober の合計）
}

func (h *ReadinessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

**組み込み Prober**:
- `cmsx.Prober{Client: client}` — CMS の `/health` または workspace 取得で疎通
- `firebasex.Prober{App: app}` — Auth client の `VerifyIDToken("")` でダミー呼び出し
- `pubsubx.Prober{Client: client}` — Topic 存在確認（該当サービスのみ）

### 3.3 Cloud Run 設定

- `--startup-probe=/healthz`（起動確認）
- `--liveness-probe=/healthz`（稼働確認）
- Cloud Run 自体は `/readyz` を直接参照しないが、Load Balancer または外部監視ツールで利用

**起動時の挙動**: Secret 取得完了までは `/readyz` を 503 にし、Cloud Run のトラフィックルーティングが開始される前に ready 化を保証する。

---

## 4. 外部 API Rate Limiting（Q4 [A]）

### 4.1 Token Bucket パターン

**ライブラリ**: `golang.org/x/time/rate`（標準拡張、依存軽量）

**共通ヘルパ**: `platform/ratelimit/ratelimit.go`

```go
package ratelimit

type Limiter struct {
    inner *rate.Limiter
    name  string
}

func New(rpm int, burst int, name string) *Limiter {
    return &Limiter{
        inner: rate.NewLimiter(rate.Limit(float64(rpm)/60.0), burst),
        name:  name,
    }
}

// Allow は即時判定。超過時は errs.KindExternal を返す（ブロックしない）。
func (l *Limiter) Allow() error {
    if !l.inner.Allow() {
        return errs.Wrap("ratelimit", errs.KindExternal,
            fmt.Errorf("%s rate limit exceeded", l.name))
    }
    return nil
}
```

### 4.2 デフォルト値（Infrastructure Design で env 調整可能）

| サービス | RPM | Burst | 根拠 |
|---|:-:|:-:|---|
| Claude | 10 | 3 | LLM 推論コスト保護、Haiku クラスで実用十分 |
| Mapbox Geocoding | 600 | 10 | 無料枠（100k req/month）の日割り ≒ 2 RPM、有料想定で 600 |
| reearth-cms | 60 | 5 | 自ホスト SaaS の許容値を保守的に |
| Firebase Auth `Verify` | 制限なし | — | Firebase 側が管理 |
| FCM | 制限なし | — | Firebase 側が管理 |

### 4.3 超過時の観測性

- `slog.Warn("ratelimit exceeded", "service", name, "rpm", rpm)`
- メトリクス: `app.ratelimit.exceeded.count{service}`

### 4.4 Circuit Breaker（将来の拡張）

MVP では未採用。外部サービスの連続失敗 → Open にする Circuit Breaker は将来導入可能（`sony/gobreaker` 想定）。`platform/retry.ShouldRetry` と合わせて拡張する。

---

## 5. SDK Client Lifecycle（Q5 [A]）

### 5.1 Per-process Singleton パターン

各 SDK Client は **`cmd/*/main.go` で 1 回生成 → DI** で配布。リクエスト毎の生成はしない。

**理由**:
- Firebase Admin: 初期化に ADC 解決 / JWKS 取得など重い処理を伴う（~100ms 以上）
- Pub/Sub / Firestore: gRPC コネクション維持（HTTP/2 multiplexing）
- HTTP Client: `http.DefaultTransport` が持つコネクションプール再利用

**DI フレームワークは使わない** — 手動 Constructor Injection（`main.go` が長くなるがシンプル）。

### 5.2 Shutdown パターン

すべてのシングルトンは **Closer インターフェイス** を公開:

```go
type Closer interface {
    Close(ctx context.Context) error
}
```

`cmd/*/main.go` の終了時に **逆順で Close** を呼ぶ（LIFO）。`observability.Setup` の返り値 `shutdown` が集約する仕組みでも可。

### 5.3 Client の共有範囲

| Client | 共有 |
|---|---|
| `firebasex.App` | BFF / Notifier / （future: API で Firestore 直接書き） |
| `pubsubx.Client` | Ingestion（publisher）/ Notifier（subscriber）|
| `cmsx.Client` | Ingestion / BFF / Setup 各自で生成（Deployable 毎に独立、同じ型を使う）|
| `mapboxx.Client` | Ingestion のみ |

---

## 6. Observability パターン（Functional Design から継続）

- ログ: `slog` JSON、共通 9 属性自動付与、ドメイン属性は `observability.With(ctx, k, v)` で埋め込み
- メトリクス: OpenTelemetry Metrics、起動・panic・外部 API・retry・rate limit のカウンタ
- トレース: OpenTelemetry Traces、W3C Trace Context、Connect Interceptor / Pub/Sub メッセージ属性で伝播

詳細は [Functional Design §2](../functional-design/business-logic-model.md) を参照。

---

## 7. Security パターン

### 7.1 Secret Manager Resolver（起動時）

`platform/config` の拡張として、環境変数値が `projects/*/secrets/*/versions/*` の形式なら **自動で Secret Manager から解決**:

```go
// platform/config/secretresolver.go
func ResolveSecrets(ctx context.Context, cfg any) error {
    // struct tag `secret:"true"` を付けた string フィールドを
    // Secret Manager リソース名として解決し、実値に書き換える
}
```

### 7.2 PII Redaction

`shared/errs` に redaction helper を提供:

```go
func Redact(s string) string {
    if len(s) <= 8 { return "[REDACTED]" }
    return s[:2] + "..." + s[len(s)-2:]  // 両端 2 文字のみ
}
```

ログ・エラーメッセージ組み立て時に token / email をこれで包む。

### 7.3 Auth Interceptor（BFF のみ）

`user.AuthVerifier` を使う Connect Interceptor を `interfaces/rpc/auth_interceptor.go` で実装（U-PLT では interface のみ、実装は U-USR で）。

---

## 8. パターン一覧（Quick Reference）

| # | パターン | 名称 | 配置 |
|---|---|---|---|
| 1 | Panic Recovery Middleware | `RecoverInterceptor` | `platform/observability` |
| 2 | Graceful Shutdown | Context Cancellation + Drain | `cmd/*/main.go` 雛形 |
| 3 | Exponential Backoff + Jitter | `retry.Do` | `platform/retry` |
| 4 | 2-tier Health Check | `/healthz` + `/readyz` | `platform/connectserver` |
| 5 | Token Bucket Rate Limit | `ratelimit.Limiter` | `platform/ratelimit` |
| 6 | Per-process Singleton | SDK client 生成パターン | `cmd/*/main.go` |
| 7 | Secret Manager Resolver | `config.ResolveSecrets` | `platform/config` |
| 8 | PII Redaction | `errs.Redact` | `shared/errs` |
| 9 | Structured Log w/ Context | `observability.Logger(ctx)` | `platform/observability` |
| 10 | OTel Trace Propagation | Connect + Pub/Sub middleware | `platform/observability` |
