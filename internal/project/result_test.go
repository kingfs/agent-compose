package project

import "testing"

func TestApplyResultStateHelpers(t *testing.T) {
	result := ApplyResult{
		Changes: []Change{{
			Kind:     ChangeKindUpdated,
			Resource: "agent",
			ID:       "agent-1",
		}},
		Issues: []ValidationIssue{{
			Field:   "agents[0].name",
			Message: "name is required",
		}},
	}

	if !result.Changed() {
		t.Fatalf("Changed() = false, want true")
	}
	if !result.HasIssues() {
		t.Fatalf("HasIssues() = false, want true")
	}
}
