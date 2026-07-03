package project

import (
	"context"
	"errors"
	"fmt"
)

// ApplyRequest carries transport-neutral apply options.
type ApplyRequest struct {
	DryRun bool
}

// ApplyPersistResult summarizes persistence decisions needed by the apply
// usecase to compute final state.
type ApplyPersistResult struct {
	Result           ApplyResult
	ProjectFound     bool
	RevisionCreated  bool
	ProjectUnchanged bool
}

// ApplyReconcileResult summarizes runtime reconciliation decisions needed by
// the apply usecase to compute final state.
type ApplyReconcileResult struct {
	Result             ApplyResult
	ResourcesUnchanged bool
	Failure            *ApplyResult
}

// ApplyHooks adapts infrastructure-specific project apply steps to the domain
// usecase without exposing transport or storage record types to this package.
type ApplyHooks struct {
	Normalize     func(context.Context) (ApplyResult, error)
	CheckStore    func(context.Context, ApplyResult) error
	Prepare       func(context.Context, ApplyResult, int64) (ApplyResult, error)
	EnsureRuntime func(context.Context, ApplyResult) error
	Persist       func(context.Context, ApplyResult) (ApplyPersistResult, error)
	Reload        func(context.Context, ApplyResult, ApplyPersistResult) (ApplyResult, error)
	Reconcile     func(context.Context, ApplyResult, ApplyPersistResult) (ApplyReconcileResult, error)
}

// ApplyService coordinates the project apply usecase. It owns the apply state
// machine and result/error decisions while callers provide concrete adapters.
type ApplyService struct {
	hooks ApplyHooks
}

func NewApplyService(hooks ApplyHooks) *ApplyService {
	return &ApplyService{hooks: hooks}
}

func (s *ApplyService) Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error) {
	if s == nil {
		return ApplyResult{}, NewError(ErrorKindUnknown, "project apply service is required", nil)
	}
	if s.hooks.Normalize == nil {
		return ApplyResult{}, NewError(ErrorKindUnknown, "project apply normalize hook is required", nil)
	}
	normalized, err := s.hooks.Normalize(ctx)
	if err != nil {
		return ApplyResult{}, ensureApplyError(ErrorKindUnknown, err)
	}
	if normalized.HasIssues() {
		return normalized, nil
	}
	if s.hooks.CheckStore != nil {
		if err := s.hooks.CheckStore(ctx, normalized); err != nil {
			return ApplyResult{}, ensureApplyError(ErrorKindUnknown, err)
		}
	}
	prepared, err := s.prepare(ctx, normalized, 0)
	if err != nil {
		return ApplyResult{}, err
	}
	if prepared.HasIssues() {
		return prepared, nil
	}
	if req.DryRun {
		prepared.DryRun = true
		prepared.Applied = false
		prepared.Unchanged = false
		return prepared, nil
	}
	if s.hooks.EnsureRuntime != nil {
		if err := s.hooks.EnsureRuntime(ctx, prepared); err != nil {
			return ApplyResult{}, ensureApplyError(ErrorKindRuntime, err)
		}
	}
	if s.hooks.Persist == nil {
		return ApplyResult{}, NewError(ErrorKindUnknown, "project apply persist hook is required", nil)
	}
	persisted, err := s.hooks.Persist(ctx, prepared)
	if err != nil {
		return ApplyResult{}, ensureApplyError(ErrorKindStorage, err)
	}
	current := mergeApplyResult(prepared, persisted.Result)
	if s.hooks.Reload != nil {
		current, err = s.hooks.Reload(ctx, current, persisted)
		if err != nil {
			return ApplyResult{}, ensureApplyError(ErrorKindStorage, err)
		}
	}
	if s.hooks.Reconcile == nil {
		return ApplyResult{}, NewError(ErrorKindUnknown, "project apply reconcile hook is required", nil)
	}
	reconciled, err := s.hooks.Reconcile(ctx, current, persisted)
	if err != nil {
		return ApplyResult{}, err
	}
	if reconciled.Failure != nil {
		return *reconciled.Failure, nil
	}
	final := mergeApplyResult(current, reconciled.Result)
	final.Applied = true
	final.DryRun = false
	final.Unchanged = persisted.ProjectFound &&
		!persisted.RevisionCreated &&
		persisted.ProjectUnchanged &&
		reconciled.ResourcesUnchanged
	return final, nil
}

func (s *ApplyService) prepare(ctx context.Context, base ApplyResult, revision int64) (ApplyResult, error) {
	if s.hooks.Prepare == nil {
		return ApplyResult{}, NewError(ErrorKindUnknown, "project apply prepare hook is required", nil)
	}
	prepared, err := s.hooks.Prepare(ctx, base, revision)
	if err != nil {
		return ApplyResult{}, ensureApplyError(ErrorKindValidation, err)
	}
	return mergeApplyResult(base, prepared), nil
}

func ensureApplyError(defaultKind ErrorKind, err error) error {
	if err == nil {
		return nil
	}
	var projectErr *Error
	if errors.As(err, &projectErr) {
		return err
	}
	return NewError(defaultKind, err.Error(), err)
}

func mergeApplyResult(base, next ApplyResult) ApplyResult {
	result := base
	if next.ProjectID != "" {
		result.ProjectID = next.ProjectID
	}
	if next.ProjectName != "" {
		result.ProjectName = next.ProjectName
	}
	if next.Revision != 0 {
		result.Revision = next.Revision
	}
	if next.SpecHash != "" {
		result.SpecHash = next.SpecHash
	}
	result.DryRun = next.DryRun
	result.Applied = next.Applied
	result.Unchanged = next.Unchanged
	if next.Changes != nil {
		result.Changes = next.Changes
	}
	if next.Issues != nil {
		result.Issues = next.Issues
	}
	return result
}

func ApplyStoreRequiredError(projectName string) error {
	if projectName == "" {
		return NewError(ErrorKindUnknown, "apply project: config store is required", nil)
	}
	return NewError(ErrorKindUnknown, fmt.Sprintf("apply project %s: config store is required", projectName), nil)
}
