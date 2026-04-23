package application_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// fakeApplier is an in-memory SchemaApplier for use case tests. It records
// every call so tests can assert the sequence (find-then-create) as well as
// the final state.
type fakeApplier struct {
	projects map[string]*application.RemoteProject
	models   map[string]*application.RemoteModel // key = projectID + "/" + alias
	fields   map[string]*application.RemoteField // key = modelID + "/" + alias
	idSeq    atomic.Int64

	// failCreateField, if set, returns the provided error the first time a
	// CreateField call matches fieldAlias. Used to exercise fail-fast.
	failCreateField string
	failErr         error

	// raceCreateProject simulates a concurrent writer that inserted the
	// project named raceCreateProject between FindProject and CreateProject:
	// the next CreateProject call for that alias returns KindConflict, and
	// the next FindProject for that alias returns the seeded record.
	raceCreateProject     string
	raceCreateProjectSeed *application.RemoteProject

	// raceCreateModel and raceCreateField behave the same way for their
	// respective resources. The key for race* maps embeds the parent ID so
	// the simulation matches the exact (parent, alias) FindXxx will issue.
	raceCreateModel     string // alias the next CreateModel should conflict on
	raceCreateModelSeed *application.RemoteModel
	raceCreateField     string // alias the next CreateField should conflict on
	raceCreateFieldSeed *application.RemoteField

	// calls accumulates a breadcrumb trail for ordering assertions.
	calls []string
}

func newFakeApplier() *fakeApplier {
	return &fakeApplier{
		projects: map[string]*application.RemoteProject{},
		models:   map[string]*application.RemoteModel{},
		fields:   map[string]*application.RemoteField{},
	}
}

func (f *fakeApplier) nextID(prefix string) string {
	n := f.idSeq.Add(1)
	return prefix + "-" + itoa(n)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func (f *fakeApplier) FindProject(_ context.Context, alias string) (*application.RemoteProject, error) {
	f.calls = append(f.calls, "FindProject:"+alias)
	return f.projects[alias], nil
}

func (f *fakeApplier) CreateProject(_ context.Context, def domain.ProjectDefinition) (*application.RemoteProject, error) {
	f.calls = append(f.calls, "CreateProject:"+def.Alias)
	if f.raceCreateProject != "" && def.Alias == f.raceCreateProject {
		// Plant the racing writer's record so the next FindProject finds it.
		f.projects[def.Alias] = f.raceCreateProjectSeed
		f.raceCreateProject = ""
		return nil, errs.Wrap("fake.CreateProject", errs.KindConflict, errors.New("raced"))
	}
	if _, dup := f.projects[def.Alias]; dup {
		return nil, errs.Wrap("fake.CreateProject", errs.KindConflict, errors.New("already exists"))
	}
	p := &application.RemoteProject{ID: f.nextID("p"), Alias: def.Alias, Name: def.Name}
	f.projects[def.Alias] = p
	return p, nil
}

func (f *fakeApplier) FindModel(_ context.Context, projectID, alias string) (*application.RemoteModel, error) {
	f.calls = append(f.calls, "FindModel:"+projectID+"/"+alias)
	return f.models[projectID+"/"+alias], nil
}

func (f *fakeApplier) CreateModel(_ context.Context, projectID string, def domain.ModelDefinition) (*application.RemoteModel, error) {
	f.calls = append(f.calls, "CreateModel:"+projectID+"/"+def.Alias)
	key := projectID + "/" + def.Alias
	if f.raceCreateModel != "" && def.Alias == f.raceCreateModel {
		f.models[key] = f.raceCreateModelSeed
		f.raceCreateModel = ""
		return nil, errs.Wrap("fake.CreateModel", errs.KindConflict, errors.New("raced"))
	}
	if _, dup := f.models[key]; dup {
		return nil, errs.Wrap("fake.CreateModel", errs.KindConflict, errors.New("already exists"))
	}
	m := &application.RemoteModel{ID: f.nextID("m"), Alias: def.Alias, Name: def.Name}
	f.models[key] = m
	return m, nil
}

func (f *fakeApplier) FindField(_ context.Context, modelID, alias string) (*application.RemoteField, error) {
	f.calls = append(f.calls, "FindField:"+modelID+"/"+alias)
	return f.fields[modelID+"/"+alias], nil
}

func (f *fakeApplier) CreateField(_ context.Context, modelID string, def domain.FieldDefinition) (*application.RemoteField, error) {
	f.calls = append(f.calls, "CreateField:"+modelID+"/"+def.Alias)
	if f.failCreateField != "" && def.Alias == f.failCreateField {
		f.failCreateField = "" // only fail once
		return nil, f.failErr
	}
	if f.raceCreateField != "" && def.Alias == f.raceCreateField {
		f.fields[modelID+"/"+def.Alias] = f.raceCreateFieldSeed
		f.raceCreateField = ""
		return nil, errs.Wrap("fake.CreateField", errs.KindConflict, errors.New("raced"))
	}
	key := modelID + "/" + def.Alias
	rf := &application.RemoteField{
		ID: f.nextID("f"), Alias: def.Alias, Type: def.Type,
		Required: def.Required, Unique: def.Unique, Multiple: def.Multiple,
	}
	f.fields[key] = rf
	return rf, nil
}

// countCalls returns the number of recorded calls whose prefix matches.
// Useful for asserting "CreateField was invoked N times".
func (f *fakeApplier) countCalls(prefix string) int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}
