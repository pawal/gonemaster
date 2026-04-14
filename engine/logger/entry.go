package logger

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

var numericLevels = map[string]int{
	"DEBUG3":   -2,
	"DEBUG2":   -1,
	"DEBUG":    0,
	"INFO":     1,
	"NOTICE":   2,
	"WARNING":  3,
	"ERROR":    4,
	"CRITICAL": 5,
}

var defaultStartTime = time.Now()

// Entry represents a single log entry.
type Entry struct {
	// Tag is the message tag identifier.
	Tag string
	// Args holds structured arguments for the entry.
	Args map[string]any
	// Timestamp is seconds elapsed since the logger start time.
	Timestamp float64
	// Testcase is the testcase identifier that emitted the entry.
	Testcase string
	// Module is the engine module that emitted the entry.
	Module string

	level       string
	levelSet    bool
	levelConfig map[string]map[string]string
}

// NewEntry constructs a log entry with timestamp and metadata.
func NewEntry(tag string, args map[string]any, testcase string, module string) (*Entry, error) {
	return newEntryWithTimestamp(tag, args, testcase, module, time.Since(defaultStartTime).Seconds(), nil)
}

func newEntryWithTimestamp(tag string, args map[string]any, testcase string, module string, timestamp float64, levelConfig map[string]map[string]string) (*Entry, error) {
	if tag == "" {
		return nil, fmt.Errorf("tag is required")
	}
	if testcase == "" {
		return nil, fmt.Errorf("testcase is required")
	}
	if module == "" {
		return nil, fmt.Errorf("module is required")
	}
	entry := &Entry{
		Tag:         strings.ToUpper(tag),
		Args:        args,
		Timestamp:   timestamp,
		Testcase:    testcase,
		Module:      module,
		levelConfig: levelConfig,
	}
	return entry, nil
}

// Level returns the log level for this entry.
func (e *Entry) Level() string {
	if e == nil {
		return ""
	}
	if e.levelSet {
		return e.level
	}

	level := "DEBUG"
	if e.levelConfig != nil {
		moduleLevels := e.levelConfig[strings.ToUpper(e.Module)]
		if moduleLevels != nil {
			if value, ok := moduleLevels[strings.ToUpper(e.Tag)]; ok {
				level = strings.ToUpper(value)
			}
		}
	}

	if _, ok := numericLevels[level]; !ok {
		panic(fmt.Errorf("unknown level string: %s", level))
	}

	e.level = level
	e.levelSet = true
	return level
}

func (e *Entry) setLevel(level string) {
	if e == nil {
		return
	}
	e.level = strings.ToUpper(level)
	e.levelSet = true
}

// NumericLevel returns the numeric representation of the log level.
func (e *Entry) NumericLevel() int {
	if e == nil {
		return 0
	}
	return numericLevels[e.Level()]
}

// Levels returns the configured level mapping.
func Levels() map[string]int {
	out := make(map[string]int, len(numericLevels))
	for key, value := range numericLevels {
		out[key] = value
	}
	return out
}

// StartTimeNow resets the log start timestamp.
func StartTimeNow() {
	defaultStartTime = time.Now()
}

// ResetConfig is kept for compatibility; per-run loggers hold their own config.
func ResetConfig() {
}

// String formats the entry in the raw logger textual style.
func (e *Entry) String() string {
	if e == nil {
		return ""
	}
	testcase := ""
	if e.Testcase != "" {
		testcase = ":" + e.Testcase
	}
	argstr := e.ArgString()
	if argstr != "" {
		argstr = " " + argstr
	}
	return fmt.Sprintf("%s%s:%s%s", e.Module, testcase, e.Tag, argstr)
}

// ArgString returns a stable string representation of the arguments.
func (e *Entry) ArgString() string {
	if e == nil || e.Args == nil {
		return ""
	}

	printable := e.PrintableArgs()
	keys := make([]string, 0, len(printable))
	for key := range printable {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := printable[key]
		parts = append(parts, key+"="+formatArgValue(value))
	}
	return strings.Join(parts, "; ")
}

// PrintableArgs returns a copy of args with normalized values.
func (e *Entry) PrintableArgs() map[string]any {
	if e == nil || e.Args == nil {
		return nil
	}

	out := make(map[string]any, len(e.Args))
	for key, value := range e.Args {
		if key == "asn" {
			if joined, ok := joinASN(value); ok {
				out[key] = joined
				continue
			}
		}
		out[key] = value
	}
	return out
}

func joinASN(value any) (string, bool) {
	switch v := value.(type) {
	case []string:
		return strings.Join(v, ","), true
	case []int:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = fmt.Sprint(item)
		}
		return strings.Join(parts, ","), true
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = fmt.Sprint(item)
		}
		return strings.Join(parts, ","), true
	default:
		return "", false
	}
}

func formatArgValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		rv := reflect.ValueOf(value)
		kind := rv.Kind()
		if kind == reflect.Array || kind == reflect.Slice || kind == reflect.Map || kind == reflect.Struct || kind == reflect.Pointer {
			if data, err := json.Marshal(value); err == nil {
				return string(data)
			}
		}
		return fmt.Sprint(value)
	}
}
