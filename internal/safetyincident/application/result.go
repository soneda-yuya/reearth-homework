// Package application is the use-case layer for safetyincident. It wires
// together the domain ports without knowing any HTTP / SDK detail; the
// concrete adapters live in internal/safetyincident/infrastructure.
package application

// Phase identifies which step of per-item processing produced an error. It
// surfaces in metrics (`app.ingestion.run.failed{phase=...}`) and in logs so
// operators can grep for the failing stage.
type Phase string

const (
	PhaseFetch   Phase = "fetch"
	PhaseLookup  Phase = "lookup"
	PhaseExtract Phase = "extract"
	PhaseGeocode Phase = "geocode"
	PhaseUpsert  Phase = "upsert"
	PhasePublish Phase = "publish"
)

// IngestResult summarises a single Run for the composition root's exit log.
// Counters are deliberately exported so tests can assert on them and the
// caller can include them in the final structured log line.
type IngestResult struct {
	Fetched   int
	Skipped   int
	Processed int
	Failed    map[Phase]int
	Published int
}

// newResult returns an IngestResult with the Failed map ready to use.
func newResult() IngestResult {
	return IngestResult{Failed: make(map[Phase]int)}
}
