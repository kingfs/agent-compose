package project

import (
	"context"
	"errors"
	"testing"
)

func TestApplyServiceDryRunReturnsPreparedResult(t *testing.T) {
	service := NewApplyService(ApplyHooks{
		Normalize: func(context.Context) (ApplyResult, error) {
			return ApplyResult{ProjectName: "demo", SpecHash: "hash-1"}, nil
		},
		CheckStore: func(context.Context, ApplyResult) error {
			return nil
		},
		Prepare: func(_ context.Context, result ApplyResult, revision int64) (ApplyResult, error) {
			if revision != 0 {
				t.Fatalf("revision = %d, want 0", revision)
			}
			return ApplyResult{
				ProjectID:   "project-1",
				ProjectName: result.ProjectName,
				SpecHash:    result.SpecHash,
				Changes: []Change{{
					Kind:     ChangeKindCreated,
					Resource: "project",
					ID:       "project-1",
				}},
			}, nil
		},
		EnsureRuntime: func(context.Context, ApplyResult) error {
			t.Fatalf("EnsureRuntime called for dry run")
			return nil
		},
	})

	result, err := service.Apply(context.Background(), ApplyRequest{DryRun: true})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	if result.Applied {
		t.Fatalf("Applied = true, want false")
	}
	if result.ProjectID != "project-1" || result.ProjectName != "demo" || result.SpecHash != "hash-1" {
		t.Fatalf("result identity = (%q, %q, %q), want project-1 demo hash-1", result.ProjectID, result.ProjectName, result.SpecHash)
	}
	if len(result.Changes) != 1 || result.Changes[0].Kind != ChangeKindCreated {
		t.Fatalf("changes = %#v, want one created change", result.Changes)
	}
}

func TestApplyServiceReturnsValidationIssuesBeforeInfrastructure(t *testing.T) {
	service := NewApplyService(ApplyHooks{
		Normalize: func(context.Context) (ApplyResult, error) {
			return ApplyResult{
				ProjectName: "demo",
				SpecHash:    "hash-1",
				Issues: []ValidationIssue{{
					Field:   "agents[0].name",
					Message: "name is required",
				}},
			}, nil
		},
		CheckStore: func(context.Context, ApplyResult) error {
			t.Fatalf("CheckStore called when validation failed")
			return nil
		},
	})

	result, err := service.Apply(context.Background(), ApplyRequest{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.HasIssues() {
		t.Fatalf("HasIssues() = false, want true")
	}
	if result.Applied || result.DryRun {
		t.Fatalf("state = applied:%v dryRun:%v, want both false", result.Applied, result.DryRun)
	}
}

func TestApplyServiceMapsRuntimeError(t *testing.T) {
	runtimeErr := errors.New("image pull failed")
	service := NewApplyService(ApplyHooks{
		Normalize: func(context.Context) (ApplyResult, error) {
			return ApplyResult{ProjectName: "demo", SpecHash: "hash-1"}, nil
		},
		Prepare: func(context.Context, ApplyResult, int64) (ApplyResult, error) {
			return ApplyResult{ProjectID: "project-1"}, nil
		},
		EnsureRuntime: func(context.Context, ApplyResult) error {
			return runtimeErr
		},
	})

	_, err := service.Apply(context.Background(), ApplyRequest{})
	if err == nil {
		t.Fatalf("Apply() error = nil, want runtime error")
	}
	if got := ErrorKindOf(err); got != ErrorKindRuntime {
		t.Fatalf("ErrorKindOf() = %q, want %q", got, ErrorKindRuntime)
	}
	if !errors.Is(err, runtimeErr) {
		t.Fatalf("Apply() error does not wrap runtime error")
	}
}

func TestApplyServiceComputesUnchangedFinalResult(t *testing.T) {
	service := NewApplyService(ApplyHooks{
		Normalize: func(context.Context) (ApplyResult, error) {
			return ApplyResult{ProjectName: "demo", SpecHash: "hash-1"}, nil
		},
		Prepare: func(context.Context, ApplyResult, int64) (ApplyResult, error) {
			return ApplyResult{ProjectID: "project-1"}, nil
		},
		Persist: func(context.Context, ApplyResult) (ApplyPersistResult, error) {
			return ApplyPersistResult{
				Result: ApplyResult{
					ProjectID: "project-1",
					Revision:  3,
				},
				ProjectFound:     true,
				RevisionCreated:  false,
				ProjectUnchanged: true,
			}, nil
		},
		Reconcile: func(context.Context, ApplyResult, ApplyPersistResult) (ApplyReconcileResult, error) {
			return ApplyReconcileResult{
				Result: ApplyResult{
					Changes: []Change{{
						Kind:     ChangeKindUnchanged,
						Resource: "project_agent",
						ID:       "agent-1",
					}},
				},
				ResourcesUnchanged: true,
			}, nil
		},
	})

	result, err := service.Apply(context.Background(), ApplyRequest{})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if !result.Unchanged {
		t.Fatalf("Unchanged = false, want true")
	}
	if result.Revision != 3 {
		t.Fatalf("Revision = %d, want 3", result.Revision)
	}
}
