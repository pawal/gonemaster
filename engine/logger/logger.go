package logger

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine/profile"
)

// ModuleName is the default module name for log entries.
var ModuleName = "System"

// TestCaseName is the default test case name for log entries.
var TestCaseName = "Unspecified"

// Logger stores log entries and optional callbacks.
type Logger struct {
	mu      sync.Mutex
	entries []*Entry
	// Callback is invoked for each appended entry when non-nil.
	Callback        func(*Entry) error
	callbackRunning bool
	pending         []*Entry
	startTime       time.Time
	defaultModule   string
	defaultTestcase string
	configMu        sync.RWMutex
	logFilter       map[string]map[string][]profile.LogFilterRule
	testLevels      map[string]map[string]string
	captureFloor    int
	hasCapture      bool
}

// New creates a new Logger.
func New() *Logger {
	return &Logger{
		entries:         []*Entry{},
		startTime:       time.Now(),
		defaultModule:   ModuleName,
		defaultTestcase: TestCaseName,
	}
}

// SetProfile configures log filtering and levels from the provided profile.
func (l *Logger) SetProfile(p *profile.Profile) {
	if l == nil || p == nil {
		return
	}
	l.configMu.Lock()
	l.logFilter = p.LogFilter
	l.testLevels = p.TestLevels
	l.configMu.Unlock()
}

// SetCaptureLevel drops entries below level before they are stored or passed to
// callbacks. An empty level captures everything.
func (l *Logger) SetCaptureLevel(level string) error {
	if l == nil {
		return nil
	}
	level = strings.ToUpper(strings.TrimSpace(level))
	if level == "" {
		l.configMu.Lock()
		l.captureFloor = 0
		l.hasCapture = false
		l.configMu.Unlock()
		return nil
	}
	value, ok := numericLevels[level]
	if !ok {
		return fmt.Errorf("unknown level %q", level)
	}
	l.configMu.Lock()
	l.captureFloor = value
	l.hasCapture = true
	l.configMu.Unlock()
	return nil
}

// Wants reports whether an entry with this tag would be captured. Callers use it
// to skip building expensive arguments for entries nothing will read.
func (l *Logger) Wants(module string, tag string) bool {
	if l == nil {
		return false
	}
	l.configMu.RLock()
	floor, has := l.captureFloor, l.hasCapture
	testLevels, logFilter := l.testLevels, l.logFilter
	l.configMu.RUnlock()
	if !has {
		return true
	}
	if module == "" {
		module = l.defaultModule
	}
	// A filter rule can raise the level from the arguments, so never gate those.
	if len(logFilter[strings.ToUpper(module)][strings.ToUpper(tag)]) > 0 {
		return true
	}
	value, ok := numericLevels[levelForTag(testLevels, module, tag)]
	if !ok {
		return true
	}
	return value >= floor
}

// captured reports whether the entry is at or above the capture level.
func (l *Logger) captured(entry *Entry) bool {
	l.configMu.RLock()
	floor, has := l.captureFloor, l.hasCapture
	l.configMu.RUnlock()
	return !has || entry.NumericLevel() >= floor
}

// CopyConfigFrom copies log filtering, levels and capture level from another logger.
func (l *Logger) CopyConfigFrom(other *Logger) {
	if l == nil || other == nil {
		return
	}
	other.configMu.RLock()
	logFilter := other.logFilter
	testLevels := other.testLevels
	captureFloor := other.captureFloor
	hasCapture := other.hasCapture
	other.configMu.RUnlock()

	l.configMu.Lock()
	l.logFilter = logFilter
	l.testLevels = testLevels
	l.captureFloor = captureFloor
	l.hasCapture = hasCapture
	l.configMu.Unlock()
}

// CopyStartTimeFrom aligns the timestamp base with another logger.
func (l *Logger) CopyStartTimeFrom(other *Logger) {
	if l == nil || other == nil {
		return
	}
	l.startTime = other.startTime
}

// ResetStartTime resets the timestamp base for this logger.
func (l *Logger) ResetStartTime() {
	if l == nil {
		return
	}
	l.startTime = time.Now()
}

// Entries returns the current log entries.
func (l *Logger) Entries() []*Entry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*Entry, len(l.entries))
	copy(out, l.entries)
	return out
}

// Add creates and stores a new log entry.
func (l *Logger) Add(tag string, args map[string]any, module string, testcase string) (*Entry, error) {
	if l == nil {
		return nil, fmt.Errorf("logger is nil")
	}
	if module == "" {
		module = l.defaultModule
	}
	if testcase == "" {
		testcase = l.defaultTestcase
	}
	testLevels, logFilter := l.configSnapshot()
	entry, err := newEntryWithTimestamp(strings.ToUpper(tag), args, testcase, module, time.Since(l.startTime).Seconds(), testLevels)
	if err != nil {
		return nil, err
	}

	l.checkFilter(entry, logFilter)
	// Filters run first: a rule can raise an entry above the capture level.
	if !l.captured(entry) {
		return entry, nil
	}
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	if l.Callback == nil {
		l.mu.Unlock()
		return entry, nil
	}
	if l.callbackRunning {
		l.pending = append(l.pending, entry)
		l.mu.Unlock()
		return entry, nil
	}
	l.callbackRunning = true
	l.mu.Unlock()
	l.runCallbacks(entry)

	return entry, nil
}

