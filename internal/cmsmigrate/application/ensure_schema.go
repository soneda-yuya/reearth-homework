package application

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// EnsureSchemaUseCase walks a SchemaDefinition top-down and makes the CMS
// match it via idempotent CREATE. Field-level drift (type / required / unique
// / multiple mismatch) is reported but NOT corrected — see U-CSS Design Q2.
type EnsureSchemaUseCase struct {
	applier SchemaApplier
	logger  *slog.Logger
	tracer  trace.Tracer
	meter   metric.Meter

	// Metrics are looked up eagerly so Execute does not pay the cost of
	// creating an instrument on every run.
	projectCreated metric.Int64Counter
	modelCreated   metric.Int64Counter
	fieldCreated   metric.Int64Counter
	driftDetected  metric.Int64Counter
}

// EnsureSchemaInput carries the declaration that should be reflected in CMS.
type EnsureSchemaInput struct {
	Definition domain.SchemaDefinition
}

// EnsureSchemaResult summarises what the run actually did so the composition
// root can log a single structured line at exit.
type EnsureSchemaResult struct {
	ProjectCreated bool
	ModelsCreated  []string
	FieldsCreated  []string // "<model_alias>.<field_alias>"
	DriftWarnings  []DriftWarning
}

// NewEnsureSchemaUseCase wires the use case. A nil logger / tracer / meter
// is replaced with no-op fallbacks so tests can pass nil without guards.
func NewEnsureSchemaUseCase(
	applier SchemaApplier,
	logger *slog.Logger,
	tracer trace.Tracer,
	meter metric.Meter,
) *EnsureSchemaUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	if tracer == nil {
		tracer = tracenoop.NewTracerProvider().Tracer("cmsmigrate")
	}
	if meter == nil {
		meter = metricnoop.NewMeterProvider().Meter("cmsmigrate")
	}
	// Best-effort counters; errors are ignored because a missing meter must
	// not fail the schema migration itself.
	projectCreated, _ := meter.Int64Counter("app.cmsmigrate.project.created")
	modelCreated, _ := meter.Int64Counter("app.cmsmigrate.model.created")
	fieldCreated, _ := meter.Int64Counter("app.cmsmigrate.field.created")
	driftDetected, _ := meter.Int64Counter("app.cmsmigrate.drift.detected")
	return &EnsureSchemaUseCase{
		applier:        applier,
		logger:         logger,
		tracer:         tracer,
		meter:          meter,
		projectCreated: projectCreated,
		modelCreated:   modelCreated,
		fieldCreated:   fieldCreated,
		driftDetected:  driftDetected,
	}
}

// Execute applies the declaration. It validates up-front, then walks the tree
// Project → Model → Field. Any adapter error returns immediately (fail-fast)
// so partial state is left intact for the next idempotent run to pick up.
func (u *EnsureSchemaUseCase) Execute(ctx context.Context, in EnsureSchemaInput) (EnsureSchemaResult, error) {
	ctx, span := u.tracer.Start(ctx, "cmsmigrate.EnsureSchema")
	defer span.End()

	if err := in.Definition.Validate(); err != nil {
		return EnsureSchemaResult{}, errs.Wrap("cmsmigrate.application.Execute.validate", errs.KindInvalidInput, err)
	}

	var result EnsureSchemaResult

	proj, err := u.ensureProject(ctx, in.Definition.Project, &result)
	if err != nil {
		return result, err
	}

	for _, m := range in.Definition.Models {
		if err := u.ensureModel(ctx, proj.ID, m, &result); err != nil {
			return result, err
		}
	}

	u.flushDriftWarnings(ctx, result.DriftWarnings)
	return result, nil
}

func (u *EnsureSchemaUseCase) ensureProject(ctx context.Context, def domain.ProjectDefinition, result *EnsureSchemaResult) (*RemoteProject, error) {
	ctx, span := u.tracer.Start(ctx, "cmsmigrate.EnsureProject", trace.WithAttributes(attribute.String("project.alias", def.Alias)))
	defer span.End()

	existing, err := u.applier.FindProject(ctx, def.Alias)
	if err != nil {
		return nil, errs.Wrap("cmsmigrate.application.FindProject", errs.KindOf(err), err)
	}
	if existing != nil {
		u.logger.InfoContext(ctx, "project exists",
			"app.cmsmigrate.phase", "find-project",
			"project.alias", def.Alias,
		)
		return existing, nil
	}

	created, err := u.applier.CreateProject(ctx, def)
	if err != nil {
		return nil, errs.Wrap("cmsmigrate.application.CreateProject", errs.KindOf(err), err)
	}
	result.ProjectCreated = true
	if u.projectCreated != nil {
		u.projectCreated.Add(ctx, 1, metric.WithAttributes(attribute.String("project.alias", def.Alias)))
	}
	u.logger.InfoContext(ctx, "project created",
		"app.cmsmigrate.phase", "create-project",
		"project.alias", def.Alias,
	)
	return created, nil
}

