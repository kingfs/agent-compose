package project

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestStableIDHelpers(t *testing.T) {
	projectID, err := StableProjectID("demo", filepath.Join("tmp", "agent-compose.yml"))
	if err != nil {
		t.Fatalf("StableProjectID returned error: %v", err)
	}
	sameProjectID, err := StableProjectID("demo", filepath.Join("tmp", "agent-compose.yml"))
	if err != nil {
		t.Fatalf("second StableProjectID returned error: %v", err)
	}
	if sameProjectID != projectID {
		t.Fatalf("project id changed: %q != %q", sameProjectID, projectID)
	}
	otherProjectID, err := StableProjectID("demo", filepath.Join("other", "agent-compose.yml"))
	if err != nil {
		t.Fatalf("other StableProjectID returned error: %v", err)
	}
	if otherProjectID == projectID {
		t.Fatalf("project id did not include source path: %q", projectID)
	}

	agentID, err := StableManagedAgentID(projectID, "reviewer")
	if err != nil {
		t.Fatalf("StableManagedAgentID returned error: %v", err)
	}
	schedulerID, err := StableSchedulerID(projectID, "reviewer", "")
	if err != nil {
		t.Fatalf("StableSchedulerID returned error: %v", err)
	}
	loaderID, err := StableManagedLoaderID(projectID, "reviewer", "")
	if err != nil {
		t.Fatalf("StableManagedLoaderID returned error: %v", err)
	}
	triggerID, err := StableManagedTriggerID(projectID, "reviewer", "", "", 0)
	if err != nil {
		t.Fatalf("StableManagedTriggerID returned error: %v", err)
	}
	runID, err := StableRunID(projectID, "reviewer", RunSourceManual, "client-request-1")
	if err != nil {
		t.Fatalf("StableRunID returned error: %v", err)
	}
	otherRunID, err := StableRunID(projectID, "reviewer", RunSourceManual, "client-request-2")
	if err != nil {
		t.Fatalf("other StableRunID returned error: %v", err)
	}
	for label, id := range map[string]string{
		"project":   projectID,
		"agent":     agentID,
		"scheduler": schedulerID,
		"loader":    loaderID,
		"run":       runID,
	} {
		if len(id) > 80 {
			t.Fatalf("%s id too long: %q", label, id)
		}
		if label != "project" && !strings.Contains(id, "-reviewer-") && !strings.Contains(id, "-reviewer-default-") {
			t.Fatalf("%s id missing readable agent name: %q", label, id)
		}
	}
	if len(triggerID) > 80 {
		t.Fatalf("trigger id too long: %q", triggerID)
	}
	if !strings.Contains(triggerID, "-trigger-1-") {
		t.Fatalf("trigger id missing readable trigger fallback: %q", triggerID)
	}
	if otherRunID == runID {
		t.Fatalf("run id did not include idempotency key: %q", runID)
	}
	if _, err := StableProjectID("Demo", ""); err == nil {
		t.Fatalf("StableProjectID accepted non-normalized project name")
	}
	if _, err := StableManagedAgentID(projectID, "Bad Agent"); err == nil {
		t.Fatalf("StableManagedAgentID accepted non-normalized agent name")
	}
}

