package project

// ValidationIssue describes a usecase validation problem in a transport-neutral
// shape. Field is optional for whole-document issues.
type ValidationIssue struct {
	Field   string
	Message string
}

// ChangeKind describes the kind of mutation represented in an ApplyResult.
type ChangeKind string

const (
	ChangeKindCreated   ChangeKind = "created"
	ChangeKindUpdated   ChangeKind = "updated"
	ChangeKindDeleted   ChangeKind = "deleted"
	ChangeKindUnchanged ChangeKind = "unchanged"
)

// Change is a lightweight description of a project resource mutation.
type Change struct {
	Kind     ChangeKind
	Resource string
	ID       string
	Message  string
}

// ApplyResult is the transport-agnostic result shape for project apply flows.
type ApplyResult struct {
	ProjectID   string
	ProjectName string
	Revision    int64
	SpecHash    string
	DryRun      bool
	Applied     bool
	Unchanged   bool
	Changes     []Change
	Issues      []ValidationIssue
}

func (r ApplyResult) HasIssues() bool {
	return len(r.Issues) > 0
}

func (r ApplyResult) Changed() bool {
	return len(r.Changes) > 0
}
