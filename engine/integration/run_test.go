package integration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

func stubEngine(t *testing.T, fn func(engine.RunRequest) ([]engine.LogEntry, error)) {
	t.Helper()
	previous := runEngine
	runEngine = fn
	t.Cleanup(func() { runEngine = previous })
}

// emit feeds entries to the request's callback from inside a stub.
func emit(req engine.RunRequest, tags ...string) {
	for _, tag := range tags {
		entry, err := logger.NewEntry(tag, map[string]any{"n": 1}, "Basic01", "Basic")
		if err != nil {
			panic(err)
		}
		if err := req.LogCallback(entry); err != nil {
			panic(err)
		}
	}
}

func drain(t *testing.T, run *Run) []Event {
	t.Helper()
	var events []Event
	for {
		ev, err := run.Next(t.Context())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		events = append(events, ev)
		if ev.Type == EventDone || ev.Type == EventError {
			return events
		}
	}
}

func eventTypes(events []Event) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Type)
	}
	return out
}

func TestStartStreamsEntriesThenDone(t *testing.T) {
	stubEngine(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		emit(req, "ONE", "TWO")
		return nil, nil
	})
	run := Start(t.Context(), Request{Domain: "example.com"})
	events := drain(t, run)
	if !slices.Equal(eventTypes(events), []string{EventEntry, EventEntry, EventDone}) {
		t.Fatalf("events = %v", eventTypes(events))
	}
	first := events[0].Entry
	if first.Tag != "ONE" || first.Level != "DEBUG" || first.Module != "Basic" || first.Testcase != "Basic01" || first.Args["n"] != 1 {
		t.Fatalf("first entry = %+v", first)
	}
	if again, err := run.Next(t.Context()); err != nil || again.Type != EventDone {
		t.Fatalf("Next after done = %+v, %v", again, err)
	}
}

func TestStartDeliversResultEntriesTheCallbackMissed(t *testing.T) {
	stubEngine(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		emit(req, "SEEN")
		return []engine.LogEntry{
			{Timestamp: 0.5, Module: "System", Tag: "GLOBAL_VERSION", Level: "INFO"},
			{Timestamp: 0.5, Module: "System", Tag: "GLOBAL_VERSION", Level: "INFO"},
		}, nil
	})
	events := drain(t, Start(t.Context(), Request{Domain: "example.com"}))
	var tags []string
	for _, ev := range events[:len(events)-1] {
		tags = append(tags, ev.Entry.Tag)
	}
	if !slices.Equal(tags, []string{"SEEN", "GLOBAL_VERSION", "GLOBAL_VERSION"}) {
		t.Fatalf("tags = %v", tags)
	}
}

func TestStartSkipsResultEntriesAlreadyStreamed(t *testing.T) {
	stubEngine(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		entry, err := logger.NewEntry("TWICE", nil, "Basic01", "Basic")
		if err != nil {
			panic(err)
		}
		if err := req.LogCallback(entry); err != nil {
			panic(err)
		}
		return []engine.LogEntry{*convertEntry(entry)}, nil
	})
	events := drain(t, Start(t.Context(), Request{Domain: "example.com"}))
	if !slices.Equal(eventTypes(events), []string{EventEntry, EventDone}) {
		t.Fatalf("events = %v", eventTypes(events))
	}
}

func TestStartAppliesRequestToEngine(t *testing.T) {
	var got engine.RunRequest
	stubEngine(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		got = req
		return nil, nil
	})
	drain(t, Start(t.Context(), Request{Domain: "example.com", MinLevel: "notice"}))
	if got.Domain != "example.com" || got.MinLevel != "NOTICE" || got.CaptureMinLevel != "NOTICE" {
		t.Fatalf("request = %+v", got)
	}
	if got.Context == nil || got.NameserverCache == nil || got.LogCallback == nil {
		t.Fatalf("run wiring missing: %+v", got)
	}
}

func TestStartEmitsTimingsBeforeDone(t *testing.T) {
	stubEngine(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		req.NameserverCache.RecordQueryTime("ns1.example.com/192.0.2.1", 20*time.Millisecond)
		return nil, nil
	})
	events := drain(t, Start(t.Context(), Request{Domain: "example.com", IncludeTimings: true}))
	if !slices.Equal(eventTypes(events), []string{EventTimings, EventDone}) {
		t.Fatalf("events = %v", eventTypes(events))
	}
	timings := events[0].Timings
	if len(timings) != 1 {
		t.Fatalf("timings = %+v", timings)
	}
	if timings[0].Nameserver != "ns1.example.com" || timings[0].Address != "192.0.2.1" || timings[0].Count != 1 || timings[0].MedianMS != 20 || timings[0].Status != "ok" {
		t.Fatalf("timing = %+v", timings[0])
	}
}

func TestStartMapsEngineErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"canceled", context.Canceled, CodeCanceled},
		{"unknown testcase", fmt.Errorf("unknown testcase %q: %w", "nope", engine.ErrNotImplemented), CodeUnknownTestcase},
		{"panic error", &engine.PanicError{Value: "boom"}, CodePanic},
		{"other", errors.New("disk full"), CodeRunFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubEngine(t, func(engine.RunRequest) ([]engine.LogEntry, error) { return nil, tc.err })
			events := drain(t, Start(t.Context(), Request{Domain: "example.com"}))
			if len(events) != 1 || events[0].Type != EventError || events[0].Error.Code != tc.code {
				t.Fatalf("events = %+v, want one %s error", events, tc.code)
			}
		})
	}
}

func TestStartRecoversAPanic(t *testing.T) {
	stubEngine(t, func(engine.RunRequest) ([]engine.LogEntry, error) { panic("boom") })
	events := drain(t, Start(t.Context(), Request{Domain: "example.com"}))
	if len(events) != 1 || events[0].Error == nil || events[0].Error.Code != CodePanic || events[0].Error.Message != "panic: boom" {
		t.Fatalf("events = %+v", events)
	}
}

func TestCancelEndsTheStream(t *testing.T) {
	stubEngine(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		<-req.Context.Done()
		return nil, req.Context.Err()
	})
	run := Start(t.Context(), Request{Domain: "example.com"})
	run.Cancel()
	events := drain(t, run)
	if len(events) != 1 || events[0].Type != EventError || events[0].Error.Code != CodeCanceled {
		t.Fatalf("events = %+v", events)
	}
}

func TestCancelUnblocksAProducer(t *testing.T) {
	started := make(chan struct{})
	stubEngine(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		close(started)
		for range 10 * eventBuffer {
			emit(req, "FLOOD")
		}
		return nil, req.Context.Err()
	})
	run := Start(t.Context(), Request{Domain: "example.com"})
	<-started
	run.Cancel()
	events := drain(t, run)
	last := events[len(events)-1]
	if last.Type != EventError || last.Error.Code != CodeCanceled {
		t.Fatalf("last event = %+v", last)
	}
}

func TestStartRejectsAnInvalidRequest(t *testing.T) {
	stubEngine(t, func(engine.RunRequest) ([]engine.LogEntry, error) {
		t.Error("engine ran")
		return nil, nil
	})
	run := Start(t.Context(), Request{})
	events := drain(t, run)
	if len(events) != 1 || events[0].Error == nil || events[0].Error.Code != CodeInvalidDomain {
		t.Fatalf("events = %+v", events)
	}
	if again, err := run.Next(t.Context()); err != nil || again.Error == nil || again.Error.Code != CodeInvalidDomain {
		t.Fatalf("Next after error = %+v, %v", again, err)
	}
}

func TestStartJSONRejectsBadJSON(t *testing.T) {
	events := drain(t, StartJSON(t.Context(), []byte(`{"domain":`)))
	if len(events) != 1 || events[0].Error == nil || events[0].Error.Code != CodeInvalidRequest {
		t.Fatalf("events = %+v", events)
	}
}

func TestStartJSONRunsADecodedRequest(t *testing.T) {
	var got engine.RunRequest
	stubEngine(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		got = req
		return nil, nil
	})
	drain(t, StartJSON(t.Context(), []byte(`{"domain":"example.com","testcases":["basic01"]}`)))
	if got.Domain != "example.com" || !slices.Equal(got.Testcases, []string{"basic01"}) {
		t.Fatalf("request = %+v", got)
	}
}

func TestNextHonoursItsContext(t *testing.T) {
	stubEngine(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		<-req.Context.Done()
		return nil, req.Context.Err()
	})
	run := Start(t.Context(), Request{Domain: "example.com"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := run.Next(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Next = %v, want context.Canceled", err)
	}
	run.Cancel()
	drain(t, run)
}

func TestStartRunsTheEngineOffline(t *testing.T) {
	off := false
	run := Start(t.Context(), Request{Domain: "example.com", Testcases: []string{"syntax01"}, IPv4: &off, IPv6: &off, MinLevel: "INFO", IncludeTimings: true})
	events := drain(t, run)
	if !slices.Equal(eventTypes(events), []string{EventEntry, EventEntry, EventTimings, EventDone}) {
		t.Fatalf("events = %v", eventTypes(events))
	}
	if events[0].Entry.Tag != "GLOBAL_VERSION" || events[0].Entry.Level != "INFO" || events[1].Entry.Tag != "NO_NETWORK" || events[1].Entry.Level != "CRITICAL" {
		t.Fatalf("entries = %+v, %+v", events[0].Entry, events[1].Entry)
	}
	if len(events[2].Timings) != 0 {
		t.Fatalf("timings = %+v, want none", events[2].Timings)
	}
}
