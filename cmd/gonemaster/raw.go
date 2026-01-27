package main

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

type rawReporter struct {
	out      io.Writer
	minLevel int
	hasMin   bool
	mu       sync.Mutex
}

func newRawReporter(out io.Writer, minLevel string) *rawReporter {
	if out == nil {
		return nil
	}
	minLevel = strings.ToUpper(strings.TrimSpace(minLevel))
	minValue, hasMin := logger.Levels()[minLevel]
	return &rawReporter{
		out:      out,
		minLevel: minValue,
		hasMin:   hasMin && minLevel != "",
	}
}

func (r *rawReporter) Callback(entry *logger.Entry) error {
	if r == nil || entry == nil {
		return nil
	}
	if r.hasMin && entry.NumericLevel() < r.minLevel {
		return nil
	}
	line := entry.String()
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.out, line)
	return nil
}
