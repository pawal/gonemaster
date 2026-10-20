package integration

import (
	"context"
	"errors"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

// Error codes.
const (
	CodeInvalidRequest  = "invalid_request"
	CodeInvalidDomain   = "invalid_domain"
	CodeUnknownModule   = "unknown_module"
	CodeUnknownTestcase = "unknown_testcase"
	CodeInvalidProfile  = "invalid_profile"
	CodeCanceled        = "canceled"
	CodePanic           = "panic"
	CodeRunFailed       = "run_failed"
)

// Error is a failure with a stable code.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// errorFor maps an engine error onto a code.
func errorFor(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var pe *engine.PanicError
	if errors.As(err, &pe) {
		return &Error{Code: CodePanic, Message: pe.Error()}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: CodeCanceled, Message: err.Error()}
	}
	if errors.Is(err, engine.ErrNotImplemented) {
		return &Error{Code: CodeUnknownTestcase, Message: err.Error()}
	}
	var prop *profile.PropertyError
	if errors.As(err, &prop) {
		return &Error{Code: CodeInvalidProfile, Message: err.Error()}
	}
	return &Error{Code: CodeRunFailed, Message: err.Error()}
}
