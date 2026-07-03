package project

import "testing"

func TestApplyResultStateHelpers(t *testing.T) {
	result := ApplyResult{
		ProjectID:   "project-1",
		ProjectName: "example",
		Revision:    7,
		SpecHash:    "hash-1",
		Applied:     true,
		Unchanged:   false,
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
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if result.Unchanged {
		t.Fatalf("Unchanged = true, want false")
	}
}
