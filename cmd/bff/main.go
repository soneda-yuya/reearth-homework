// Command bff is the Flutter-facing Connect server. It is deployed as a
// Cloud Run Service; U-BFF fills in the Connect handlers, auth interceptor,
// and use-case wiring.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/soneda-yuya/reearth-homework/internal/platform/config"
	"github.com/soneda-yuya/reearth-homework/internal/platform/connectserver"
	"github.com/soneda-yuya/reearth-homework/internal/platform/observability"
)

type bffConfig struct {
	config.Common
	Port int `envconfig:"BFF_PORT" default:"8080"`
	// TODO(U-BFF): FirebaseProjectID, CMSBaseURL, CMSWorkspaceID, CMSIntegrationToken.
}

func main() {
	var cfg bffConfig
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

	srv := connectserver.New(
		connectserver.Config{Port: cfg.Port},
		// TODO(U-BFF): register SafetyIncidentService / CrimeMapService / UserProfileService handlers.
		nil,
		// TODO(U-BFF): interceptors including AuthInterceptor (user.AuthVerifier).
		nil,
		// TODO(U-BFF): Probers (cmsx, firebasex).
		nil,
	)
	observability.Logger(ctx).Info("bff starting", "port", cfg.Port)
	if err := srv.Start(ctx); err != nil {
		observability.Logger(ctx).Error("bff server stopped with error", "err", err)
		os.Exit(1)
	}
	observability.Logger(ctx).Info("bff stopped cleanly")
}
