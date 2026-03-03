package main

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

type rawReporter struct {
	out      io.Writer
	minLevel int
	hasMin   bool
	mu       sync.Mutex
}

func newRawReporter(out io.Writer, minLevel string) *rawReporter {
	if out == nil {
		return nil
	}
	minLevel = strings.ToUpper(strings.TrimSpace(minLevel))
	minValue, hasMin := logger.Levels()[minLevel]
	return &rawReporter{
		out:      out,
		minLevel: minValue,
		hasMin:   hasMin && minLevel != "",
	}
}

// Callback writes one raw log entry line when it matches the level filter.
func (r *rawReporter) Callback(entry *logger.Entry) error {
	if r == nil || entry == nil {
		return nil
	}
	if r.hasMin && entry.NumericLevel() < r.minLevel {
		return nil
	}
	line := formatRawEntry(entry)
	r.mu.Lock()
	defer r.mu.Unlock()
	_, _ = fmt.Fprintln(r.out, line)
	return nil
}

func formatRawEntry(entry *logger.Entry) string {
	if entry == nil {
		return ""
	}
	testcase := ""
	if entry.Testcase != "" {
		testcase = ":" + entry.Testcase
	}
	argStr := formatRawArgs(entry.PrintableArgs())
	if argStr != "" {
		argStr = " " + argStr
	}
	return fmt.Sprintf("%s%s:%s%s", entry.Module, testcase, entry.Tag, argStr)
}

func formatRawArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+formatRawArgValue(args[key]))
	}
	return strings.Join(parts, "; ")
}

func formatRawArgValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int8, int16, int32, int64:
		return fmt.Sprint(v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(v)
	case float32, float64:
		return fmt.Sprint(v)
	case map[string]any:
		if endpoint, ok := endpointFromMap(v); ok {
			return endpoint
		}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+"="+formatRawArgValue(v[key]))
		}
		return "{" + strings.Join(parts, ";") + "}"
	}

	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		parts := make([]string, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			parts = append(parts, formatRawArgValue(rv.Index(i).Interface()))
		}
		if len(parts) == 0 {
			return "<empty>"
		}
		return strings.Join(parts, ",")
	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			keys := make([]string, 0, rv.Len())
			for _, key := range rv.MapKeys() {
				keys = append(keys, key.String())
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, key := range keys {
				val := rv.MapIndex(reflect.ValueOf(key))
				var item any
				if val.IsValid() {
					item = val.Interface()
				}
				parts = append(parts, key+"="+formatRawArgValue(item))
			}
			return "{" + strings.Join(parts, ";") + "}"
		}
		return fmt.Sprint(value)
	default:
		return fmt.Sprint(value)
	}
}

func endpointFromMap(item map[string]any) (string, bool) {
	if item == nil {
		return "", false
	}
	ns, _ := item["ns"].(string)
	address, _ := item["address"].(string)
	ns = strings.TrimSpace(ns)
	address = strings.TrimSpace(address)
	if ns == "" && address == "" {
		return "", false
	}
	if ns != "" && address != "" {
		return ns + "/" + address, true
	}
	if ns != "" {
		return ns, true
	}
	return address, true
}
