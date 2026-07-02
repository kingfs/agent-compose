package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMatchesListOptionsAndBounds(t *testing.T) {
	now := time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)
	item := &Session{
		Summary: SessionSummary{
			ID:            "session-branch",
			Title:         "Branch Session",
			TriggerSource: "script:loader-1",
			Driver:        "docker",
			VMStatus:      VMStatusRunning,
			WorkspacePath: "/workspaces/branch",
			CreatedAt:     now,
			UpdatedAt:     now.Add(time.Minute),
		},
		WorkspaceID: "workspace-1",
		Workspace:   &SessionWorkspace{ID: "workspace-1", Name: "Workspace One", Type: "file"},
	}
	if !MatchesListOptions(item, SessionListOptions{
		SessionType:        SessionTypeScript,
		TriggerSourceQuery: "loader",
		TitleQuery:         "branch",
		WorkspaceQuery:     "workspace one",
		Driver:             "DOCKER",
		VMStatus:           "running",
		CreatedFrom:        now.Add(-time.Second),
		CreatedTo:          now.Add(time.Second),
		UpdatedFrom:        now,
		UpdatedTo:          now.Add(2 * time.Minute),
	}) {
		t.Fatalf("session should match full list options")
	}
	for _, options := range []SessionListOptions{
		{SessionType: SessionTypeManual},
		{TriggerSourceQuery: "missing"},
		{TitleQuery: "missing"},
		{WorkspaceQuery: "missing"},
		{Driver: "boxlite"},
		{VMStatus: VMStatusStopped},
		{CreatedFrom: now.Add(time.Second)},
		{CreatedTo: now.Add(-time.Second)},
		{UpdatedFrom: now.Add(2 * time.Minute)},
		{UpdatedTo: now.Add(-time.Second)},
	} {
		if MatchesListOptions(item, options) {
			t.Fatalf("session unexpectedly matched options %#v", options)
		}
	}
	if MatchesListOptions(nil, SessionListOptions{}) {
		t.Fatalf("nil session matched list options")
	}

	offset, limit := NormalizeListBounds(-1, 0)
	if offset != 0 || limit != defaultSessionListLimit {
		t.Fatalf("NormalizeListBounds = %d/%d", offset, limit)
	}
	if got := Paginate([]*Session{item}, 5, 10); got != nil {
		t.Fatalf("Paginate beyond end = %#v", got)
	}
}

func TestNormalizeTriggerSourceFromTags(t *testing.T) {
	tags := []SessionTag{{Name: "origin", Value: "loader"}, {Name: "loader_id", Value: "loader-9"}}
	if got := NormalizeTriggerSource("", tags); got != "script:loader-9" {
		t.Fatalf("NormalizeTriggerSource tags = %q", got)
	}
	if got := NormalizeTriggerSource("custom", tags); got != "script:loader-9" {
		t.Fatalf("NormalizeTriggerSource custom fallback = %q", got)
	}
	if got := NormalizeTriggerSource(SessionTypeManual, tags); got != SessionTypeManual {
		t.Fatalf("NormalizeTriggerSource manual = %q", got)
	}
	if got := TypeFromTriggerSource("script:loader-9"); got != SessionTypeScript {
		t.Fatalf("TypeFromTriggerSource script = %q", got)
	}
}

func TestExecHelpersRecoverAndMergeResults(t *testing.T) {
	cellDir := filepath.Join(t.TempDir(), "cell")
	if err := os.MkdirAll(cellDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := WriteCellArtifacts(cellDir, "echo hi", ExecResult{
		Stdout:   "out\n",
		Stderr:   "err\n",
		Output:   "combined\n",
		ExitCode: 7,
		Success:  false,
	}); err != nil {
		t.Fatalf("WriteCellArtifacts returned error: %v", err)
	}
	recovered := RecoverExecResultFromCellArtifacts(cellDir, ExecResult{
		Stdout:   "fallback-out",
		Stderr:   "fallback-err",
		Output:   "fallback-output",
		ExitCode: 1,
		Success:  false,
	})
	if recovered.Stdout != "out\n" || recovered.Stderr != "err\n" || recovered.Output != "combined\n" {
		t.Fatalf("recovered streams = %#v", recovered)
	}
	if recovered.ExitCode != 7 || recovered.Success {
		t.Fatalf("recovered exit state = %#v", recovered)
	}

	stdoutOnlyDir := filepath.Join(t.TempDir(), "stdout-only")
	if err := os.MkdirAll(stdoutOnlyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stdoutOnlyDir, "stdout.txt"), []byte("out"), 0o644); err != nil {
		t.Fatalf("WriteFile stdout returned error: %v", err)
	}
	stdoutOnly := RecoverExecResultFromCellArtifacts(stdoutOnlyDir, ExecResult{})
	if stdoutOnly.Output != "out" {
		t.Fatalf("blank output was not rebuilt from streams: %#v", stdoutOnly)
	}

	merged := MergeExecResults(ExecResult{Stdout: "primary", Success: true}, ExecResult{
		Stdout:   "fallback-out",
		Stderr:   "fallback-err",
		Output:   "fallback-output",
		ExitCode: 3,
		Success:  false,
	})
	if merged.Stdout != "primary" || merged.Stderr != "fallback-err" || merged.Output != "fallback-output" || merged.ExitCode != 3 || !merged.Success {
		t.Fatalf("MergeExecResults returned %#v", merged)
	}

	var accumulator ExecStreamAccumulator
	accumulator.WriteChunk(ExecChunk{Text: "out"})
	accumulator.WriteChunk(ExecChunk{Text: "err", IsStderr: true})
	accumulator.WriteChunk(ExecChunk{})
	result := accumulator.Result(0, true)
	if result.Stdout != "out" || result.Stderr != "err" || result.Output != "outerr" || !result.Success {
		t.Fatalf("accumulated result = %#v", result)
	}
}
