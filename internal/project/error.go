package project

import "errors"

// ErrorKind classifies project usecase failures without binding callers to a
// transport status code.
type ErrorKind string

const (
	ErrorKindUnknown    ErrorKind = "unknown"
	ErrorKindValidation ErrorKind = "validation"
	ErrorKindNotFound   ErrorKind = "not_found"
	ErrorKindConflict   ErrorKind = "conflict"
	ErrorKindStorage    ErrorKind = "storage"
	ErrorKindRuntime    ErrorKind = "runtime"
)

// Error is the package-level error shape returned by project usecases.
type Error struct {
	Kind    ErrorKind
	Message string
	Err     error
}

// NewError creates an Error with the provided kind, message, and wrapped cause.
func NewError(kind ErrorKind, message string, err error) *Error {
	if kind == "" {
		kind = ErrorKindUnknown
	}
	return &Error{Kind: kind, Message: message, Err: err}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ErrorKindOf returns a project ErrorKind from err when one is present.
func ErrorKindOf(err error) ErrorKind {
	var projectErr *Error
	if errors.As(err, &projectErr) {
		return projectErr.Kind
	}
	return ErrorKindUnknown
}
