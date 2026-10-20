package main

import (
	"context"
	"encoding/json"
	"errors"
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
	return guard(func() string { return encode(integration.ReportProfile([]byte(profile))) })
}

func translateJSON(request string) string {
	return guard(func() string {
		req, err := integration.ParseTranslateRequest([]byte(request))
		if err != nil {
			var e *integration.Error
			if errors.As(err, &e) {
				return encode(integration.Event{Type: integration.EventError, Error: e})
			}
			return errorJSON(integration.CodeInvalidRequest, err.Error())
		}
		return encode(struct {
			Messages []integration.Translation `json:"messages"`
		}{integration.Translate(req)})
	})
}

func normalizeNameJSON(name string) string {
	return guard(func() string { return encode(integration.NormalizeName(name)) })
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
			out = errorJSON(integration.CodeRunFailed, fmt.Sprintf("panic: %v", v))
		}
	}()
	return fn()
}

func encode(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return errorJSON(integration.CodeRunFailed, err.Error())
	}
	return string(data)
}

func errorJSON(code string, message string) string {
	data, _ := json.Marshal(integration.Event{Type: integration.EventError, Error: &integration.Error{Code: code, Message: message}})
	return string(data)
}
