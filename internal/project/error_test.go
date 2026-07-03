package project

import (
	"errors"
	"testing"
)

func TestErrorUnwrapAndKind(t *testing.T) {
	cause := errors.New("write failed")
	err := NewError(ErrorKindStorage, "persist project", cause)

	if got := err.Error(); got != "persist project" {
		t.Fatalf("Error() = %q, want %q", got, "persist project")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is did not match wrapped cause")
	}
	if got := ErrorKindOf(err); got != ErrorKindStorage {
		t.Fatalf("ErrorKindOf() = %q, want %q", got, ErrorKindStorage)
	}
}

func TestErrorKindOfDefaultsToUnknown(t *testing.T) {
	if got := ErrorKindOf(errors.New("plain")); got != ErrorKindUnknown {
		t.Fatalf("ErrorKindOf() = %q, want %q", got, ErrorKindUnknown)
	}

	err := NewError("", "", nil)
	if got := err.Kind; got != ErrorKindUnknown {
		t.Fatalf("NewError empty kind = %q, want %q", got, ErrorKindUnknown)
	}
}
