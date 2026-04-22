# U-PLT Business Rules (Minimal)

U-PLT にはドメインのビジネスルールは無いが、**他 Unit が必ず従うべき横断ルール（バリデーション・ラップ規約・ログ出力規約）** を規約として定義する。これらは CI の lint ルールまたは code review checklist に落とし込む。

---

## 1. proto バリデーション規約

### 1.1 Request メッセージの最小バリデーション

Connect ハンドラで **すべての Request は最初にバリデーション** を行う。

| ルール | 対象 | エラー Kind |
|---|---|---|
| 必須 ID 系フィールドの空文字チェック | `key_cd`, `uid`, `country_cd`（必須の API のみ） | `InvalidInput` |
| `limit` の範囲（1〜500） | `ListSafetyIncidentsRequest.filter.limit` 等 | `InvalidInput` |
| `radius_km` の範囲（0 < x <= 5000） | `ListNearbyRequest.radius_km` | `InvalidInput` |
| 期間フィルタの順序（`leave_from <= leave_to`） | `SafetyIncidentFilter` / `CrimeMapFilter` | `InvalidInput` |
| 座標の範囲（`-90 <= lat <= 90`、`-180 <= lng <= 180`） | `Point` | `InvalidInput` |

### 1.2 実装場所

- 共通バリデーションヘルパ: `internal/shared/validate/validate.go`
- ハンドラ冒頭で `validate.Request(req)` のように呼び、エラーを返す

### 1.3 ハンドラ側での Kind → Code 変換

ハンドラが返す `error` を **Connect Interceptor**（`internal/interfaces/rpc/error_interceptor.go`）で `errs.Kind` → `connect.Code` に変換する。直接 `connect.NewError(code, err)` を書かない。

---

## 2. エラーラップ規約

### 2.1 `%w` ラップの使いどころ

- **外部 API 呼び出しを受けた直後**: `errs.Wrap("mapbox.geocode", errs.KindExternal, err)`
- **Repository 実装で 404 相当を受けた時**: `errs.Wrap("cms.repository.get", errs.KindNotFound, err)`
- **Application Service の先頭で呼び出しをまとめる時**: `errs.Wrap("safetyincident.ingest", errs.KindInternal, err)`（root Kind の確定はここで）

### 2.2 禁止事項

- `fmt.Errorf("failed: %v", err)` のような **`%v` による情報損失** は禁止（`%w` を使う）
- **`errors.New("...")` の sentinel error を Context の外に漏らさない**（パッケージ外には `errs.Kind` で包んでから返す）
- **panic は recover で `errs.KindInternal` にラップ** して返す（Connect Interceptor と Job Runner 両方で）

### 2.3 ラップチェーンの深さ

- 1 つのエラーに対して `Wrap` は最大 3 段（ドメイン→アプリ→BFF）まで。それ以上はチェーンが長すぎるため、ドメインで Kind を確定し、上位は `%w` のみ（Kind を差し替えない）。

---

## 3. ログ出力ルール

### 3.1 必須属性の付与

すべての Logger 呼び出しで、以下は **自動付与** される前提（`observability.Setup()` + Connect Interceptor + Job Runner が責任を持つ）。

- `service`, `env`, `trace_id`, `span_id`, `caller`, `time`, `level`, `msg`

### 3.2 ドメイン属性の付与ルール

| 属性 | 付与タイミング |
|---|---|
| `key_cd` | safety_incident を扱うスコープの先頭で `ctx = observability.With(ctx, "key_cd", keyCd)` |
| `uid` | `AuthInterceptor` が認証完了後に注入 |
| `country_cd` / `info_type` | ingestion / notification の 1 件処理の先頭で注入 |
| `geocode_source` | Geocoder の戻り値を確定した直後に注入 |

**共通ヘルパ**: `observability.With(ctx, key, value)` で context に埋め、以降の `observability.Logger(ctx)` 取得時に自動的に属性化する。

### 3.3 サンプリング

- `DEBUG` レベルは prod で出力しない（`LOG_LEVEL=INFO` デフォルト）
- ingestion の成功ログは 1 件につき 1 本まで（`incident upserted` INFO）。本体データは含めない
- `ERROR` は全件出力（サンプリングなし）

### 3.4 ログ内の PII 扱い

- `uid` のみログ可。**メール・FCM トークン・Integration Token は一切ログしない**（marshaler で redact）
- `mainText` などの本文テキストは `DEBUG` のみ

---

## 4. Config 読み込みルール

### 4.1 初期化順序

`cmd/*/main.go` の起動シーケンスは **以下の順序を厳守**:

1. `config.Load()` を最初に呼ぶ（失敗したら panic）
2. `observability.Setup(cfg)` を呼ぶ（slog + OTel 初期化）
3. その他の Platform factory（`cmsx`, `firebasex`, `pubsubx`, `mapboxx`）を生成
4. ドメイン Adapter / Application Service を DI
5. `interfaces/rpc.Server` または `interfaces/job.Runner` を起動
6. 終了時に `observability` の `shutdown(ctx)` を必ず呼ぶ（defer）

### 4.2 必須欠落時の挙動

- `config.Load()` は必須項目が欠落していたら即 `log.Fatalf("config: %s is required", key)` で終了（panic）
- これにより Cloud Run は起動せず、デプロイが壊れる（フェイルファスト）

### 4.3 Secret 読み込み

- 環境変数には **Secret Manager のリソース名** のみ（例: `projects/xxx/secrets/mapbox-key/versions/latest`）
- 実際の値は起動時に Secret Manager SDK で取得、メモリにのみ保持
- ログ・エラー出力に値を含めない

---

## 5. proto スキーマ互換性ルール

### 5.1 Breaking Change の防止

- CI で `buf breaking` を main ブランチに対して実行、違反があれば PR を落とす
- 破壊的変更が必要な時は新しい `v2` パッケージを並列で作り、旧 `v1` を deprecate する（利用中クライアントが切替えるまで維持）

### 5.2 許容される変更

- 新規フィールド追加（`tag` は未使用番号を使う）
- 新規 RPC 追加
- 新規 enum 値追加（末尾に）
- `optional` の追加（後方互換）

### 5.3 禁止される変更

- 既存フィールドの tag 変更
- 既存フィールドの型変更
- 既存フィールドの削除（`reserved` にする）
- enum 値の番号変更

---

## 6. 受け入れ条件（Sign-off）

- [ ] proto バリデーションヘルパ `shared/validate` に 5 ルールが実装される
- [ ] `errs.Wrap` 規約が godoc に明記される
- [ ] Log `observability.With` ヘルパが動作する
- [ ] `config.Load()` が必須欠落で panic する（テストで検証）
- [ ] CI で `buf lint` + `buf breaking` が動く
