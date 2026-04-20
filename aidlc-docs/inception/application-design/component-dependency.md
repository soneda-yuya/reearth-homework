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

    subgraph GoRepo["Go サーバーモノレポ (reearth-homework)"]
        subgraph Ingestion["cmd/ingestion"]
            IApp["IngestionApp"]
            IS["IngestionService"]
        end
        subgraph BFF["cmd/bff"]
            BApp["BffApp"]
            BS["BffApiService<br/>(Connect server)"]
        end
        subgraph Notifier["cmd/notifier"]
            NApp["NotifierApp"]
            NS["NotifierService"]
        end
        subgraph Setup["cmd/setup"]
            SApp["SetupApp"]
            SS["CmsSetupService"]
        end

        subgraph Internal["internal/*"]
            MC["mofa.Client"]
            LE["llm.LocationExtractor"]
            GC["geocode.Geocoder<br/>(Chain: Mapbox+Centroid)"]
            REPO["SafetyIncidentRepository"]
            CMSC["cms.Client"]
            PUBSUB["pubsub.Publisher/Subscriber"]
            FBG["firebase.*<br/>(Auth/UserStore/FcmSender)"]
            CMA["crimemap.Aggregator"]
            OBS["observability"]
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

    MOFA --> MC
    MC --> IS
    IS --> LE
    IS --> GC
    IS --> REPO
    IS --> PUBSUB
    LE --> Claude
    GC --> Mapbox
    REPO --> CMSC
    CMSC --> CMS
    PUBSUB --> PS

    PS --> PUBSUB
    PUBSUB --> NS
    NS --> FBG
    FBG --> FB

    BS --> REPO
    BS --> CMA
    BS --> FBG
    CMA --> REPO

    SS --> CMSC

    IApp --> IS
    BApp --> BS
    NApp --> NS
    SApp --> SS

    Views --> VMS
    VMS --> UC
    UC --> RepoI
    RepoImpl -.implements.-> RepoI
    RepoImpl --> DS
    DS --> BS
    DS --> FB

    Core --> Views
    Core --> DS

    IS --> OBS
    BS --> OBS
    NS --> OBS
    SS --> OBS

    classDef ext fill:#FFE0B2,stroke:#E65100,color:#000
    classDef srv fill:#C8E6C9,stroke:#1B5E20,color:#000
    classDef flt fill:#BBDEFB,stroke:#0D47A1,color:#000
    class MOFA,Claude,Mapbox,CMS,FB,PS ext
    class IApp,IS,BApp,BS,NApp,NS,SApp,SS,MC,LE,GC,REPO,CMSC,PUBSUB,FBG,CMA,OBS srv
    class VMS,Views,UC,RepoI,RepoImpl,DS,Core flt
```

---

## 2. 依存マトリクス（Go 側）

行の要素 → 列の要素 に依存する、を示す。

| From \ To | MofaClient | LLM Extractor | Geocoder | Repository | CMS Client | Pub/Sub | Firebase | CrimeMap Agg | Observability |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| IngestionService | ✓ | ✓ | ✓ | ✓ | — | ✓ | — | — | ✓ |
| NotifierService | — | — | — | — | — | ✓ | ✓ | — | ✓ |
| BffApiService | — | — | — | ✓ | — | — | ✓ (Auth + UserStore) | ✓ | ✓ |
| CmsSetupService | — | — | — | — | ✓ | — | — | — | ✓ |
| Repository (CMSImpl) | — | — | — | — | ✓ | — | — | — | ✓ |
| CrimeMap Aggregator | — | — | — | ✓ | — | — | — | — | ✓ |

**内部パッケージの依存ルール**:
- `internal/domain` は他の `internal/*` に依存しない（純粋ドメイン）
- `internal/repository` → `internal/domain`, `internal/cms`（Impl のみ）
- `internal/bff` → `internal/repository`, `internal/firebase`, `internal/crimemap`, `internal/observability`, Connect generated code
- `internal/ingestion` → `internal/mofa`, `internal/llm`, `internal/geocode`, `internal/repository`, `internal/pubsub`, `internal/observability`
- `internal/notifier` → `internal/pubsub`, `internal/firebase`, `internal/observability`（必要に応じ `internal/repository` で title 補完）
- `cmd/*` はそれぞれのサービスと `internal/observability` だけを import（薄い main）

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
