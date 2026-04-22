// Command notifier consumes safety-incident.new-arrival from Pub/Sub and
// fans out FCM pushes. U-NTF implements the push handler and Firestore reads.
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

type notifierConfig struct {
	config.Common
	Port int `envconfig:"NOTIFIER_PORT" default:"8080"`
	// TODO(U-NTF): PubSubSubscription, FirebaseProjectID.
}

func main() {
	var cfg notifierConfig
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

	// Notifier is reached via Pub/Sub push -> HTTP POST, so the Cloud Run
	// Service model applies. U-NTF will register /pubsub/push on the mux.
	srv := connectserver.New(connectserver.Config{Port: cfg.Port}, nil, nil)
	observability.Logger(ctx).Info("notifier starting", "port", cfg.Port)
	if err := srv.Start(ctx); err != nil {
		observability.Logger(ctx).Error("notifier stopped with error", "err", err)
		os.Exit(1)
	}
	observability.Logger(ctx).Info("notifier stopped cleanly")
}
