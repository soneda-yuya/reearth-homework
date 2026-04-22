// Package firebasex wraps the Firebase Admin SDK factory. Actual Firebase
// dependencies are added by the unit that first needs them (U-USR / U-NTF);
// U-PLT ships the shape so cmd/*/main.go wiring is possible.
package firebasex

import "context"

// Config holds Firebase project data. ServiceAccountJSON is the secret payload
// resolved from Secret Manager at startup, or nil to use ADC.
type Config struct {
	ProjectID          string
	ServiceAccountJSON []byte
}

// App is the Firebase application handle passed to downstream adapters.
type App struct {
	cfg Config
}

// NewApp creates an App. When the real SDK is wired it will call
// firebase.NewApp(ctx, &firebase.Config{ProjectID: cfg.ProjectID}) here.
func NewApp(ctx context.Context, cfg Config) (*App, error) {
	return &App{cfg: cfg}, nil
}

// ProjectID returns the configured GCP / Firebase project id.
func (a *App) ProjectID() string { return a.cfg.ProjectID }

// Close is a no-op; keeps the Closer contract for deferred cleanup.
func (a *App) Close(ctx context.Context) error { return nil }
