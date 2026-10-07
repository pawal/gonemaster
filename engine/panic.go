package engine

import (
	"fmt"
	"runtime/debug"

	"codeberg.org/pawal/gonemaster/engine/internal/parallel"
)

// PanicError is a panic raised during a run, returned by Run as an error.
type PanicError struct {
	// Value is the value passed to panic.
	Value any
	// Stack is the stack of the panicking goroutine.
	Stack []byte
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

// newPanicError wraps a recovered value, keeping a stack carried from another goroutine.
func newPanicError(v any) *PanicError {
	if p, ok := v.(*parallel.Panic); ok {
		return &PanicError{Value: p.Value, Stack: p.Stack}
	}
	return &PanicError{Value: v, Stack: debug.Stack()}
}

// recoverPanic turns a panic into a *PanicError in err; defer it.
func recoverPanic(err *error) {
	if v := recover(); v != nil {
		*err = newPanicError(v)
	}
}
