package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/integration"
)

// runTable maps handles to runs for the C callers.
type runTable struct {
	mu   sync.Mutex
	next uint64
	runs map[uint64]*integration.Run
}

var runs = runTable{runs: map[uint64]*integration.Run{}}

func (t *runTable) add(run *integration.Run) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.next++
	t.runs[t.next] = run
	return t.next
}

func (t *runTable) get(handle uint64) *integration.Run {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.runs[handle]
}

func (t *runTable) remove(handle uint64) *integration.Run {
	t.mu.Lock()
	defer t.mu.Unlock()
	run := t.runs[handle]
	delete(t.runs, handle)
	return run
}

func versionJSON() string {
	return guard(func() string { return encode(integration.Version()) })
}

func listTestsJSON() string {
	return guard(func() string { return encode(integration.ListTests()) })
}

func checkProfileJSON(profile string) string {
	return guard(func() string {
		errs := integration.CheckProfile([]byte(profile))
		if errs == nil {
			errs = []integration.ProfileError{}
		}
		return encode(struct {
			Errors []integration.ProfileError `json:"errors"`
		}{errs})
	})
}

// runStart returns a handle; a bad request yields a run whose only event is the error.
func runStart(request string) (handle uint64) {
	defer func() {
		if recover() != nil {
			handle = 0
		}
	}()
	return runs.add(integration.StartJSON(context.Background(), []byte(request)))
}

func runNext(handle uint64) string {
	return guard(func() string {
		run := runs.get(handle)
		if run == nil {
			return encode(unknownHandle(handle))
		}
		ev, _ := run.Next(context.Background())
		return encode(ev)
	})
}

func runCancel(handle uint64) {
	if run := runs.get(handle); run != nil {
		run.Cancel()
	}
}

func runFree(handle uint64) {
	if run := runs.remove(handle); run != nil {
		run.Cancel()
	}
}

func unknownHandle(handle uint64) integration.Event {
	return integration.Event{Type: integration.EventError, Error: &integration.Error{Code: integration.CodeInvalidRequest, Message: fmt.Sprintf("unknown run handle %d", handle)}}
}

// guard turns a panic into an error event so the host process survives.
func guard(fn func() string) (out string) {
	defer func() {
		if v := recover(); v != nil {
			out = errorJSON(fmt.Sprintf("panic: %v", v))
		}
	}()
	return fn()
}

func encode(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(data)
}

func errorJSON(message string) string {
	data, _ := json.Marshal(integration.Event{Type: integration.EventError, Error: &integration.Error{Code: integration.CodeRunFailed, Message: message}})
	return string(data)
}
