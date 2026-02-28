package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/i18n"
	"codeberg.org/pawal/gonemaster/engine/logger"
	textwidth "golang.org/x/text/width"
)

const (
	defaultSecondsColumnWidth = 7
	defaultLevelColumnWidth   = 8
)

type humanLayout struct {
	secondsLabel string
	levelLabel   string
	messageLabel string
	secondsWidth int
	levelWidth   int
}

func writeHuman(entries []engine.LogEntry, locale string, out io.Writer) error {
	if out == nil {
		return fmt.Errorf("missing output writer")
	}
	layout := newHumanLayout(locale)
	if err := writeHumanHeaderWithLayout(out, layout); err != nil {
		return err
	}
	if len(entries) == 0 {
		if _, err := fmt.Fprintln(out, "Looks OK."); err != nil {
			return err
		}
		return nil
	}
	for _, entry := range entries {
		message, found := i18n.TranslateWithStatus(locale, entry.Module, entry.Tag, entry.Args)
		if !found {
			message = rawEntryString(entry)
		}
		line := formatTranslatedLine(entry.Timestamp, entry.Level, message, layout)
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
	layout        humanLayout
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
		layout:   newHumanLayout(locale),
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
	line := formatTranslatedLine(entry.Timestamp, entry.Level(), message, r.layout)
	return r.printLine(line)
}

// Finish stops spinner output and leaves the terminal in a clean state.
func (r *humanReporter) Finish() {
	if r == nil {
		return
	}
	r.stopSpinner()
}

func (r *humanReporter) PrintLooksOK() error {
	if r == nil {
		return nil
	}
	return r.printLine("Looks OK.")
}

func (r *humanReporter) writeHeader() error {
	return writeHumanHeaderWithLayout(r.out, r.layout)
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

func formatTranslatedLine(timestamp float64, level string, message string, layout humanLayout) string {
	seconds := fmt.Sprintf("%*.2f", layout.secondsWidth, timestamp)
	level = padRightDisplay(strings.TrimSpace(level), layout.levelWidth)
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

func writeHumanHeader(out io.Writer, locale string) error {
	return writeHumanHeaderWithLayout(out, newHumanLayout(locale))
}

func newHumanLayout(locale string) humanLayout {
	seconds := translatedHeaderLabel(locale, "CLI_HEADER_SECONDS", "Seconds")
	level := translatedHeaderLabel(locale, "CLI_HEADER_LEVEL", "Level")
	message := translatedHeaderLabel(locale, "CLI_HEADER_MESSAGE", "Message")

	return humanLayout{
		secondsLabel: seconds,
		levelLabel:   level,
		messageLabel: message,
		secondsWidth: maxInt(defaultSecondsColumnWidth, terminalCellWidth(seconds)),
		levelWidth:   maxInt(defaultLevelColumnWidth, terminalCellWidth(level)),
	}
}

func writeHumanHeaderWithLayout(out io.Writer, layout humanLayout) error {
	header := padRightDisplay(layout.secondsLabel, layout.secondsWidth) +
		" " + padRightDisplay(layout.levelLabel, layout.levelWidth) +
		" " + layout.messageLabel
	if _, err := fmt.Fprintln(out, header); err != nil {
		return err
	}

	messageWidth := maxInt(7, terminalCellWidth(layout.messageLabel))
	divider := strings.Repeat("=", layout.secondsWidth) +
		" " + strings.Repeat("=", layout.levelWidth) +
		" " + strings.Repeat("=", messageWidth)
	if _, err := fmt.Fprintln(out, divider); err != nil {
		return err
	}
	return nil
}

func translatedHeaderLabel(locale string, tag string, fallback string) string {
	value, found := i18n.TranslateWithStatus(locale, "SYSTEM", tag, nil)
	if !found {
		return fallback
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func padRightDisplay(s string, width int) string {
	diff := width - terminalCellWidth(s)
	if diff <= 0 {
		return s
	}
	return s + strings.Repeat(" ", diff)
}

func terminalCellWidth(s string) int {
	total := 0
	for _, r := range s {
		switch {
		case r == 0:
			continue
		case r < 0x20 || (r >= 0x7f && r < 0xa0):
			continue
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r):
			continue
		}

		kind := textwidth.LookupRune(r).Kind()
		if kind == textwidth.EastAsianWide || kind == textwidth.EastAsianFullwidth {
			total += 2
			continue
		}
		total++
	}
	return total
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
