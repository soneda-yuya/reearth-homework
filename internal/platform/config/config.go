// Package config loads application configuration from environment variables.
//
// MustLoad is a generic helper that any cmd/* main can call with its own
// config struct. Required fields that are missing cause the process to exit
// immediately (fail-fast, NFR-PLT-REL-01).
package config

import (
	"log/slog"
	"os"

	"github.com/kelseyhightower/envconfig"
)

// Common is embedded into each deployable-specific config. It covers the
// PLATFORM_* variables every service needs.
type Common struct {
	ServiceName  string `envconfig:"PLATFORM_SERVICE_NAME" required:"true"`
	Env          string `envconfig:"PLATFORM_ENV" required:"true"`
	LogLevel     string `envconfig:"PLATFORM_LOG_LEVEL" default:"INFO"`
	OTelExporter string `envconfig:"PLATFORM_OTEL_EXPORTER" default:"stdout"`
	GCPProjectID string `envconfig:"PLATFORM_GCP_PROJECT_ID" required:"true"`
}

// MustLoad reads environment variables into *cfg. If cfg is not a pointer or
// a required field is missing, the process exits with a fatal log entry.
//
// Deployable-specific structs should embed [Common]:
//
//	type BffConfig struct {
//	    config.Common
//	    Port int `envconfig:"BFF_PORT" default:"8080"`
//	}
func MustLoad[T any](cfg *T) {
	if err := envconfig.Process("", cfg); err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}
}
