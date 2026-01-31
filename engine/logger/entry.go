package logger

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine/profile"
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

var startTime = time.Now()

var testLevelsConfig map[string]map[string]string

// Entry represents a single log entry.
type Entry struct {
	Tag       string
	Args      map[string]any
	Timestamp float64
	Testcase  string
	Module    string

	level    string
	levelSet bool
}

// NewEntry constructs a log entry with timestamp and metadata.
func NewEntry(tag string, args map[string]any, testcase string, module string) (*Entry, error) {
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
		Tag:       strings.ToUpper(tag),
		Args:      args,
		Timestamp: time.Since(startTime).Seconds(),
		Testcase:  testcase,
		Module:    module,
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

	configMu.Lock()
	if testLevelsConfig == nil {
		testLevelsConfig = profile.Effective().TestLevels
	}
	levelConfig := testLevelsConfig
	configMu.Unlock()

	level := "DEBUG"
	if levelConfig != nil {
		moduleLevels := levelConfig[strings.ToUpper(e.Module)]
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
	startTime = time.Now()
}

// ResetConfig clears cached config for log levels.
func ResetConfig() {
	configMu.Lock()
	testLevelsConfig = nil
	logFilter = nil
	configMu.Unlock()
}

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
