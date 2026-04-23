// Package firebasex wraps the Firebase Admin SDK factory for the notifier
// unit (and any future caller). The rest of the codebase interacts with
// firebase.App via this thin facade so swapping credential sources or
// mocking at the composition-root boundary stays straightforward.
package firebasex

import (
	"context"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Config holds Firebase project data. Authentication always uses Application
// Default Credentials: the Cloud Run Runtime SA supplies them in production;
// local dev uses `gcloud auth application-default login` or sets
// GOOGLE_APPLICATION_CREDENTIALS. We don't expose an explicit service account
// JSON override because we've never needed it — add it here if we do.
type Config struct {
	ProjectID string
}

// App wraps a firebase.App plus the singleton clients derived from it.
// Callers grab a Firestore / Messaging client via the accessor methods and
// close the App at process exit via Close.
type App struct {
	cfg       Config
	app       *firebase.App
	firestore *firestore.Client
	messaging *messaging.Client
}

// NewApp initialises the Firebase Admin SDK using ADC. Production reads the
// Runtime SA attached to Cloud Run; local dev picks up whatever ADC token
// `gcloud auth application-default login` produced.
func NewApp(ctx context.Context, cfg Config) (*App, error) {
	if cfg.ProjectID == "" {
		return nil, errs.Wrap("firebasex.config", errs.KindInvalidInput,
			errMissingProjectID)
	}
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: cfg.ProjectID})
	if err != nil {
		return nil, errs.Wrap("firebasex.new_app", errs.KindExternal, err)
	}
	return &App{cfg: cfg, app: app}, nil
}

// ProjectID returns the configured GCP / Firebase project id.
func (a *App) ProjectID() string { return a.cfg.ProjectID }

// Firestore returns the singleton Firestore client. Construction is lazy so
// callers that only need Messaging (or vice versa) don't pay the startup
// cost of the other.
func (a *App) Firestore(ctx context.Context) (*firestore.Client, error) {
	if a.firestore != nil {
		return a.firestore, nil
	}
	c, err := a.app.Firestore(ctx)
	if err != nil {
		return nil, errs.Wrap("firebasex.firestore", errs.KindExternal, err)
	}
	a.firestore = c
	return c, nil
}

// Messaging returns the singleton FCM client.
func (a *App) Messaging(ctx context.Context) (*messaging.Client, error) {
	if a.messaging != nil {
		return a.messaging, nil
	}
	c, err := a.app.Messaging(ctx)
	if err != nil {
		return nil, errs.Wrap("firebasex.messaging", errs.KindExternal, err)
	}
	a.messaging = c
	return c, nil
}

// Close releases the Firestore client (Messaging client does not need explicit
// close; it rides on the same gRPC connection pool).
func (a *App) Close(_ context.Context) error {
	if a.firestore == nil {
		return nil
	}
	return a.firestore.Close()
}

type stringErr string

func (s stringErr) Error() string { return string(s) }

var errMissingProjectID = stringErr("ProjectID is required")
