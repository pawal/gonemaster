package integration

import (
	"context"
	"fmt"
	"sync"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

// Event types.
const (
	EventEntry   = "entry"
	EventTimings = "timings"
	EventDone    = "done"
	EventError   = "error"
)

// Event is one item of a run's stream.
type Event struct {
	Type    string                        `json:"type"`
	Entry   *engine.LogEntry              `json:"entry,omitempty"`
	Timings []nameserver.NameserverTiming `json:"timings,omitempty"`
	Error   *Error                        `json:"error,omitempty"`
}

// Run is one engine run whose events are pulled with Next.
type Run struct {
	ctx    context.Context
	cancel context.CancelFunc
	events chan Event
	mu     sync.Mutex
	final  *Event
}

// runEngine is the seam tests stub.
var runEngine = engine.Run

const eventBuffer = 64

// Start begins a run; a request that fails validation yields one error event.
func Start(ctx context.Context, req Request) *Run {
	r := newRun(ctx)
	engineReq, err := req.runRequest()
	if err != nil {
		r.finish(Event{Type: EventError, Error: errorFor(err)})
		return r
	}
	go r.execute(engineReq, req.IncludeTimings)
	return r
}

// StartJSON is Start for an encoded request.
func StartJSON(ctx context.Context, data []byte) *Run {
	req, err := ParseRequest(data)
	if err != nil {
		r := newRun(ctx)
		r.finish(Event{Type: EventError, Error: errorFor(err)})
		return r
	}
	return Start(ctx, req)
}

func newRun(ctx context.Context) *Run {
	ctx, cancel := context.WithCancel(ctx)
	return &Run{events: make(chan Event, eventBuffer), cancel: cancel, ctx: ctx}
}

// Next returns the next event; after the last event it returns that event again.
func (r *Run) Next(ctx context.Context) (Event, error) {
	select {
	case ev, ok := <-r.events:
		if ok {
			return ev, nil
		}
		return r.finalEvent(), nil
	case <-ctx.Done():
		return Event{}, ctx.Err()
	}
}

// Cancel stops the run; the stream ends with a canceled error.
func (r *Run) Cancel() { r.cancel() }

func (r *Run) execute(req engine.RunRequest, timings bool) {
	var final Event
	defer func() {
		if v := recover(); v != nil {
			final = Event{Type: EventError, Error: &Error{Code: CodePanic, Message: fmt.Sprintf("panic: %v", v)}}
		}
		r.finish(final)
	}()
	store := nameserver.NewCacheStore()
	req.NameserverCache = store
	req.Context = r.ctx
	streamed := map[entryKey]int{}
	req.LogCallback = func(entry *logger.Entry) error {
		converted := convertEntry(entry)
		r.mu.Lock()
		streamed[keyOf(converted)]++
		r.mu.Unlock()
		r.send(Event{Type: EventEntry, Entry: converted})
		return nil
	}
	entries, err := runEngine(req)
	switch {
	case r.ctx.Err() != nil:
		final = Event{Type: EventError, Error: &Error{Code: CodeCanceled, Message: context.Cause(r.ctx).Error()}}
	case err != nil:
		final = Event{Type: EventError, Error: errorFor(err)}
	default:
		// Entries the callback did not see, if any, arrive here.
		for i := range entries {
			if key := keyOf(&entries[i]); streamed[key] > 0 {
				streamed[key]--
				continue
			}
			r.send(Event{Type: EventEntry, Entry: &entries[i]})
		}
		if timings {
			r.send(Event{Type: EventTimings, Timings: nameserver.TimingsFromQueryMap(store.QueryTimings(), store.QueryTimeouts(), store.QueryRefused())})
		}
		final = Event{Type: EventDone}
	}
}

type entryKey struct {
	timestamp float64
	module    string
	testcase  string
	tag       string
}

func keyOf(entry *engine.LogEntry) entryKey {
	return entryKey{entry.Timestamp, entry.Module, entry.Testcase, entry.Tag}
}

// send queues an event; a canceled run drops it.
func (r *Run) send(ev Event) {
	select {
	case r.events <- ev:
	case <-r.ctx.Done():
	}
}

func (r *Run) finish(ev Event) {
	r.mu.Lock()
	r.final = &ev
	r.mu.Unlock()
	close(r.events)
	r.cancel()
}

func (r *Run) finalEvent() Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *r.final
}

func convertEntry(entry *logger.Entry) *engine.LogEntry {
	return &engine.LogEntry{
		Timestamp: entry.Timestamp,
		Module:    entry.Module,
		Testcase:  entry.Testcase,
		Tag:       entry.Tag,
		Level:     entry.Level(),
		Args:      entry.Args,
	}
}