func (u *EnsureSchemaUseCase) ensureModel(ctx context.Context, projectID string, def domain.ModelDefinition, result *EnsureSchemaResult) error {
	ctx, span := u.tracer.Start(ctx, "cmsmigrate.EnsureModel", trace.WithAttributes(attribute.String("model.alias", def.Alias)))
	defer span.End()

	existing, err := u.applier.FindModel(ctx, projectID, def.Alias)
	if err != nil {
		return errs.Wrap("cmsmigrate.application.FindModel", errs.KindOf(err), err)
	}

	var modelID string
	if existing == nil {
		created, err := u.applier.CreateModel(ctx, projectID, def)
		if err != nil {
			return errs.Wrap("cmsmigrate.application.CreateModel", errs.KindOf(err), err)
		}
		result.ModelsCreated = append(result.ModelsCreated, def.Alias)
		if u.modelCreated != nil {
			u.modelCreated.Add(ctx, 1, metric.WithAttributes(attribute.String("model.alias", def.Alias)))
		}
		u.logger.InfoContext(ctx, "model created",
			"app.cmsmigrate.phase", "create-model",
			"model.alias", def.Alias,
		)
		modelID = created.ID
	} else {
		u.logger.InfoContext(ctx, "model exists",
			"app.cmsmigrate.phase", "find-model",
			"model.alias", def.Alias,
		)
		modelID = existing.ID
	}

	for _, f := range def.Fields {
		if err := u.ensureField(ctx, modelID, def.Alias, f, result); err != nil {
			return err
		}
	}
	return nil
}

func (u *EnsureSchemaUseCase) ensureField(ctx context.Context, modelID, modelAlias string, def domain.FieldDefinition, result *EnsureSchemaResult) error {
	ctx, span := u.tracer.Start(ctx, "cmsmigrate.EnsureField", trace.WithAttributes(
		attribute.String("model.alias", modelAlias),
		attribute.String("field.alias", def.Alias),
	))
	defer span.End()

	existing, err := u.applier.FindField(ctx, modelID, def.Alias)
	if err != nil {
		return errs.Wrap("cmsmigrate.application.FindField", errs.KindOf(err), err)
	}
	if existing != nil {
		if warn := detectFieldDrift(modelAlias, *existing, def); warn != nil {
			result.DriftWarnings = append(result.DriftWarnings, *warn)
			if u.driftDetected != nil {
				u.driftDetected.Add(ctx, 1, metric.WithAttributes(
					attribute.String("resource", warn.Resource),
				))
			}
		}
		return nil
	}

	if _, err := u.applier.CreateField(ctx, modelID, def); err != nil {
		u.logger.ErrorContext(ctx, "create field failed",
			"app.cmsmigrate.phase", "create-field",
			"model.alias", modelAlias,
			"field.alias", def.Alias,
			"err", err,
		)
		return errs.Wrap("cmsmigrate.application.CreateField", errs.KindOf(err), err)
	}
	result.FieldsCreated = append(result.FieldsCreated, fmt.Sprintf("%s.%s", modelAlias, def.Alias))
	if u.fieldCreated != nil {
		u.fieldCreated.Add(ctx, 1, metric.WithAttributes(
			attribute.String("model.alias", modelAlias),
			attribute.String("field.alias", def.Alias),
			attribute.String("field.type", def.Type.String()),
		))
	}
	return nil
}

func (u *EnsureSchemaUseCase) flushDriftWarnings(ctx context.Context, warnings []DriftWarning) {
	if len(warnings) == 0 {
		return
	}
	attrs := make([]any, 0, 2*len(warnings)+2)
	attrs = append(attrs, "app.cmsmigrate.phase", "drift")
	for i, w := range warnings {
		attrs = append(attrs,
			fmt.Sprintf("drift.%d.resource", i), w.Resource,
			fmt.Sprintf("drift.%d.reason", i), w.Reason,
		)
	}
	u.logger.WarnContext(ctx, "schema drift detected (no auto-apply)", attrs...)
}
