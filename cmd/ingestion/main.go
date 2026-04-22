// Command ingestion is the MOFA XML fetcher / geocoder / CMS upserter.
// It is expected to run as a Cloud Run Job triggered every 5 minutes by
// Cloud Scheduler.
//
// The full implementation is provided in U-ING; this main wires up the
// platform primitives (config, observability, graceful shutdown) so the
// deployable can be built and deployed from day one.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/soneda-yuya/reearth-homework/internal/platform/config"
	"github.com/soneda-yuya/reearth-homework/internal/platform/observability"
)

// ingestionConfig is the full env schema for this deployable.
// Additional fields are added in U-ING.
type ingestionConfig struct {
	config.Common
	// TODO(U-ING): MofaBaseURL, PubsubTopic, ClaudeAPIKey, MapboxAPIKey,
	//              CMSBaseURL, CMSWorkspaceID, CMSIntegrationToken, Mode.
}

func main() {
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
		slog.Error("observability setup failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(context.Background()) }()

	observability.Logger(ctx).Info("ingestion starting (skeleton)",
		"env", cfg.Env,
		"project", cfg.GCPProjectID,
	)

	// TODO(U-ING): build IngestUseCase and run it once (Cloud Run Job semantics).
	// err := observability.WrapJobRun(ctx, "ingestion", func(ctx context.Context) error { ... })
	// Then exit with non-zero on failure so Cloud Scheduler can alert.
	observability.Logger(ctx).Info("ingestion skeleton finished (no-op)")
}
