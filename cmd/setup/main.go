// Command setup applies the reearth-cms Project / Model / Field schema for
// SafetyIncident. It runs as a one-shot Cloud Run Job (idempotent) during
// initial deployment and after any schema change.
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

type setupConfig struct {
	config.Common
	// TODO(U-CSS): CMSBaseURL, CMSWorkspaceID, CMSIntegrationToken.
}

func main() {
	var cfg setupConfig
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

	observability.Logger(ctx).Info("setup starting (skeleton)")

	// TODO(U-CSS): Wrap the ensure-schema use case so panics are recovered
	//              and exit status reflects success.
	// if err := observability.WrapJobRun(ctx, "setup", ...); err != nil { os.Exit(1) }

	observability.Logger(ctx).Info("setup skeleton finished (no-op)")
}
