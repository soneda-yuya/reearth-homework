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
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/application"
	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/infrastructure/cmsclient"
	"github.com/soneda-yuya/reearth-homework/internal/platform/cmsx"
	"github.com/soneda-yuya/reearth-homework/internal/platform/config"
	"github.com/soneda-yuya/reearth-homework/internal/platform/observability"
)

// cmsmigrateConfig embeds the platform Common block and layers on the three
// CMS-specific fields. The token is a Secret Manager reference at deploy time
// (see terraform/modules/cmsmigrate/main.tf).
type cmsmigrateConfig struct {
	config.Common
	CMSBaseURL          string `envconfig:"CMSMIGRATE_CMS_BASE_URL" required:"true"`
	CMSWorkspaceID      string `envconfig:"CMSMIGRATE_CMS_WORKSPACE_ID" required:"true"`
	CMSIntegrationToken string `envconfig:"CMSMIGRATE_CMS_INTEGRATION_TOKEN" required:"true"`
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
	defer func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()
		_ = shutdown(flushCtx)
	}()

	logger := observability.Logger(ctx)
	logger.InfoContext(ctx, "cmsmigrate starting",
		"app.cmsmigrate.phase", "start",
		"cms.base_url", cfg.CMSBaseURL,
		"cms.workspace_id", cfg.CMSWorkspaceID,
	)

	client := cmsx.NewClient(cmsx.Config{
		BaseURL:     cfg.CMSBaseURL,
		WorkspaceID: cfg.CMSWorkspaceID,
		Token:       cfg.CMSIntegrationToken,
		Timeout:     30 * time.Second,
	})
	defer func() { _ = client.Close(ctx) }()

	applier := cmsclient.New(client)
	usecase := application.NewEnsureSchemaUseCase(
		applier,
		logger,
		observability.Tracer(ctx),
		observability.Meter(ctx),
	)

	result, err := usecase.Execute(ctx, application.EnsureSchemaInput{
		Definition: domain.SafetyMapSchema(),
	})
	if err != nil {
		logger.ErrorContext(ctx, "ensure schema failed",
			"app.cmsmigrate.phase", "failed",
			"err", err,
		)
		os.Exit(1)
	}

	logger.InfoContext(ctx, "cmsmigrate finished",
		"app.cmsmigrate.phase", "done",
		"project_created", result.ProjectCreated,
		"models_created", result.ModelsCreated,
		"fields_created", result.FieldsCreated,
		"drift_warnings", len(result.DriftWarnings),
	)
}
