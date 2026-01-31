package main

import (
	"encoding/json"
	"io"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

type jsonStreamReporter struct {
	enc      *json.Encoder
	minLevel int
	hasMin   bool
	mu       sync.Mutex
}

func newJSONStreamReporter(out io.Writer, minLevel string) *jsonStreamReporter {
	if out == nil {
		return nil
	}
	minLevel = strings.ToUpper(strings.TrimSpace(minLevel))
	minValue, hasMin := logger.Levels()[minLevel]
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return &jsonStreamReporter{
		enc:      enc,
		minLevel: minValue,
		hasMin:   hasMin && minLevel != "",
	}
}

func (r *jsonStreamReporter) Callback(entry *logger.Entry) error {
	if r == nil || entry == nil {
		return nil
	}
	if r.hasMin && entry.NumericLevel() < r.minLevel {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.enc.Encode(entry)
	return nil
}
