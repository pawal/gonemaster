package logger

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/profile"
)

// ModuleName is the default module name for log entries.
var ModuleName = "System"

// TestCaseName is the default test case name for log entries.
var TestCaseName = "Unspecified"

var (
	logFilter map[string]map[string][]profile.LogFilterRule
	configMu  sync.Mutex
)

// Logger stores log entries and optional callbacks.
type Logger struct {
	mu              sync.Mutex
	entries         []*Entry
	Callback        func(*Entry) error
	callbackRunning bool
	pending         []*Entry
}

// New creates a new Logger.
func New() *Logger {
	return &Logger{entries: []*Entry{}}
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
		module = ModuleName
	}
	if testcase == "" {
		testcase = TestCaseName
	}
	entry, err := NewEntry(strings.ToUpper(tag), args, testcase, module)
	if err != nil {
		return nil, err
	}

	l.checkFilter(entry)
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
			_, _ = l.Add("LOGGER_CALLBACK_ERROR", map[string]any{"exception": err.Error()}, ModuleName, TestCaseName)
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

func (l *Logger) checkFilter(entry *Entry) {
	if entry == nil {
		return
	}
	configMu.Lock()
	if logFilter == nil {
		logFilter = profile.Effective().LogFilter
	}
	filter := logFilter
	configMu.Unlock()
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
		for _, item := range v {
			if item == valueStr {
				return true
			}
		}
		return false
	default:
		return fmt.Sprint(cond) == fmt.Sprint(value)
	}
}
