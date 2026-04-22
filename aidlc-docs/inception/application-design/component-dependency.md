# 依存関係とデータフロー — overseas-safety-map

## 1. 全体依存図（コンポーネントレベル）

```mermaid
graph LR
    subgraph External["外部サービス"]
        MOFA["MOFA XML<br/>area/{00A,newarrivalA}.xml"]
        Claude["Anthropic Claude<br/>Haiku"]
        Mapbox["Mapbox<br/>Geocoding API"]
        CMS["reearth-cms<br/>Integration REST API"]
        FB["Firebase<br/>Auth / Firestore / FCM"]
        PS["Cloud Pub/Sub"]
    end

    subgraph GoRepo["Go サーバーモノレポ (reearth-homework) — DDD"]
        subgraph CmdLayer["cmd/* (Composition Roots)"]
            CI["cmd/ingestion"]
            CB["cmd/bff"]
            CN["cmd/notifier"]
            CS["cmd/setup"]
        end

        subgraph Interfaces["internal/interfaces"]
            RPC["interfaces/rpc<br/>(Connect Handlers +<br/>AuthInterceptor)"]
            JOB["interfaces/job<br/>(Ingestion/Notifier/Setup Runner)"]
        end

        subgraph SafetyCtx["Context: safetyincident (Core)"]
            SI_APP["application<br/>(Ingest/List/Get/Search/Nearby UseCase)"]
            SI_DOM["domain<br/>(SafetyIncident Aggregate, Ports:<br/>MofaSource/LocationExtractor/Geocoder/Repository/EventPublisher)"]
            SI_INF["infrastructure<br/>(mofa/cms/llm/geocode/eventbus adapters)"]
            subgraph CrimeSub["Subdomain: crimemap"]
                CM_APP["application<br/>(Choropleth/Heatmap UseCase)"]
                CM_DOM["domain<br/>(Aggregator, InfoTypePolicy)"]
                CM_INF["infrastructure<br/>(RepositoryAggregator)"]
            end
        end

        subgraph UserCtx["Context: user (Supporting)"]
            U_APP["application"]
            U_DOM["domain<br/>(UserProfile, AuthVerifier, ProfileRepository)"]
            U_INF["infrastructure<br/>(firebaseauth, firestore)"]
        end

        subgraph NotifCtx["Context: notification (Supporting)"]
            N_APP["application<br/>(DispatchOnNewArrival)"]
            N_DOM["domain<br/>(DispatchPolicy, SubscriberStore,<br/>PushSender, NewArrivalConsumer)"]
            N_INF["infrastructure<br/>(firestore, fcm, eventbus)"]
        end

        subgraph SetupCtx["Context: cmssetup (Supporting)"]
            CS_APP["application"]
            CS_DOM["domain<br/>(SchemaDefinition)"]
            CS_INF["infrastructure/cms"]
        end

        subgraph Platform["internal/platform/*"]
            OBS["observability<br/>(slog + OTel)"]
            CMSX["cmsx<br/>(HTTP client)"]
            FBX["firebasex<br/>(SDK factory)"]
            PSX["pubsubx"]
            MBX["mapboxx"]
            CFG["config"]
        end
    end

    subgraph FlutterRepo["Flutter アプリ (overseas-safety-map-app)"]
        subgraph Pres["presentation (MVVM)"]
            VMS["ViewModels<br/>(AsyncNotifier)"]
            Views["Views / Widgets"]
        end
        subgraph DomainLy["domain"]
            UC["UseCases"]
            RepoI["Repository I/F"]
        end
        subgraph DataLy["data"]
            RepoImpl["Repository Impl"]
            DS["DataSources<br/>(Connect / Firebase)"]
        end
        Core["core<br/>(Providers / Router / Theme)"]
    end

    %% External dependencies
    MOFA --> SI_INF
    SI_INF --> Claude
    SI_INF --> Mapbox
    SI_INF --> CMSX
    CMSX --> CMS
    SI_INF --> PSX
    PSX --> PS
    PS --> N_INF
    N_INF --> FBX
    FBX --> FB
    U_INF --> FBX
    CS_INF --> CMSX

    %% Interfaces layer --> Application layer (multi-context orchestration allowed here only)
    CI --> JOB
    CB --> RPC
    CN --> JOB
    CS --> JOB
    JOB --> SI_APP
    JOB --> N_APP
    JOB --> CS_APP
    RPC --> SI_APP
    RPC --> CM_APP
    RPC --> U_APP
    RPC --> U_DOM

    %% Within a Context: application depends on domain
    SI_APP --> SI_DOM
    CM_APP --> CM_DOM
    U_APP --> U_DOM
    N_APP --> N_DOM
    CS_APP --> CS_DOM

    %% Infrastructure implements domain ports
    SI_INF -.implements.-> SI_DOM
    CM_INF -.implements.-> CM_DOM
    U_INF -.implements.-> U_DOM
    N_INF -.implements.-> N_DOM
    CS_INF -.implements.-> CS_DOM

    %% CrimeMap subdomain consumes SafetyIncident Repository
    CM_INF --> SI_DOM

    %% Observability is horizontal
    SI_INF --> OBS
    SI_APP --> OBS
    N_INF --> OBS
    N_APP --> OBS
    U_INF --> OBS
    RPC --> OBS
    JOB --> OBS

    %% Flutter side
    Views --> VMS
    VMS --> UC
    UC --> RepoI
    RepoImpl -.implements.-> RepoI
    RepoImpl --> DS
    DS --> RPC
    DS --> FB
    Core --> Views
    Core --> DS

    classDef ext fill:#FFE0B2,stroke:#E65100,color:#000
    classDef dom fill:#C8E6C9,stroke:#1B5E20,color:#000
    classDef app fill:#FFF59D,stroke:#827717,color:#000
    classDef inf fill:#FFCCBC,stroke:#BF360C,color:#000
    classDef iface fill:#D1C4E9,stroke:#311B92,color:#000
    classDef plat fill:#B3E5FC,stroke:#01579B,color:#000
    classDef flt fill:#BBDEFB,stroke:#0D47A1,color:#000
    class MOFA,Claude,Mapbox,CMS,FB,PS ext
    class SI_DOM,CM_DOM,U_DOM,N_DOM,CS_DOM dom
    class SI_APP,CM_APP,U_APP,N_APP,CS_APP app
    class SI_INF,CM_INF,U_INF,N_INF,CS_INF inf
    class RPC,JOB,CI,CB,CN,CS iface
    class OBS,CMSX,FBX,PSX,MBX,CFG plat
    class VMS,Views,UC,RepoI,RepoImpl,DS,Core flt
```

