// Command ingestion is the MOFA XML fetcher / geocoder / CMS upserter. It
// runs as a Cloud Run Job triggered every 5 minutes by Cloud Scheduler in
// `incremental` mode; operators trigger it manually with INGESTION_MODE=
// initial for one-shot backfills.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/platform/cmsx"
	"github.com/soneda-yuya/reearth-homework/internal/platform/config"
	"github.com/soneda-yuya/reearth-homework/internal/platform/llm"
	"github.com/soneda-yuya/reearth-homework/internal/platform/mapboxx"
	"github.com/soneda-yuya/reearth-homework/internal/platform/observability"
	"github.com/soneda-yuya/reearth-homework/internal/platform/pubsubx"
	"github.com/soneda-yuya/reearth-homework/internal/platform/ratelimit"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/application"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/cms"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/eventbus"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/geocode"
	infralm "github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/llm"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/mofa"
	"github.com/soneda-yuya/reearth-homework/internal/shared/clock"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// ingestionConfig is the full env schema for this deployable. Fields with
// `default:"..."` tags are tuning parameters intentionally left to envconfig
// so operators can change them without a Terraform apply (U-ING infra Q5 [A]).
type ingestionConfig struct {
	config.Common

	// Mode selects the MOFA endpoint Source.Fetch will hit. Terraform pins
	// it to "incremental" for the Scheduler-triggered runs; manual initial
	// backfills override it via gcloud --update-env-vars.
	Mode string `envconfig:"INGESTION_MODE" default:"incremental"`

	// MOFA
	MofaBaseURL string `envconfig:"INGESTION_MOFA_BASE_URL" default:"https://www.ezairyu.mofa.go.jp/html/opendata"`

	// CMS — model resolution is by alias because CSS is the schema source of truth.
	CMSBaseURL          string `envconfig:"INGESTION_CMS_BASE_URL" required:"true"`
	CMSWorkspaceID      string `envconfig:"INGESTION_CMS_WORKSPACE_ID" required:"true"`
	CMSIntegrationToken string `envconfig:"INGESTION_CMS_INTEGRATION_TOKEN" required:"true"`
	CMSProjectAlias     string `envconfig:"INGESTION_CMS_PROJECT_ALIAS" default:"overseas-safety-map"`
	CMSModelAlias       string `envconfig:"INGESTION_CMS_MODEL_ALIAS" default:"safety-incident"`
	CMSKeyField         string `envconfig:"INGESTION_CMS_KEY_FIELD" default:"key_cd"`

	// LLM
	ClaudeAPIKey string `envconfig:"INGESTION_CLAUDE_API_KEY" required:"true"`
	ClaudeModel  string `envconfig:"INGESTION_CLAUDE_MODEL" default:"claude-haiku-4-5"`

	// Mapbox
	MapboxAPIKey   string  `envconfig:"INGESTION_MAPBOX_API_KEY" required:"true"`
	MapboxMinScore float64 `envconfig:"INGESTION_MAPBOX_MIN_SCORE" default:"0.5"`

	// Pub/Sub
	PubSubTopicID string `envconfig:"INGESTION_PUBSUB_TOPIC_ID" required:"true"`

	// Tuning
	Concurrency        int `envconfig:"INGESTION_CONCURRENCY" default:"5"`
	LLMRateLimitRPM    int `envconfig:"INGESTION_LLM_RATE_LIMIT_RPM" default:"300"`     // 5 req/s
	GeocodeRateRPM     int `envconfig:"INGESTION_GEOCODE_RATE_LIMIT_RPM" default:"600"` // 10 req/s
	HTTPTimeoutSeconds int `envconfig:"INGESTION_HTTP_TIMEOUT_SECONDS" default:"30"`
}

