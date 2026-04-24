// Command bff is the Flutter-facing Connect server. It verifies Firebase ID
// tokens, exposes three Services (SafetyIncidentService / CrimeMapService /
// UserProfileService, 11 RPCs total), and reads incidents from reearth-cms
// plus user profiles from Firestore. See aidlc-docs/construction/U-BFF/ for
// the full design.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"

	overseasmapv1connect "github.com/soneda-yuya/overseas-safety-map/gen/go/v1/overseasmapv1connect"
	"github.com/soneda-yuya/overseas-safety-map/internal/interfaces/rpc"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/cmsx"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/config"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/connectserver"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/firebasex"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/observability"
	safetyapp "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/application"
	crimemapapp "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/crimemap/application"
	cmsadapter "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/infrastructure/cms"
	userapp "github.com/soneda-yuya/overseas-safety-map/internal/user/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/infrastructure/firebaseauth"
	userfirestore "github.com/soneda-yuya/overseas-safety-map/internal/user/infrastructure/firestore"
)

// bffConfig is the full env schema for this deployable. Tuning fields
// with `default:"..."` tags are intentionally envconfig-only (U-BFF Infra
// Q5 [A]) so request-body limits, collection names and tokens-per-user caps
// can change without a Terraform apply.
type bffConfig struct {
	config.Common

	Port int `envconfig:"BFF_PORT" default:"8080"`

	// CMS connectivity — Terraform-supplied (base URL / workspace change
	// between envs, integration token is a Secret Manager ref).
	CMSBaseURL          string `envconfig:"BFF_CMS_BASE_URL" required:"true"`
	CMSWorkspaceID      string `envconfig:"BFF_CMS_WORKSPACE_ID" required:"true"`
	CMSIntegrationToken string `envconfig:"BFF_CMS_INTEGRATION_TOKEN" required:"true"`

	// CMS Model resolution — the model the BFF reads from (safety-incident).
	// The write side (U-ING) resolves the same alias, so this mirrors the
	// ingestion env. Falls back to a sensible default so local dev works
	// out-of-the-box.
	CMSProjectAlias string `envconfig:"BFF_CMS_PROJECT_ALIAS" default:"overseas-safety-map"`
	CMSModelAlias   string `envconfig:"BFF_CMS_MODEL_ALIAS"   default:"safety-incident"`
	CMSKeyField     string `envconfig:"BFF_CMS_KEY_FIELD"     default:"key_cd"`

	// Firestore / tuning — envconfig default, not Terraform.
	UsersCollection      string `envconfig:"BFF_USERS_COLLECTION" default:"users"`
	ShutdownGraceSeconds int    `envconfig:"BFF_SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// main delegates to run() so defers (Firebase Close, CMS client close,
// observability flush) fire even on the fail path.
func main() {
	if err := run(); err != nil {
		slog.Error("bff failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
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
		return err
	}
	defer func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()
		_ = shutdown(flushCtx)
	}()

	logger := observability.Logger(ctx)
	logger.InfoContext(ctx, "bff starting",
		"app.bff.phase", "start",
		"port", cfg.Port,
	)

	// --- CMS client ------------------------------------------------------
	cmsClient := cmsx.NewClient(cmsx.Config{
		BaseURL:     cfg.CMSBaseURL,
		WorkspaceID: cfg.CMSWorkspaceID,
		Token:       cfg.CMSIntegrationToken,
	})
	defer func() { _ = cmsClient.Close(ctx) }()

	// Resolve the CMS Model id once at startup so the hot path does not
	// re-discover it on every request. A missing project / model is a
	// deployment bug (U-CSS should have created them) — fail fast.
	project, err := cmsClient.FindProjectByAlias(ctx, cfg.CMSProjectAlias)
	if err != nil {
		return err
	}
	if project == nil {
		return errProjectNotFound(cfg.CMSProjectAlias)
	}
	model, err := cmsClient.FindModelByAlias(ctx, project.ID, cfg.CMSModelAlias)
	if err != nil {
		return err
	}
	if model == nil {
		return errModelNotFound(cfg.CMSModelAlias)
	}

	// --- Firebase (Firestore + Auth) ------------------------------------
	fbApp, err := firebasex.NewApp(ctx, firebasex.Config{ProjectID: cfg.GCPProjectID})
	if err != nil {
		return err
	}
	defer func() { _ = fbApp.Close(ctx) }()

	fsClient, err := fbApp.Firestore(ctx)
	if err != nil {
		return err
	}
	authClient, err := fbApp.Auth(ctx)
	if err != nil {
		return err
	}

	// --- Adapters -------------------------------------------------------
	reader := cmsadapter.NewReader(cmsClient, project.ID, model.ID, cfg.CMSKeyField)
	profileRepo := userfirestore.New(fsClient, userfirestore.Config{Collection: cfg.UsersCollection})
	authVerifier := firebaseauth.New(authClient)

	// --- Use cases ------------------------------------------------------
	listUC := safetyapp.NewListUseCase(reader)
	getUC := safetyapp.NewGetUseCase(reader)
	searchUC := safetyapp.NewSearchUseCase(reader)
	nearbyUC := safetyapp.NewNearbyUseCase(reader)
	geojsonUC := safetyapp.NewGeoJSONUseCase(reader)
	aggregator := crimemapapp.NewAggregator(reader)
	getProfileUC := userapp.NewGetProfileUseCase(profileRepo)
	toggleFavUC := userapp.NewToggleFavoriteCountryUseCase(profileRepo)
	updatePrefUC := userapp.NewUpdateNotificationPreferenceUseCase(profileRepo)
	registerFCMUC := userapp.NewRegisterFcmTokenUseCase(profileRepo)

	// --- RPC servers ----------------------------------------------------
	safetyServer := rpc.NewSafetyIncidentServer(listUC, getUC, searchUC, nearbyUC, geojsonUC)
	crimeServer := rpc.NewCrimeMapServer(aggregator)
	userServer := rpc.NewUserProfileServer(getProfileUC, toggleFavUC, updatePrefUC, registerFCMUC)

	// --- Interceptor chain ---------------------------------------------
	// Order applied at request-time is outer → inner: Recover → Error → Auth
	// → handler. Recover is outermost so a panic in Auth / Error still
	// surfaces as a clean KindInternal. Auth is innermost so a rejected
	// token is counted as an auth failure (not an internal error).
	interceptors := connect.WithInterceptors(
		observability.RecoverInterceptor(),
		rpc.NewErrorInterceptor(cfg.Env),
		rpc.NewAuthInterceptor(authVerifier, logger),
	)

	siPath, siHandler := overseasmapv1connect.NewSafetyIncidentServiceHandler(safetyServer, interceptors)
	cmPath, cmHandler := overseasmapv1connect.NewCrimeMapServiceHandler(crimeServer, interceptors)
	upPath, upHandler := overseasmapv1connect.NewUserProfileServiceHandler(userServer, interceptors)
	handlers := []connectserver.HandlerRegistration{
		{Path: siPath, Handler: siHandler},
		{Path: cmPath, Handler: cmHandler},
		{Path: upPath, Handler: upHandler},
	}

	// /readyz surfaces the dependencies we rely on, so Cloud Run can mark
	// a revision not-ready when a backing service is unreachable. Both
	// probers are stubs today; the wiring is what matters — a real probe
	// can land without touching composition-root code.
	probers := []connectserver.Prober{
		cmsClient.Prober(),
		fbApp.Prober(),
	}

	// --- HTTP server ---------------------------------------------------
	srv := connectserver.New(connectserver.Config{
		Port:            cfg.Port,
		ShutdownTimeout: time.Duration(cfg.ShutdownGraceSeconds) * time.Second,
	}, handlers, probers)

	logger.InfoContext(ctx, "bff ready",
		"app.bff.phase", "ready",
		"cms.project.id", project.ID,
		"cms.model.id", model.ID,
	)
	if err := srv.Start(ctx); err != nil {
		return err
	}
	logger.InfoContext(context.Background(), "bff stopped cleanly",
		"app.bff.phase", "stopped",
	)
	return nil
}

func errProjectNotFound(alias string) error {
	return errs.Wrap("bff.startup", errs.KindInvalidInput,
		fmt.Errorf("CMS project alias %q not found — deploy U-CSS first", alias))
}

func errModelNotFound(alias string) error {
	return errs.Wrap("bff.startup", errs.KindInvalidInput,
		fmt.Errorf("CMS model alias %q not found — deploy U-CSS first", alias))
}
