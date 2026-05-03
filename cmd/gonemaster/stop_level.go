package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

var errStopLevelReached = errors.New("stop level reached")

func normalizeStopLevel(value string) (string, error) {
	level := strings.ToUpper(strings.TrimSpace(value))
	if level == "" {
		return "", fmt.Errorf("--stop-level must not be empty")
	}
	if _, ok := logger.Levels()[level]; !ok {
		return "", fmt.Errorf("--stop-level must be one of %s", strings.Join(stopLevelNames(), ", "))
	}
	return level, nil
}

func stopLevelNames() []string {
	levels := logger.Levels()
	names := make([]string, 0, len(levels))
	for name := range levels {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type stopLevelController struct {
	threshold int
	cancel    context.CancelCauseFunc
	once      sync.Once
	triggered atomic.Bool
}

func newStopLevelController(level string, cancel context.CancelCauseFunc) *stopLevelController {
	return &stopLevelController{
		threshold: logger.Levels()[level],
		cancel:    cancel,
	}
}

func (c *stopLevelController) Callback(entry *logger.Entry) error {
	if c == nil || entry == nil {
		return nil
	}
	if entry.NumericLevel() < c.threshold {
		return nil
	}
	c.once.Do(func() {
		c.triggered.Store(true)
		if c.cancel != nil {
			c.cancel(errStopLevelReached)
		}
	})
	return nil
}

func (c *stopLevelController) Triggered() bool {
	if c == nil {
		return false
	}
	return c.triggered.Load()
}

type capturedLogEntry struct {
	entry   engine.LogEntry
	numeric int
}

type entryCaptureReporter struct {
	mu      sync.Mutex
	entries []capturedLogEntry
}

func newEntryCaptureReporter() *entryCaptureReporter {
	return &entryCaptureReporter{}
}

func (r *entryCaptureReporter) Callback(entry *logger.Entry) error {
	if r == nil || entry == nil {
		return nil
	}
	item := capturedLogEntry{
		entry: engine.LogEntry{
			Timestamp: entry.Timestamp,
			Module:    entry.Module,
			Testcase:  entry.Testcase,
			Tag:       entry.Tag,
			Level:     entry.Level(),
			Args:      cloneArgs(entry.Args),
		},
		numeric: entry.NumericLevel(),
	}
	r.mu.Lock()
	r.entries = append(r.entries, item)
	r.mu.Unlock()
	return nil
}

func (r *entryCaptureReporter) FilteredEntries(minLevel string) ([]engine.LogEntry, error) {
	if r == nil {
		return nil, nil
	}
	minLevel = strings.ToUpper(strings.TrimSpace(minLevel))
	minValue, hasMin := logger.Levels()[minLevel]
	if minLevel != "" && !hasMin {
		return nil, fmt.Errorf("unknown min level %q", minLevel)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]engine.LogEntry, 0, len(r.entries))
	for _, item := range r.entries {
		if hasMin && item.numeric < minValue {
			continue
		}
		out = append(out, item.entry)
	}
	return out, nil
}

func cloneArgs(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	out := make(map[string]any, len(args))
	maps.Copy(out, args)
	return out
}
