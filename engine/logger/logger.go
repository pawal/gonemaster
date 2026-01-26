package logger

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pawal/gonemaster/engine/profile"
)

// ModuleName is the default module name for log entries.
var ModuleName = "System"

// TestCaseName is the default test case name for log entries.
var TestCaseName = "Unspecified"

var logFilter map[string]map[string][]profile.LogFilterRule

// Logger stores log entries and optional callbacks.
type Logger struct {
	entries  []*Entry
	Callback func(*Entry) error
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
	return l.entries
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
	l.entries = append(l.entries, entry)

	if l.Callback != nil {
		_ = callCallback(l, entry)
	}

	return entry, nil
}

// ClearHistory clears all stored entries.
func (l *Logger) ClearHistory() {
	if l == nil {
		return
	}
	l.entries = []*Entry{}
}

// GetMaxLevel returns the maximum log level seen.
func (l *Logger) GetMaxLevel() string {
	if l == nil {
		return ""
	}
	maxLevel := 0
	for _, entry := range l.entries {
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
	for _, entry := range l.entries {
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

func callCallback(l *Logger, entry *Entry) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("logger callback panic: %v", recovered)
		}
	}()
	if cbErr := l.Callback(entry); cbErr != nil {
		err = cbErr
	}
	if err != nil {
		l.Callback = nil
		_, addErr := l.Add("LOGGER_CALLBACK_ERROR", map[string]any{"exception": err.Error()}, ModuleName, TestCaseName)
		if addErr != nil {
			return addErr
		}
	}
	return err
}

func (l *Logger) checkFilter(entry *Entry) {
	if entry == nil {
		return
	}
	if logFilter == nil {
		logFilter = profile.Effective().LogFilter
	}
	if logFilter == nil {
		return
	}

	moduleFilter := logFilter[strings.ToUpper(entry.Module)]
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
