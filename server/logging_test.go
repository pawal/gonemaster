package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// capturedRecord is a decoded log line used for asserting on structured
// attributes rather than a pre-formatted message string.
type capturedRecord struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

// chanHandler is a slog.Handler that pushes each record onto a channel. It lets
// tests block until an asynchronous loop (e.g. the purge loop) emits a line and
// then assert on the individual attributes. Shared by the logging, access-log,
// and purge tests.
type chanHandler struct {
	ch    chan capturedRecord
	attrs []slog.Attr
}

// newChanLogger returns a logger whose records are delivered on ch. buffer sizes
// the channel so a producing goroutine never blocks in these tests.
func newChanLogger(buffer int) (*slog.Logger, chan capturedRecord) {
	ch := make(chan capturedRecord, buffer)
	return slog.New(&chanHandler{ch: ch}), ch
}

func (h *chanHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *chanHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{}
	for _, a := range h.attrs {
		attrs[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.ch <- capturedRecord{Level: r.Level, Message: r.Message, Attrs: attrs}
	return nil
}

func (h *chanHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &chanHandler{ch: h.ch, attrs: append(append([]slog.Attr{}, h.attrs...), as...)}
}

func (h *chanHandler) WithGroup(string) slog.Handler { return h }

// decodeLogLines parses each JSON line written into buf.
func decodeLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestNewLoggerJSONEmitsParseableObject(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger("json", "info", &buf)
	logger.Info("hello", "count", 3, "who", "world")

	lines := decodeLogLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSON line, got %d", len(lines))
	}
	line := lines[0]
	if line["msg"] != "hello" {
		t.Fatalf("msg = %v, want hello", line["msg"])
	}
	if line["level"] != "INFO" {
		t.Fatalf("level = %v, want INFO", line["level"])
	}
	// JSON numbers decode to float64.
	if line["count"] != float64(3) {
		t.Fatalf("count = %v, want 3", line["count"])
	}
	if line["who"] != "world" {
		t.Fatalf("who = %v, want world", line["who"])
	}
}

func TestNewLoggerTextIsHumanReadable(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger("text", "info", &buf)
	logger.Info("hello", "who", "world")

	out := buf.String()
	// The text handler renders key=value pairs, not a JSON object.
	if !strings.Contains(out, "msg=hello") {
		t.Fatalf("text output missing msg=hello: %q", out)
	}
	if !strings.Contains(out, "who=world") {
		t.Fatalf("text output missing who=world: %q", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("text output should not be JSON: %q", out)
	}
}

func TestNewLoggerUnknownFormatDefaultsToText(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger("yaml", "info", &buf)
	logger.Info("hello")
	if strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Fatalf("unknown format should fall back to text, got %q", buf.String())
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{" Error ", slog.LevelError},
		{"", slog.LevelInfo},
		{"bogus", slog.LevelInfo},
	}
	for _, tc := range cases {
		if got := parseLevel(tc.in); got != tc.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNewLoggerLevelFiltersDebugAtInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger("json", "info", &buf)
	logger.Debug("should-be-dropped")
	if buf.Len() != 0 {
		t.Fatalf("expected debug line dropped at level=info, got %q", buf.String())
	}
	logger.Info("should-appear")
	if buf.Len() == 0 {
		t.Fatal("expected info line to appear at level=info")
	}
}

func TestValidateLogConfig(t *testing.T) {
	valid := []Config{
		{LogFormat: "text", LogLevel: "info"},
		{LogFormat: "json", LogLevel: "debug"},
		{LogFormat: "JSON", LogLevel: "WARN"},
		{LogFormat: "", LogLevel: ""}, // empty resolves to defaults
	}
	for _, cfg := range valid {
		if err := ValidateLogConfig(cfg); err != nil {
			t.Errorf("ValidateLogConfig(%+v) unexpected error: %v", cfg, err)
		}
	}

	badFormat := Config{LogFormat: "xml", LogLevel: "info"}
	if err := ValidateLogConfig(badFormat); err == nil {
		t.Error("expected error for invalid log_format")
	}
	badLevel := Config{LogFormat: "text", LogLevel: "verbose"}
	if err := ValidateLogConfig(badLevel); err == nil {
		t.Error("expected error for invalid log_level")
	}
}

func TestNewRequestIDIsUniqueHex(t *testing.T) {
	a := newRequestID()
	b := newRequestID()
	if a == "" || b == "" {
		t.Fatal("newRequestID returned empty")
	}
	if a == b {
		t.Fatalf("expected distinct IDs, got %q twice", a)
	}
	if len(a) != 16 {
		t.Fatalf("expected 16 hex chars, got %d (%q)", len(a), a)
	}
}

func TestSanitizeRequestID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"abc123-DEF_.", "abc123-DEF_."},
		{"  trimmed  ", "trimmed"},
		{"has space", ""},
		{"inject\nline", ""},
		{"semi;colon", ""},
		{"", ""},
		{strings.Repeat("a", 129), ""},
	}
	for _, tc := range cases {
		if got := sanitizeRequestID(tc.in); got != tc.want {
			t.Errorf("sanitizeRequestID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
