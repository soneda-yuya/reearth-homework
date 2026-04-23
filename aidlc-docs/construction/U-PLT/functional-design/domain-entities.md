# U-PLT Domain Entities (Minimal)

U-PLT には純ドメインエンティティは無いが、以下の **型マッピング** を確定させる。これらは他 Unit が実装時に参照する。

---

## 1. Proto ↔ Go 型マッピング

`buf generate` により `proto/v1/*.proto` から `gen/go/v1/*.pb.go` が自動生成される。Domain 層との橋渡しは **各 Context の infrastructure 層で実装する変換関数** が担う。U-PLT ではマッピング規約のみを定義する。

### 1.1 基本型対応

| proto 型 | Go 生成型 | domain 型（例: `safetyincident` Context） |
|---|---|---|
| `string` | `string` | `KeyCd` / `CountryCode` / `AreaCode` / `InfoType`（named string） |
| `int32` | `int32` | `int`（domain では int） |
| `double` | `float64` | `float64` |
| `bool` | `bool` | `bool` |
| `google.protobuf.Timestamp` | `*timestamppb.Timestamp` | `time.Time` |
| `overseasmap.v1.Point` | `*overseasmapv1.Point` | `safetyincident.LatLng{Lat, Lng}` |
| `overseasmap.v1.GeocodeSource` | `overseasmapv1.GeocodeSource` | `safetyincident.GeocodeSource`（enum） |
| `repeated T` | `[]T` | `[]DomainT` |

### 1.2 変換関数の配置

- **`infrastructure/cms/dto.go`**: CMS JSON レスポンス → domain
- **`interfaces/rpc/*_handler.go`**: domain → proto Response、proto Request → domain
- **`infrastructure/eventbus/converter.go`**: domain Event → proto Event

### 1.3 ゼロ値ポリシー

- proto で `optional` を付けない方針（Q1 [A]）のため、Go 生成型はすべて nil 不可の値型
- `Timestamp` は `*timestamppb.Timestamp` が nil の場合 `time.Time{}`（zero）に変換
- `Point` が nil の場合は Geometry 未設定として扱う（`GeocodeSource = Unspecified`）

---

## 2. `shared/errs.AppError` の構造

```go
package errs

type AppError struct {
    Op   string  // "cms.repository.get" のような操作識別子（ログで追跡用）
    Kind Kind    // "not_found" などの string
    Err  error   // Unwrap 対象（%w で包まれた原因）
}

func (e *AppError) Error() string {
    return fmt.Sprintf("%s: [%s] %v", e.Op, e.Kind, e.Err)
}

func (e *AppError) Unwrap() error { return e.Err }

// Wrap はエラーを AppError で包む。Kind が "" の場合は不明扱い。
func Wrap(op string, kind Kind, err error) error {
    if err == nil {
        return nil
    }
    return &AppError{Op: op, Kind: kind, Err: err}
}

// IsKind はエラーチェーン内に指定 Kind の AppError が含まれるか判定。
func IsKind(err error, kind Kind) bool {
    var ae *AppError
    for err != nil {
        if errors.As(err, &ae) && ae.Kind == kind {
            return true
        }
        err = errors.Unwrap(err)
    }
    return false
}

// KindOf はエラーチェーンで最初に見つかった Kind を返す。なければ KindUnknown。
func KindOf(err error) Kind {
    var ae *AppError
    if errors.As(err, &ae) {
        return ae.Kind
    }
    return KindUnknown
}
```

---

## 3. `platform/config.Config` の構造（共通部）

```go
package config

import (
    "net/url"
    "time"
)

type Config struct {
    // 共通
    ServiceName  string  `envconfig:"PLATFORM_SERVICE_NAME" required:"true"`
    Env          string  `envconfig:"PLATFORM_ENV" required:"true"`
    LogLevel     string  `envconfig:"PLATFORM_LOG_LEVEL" default:"INFO"`
    GCPProjectID string  `envconfig:"PLATFORM_GCP_PROJECT_ID" required:"true"`

    // Deployable 別は Config の埋め込み型として各 cmd/* で追加する
}
```

各 Deployable は `type AppConfig struct { config.Config; ... }` のように埋め込む。

```go
// 例: cmd/ingestion/main.go
type IngestionConfig struct {
    config.Config                                       // 共通
    MofaBaseURL            url.URL       `envconfig:"INGESTION_MOFA_BASE_URL" required:"true"`
    PubSubTopic            string        `envconfig:"INGESTION_PUBSUB_TOPIC" required:"true"`
    MapboxSecretName       string        `envconfig:"INGESTION_MAPBOX_SECRET_NAME" required:"true"`
    ClaudeSecretName       string        `envconfig:"INGESTION_CLAUDE_SECRET_NAME" required:"true"`
    CMSBaseURL             url.URL       `envconfig:"INGESTION_CMS_BASE_URL" required:"true"`
    CMSWorkspaceID         string        `envconfig:"INGESTION_CMS_WORKSPACE_ID" required:"true"`
    CMSTokenSecretName     string        `envconfig:"INGESTION_CMS_TOKEN_SECRET_NAME" required:"true"`
    IngestionMode          string        `envconfig:"INGESTION_MODE" default:"new-arrival"`  // "initial" | "new-arrival"
    HTTPTimeout            time.Duration `envconfig:"INGESTION_HTTP_TIMEOUT" default:"30s"`
}
```

---

