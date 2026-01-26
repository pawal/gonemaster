package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/pawal/gonemaster/engine/logger"
)

type progressReporter struct {
	out          io.Writer
	total        int
	completed    int
	started      map[string]bool
	completedSet map[string]bool
	planned      map[string]bool
	lastLine     string
	printed      bool
	mu           sync.Mutex
}

func newProgressReporter(out io.Writer, planned []string) *progressReporter {
	plannedSet := make(map[string]bool, len(planned))
	for _, name := range planned {
		key := normalizeTestcase(name)
		if key != "" {
			plannedSet[key] = true
		}
	}
	if len(plannedSet) == 0 {
		return nil
	}
	return &progressReporter{
		out:          out,
		total:        len(plannedSet),
		started:      map[string]bool{},
		completedSet: map[string]bool{},
		planned:      plannedSet,
	}
}

func (p *progressReporter) Callback(entry *logger.Entry) error {
	if p == nil || entry == nil || p.total == 0 {
		return nil
	}

	testcaseKey := normalizeTestcase(entry.Testcase)
	if testcaseKey == "" {
		return nil
	}
	if p.planned != nil && !p.planned[testcaseKey] {
		return nil
	}

	switch entry.Tag {
	case "TEST_CASE_START":
		p.onStart(testcaseKey, entry.Testcase)
	case "TEST_CASE_END":
		p.onEnd(testcaseKey, entry.Testcase)
	default:
		p.onNonMarker(testcaseKey, entry.Testcase)
	}
	return nil
}

func (p *progressReporter) Finish() {
	if p == nil || p.total == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.printed {
		return
	}
	current := p.completed
	if current < p.total {
		current = p.total
	}
	p.printLine(current, "done")
	fmt.Fprint(p.out, "\n")
}

func (p *progressReporter) onStart(key string, display string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started[key] {
		return
	}
	p.started[key] = true
	current := p.completed + 1
	if current > p.total {
		current = p.total
	}
	p.printLine(current, display)
}

func (p *progressReporter) onEnd(key string, display string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.completedSet[key] {
		return
	}
	p.completedSet[key] = true
	if !p.started[key] {
		p.started[key] = true
	}
	p.completed++
	current := p.completed
	if current > p.total {
		current = p.total
	}
	p.printLine(current, display)
}

func (p *progressReporter) onNonMarker(key string, display string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started[key] || p.completedSet[key] {
		return
	}
	p.started[key] = true
	p.completedSet[key] = true
	p.completed++
	current := p.completed
	if current > p.total {
		current = p.total
	}
	p.printLine(current, display)
}

func (p *progressReporter) printLine(current int, label string) {
	if p.total == 0 {
		return
	}
	percent := current * 100 / p.total
	line := fmt.Sprintf("\rProgress %d/%d (%d%%) %s", current, p.total, percent, label)
	if len(p.lastLine) > len(line) {
		line += strings.Repeat(" ", len(p.lastLine)-len(line))
	}
	if line == p.lastLine {
		return
	}
	fmt.Fprint(p.out, line)
	p.lastLine = line
	p.printed = true
}

func normalizeTestcase(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func isTerminalWriter(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
