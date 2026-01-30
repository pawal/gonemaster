package testcase

import (
	"context"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/util"
)

// Case runs a single test case and returns its log entries.
type Case func(context.Context) ([]*logger.Entry, error)

// Run executes a test case with an isolated logger and flushes entries to the parent logger.
func Run(ctx context.Context, fn Case) ([]*logger.Entry, error) {
	if fn == nil {
		return nil, nil
	}

	parent := util.Logger()
	buf := logger.New()

	util.SetLogger(buf)
	entries, err := fn(ctx)
	util.SetLogger(parent)

	if entries == nil {
		entries = buf.Entries()
	}
	if parent != nil && len(entries) > 0 {
		_ = parent.Append(entries...)
	}

	return entries, err
}
