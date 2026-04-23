package config_test

import (
	"testing"

	"github.com/kelseyhightower/envconfig"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/config"
)

type testConfig struct {
	config.Common
	CustomField string `envconfig:"TEST_CUSTOM_FIELD" required:"true"`
}

func setEnv(t *testing.T, k, v string) {
	t.Helper()
	t.Setenv(k, v)
}

func TestMustLoad_Happy(t *testing.T) {
	setEnv(t, "PLATFORM_SERVICE_NAME", "bff")
	setEnv(t, "PLATFORM_ENV", "dev")
	setEnv(t, "PLATFORM_GCP_PROJECT_ID", "overseas-safety-map")
	setEnv(t, "TEST_CUSTOM_FIELD", "hello")

	var cfg testConfig
	config.MustLoad(&cfg)

	if cfg.ServiceName != "bff" {
		t.Errorf("ServiceName = %q, want bff", cfg.ServiceName)
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("LogLevel should default to INFO, got %q", cfg.LogLevel)
	}
	if cfg.OTelExporter != "stdout" {
		t.Errorf("OTelExporter should default to stdout, got %q", cfg.OTelExporter)
	}
	if cfg.CustomField != "hello" {
		t.Errorf("CustomField = %q, want hello", cfg.CustomField)
	}
}

// Bypass MustLoad (which os.Exit's on failure) by calling envconfig directly
// with a struct whose required field has no matching env var. This verifies
// the required-tag semantics that MustLoad relies on. We deliberately use
// a unique env var name so it is guaranteed not to be set in the test process.
func TestRequiredField_FailsWhenMissing(t *testing.T) {
	type narrow struct {
		MustBeSet string `envconfig:"CONFIG_TEST_UNIQUE_FIELD_zQv8Rn" required:"true"`
	}
	var cfg narrow
	if err := envconfig.Process("", &cfg); err == nil {
		t.Fatalf("expected missing required env var to fail")
	}
}
