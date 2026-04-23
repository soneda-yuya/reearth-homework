// Command notifier consumes safety-incident.new-arrival from Pub/Sub Push
// Subscription and fans out FCM pushes to Firestore subscribers. See
// aidlc-docs/construction/U-NTF/ for the full design.
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

	"github.com/soneda-yuya/overseas-safety-map/internal/interfaces/job"
	"github.com/soneda-yuya/overseas-safety-map/internal/notification/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/notification/infrastructure/dedup"
	"github.com/soneda-yuya/overseas-safety-map/internal/notification/infrastructure/eventdecoder"
	"github.com/soneda-yuya/overseas-safety-map/internal/notification/infrastructure/fcm"
	"github.com/soneda-yuya/overseas-safety-map/internal/notification/infrastructure/userrepo"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/config"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/firebasex"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/observability"
)

// notifierConfig is the full env schema for this deployable. Tuning fields
// with `default:"..."` tags are intentionally envconfig-only (U-NTF Infra
// Q4 [A]) so rate/collection name changes don't require Terraform.
type notifierConfig struct {
	config.Common

	Port               string `envconfig:"NOTIFIER_PORT" default:"8080"`
	PubSubSubscription string `envconfig:"NOTIFIER_PUBSUB_SUBSCRIPTION" required:"true"`

	// Firestore / FCM tuning — envconfig default, not Terraform.
	DedupCollection      string `envconfig:"NOTIFIER_DEDUP_COLLECTION" default:"notifier_dedup"`
	DedupTTLHours        int    `envconfig:"NOTIFIER_DEDUP_TTL_HOURS" default:"24"`
	UsersCollection      string `envconfig:"NOTIFIER_USERS_COLLECTION" default:"users"`
	FCMConcurrency       int    `envconfig:"NOTIFIER_FCM_CONCURRENCY" default:"5"`
	ShutdownGraceSeconds int    `envconfig:"NOTIFIER_SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// main delegates to run() so defers (Firebase Close, HTTP Shutdown,
// observability flush) fire even on the fail path.
func main() {
	if err := run(); err != nil {
		slog.Error("notifier failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
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
		return err
	}
	defer func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()
		_ = shutdown(flushCtx)
	}()

	logger := observability.Logger(ctx)
	logger.InfoContext(ctx, "notifier starting",
		"app.notifier.phase", "start",
		"port", cfg.Port,
		"subscription", cfg.PubSubSubscription,
	)

	// --- Firebase (Firestore + FCM) -------------------------------------
	fbApp, err := firebasex.NewApp(ctx, firebasex.Config{ProjectID: cfg.GCPProjectID})
	if err != nil {
		return err
	}
	defer func() { _ = fbApp.Close(ctx) }()

	fsClient, err := fbApp.Firestore(ctx)
	if err != nil {
		return err
	}
	fcmClient, err := fbApp.Messaging(ctx)
	if err != nil {
		return err
	}

	// --- Adapters --------------------------------------------------------
	dedupAdapter := dedup.New(fsClient, dedup.Config{
		Collection: cfg.DedupCollection,
		TTL:        time.Duration(cfg.DedupTTLHours) * time.Hour,
	})
	userAdapter := userrepo.New(fsClient, userrepo.Config{Collection: cfg.UsersCollection})
	fcmAdapter := fcm.New(fcmClient)
	decoder := eventdecoder.New()

	// --- Use Case --------------------------------------------------------
	uc := application.NewDeliverNotificationUseCase(
		dedupAdapter, userAdapter, fcmAdapter,
		application.Deps{
			Logger:      logger,
			Tracer:      observability.Tracer(ctx),
			Meter:       observability.Meter(ctx),
			Concurrency: cfg.FCMConcurrency,
		},
	)

	// --- HTTP handler ----------------------------------------------------
	handler := job.NewNotifierHandler(decoder, uc, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("/pubsub/push", handler.Push)
	mux.HandleFunc("/healthz", handler.Health)

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start server in a goroutine so the signal-watching ctx can coordinate
	// graceful shutdown. Errors bubble via errCh.
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	logger.InfoContext(ctx, "http server ready",
		"app.notifier.phase", "ready",
		"addr", addr,
	)

	select {
	case <-ctx.Done():
		logger.InfoContext(ctx, "shutdown signal received", "app.notifier.phase", "shutdown")
	case err := <-errCh:
		return fmt.Errorf("http server failed: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(),
		time.Duration(cfg.ShutdownGraceSeconds)*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Shutdown timed out; best-effort close.
		logger.WarnContext(shutdownCtx, "http server shutdown timed out",
			"app.notifier.phase", "shutdown",
			"err", err,
		)
	}
	logger.InfoContext(context.Background(), "notifier stopped cleanly",
		"app.notifier.phase", "stopped",
	)
	return nil
}
