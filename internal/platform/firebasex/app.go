// Package firebasex wraps the Firebase Admin SDK factory for the notifier
// unit (and any future caller). The rest of the codebase interacts with
// firebase.App via this thin facade so swapping credential sources or
// mocking at the composition-root boundary stays straightforward.
package firebasex

import (
	"context"
	"sync"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"firebase.google.com/go/v4/messaging"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
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
// close the App at process exit via Close. The lazy client fields are
// guarded by sync.Once so Firestore() / Messaging() are safe to call
// concurrently.
type App struct {
	cfg Config
	app *firebase.App

	firestoreOnce sync.Once
	firestore     *firestore.Client
	firestoreErr  error

	messagingOnce sync.Once
	messaging     *messaging.Client
	messagingErr  error

	authOnce sync.Once
	auth     *auth.Client
	authErr  error
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
// cost of the other. sync.Once guarantees concurrent callers see the same
// client (or the same initial error).
func (a *App) Firestore(ctx context.Context) (*firestore.Client, error) {
	a.firestoreOnce.Do(func() {
		c, err := a.app.Firestore(ctx)
		if err != nil {
			a.firestoreErr = errs.Wrap("firebasex.firestore", errs.KindExternal, err)
			return
		}
		a.firestore = c
	})
	if a.firestoreErr != nil {
		return nil, a.firestoreErr
	}
	return a.firestore, nil
}

// Messaging returns the singleton FCM client. Same sync.Once contract as
// Firestore.
func (a *App) Messaging(ctx context.Context) (*messaging.Client, error) {
	a.messagingOnce.Do(func() {
		c, err := a.app.Messaging(ctx)
		if err != nil {
			a.messagingErr = errs.Wrap("firebasex.messaging", errs.KindExternal, err)
			return
		}
		a.messaging = c
	})
	if a.messagingErr != nil {
		return nil, a.messagingErr
	}
	return a.messaging, nil
}

// Auth returns the singleton Firebase Auth client (used by the BFF unit to
// verify ID tokens). Same sync.Once contract as Firestore / Messaging.
func (a *App) Auth(ctx context.Context) (*auth.Client, error) {
	a.authOnce.Do(func() {
		c, err := a.app.Auth(ctx)
		if err != nil {
			a.authErr = errs.Wrap("firebasex.auth", errs.KindExternal, err)
			return
		}
		a.auth = c
	})
	if a.authErr != nil {
		return nil, a.authErr
	}
	return a.auth, nil
}

// Close releases the Firestore client (Messaging / Auth clients do not need
// explicit close; they ride on the same gRPC / HTTP pools).
func (a *App) Close(_ context.Context) error {
	if a.firestore == nil {
		return nil
	}
	return a.firestore.Close()
}

type stringErr string

func (s stringErr) Error() string { return string(s) }

var errMissingProjectID = stringErr("ProjectID is required")