---

## 2. 依存マトリクス（Go 側）

行の要素 → 列の要素 に依存する、を示す。

各列は **Port（domain I/F）** で、行は **UseCase / Adapter / Interface レイヤ**。依存は `application → domain` が基本で、`infrastructure` は他 Context の domain を参照しない（Composition Root で組まれる）。

| From \ To (Port) | MofaSource | LocationExtractor | Geocoder | SI.Repository | EventPublisher | SubscriberStore | PushSender | AuthVerifier | ProfileRepository | cmsx.Client | crimemap.Aggregator | observability |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| `safetyincidentapp.IngestUseCase` | ✓ | ✓ | ✓ | ✓ | ✓ | — | — | — | — | — | — | ✓ |
| `safetyincidentapp.{List,Get,Search,Nearby}UseCase` | — | — | — | ✓ | — | — | — | — | — | — | — | ✓ |
| `crimemapapp.{Choropleth,Heatmap}UseCase` | — | — | — | — | — | — | — | — | — | — | ✓ | ✓ |
| `notificationapp.DispatchOnNewArrivalUseCase` | — | — | — | — | — | ✓ | ✓ | — | — | — | — | ✓ |
| `userapp.*` | — | — | — | — | — | — | — | — | ✓ | — | — | ✓ |
| `cmssetupapp.EnsureSchemaUseCase` | — | — | — | — | — | — | — | — | — | ✓ (Adapter 経由) | — | ✓ |
| `interfaces/rpc.AuthInterceptor` | — | — | — | — | — | — | — | ✓ | — | — | — | ✓ |
| `interfaces/rpc.*Handler` | — | — | — | — | — | — | — | — | — | — | — | ✓ |
| `safetyincident.infrastructure.cms.Repository` (Adapter) | — | — | — | — | — | — | — | — | — | ✓ | — | ✓ |
| `crimemap.infrastructure.RepositoryAggregator` (Adapter) | — | — | — | ✓ | — | — | — | — | — | — | — | ✓ |

**DDD レイヤ依存ルール**（Bounded Context 内）:
- `{context}/domain` は他のどこにも依存しない（標準ライブラリ + `time` + `errors` のみ）
- `{context}/application` → ✅ 同 Context の `domain` のみ
- `{context}/infrastructure/*` → ✅ 同 Context の `domain`（Port 実装）、`platform/*`、`shared/*`、generated proto
- `interfaces/rpc` → ✅ 複数 Context の `application`、`shared/*`、generated proto
- `interfaces/job` → ✅ 同一 UseCase グループの `application`、`shared/*`
- `platform/*` → ✅ `shared/*` のみ（ドメイン知識なし）
- `shared/*` → ❌ 他 `internal/*` 依存禁止
- `cmd/*`（Composition Root）→ ✅ 全方向 import 可（DI ワイヤリングのため）

