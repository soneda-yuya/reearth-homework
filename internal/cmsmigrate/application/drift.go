package application

import (
	"fmt"

	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/domain"
)

// DriftWarning records one resource whose CMS-side state differs from the
// declaration. cmsmigrate does NOT auto-correct; the use case emits these on
// slog.Warn and leaves the decision to the operator (U-CSS Design Q2 [A]).
type DriftWarning struct {
	// Resource is a human label like "Field:safety-incident.title" that points
	// at the drifting resource in logs.
	Resource string
	Reason   string
}

// detectFieldDrift compares what the CMS returned against what we declared.
// Unknown / non-blocking differences (e.g. ID, underlying CMS type aliases)
// are ignored on purpose so the drift signal stays high-signal.
//
// Uniqueness is deliberately NOT compared: reearth-cms's Integration API does
// not surface the unique flag on field GET, so RemoteField.Unique is always
// false. Comparing it would emit a permanent drift warning for every field
// declared Unique (notably key_cd), drowning real drift in noise. We accept
// that undetected unique flips on the CMS side will not surface here and
// rely on schema changes being driven through cmsmigrate.
func detectFieldDrift(modelAlias string, got RemoteField, want domain.FieldDefinition) *DriftWarning {
	var reasons []string
	if got.Type != want.Type {
		reasons = append(reasons, fmt.Sprintf("type=%s want=%s", got.Type, want.Type))
	}
	if got.Required != want.Required {
		reasons = append(reasons, fmt.Sprintf("required=%t want=%t", got.Required, want.Required))
	}
	if got.Multiple != want.Multiple {
		reasons = append(reasons, fmt.Sprintf("multiple=%t want=%t", got.Multiple, want.Multiple))
	}
	if len(reasons) == 0 {
		return nil
	}
	return &DriftWarning{
		Resource: fmt.Sprintf("Field:%s.%s", modelAlias, want.Alias),
		Reason:   joinReasons(reasons),
	}
}

func joinReasons(rs []string) string {
	out := ""
	for i, r := range rs {
		if i > 0 {
			out += "; "
		}
		out += r
	}
	return out
}
