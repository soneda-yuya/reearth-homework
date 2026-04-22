// Package observability initialises slog and OpenTelemetry (Metrics + Traces)
// for all deployables. A single Setup call configures logging, tracing, and
// metrics according to the deployable's environment.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config configures the observability pipeline.
type Config struct {
	// ServiceName is the logical deployable, e.g. "bff" / "ingestion".
	ServiceName string
	// Env is "dev" or "prod".
	Env string
	// LogLevel is one of DEBUG / INFO / WARN / ERROR (case-insensitive).
	LogLevel string
	// ExporterKind is "stdout" or "gcp". Unknown values fall back to stdout.
	ExporterKind string
}

// ShutdownFunc flushes and closes the observability pipeline.
type ShutdownFunc func(context.Context) error

type ctxKey int

const (
	loggerKey ctxKey = iota
)

var (
	globalLogger *slog.Logger
	globalTracer trace.Tracer
	globalMeter  metric.Meter
)

// Setup configures slog and OpenTelemetry. Callers must defer the returned
// ShutdownFunc so spans and metrics are flushed at exit.
func Setup(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	// --- slog ------------------------------------------------------------
	level := parseLevel(cfg.LogLevel)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})
	logger := slog.New(handler).With(
		slog.String("service", cfg.ServiceName),
		slog.String("env", cfg.Env),
	)
	slog.SetDefault(logger)
	globalLogger = logger

	// --- OpenTelemetry resource -----------------------------------------
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironment(cfg.Env),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: resource: %w", err)
	}

	// --- Trace exporter -------------------------------------------------
	// Only stdout is implemented here; the "gcp" branch would plug in the
	// Cloud Trace exporter. We log a warning instead of failing so deploys
	// are not blocked while the gcp exporter is wired up later.
	// Pretty print is dev-only: prod/staging should emit compact JSON to
	// avoid log-volume blow-up in Cloud Logging.
	traceOpts := []stdouttrace.Option{}
	if strings.EqualFold(cfg.Env, "dev") || strings.EqualFold(cfg.Env, "test") {
		traceOpts = append(traceOpts, stdouttrace.WithPrettyPrint())
	}
	traceExporter, err := stdouttrace.New(traceOpts...)
	if err != nil {
		return nil, fmt.Errorf("observability: trace exporter: %w", err)
	}
	if !strings.EqualFold(cfg.ExporterKind, "stdout") && !strings.EqualFold(cfg.ExporterKind, "gcp") {
		logger.Warn("unknown OTel exporter, falling back to stdout", "exporter", cfg.ExporterKind)
	}
	if strings.EqualFold(cfg.ExporterKind, "gcp") {
		logger.Info("gcp exporter selected; stdout is used as fallback until cloud-trace exporter is wired")
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	globalTracer = tp.Tracer(cfg.ServiceName)

	// --- Metric exporter ------------------------------------------------
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		return nil, fmt.Errorf("observability: metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	globalMeter = mp.Meter(cfg.ServiceName)

	shutdown := func(ctx context.Context) error {
		var firstErr error
		if err := tp.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("trace shutdown: %w", err)
		}
		if err := mp.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("metric shutdown: %w", err)
		}
		return firstErr
	}
	return shutdown, nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Logger returns the logger attached to ctx (via With) or the global logger.
func Logger(ctx context.Context) *slog.Logger {
	if v, ok := ctx.Value(loggerKey).(*slog.Logger); ok && v != nil {
		return v
	}
	if globalLogger != nil {
		return globalLogger
	}
	return slog.Default()
}

// Tracer returns the global tracer created by Setup.
func Tracer(ctx context.Context) trace.Tracer {
	if globalTracer != nil {
		return globalTracer
	}
	return otel.Tracer("default")
}

// Meter returns the global meter created by Setup.
func Meter(ctx context.Context) metric.Meter {
	if globalMeter != nil {
		return globalMeter
	}
	return otel.Meter("default")
}

// With returns a new context whose logger carries the given attribute.
func With(ctx context.Context, key string, value any) context.Context {
	child := Logger(ctx).With(slog.Any(key, value))
	return context.WithValue(ctx, loggerKey, child)
}