## 4. `shared/clock.Clock` インターフェイス

```go
package clock

import "time"

type Clock interface {
    Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// テスト用: 固定時刻を返す
type FixedClock struct {
    FixedTime time.Time
}

func (c FixedClock) Now() time.Time { return c.FixedTime.UTC() }
```

- 原則として **UTC で統一**
- ドメインで `time.Now()` を直接呼ばず、`Clock` 経由で取得する（テスト容易性・PBT で時間を固定できる）

---

## 5. `platform/observability` の公開型

```go
package observability

import (
    "context"
    "log/slog"

    "go.opentelemetry.io/otel/metric"
    "go.opentelemetry.io/otel/trace"
)

type Config struct {
    ServiceName string
    Env         string
    LogLevel    string
}

// Setup は slog ロガー・Tracer・Meter を初期化し、終了用関数を返す
func Setup(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error)

// Logger はコンテキストに紐づく slog.Logger を返す（With で追加された属性を反映）
func Logger(ctx context.Context) *slog.Logger

// Tracer は OTel Tracer を返す
func Tracer(ctx context.Context) trace.Tracer

// Meter は OTel Meter を返す
func Meter(ctx context.Context) metric.Meter

// With は context に属性を埋め、以降の Logger 取得で自動付与
func With(ctx context.Context, key string, value any) context.Context
```

---

## 6. Platform SDK ラッパー（インターフェイスのみ定義、実装は U-PLT 実装時に詳細）

```go
// platform/cmsx
package cmsx

import "context"

type Config struct {
    BaseURL     string  // CMS インスタンス URL
    WorkspaceID string
    Token       string  // 実値（Secret Manager から取得済み）
    Timeout     time.Duration
}

type Client interface {
    // Integration API メソッド群（詳細は Application Design の C-05 参照）
}

func NewClient(cfg Config) (Client, error)

// platform/firebasex
package firebasex

type Config struct {
    ProjectID           string
    ServiceAccountJSON  []byte  // Secret Manager 経由、nil なら ADC を利用
}

type App struct{ /* Auth / Firestore / FCM の共通基盤 */ }

func NewApp(ctx context.Context, cfg Config) (*App, error)

// platform/pubsubx
package pubsubx

type Config struct {
    ProjectID string
}

type Client struct{ /* Pub/Sub クライアント wrapper */ }

func NewClient(ctx context.Context, cfg Config) (*Client, error)

// platform/mapboxx
package mapboxx

type Config struct {
    AccessToken string  // Secret Manager 経由
    Timeout     time.Duration
}

type Client struct{ /* Mapbox Geocoding 等のラッパー */ }

func NewClient(cfg Config) *Client

// platform/connectserver
package connectserver

type Server struct{ /* http.Server + connect handler mux */ }

func New(handlers []ConnectHandler, interceptors []connect.Interceptor, port int) *Server
func (s *Server) Start(ctx context.Context) error
```

---

## 7. リポジトリ構造の最終確定（U-PLT として一次生成する範囲）

```
overseas-safety-map/
├── cmd/
│   ├── ingestion/    (空の main.go、U-ING で実装)
│   ├── bff/          (空の main.go、U-BFF で実装)
│   ├── notifier/     (空の main.go、U-NTF で実装)
│   └── cmsmigrate/   (空の main.go、U-CSS で実装)
├── internal/
│   ├── platform/
│   │   ├── config/          ✅ U-PLT
│   │   ├── observability/   ✅ U-PLT
│   │   ├── connectserver/   ✅ U-PLT
│   │   ├── pubsubx/         ✅ U-PLT
│   │   ├── cmsx/            ✅ U-PLT（実装の雛形、詳細は U-CSS/ING/BFF で追加）
│   │   ├── firebasex/       ✅ U-PLT
│   │   └── mapboxx/         ✅ U-PLT
│   └── shared/
│       ├── errs/            ✅ U-PLT
│       ├── clock/           ✅ U-PLT
│       └── validate/        ✅ U-PLT
├── proto/
│   └── v1/
│       ├── common.proto     ✅ U-PLT
│       ├── safetymap.proto  ✅ U-PLT（全サービス骨子）
│       └── pubsub.proto     ✅ U-PLT
├── gen/go/v1/               ✅ buf generate で自動生成
├── buf.yaml                 ✅ U-PLT
├── buf.gen.yaml             ✅ U-PLT
├── go.mod
├── tools.go                 ✅ U-PLT（buf, connect-go, protoc-gen-go）
└── .github/workflows/       ✅ U-PLT（ci.yml、再利用可能ワークフロー）
```

**U-PLT で一次生成する:**
- 上記の ✅ 全て
- ただし Context の `domain` / `application` / `infrastructure` パッケージは空のディレクトリのみ（各 Unit で実装）

---

## 受け入れ条件（Sign-off）

- [ ] proto 3 ファイルが定義済みで `buf generate` が成功
- [ ] `shared/errs.AppError` と Wrap/IsKind/KindOf が単体テストで動作
- [ ] `shared/clock.Clock` が SystemClock / FixedClock を提供
- [ ] `platform/config.Config` の envconfig 読み込みが動作、必須欠落で panic
- [ ] `platform/observability.Setup` が呼び出せ、stdout に構造化ログが出る
- [ ] Platform SDK factory の `NewClient` / `NewApp` がコンパイルでき、最小接続テスト（CI で疎通可能なものはスタブ）
