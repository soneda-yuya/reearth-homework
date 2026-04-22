// Command cmsmigrate applies the reearth-cms Project / Model / Field schema
// for SafetyIncident. It runs as an idempotent one-shot Cloud Run Job during
// initial deployment and after any schema change (Terraform-like apply over
// the CMS Integration API).
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

type cmsmigrateConfig struct {
	config.Common
	// TODO(U-CSS): CMSBaseURL, CMSWorkspaceID, CMSIntegrationToken.
}

func main() {
	var cfg cmsmigrateConfig
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

	observability.Logger(ctx).Info("cmsmigrate starting (skeleton)")

	// TODO(U-CSS): Wrap the ensure-schema use case so panics are recovered
	//              and exit status reflects success.
	// if err := observability.WrapJobRun(ctx, "cmsmigrate", ...); err != nil { os.Exit(1) }

	observability.Logger(ctx).Info("cmsmigrate skeleton finished (no-op)")
}