**Context 間の結合ルール**:
- Context A の `application` / `infrastructure` が Context B の `domain` / `application` / `infrastructure` を **直接 import 禁止**
- 結合の許可チャネル:
  1. `interfaces/rpc` での Application Service オーケストレーション（BFF のみ、複数 Context を横断可）
  2. Domain Event を proto 化して Pub/Sub で受け渡し（`safetyincident.NewArrivalEvent` → `notification.NewArrivalMessage`）
  3. Composition Root（`cmd/*`）での DI 配線
- **例外（実用上の妥協）**: `user.domain` が `safetyincident.CountryCode` / `InfoType` を値オブジェクトとして参照する点のみ許可。これらは MOFA 由来の識別子コードで実質アプリ全体の共有語彙のため（Shared Kernel 相当、将来 `shared/codes` へ独立化する選択肢あり）。

**Context 別パッケージ一覧**（Go パッケージと配置先）:
| Context | domain | application | infrastructure |
|---|---|---|---|
| `safetyincident` | `internal/safetyincident/domain` → package `safetyincident` | `internal/safetyincident/application` → package `safetyincidentapp` | `internal/safetyincident/infrastructure/{mofa,cms,llm,geocode,eventbus}` |
| `safetyincident/crimemap` | `internal/safetyincident/crimemap/domain` → package `crimemap` | `internal/safetyincident/crimemap/application` → package `crimemapapp` | `internal/safetyincident/crimemap/infrastructure` → package `crimemapinfra` |
| `user` | `internal/user/domain` → package `user` | `internal/user/application` → package `userapp` | `internal/user/infrastructure/{firebaseauth,firestore}` |
| `notification` | `internal/notification/domain` → package `notification` | `internal/notification/application` → package `notificationapp` | `internal/notification/infrastructure/{firestore,fcm,eventbus}` |
| `cmssetup` | `internal/cmssetup/domain` → package `cmssetup` | `internal/cmssetup/application` → package `cmssetupapp` | `internal/cmssetup/infrastructure/cms` → package `cmsapplier` |
| Interface | — | — | `internal/interfaces/{rpc,job}` |
| Platform | — | — | `internal/platform/{config,observability,connectserver,pubsubx,cmsx,firebasex,mapboxx}` |
| Shared | — | — | `internal/shared/{errs,clock}` |

---

## 3. Flutter 側レイヤ依存

```mermaid
graph TD
    Pres["presentation<br/>(Views + ViewModels)"]
    Dom["domain<br/>(Entities + UseCases + Repo I/F)"]
    Data["data<br/>(Repo Impl + DataSources + DTO)"]
    Core["core<br/>(Providers / Router / Theme)"]

    Pres --> Dom
    Data --> Dom
    Pres --> Core
    Data --> Core
```

- **domain** は他のどのレイヤにも依存しない（Clean Architecture）
- **data** は **domain** に依存（Repo I/F の実装）
- **presentation** は **domain**（UseCase と Entity）と **core**（DI, Router）に依存
- **core** は Riverpod Provider 定義で **data** の Repo Impl を DI する（一方向の依存に留めるため Provider 登録のみ）

---

## 4. コミュニケーションパターン

| 呼び出し元 → 呼び出し先 | プロトコル | 認証 | 形式 |
|---|---|---|---|
| IngestionService → MOFA | HTTPS GET | なし | XML |
| IngestionService → Claude | HTTPS | API Key | JSON (Anthropic API) |
| IngestionService → Mapbox | HTTPS | API Key | JSON (Geocoding v6) |
| IngestionService → reearth-cms | HTTPS | Integration Token（Bearer） | JSON |
| IngestionService → Pub/Sub | gRPC (Google SDK) | Service Account | proto |
| Pub/Sub → NotifierService | Push / Pull | Service Account | proto |
| NotifierService → Firestore/FCM | gRPC (Firebase SDK) | Service Account | proto |
| Flutter → BFF | Connect (HTTPS) | Firebase ID Token（Bearer） | proto |
| BFF → reearth-cms | HTTPS | Integration Token | JSON |
| BFF → Firebase Auth | gRPC (Admin SDK) | Service Account | — |
| Flutter → Firestore | gRPC (Firebase SDK) | Firebase ID Token | proto |
| Flutter → FCM (登録) | gRPC (Firebase SDK) | 端末側 | — |

---

## 5. データフロー図

### 5.1 取り込みパイプライン（Ingestion Flow）

