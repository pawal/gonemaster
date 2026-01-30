package testlogger

import (
	"fmt"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

// Buffer captures log entries with a fixed module/testcase context.
type Buffer struct {
	log      *logger.Logger
	module   string
	testcase string
}

// New returns a new Buffer for the given module and testcase.
func New(module string, testcase string) *Buffer {
	return &Buffer{
		log:      logger.New(),
		module:   module,
		testcase: testcase,
	}
}

// Add records a log entry using the buffer's module/testcase.
func (b *Buffer) Add(tag string, args map[string]any) (*logger.Entry, error) {
	if b == nil || b.log == nil {
		return nil, fmt.Errorf("log buffer is nil")
	}
	return b.log.Add(tag, args, b.module, b.testcase)
}

// Append records a log entry and appends it to the provided slice.
func (b *Buffer) Append(results *[]*logger.Entry, tag string, args map[string]any) error {
	entry, err := b.Add(tag, args)
	if err != nil {
		return err
	}
	if entry != nil {
		*results = append(*results, entry)
	}
	return nil
}

// Entries returns the buffered log entries.
func (b *Buffer) Entries() []*logger.Entry {
	if b == nil || b.log == nil {
		return nil
	}
	return b.log.Entries()
}
