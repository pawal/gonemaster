package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/i18n"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

func writeHuman(entries []engine.LogEntry, locale string, out io.Writer) error {
	if out == nil {
		return fmt.Errorf("missing output writer")
	}
	if _, err := fmt.Fprintln(out, "Seconds Level    Message"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "======= ======== ======="); err != nil {
		return err
	}
	for _, entry := range entries {
		message, found := i18n.TranslateWithStatus(locale, entry.Module, entry.Tag, entry.Args)
		if !found {
			message = rawEntryString(entry)
		}
		line := formatTranslatedLine(entry.Timestamp, entry.Level, message)
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

type humanReporter struct {
	out           io.Writer
	locale        string
	minLevel      int
	hasMin        bool
	spinner       *spinner
	spinnerShown  bool
	cursorHidden  bool
	spinnerStop   chan struct{}
	spinnerDone   chan struct{}
	spinnerTicker *time.Ticker
	mu            sync.Mutex
}

func newHumanReporter(out io.Writer, locale string, minLevel string, showSpinner bool) *humanReporter {
	if out == nil {
		return nil
	}
	minLevel = strings.ToUpper(strings.TrimSpace(minLevel))
	minValue, hasMin := logger.Levels()[minLevel]

	r := &humanReporter{
		out:      out,
		locale:   locale,
		minLevel: minValue,
		hasMin:   hasMin && minLevel != "",
	}
	if err := r.writeHeader(); err != nil {
		return nil
	}
	if showSpinner {
		r.startSpinner()
	}
	return r
}

// Callback renders a translated log entry in human-readable format.
func (r *humanReporter) Callback(entry *logger.Entry) error {
	if r == nil || entry == nil {
		return nil
	}
	if r.hasMin && entry.NumericLevel() < r.minLevel {
		return nil
	}
	message, found := i18n.TranslateWithStatus(r.locale, entry.Module, entry.Tag, entry.Args)
	if !found {
		message = entry.String()
	}
	line := formatTranslatedLine(entry.Timestamp, entry.Level(), message)
	return r.printLine(line)
}

// Finish stops spinner output and leaves the terminal in a clean state.
func (r *humanReporter) Finish() {
	if r == nil {
		return
	}
	r.stopSpinner()
}

func (r *humanReporter) writeHeader() error {
	if _, err := fmt.Fprintln(r.out, "Seconds Level    Message"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.out, "======= ======== ======="); err != nil {
		return err
	}
	return nil
}

func (r *humanReporter) printLine(line string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spinnerShown {
		_, _ = fmt.Fprint(r.out, "\r \r")
		r.spinnerShown = false
	}
	_, err := fmt.Fprintln(r.out, line)
	return err
}

func (r *humanReporter) startSpinner() {
	if r == nil || r.spinner != nil {
		return
	}
	r.spinner = newSpinner()
	r.spinnerStop = make(chan struct{})
	r.spinnerDone = make(chan struct{})
	r.spinnerTicker = time.NewTicker(120 * time.Millisecond)
	r.hideCursor()
	go func() {
		defer close(r.spinnerDone)
		for {
			select {
			case <-r.spinnerTicker.C:
				r.tickSpinner()
			case <-r.spinnerStop:
				r.spinnerTicker.Stop()
				return
			}
		}
	}()
}

func (r *humanReporter) tickSpinner() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spinner == nil {
		return
	}
	_, _ = fmt.Fprintf(r.out, "\r%s", r.spinner.Next())
	r.spinnerShown = true
}

func (r *humanReporter) stopSpinner() {
	if r == nil || r.spinnerStop == nil {
		return
	}
	close(r.spinnerStop)
	<-r.spinnerDone
	r.mu.Lock()
	if r.spinnerShown {
		_, _ = fmt.Fprint(r.out, "\r \r")
		r.spinnerShown = false
	}
	r.mu.Unlock()
	r.showCursor()
}

func (r *humanReporter) hideCursor() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cursorHidden {
		return
	}
	_, _ = fmt.Fprint(r.out, "\x1b[?25l")
	r.cursorHidden = true
}

func (r *humanReporter) showCursor() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.cursorHidden {
		return
	}
	_, _ = fmt.Fprint(r.out, "\x1b[?25h")
	r.cursorHidden = false
}

func formatTranslatedLine(timestamp float64, level string, message string) string {
	seconds := fmt.Sprintf("%7.2f", timestamp)
	level = fmt.Sprintf("%-8s", level)
	message = strings.TrimSpace(message)
	if message == "" {
		return seconds + " " + level
	}
	return seconds + " " + level + " " + message
}

func rawEntryString(entry engine.LogEntry) string {
	tmp, err := logger.NewEntry(entry.Tag, entry.Args, entry.Testcase, entry.Module)
	if err == nil {
		return tmp.String()
	}
	raw := entry.Module
	if entry.Testcase != "" {
		if raw == "" {
			raw = entry.Testcase
		} else {
			raw += ":" + entry.Testcase
		}
	}
	if entry.Tag != "" {
		if raw == "" {
			raw = entry.Tag
		} else {
			raw += ":" + entry.Tag
		}
	}
	return raw
}

type spinner struct {
	frames []string
	idx    int
}

func newSpinner() *spinner {
	return &spinner{frames: []string{"|", "/", "-", "\\"}}
}

// Next returns the next spinner frame.
func (s *spinner) Next() string {
	if s == nil || len(s.frames) == 0 {
		return ""
	}
	frame := s.frames[s.idx]
	s.idx = (s.idx + 1) % len(s.frames)
	return frame
}