func TestNormalizeRecordsTrimDefaultsAndValidate(t *testing.T) {
	project, err := NormalizeRecord(ProjectRecord{
		ID:              " project-demo ",
		Name:            " demo ",
		SourcePath:      filepath.Join("tmp", "..", "tmp", "agent-compose.yml"),
		CurrentRevision: 3,
		SpecHash:        " hash ",
	})
	if err != nil {
		t.Fatalf("NormalizeRecord returned error: %v", err)
	}
	if project.ID != "project-demo" || project.Name != "demo" || project.SpecHash != "hash" {
		t.Fatalf("project normalization did not trim fields: %#v", project)
	}
	if project.SourcePath == "" || strings.Contains(project.SourcePath, "..") {
		t.Fatalf("project source path was not normalized: %q", project.SourcePath)
	}
	if !json.Valid([]byte(project.SourceJSON)) {
		t.Fatalf("project SourceJSON is not valid JSON: %q", project.SourceJSON)
	}

	agent, err := NormalizeAgentRecord(AgentRecord{
		ProjectID: " project-demo ",
		AgentName: " reviewer ",
		Revision:  1,
		Provider:  " codex ",
	})
	if err != nil {
		t.Fatalf("NormalizeAgentRecord returned error: %v", err)
	}
	if agent.ProjectID != "project-demo" || agent.AgentName != "reviewer" || agent.Provider != "codex" {
		t.Fatalf("agent normalization did not trim fields: %#v", agent)
	}
	if agent.ManagedAgentID == "" || agent.SpecJSON != "{}" {
		t.Fatalf("agent defaults were not filled: %#v", agent)
	}

	scheduler, err := NormalizeSchedulerRecord(SchedulerRecord{
		ProjectID:    " project-demo ",
		AgentName:    " reviewer ",
		Revision:     1,
		TriggerCount: 2,
	})
	if err != nil {
		t.Fatalf("NormalizeSchedulerRecord returned error: %v", err)
	}
	if scheduler.SchedulerID == "" || scheduler.ManagedLoaderID == "" || scheduler.SpecJSON != "{}" {
		t.Fatalf("scheduler defaults were not filled: %#v", scheduler)
	}

	run, err := NormalizeRunRecord(RunRecord{
		RunID:           " run-1 ",
		ProjectID:       " project-demo ",
		ProjectRevision: 1,
		Status:          " SUCCEEDED ",
		ResultJSON:      "",
	})
	if err != nil {
		t.Fatalf("NormalizeRunRecord returned error: %v", err)
	}
	if run.RunID != "run-1" || run.ProjectID != "project-demo" || run.Status != RunStatusSucceeded || run.ResultJSON != "{}" {
		t.Fatalf("run normalization mismatch: %#v", run)
	}

	for name, test := range map[string]func() error{
		"project source json": func() error {
			_, err := NormalizeRecord(ProjectRecord{ID: "project-demo", Name: "demo", SourceJSON: "{"})
			return err
		},
		"agent revision": func() error {
			_, err := NormalizeAgentRecord(AgentRecord{ProjectID: "project-demo", AgentName: "reviewer", Revision: -1})
			return err
		},
		"scheduler trigger count": func() error {
			_, err := NormalizeSchedulerRecord(SchedulerRecord{ProjectID: "project-demo", AgentName: "reviewer", TriggerCount: -1})
			return err
		},
		"run result json": func() error {
			_, err := NormalizeRunRecord(RunRecord{RunID: "run-1", ProjectID: "project-demo", ResultJSON: "{"})
			return err
		},
	} {
		if err := test(); err == nil {
			t.Fatalf("%s accepted invalid record", name)
		}
	}
}

func TestSessionRelationHelpers(t *testing.T) {
	statuses := NormalizeRunStatusFilter([]string{
		" RUNNING ",
		"bad",
		"",
		"succeeded",
		"running",
		"FAILED",
	})
	want := []string{RunStatusRunning, RunStatusSucceeded, RunStatusFailed}
	if len(statuses) != len(want) {
		t.Fatalf("NormalizeRunStatusFilter length = %d, want %d: %#v", len(statuses), len(want), statuses)
	}
	for i := range want {
		if statuses[i] != want[i] {
			t.Fatalf("NormalizeRunStatusFilter[%d] = %q, want %q", i, statuses[i], want[i])
		}
	}
	if got := Placeholders(3); got != "?,?,?" {
		t.Fatalf("Placeholders(3) = %q, want ?,?,?", got)
	}
	if got := Placeholders(0); got != "" {
		t.Fatalf("Placeholders(0) = %q, want empty", got)
	}
}