```mermaid
sequenceDiagram
    autonumber
    participant Sched as GitHub Actions (5min cron)
    participant Ing as IngestionService
    participant M as MOFA XML
    participant L as Claude
    participant G as Mapbox (+ Centroid fallback)
    participant R as SafetyIncidentRepository
    participant C as reearth-cms
    participant P as Pub/Sub

    Sched->>Ing: Run(NewArrival)
    Ing->>M: GET newarrivalA.xml
    M-->>Ing: XML
    loop MailItem 毎
        Ing->>R: Exists(keyCd)?
        alt 既存
            R-->>Ing: true
            Note over Ing: スキップ
        else 新規
            R-->>Ing: false
            Ing->>L: Extract(title, mainText)
            L-->>Ing: locationText
            Ing->>G: Geocode(locationText, countryCd)
            alt Mapbox Hit
                G-->>Ing: (LatLng, Mapbox)
            else Mapbox Miss
                G-->>Ing: (CountryCentroid, Centroid)
            end
            Ing->>R: Upsert(SafetyIncident)
            R->>C: CreateItem
            C-->>R: ItemID
            Ing->>P: PublishNewArrival(keyCd, countryCd, infoType)
        end
    end
    Ing-->>Sched: RunReport
```

### 5.2 通知配信フロー（Notifier Flow）

```mermaid
sequenceDiagram
    autonumber
    participant P as Pub/Sub
    participant N as NotifierService
    participant FS as Firestore
    participant FCM as FCM

    P->>N: NewArrivalMessage
    N->>FS: ListSubscribersFor(countryCd, infoType)
    FS-->>N: [UserProfile]
    N->>N: filter by pref + collect FCM tokens
    N->>FCM: Send(tokens, Notification{keyCd, title})
    FCM-->>N: SendReport{success, failure, invalidTokens}
    N->>FS: RemoveInvalidTokens(invalid)
```

### 5.3 Flutter → BFF 読み取りフロー

```mermaid
sequenceDiagram
    autonumber
    participant U as User (Flutter)
    participant VM as ViewModel
    participant Repo as Repo Impl (data)
    participant Conn as Connect Client
    participant BFF as BffApiService
    participant Auth as Firebase Auth Verifier
    participant R as SafetyIncidentRepository

    U->>VM: open map
    VM->>Repo: list(filter)
    Repo->>Conn: ListSafetyIncidents RPC
    Conn->>BFF: Connect Request (Authorization: Bearer idToken)
    BFF->>Auth: Verify(idToken)
    Auth-->>BFF: VerifiedUser(uid)
    BFF->>R: List(filter)
    R-->>BFF: ListResult
    BFF-->>Conn: ListSafetyIncidentsResponse
    Conn-->>Repo: [SafetyIncident DTO]
    Repo-->>VM: [SafetyIncident Entity]
    VM->>U: AsyncValue<MapState(ready)>
```

### 5.4 通知タップ → 詳細遷移フロー

```mermaid
sequenceDiagram
    autonumber
    participant FCM
    participant App as Flutter (FCM handler)
    participant Router
    participant Login as LoginView
    participant Detail as DetailView
    participant BFF

    FCM->>App: Notification{keyCd, deeplink}
    App->>Router: push(/detail/{keyCd})
    alt 未認証
        Router->>Login: push(redirect=/detail/{keyCd})
        Login->>App: sign-in success
        App->>Router: replace(/detail/{keyCd})
    end
    Router->>Detail: build(keyCd)
    Detail->>BFF: GetSafetyIncident(keyCd)
    BFF-->>Detail: SafetyIncident
    Detail->>App: render(body+出典+元記事リンク)
```

---

## 6. 凝集・結合に関する原則

- **ドメインと実装の分離**: `internal/domain` は外部 I/O 型（HTTP レスポンス型、proto 生成型）を一切持たない。
- **インターフェイスは利用側パッケージに置く**: repository I/F は `internal/repository` に置き、CMS 実装はサブパッケージ。テスト時は同パッケージ内にモックを置いて差し替え可能。
- **Connect スキーマは唯一の契約**: `proto/v1/*.proto` が BFF と Flutter の唯一の契約。Go 側は `buf generate`、Dart 側も同 `.proto` から生成（別リポジトリへコピー or サブモジュール／CI でコピーするかは Infrastructure Design で決定）。
- **循環依存禁止**: Go / Flutter いずれも `go vet` / `lint` で循環を検出・CI で落とす。
- **Pub/Sub の契約**: メッセージ proto（`pubsub.proto`）も `proto/v1/` 以下に置き、ingestion / notifier で共有する。