// AddWithoutCallback creates and stores a new log entry without invoking callbacks.
func (l *Logger) AddWithoutCallback(tag string, args map[string]any, module string, testcase string) (*Entry, error) {
	if l == nil {
		return nil, fmt.Errorf("logger is nil")
	}
	if module == "" {
		module = l.defaultModule
	}
	if testcase == "" {
		testcase = l.defaultTestcase
	}
	testLevels, logFilter := l.configSnapshot()
	entry, err := newEntryWithTimestamp(strings.ToUpper(tag), args, testcase, module, time.Since(l.startTime).Seconds(), testLevels)
	if err != nil {
		return nil, err
	}
	l.checkFilter(entry, logFilter)
	if !l.captured(entry) {
		return entry, nil
	}
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	l.mu.Unlock()
	return entry, nil
}

// Append stores existing log entries and dispatches callbacks without re-filtering.
func (l *Logger) Append(entries ...*Entry) error {
	if l == nil {
		return fmt.Errorf("logger is nil")
	}
	for _, entry := range entries {
		if entry == nil || !l.captured(entry) {
			continue
		}
		l.appendExisting(entry)
	}
	return nil
}

// AppendWithoutCallback stores existing log entries without invoking callbacks.
func (l *Logger) AppendWithoutCallback(entries ...*Entry) error {
	if l == nil {
		return fmt.Errorf("logger is nil")
	}
	// Decide before taking the entry lock: captured reads the config lock.
	keep := make([]*Entry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || !l.captured(entry) {
			continue
		}
		keep = append(keep, entry)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, keep...)
	return nil
}

func (l *Logger) appendExisting(entry *Entry) {
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	if l.Callback == nil {
		l.mu.Unlock()
		return
	}
	if l.callbackRunning {
		l.pending = append(l.pending, entry)
		l.mu.Unlock()
		return
	}
	l.callbackRunning = true
	l.mu.Unlock()
	l.runCallbacks(entry)
}

// ClearHistory clears all stored entries.
func (l *Logger) ClearHistory() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.entries = []*Entry{}
	l.mu.Unlock()
}

// GetMaxLevel returns the maximum log level seen.
func (l *Logger) GetMaxLevel() string {
	if l == nil {
		return ""
	}
	entries := l.Entries()
	maxLevel := 0
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.NumericLevel() > maxLevel {
			maxLevel = entry.NumericLevel()
		}
	}

	reverse := map[int]string{}
	for key, value := range Levels() {
		reverse[value] = key
	}
	if level, ok := reverse[maxLevel]; ok {
		return level
	}
	return ""
}

// JSON returns a JSON representation of log entries.
func (l *Logger) JSON(minLevel string) (string, error) {
	if l == nil {
		return "[]", nil
	}

	numericLevels := Levels()
	minLevel = strings.ToUpper(minLevel)
	minLevelValue, hasMin := numericLevels[minLevel]

	out := []map[string]any{}
	for _, entry := range l.Entries() {
		if entry == nil {
			continue
		}
		if hasMin && entry.NumericLevel() < minLevelValue {
			continue
		}
		item := map[string]any{
			"timestamp": entry.Timestamp,
			"module":    entry.Module,
			"testcase":  entry.Testcase,
			"tag":       entry.Tag,
			"level":     entry.Level(),
		}
		if entry.Args != nil {
			item["args"] = entry.Args
		}
		out = append(out, item)
	}

	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (l *Logger) runCallbacks(entry *Entry) {
	for {
		l.mu.Lock()
		cb := l.Callback
		l.mu.Unlock()
		if cb == nil {
			l.mu.Lock()
			l.callbackRunning = false
			l.pending = nil
			l.mu.Unlock()
			return
		}
		if err := safeCallback(cb, entry); err != nil {
			l.mu.Lock()
			l.callbackRunning = false
			l.pending = nil
			l.Callback = nil
			l.mu.Unlock()
			_, _ = l.Add("LOGGER_CALLBACK_ERROR", map[string]any{"exception": err.Error()}, l.defaultModule, l.defaultTestcase)
			return
		}

		l.mu.Lock()
		if len(l.pending) == 0 {
			l.callbackRunning = false
			l.mu.Unlock()
			return
		}
		entry = l.pending[0]
		l.pending = l.pending[1:]
		l.mu.Unlock()
	}
}

func safeCallback(cb func(*Entry) error, entry *Entry) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("logger callback panic: %v", recovered)
		}
	}()
	if cbErr := cb(entry); cbErr != nil {
		err = cbErr
	}
	return err
}

func (l *Logger) configSnapshot() (map[string]map[string]string, map[string]map[string][]profile.LogFilterRule) {
	l.configMu.RLock()
	testLevels := l.testLevels
	logFilter := l.logFilter
	l.configMu.RUnlock()
	return testLevels, logFilter
}

func (l *Logger) checkFilter(entry *Entry, filter map[string]map[string][]profile.LogFilterRule) {
	if entry == nil {
		return
	}
	if filter == nil {
		return
	}

	moduleFilter := filter[strings.ToUpper(entry.Module)]
	if moduleFilter == nil {
		return
	}

	rules := moduleFilter[strings.ToUpper(entry.Tag)]
	if len(rules) == 0 {
		return
	}

	for _, rule := range rules {
		if ruleMatches(entry, rule) {
			entry.setLevel(rule.Set)
			return
		}
	}
}

func ruleMatches(entry *Entry, rule profile.LogFilterRule) bool {
	if entry.Args == nil {
		return false
	}
	for key, cond := range rule.When {
		value, ok := entry.Args[key]
		if !ok {
			return false
		}
		if !matchCondition(cond, value) {
			return false
		}
	}
	return true
}

func matchCondition(cond any, value any) bool {
	switch v := cond.(type) {
	case []any:
		for _, item := range v {
			if fmt.Sprint(item) == fmt.Sprint(value) {
				return true
			}
		}
		return false
	case []string:
		valueStr := fmt.Sprint(value)
		return slices.Contains(v, valueStr)
	default:
		return fmt.Sprint(cond) == fmt.Sprint(value)
	}
}