// main is intentionally tiny: it delegates to run() so that defers (OTel
// flush, Pub/Sub topic stop, CMS client close) actually fire even on the
// fail path. os.Exit(1) inside main would skip them.
func main() {
	if err := run(); err != nil {
		slog.Error("ingestion failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	var cfg ingestionConfig
	config.MustLoad(&cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	shutdown, err := observability.Setup(ctx, observability.Config{
		ServiceName:  cfg.ServiceName,
		Env:          cfg.Env,
		LogLevel:     cfg.LogLevel,
		ExporterKind: cfg.OTelExporter,
	})
	if err != nil {
		return err
	}
	defer func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()
		_ = shutdown(flushCtx)
	}()

	logger := observability.Logger(ctx)
	logger.InfoContext(ctx, "ingestion starting",
		"app.ingestion.phase", "start",
		"app.ingestion.mode", cfg.Mode,
		"cms.base_url", cfg.CMSBaseURL,
	)

	timeout := time.Duration(cfg.HTTPTimeoutSeconds) * time.Second
	mofaClient := &http.Client{Timeout: timeout}
	source := mofa.New(cfg.MofaBaseURL, mofaClient)

	llmClient := llm.NewClient(llm.Config{
		APIKey:  cfg.ClaudeAPIKey,
		Model:   cfg.ClaudeModel,
		Timeout: timeout,
	})
	extractor := infralm.New(llmClient)

	mapboxClient := mapboxx.NewClient(mapboxx.Config{
		AccessToken: cfg.MapboxAPIKey,
		Timeout:     timeout,
	})
	mapboxGeo := geocode.NewMapboxGeocoder(mapboxClient, cfg.MapboxMinScore)
	centroidGeo, err := geocode.NewCentroidGeocoder()
	if err != nil {
		return err
	}
	geocoder := geocode.NewChainGeocoder(mapboxGeo, centroidGeo, logger)

	cmsClient := cmsx.NewClient(cmsx.Config{
		BaseURL:     cfg.CMSBaseURL,
		WorkspaceID: cfg.CMSWorkspaceID,
		Token:       cfg.CMSIntegrationToken,
		Timeout:     timeout,
	})
	defer func() { _ = cmsClient.Close(ctx) }()

	// Resolve the CMS model id by walking project → model. We do this once
	// at startup so the per-item path stays free of metadata lookups.
	modelID, err := resolveModelID(ctx, cmsClient, cfg.CMSProjectAlias, cfg.CMSModelAlias)
	if err != nil {
		return err
	}
	repo := cms.New(cmsClient, modelID, cfg.CMSKeyField)

	psClient, err := pubsubx.NewClient(ctx, pubsubx.Config{ProjectID: cfg.GCPProjectID})
	if err != nil {
		return err
	}
	// Close drains every Topic this client created and then closes the
	// underlying gRPC connections — no need to defer topic.Stop separately.
	defer func() { _ = psClient.Close(ctx) }()
	topic := psClient.Topic(cfg.PubSubTopicID)
	publisher := eventbus.New(topic)

	usecase := application.NewIngestUseCase(
		source, extractor, geocoder, repo, publisher,
		application.Deps{
			Logger:         logger,
			Tracer:         observability.Tracer(ctx),
			Meter:          observability.Meter(ctx),
			Clock:          clock.System(),
			LLMLimiter:     ratelimit.New(cfg.LLMRateLimitRPM, cfg.Concurrency, "llm"),
			GeocodeLimiter: ratelimit.New(cfg.GeocodeRateRPM, cfg.Concurrency, "geocode"),
			Concurrency:    cfg.Concurrency,
		},
	)

	result, err := usecase.Execute(ctx, application.IngestInput{
		Mode: domain.IngestionMode(cfg.Mode),
	})
	if err != nil {
		// Fetch-level failure (MOFA unreachable, etc.). Per-item failures
		// are inside result and do NOT bubble here — see Q7 [A]. main()
		// owns the terminal log line so we just propagate.
		return err
	}

	logger.InfoContext(ctx, "ingestion finished",
		"app.ingestion.phase", "done",
		"app.ingestion.mode", cfg.Mode,
		"fetched", result.Fetched,
		"skipped", result.Skipped,
		"processed", result.Processed,
		"published", result.Published,
		"failed_validate", result.Failed[application.PhaseValidate],
		"failed_lookup", result.Failed[application.PhaseLookup],
		"failed_extract", result.Failed[application.PhaseExtract],
		"failed_geocode", result.Failed[application.PhaseGeocode],
		"failed_upsert", result.Failed[application.PhaseUpsert],
		"failed_publish", result.Failed[application.PhasePublish],
	)
	return nil
}

// resolveModelID walks projectAlias → modelAlias to discover the CMS model
// id. The result is cached for the rest of the run; if either lookup fails
// or returns nil we surface a clear error so operators know the U-CSS
// migration has not run against this CMS yet.
func resolveModelID(ctx context.Context, c *cmsx.Client, projectAlias, modelAlias string) (string, error) {
	project, err := c.FindProjectByAlias(ctx, projectAlias)
	if err != nil {
		return "", err
	}
	if project == nil {
		return "", errs.Wrap("ingestion.resolve_model",
			errs.KindNotFound,
			fmt.Errorf("project %q not found in CMS — has cmsmigrate Job run?", projectAlias))
	}
	model, err := c.FindModelByAlias(ctx, project.ID, modelAlias)
	if err != nil {
		return "", err
	}
	if model == nil {
		return "", errs.Wrap("ingestion.resolve_model",
			errs.KindNotFound,
			fmt.Errorf("model %q under project %q not found — has cmsmigrate Job run?", modelAlias, projectAlias))
	}
	return model.ID, nil
}
