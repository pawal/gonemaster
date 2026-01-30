package testlogger

import (
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

func TestBufferAddUsesModuleAndTestcase(t *testing.T) {
	buf := New("Basic", "Basic01")
	entry, err := buf.Add("TEST_CASE_START", map[string]any{"testcase": "Basic01"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if entry.Module != "Basic" {
		t.Fatalf("expected module Basic, got %q", entry.Module)
	}
	if entry.Testcase != "Basic01" {
		t.Fatalf("expected testcase Basic01, got %q", entry.Testcase)
	}
}

func TestBufferAppendAddsToSlice(t *testing.T) {
	buf := New("System", "Case")
	var entries []*logger.Entry
	if err := buf.Append(&entries, "CUSTOM", nil); err != nil {
		t.Fatalf("append: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Tag != "CUSTOM" {
		t.Fatalf("unexpected tag %q", entries[0].Tag)
	}
}
