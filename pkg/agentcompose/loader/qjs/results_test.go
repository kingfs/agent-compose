package qjs

import (
	"reflect"
	"testing"
)

func TestLoaderJSONResultRequiresOutputSchemaBeforeParsing(t *testing.T) {
	got, err := loaderJSONResult("not json", "", "scheduler.agent")
	if err != nil {
		t.Fatalf("loaderJSONResult without schema returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("loaderJSONResult without schema = %#v", got)
	}

	got, err = loaderJSONResult(`{"ok":true,"count":2}`, `{}`, "scheduler.agent")
	if err != nil {
		t.Fatalf("loaderJSONResult returned error: %v", err)
	}
	want := map[string]any{"ok": true, "count": float64(2)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loaderJSONResult = %#v, want %#v", got, want)
	}

	if _, err := loaderJSONResult("not json", `{}`, "scheduler.agent"); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestResultHelpersCompactJSONAndChooseFirstNonEmpty(t *testing.T) {
	got, err := marshalJSONCompact(map[string]any{"b": float64(2), "a": "x"})
	if err != nil {
		t.Fatalf("marshalJSONCompact returned error: %v", err)
	}
	if got != `{"a":"x","b":2}` {
		t.Fatalf("compact json = %s", got)
	}
	if got := firstNonEmpty(" ", "\t", " value ", "later"); got != " value " {
		t.Fatalf("first non-empty = %q", got)
	}
}
