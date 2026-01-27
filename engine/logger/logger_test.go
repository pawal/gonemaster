package logger

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestAddDefaultsAndString(t *testing.T) {
	StartTimeNow()

	log := New()
	entry, err := log.Add("custom", map[string]any{
		"asn": []int{64500, 64501},
		"ip":  "192.0.2.1",
	}, "", "")
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if entry.Tag != "CUSTOM" {
		t.Fatalf("expected tag CUSTOM, got %q", entry.Tag)
	}
	if entry.Module != ModuleName {
		t.Fatalf("expected module %q, got %q", ModuleName, entry.Module)
	}
	if entry.Testcase != TestCaseName {
		t.Fatalf("expected testcase %q, got %q", TestCaseName, entry.Testcase)
	}
	if entry.Timestamp < 0 {
		t.Fatalf("expected non-negative timestamp")
	}

	argstr := entry.ArgString()
	if argstr != "asn=64500,64501; ip=192.0.2.1" {
		t.Fatalf("unexpected arg string: %q", argstr)
	}

	want := ModuleName + ":" + TestCaseName + ":CUSTOM asn=64500,64501; ip=192.0.2.1"
	if entry.String() != want {
		t.Fatalf("unexpected string: %q", entry.String())
	}
}

func TestLevelFromProfile(t *testing.T) {
	defer profile.ResetEffective()
	ResetConfig()

	log := New()
	entry, err := log.Add("TEST_CASE_START", nil, "Basic", "Basic01")
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if entry.Level() != "DEBUG" {
		t.Fatalf("expected DEBUG level, got %q", entry.Level())
	}
}

func TestJSONMinLevel(t *testing.T) {
	ResetConfig()

	log := New()
	first, err := log.Add("FIRST", nil, "System", "Unspecified")
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}
	first.setLevel("DEBUG")

	second, err := log.Add("SECOND", map[string]any{"key": "value"}, "System", "Unspecified")
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}
	second.setLevel("ERROR")

	raw, err := log.JSON("ERROR")
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(items))
	}
	if tag, ok := items[0]["tag"].(string); !ok || strings.ToUpper(tag) != "SECOND" {
		t.Fatalf("unexpected tag in json: %#v", items[0]["tag"])
	}
}

func TestLevelsReturnsCopy(t *testing.T) {
	levels := Levels()
	levels["DEBUG"] = 99
	if Levels()["DEBUG"] == 99 {
		t.Fatalf("expected levels to be a copy")
	}
}

func TestLogFilterOverridesLevel(t *testing.T) {
	defer profile.ResetEffective()
	ResetConfig()

	profile.Effective().LogFilter = map[string]map[string][]profile.LogFilterRule{
		"SYSTEM": {
			"ALERT": {
				{
					When: map[string]any{
						"asn": []string{"64501"},
					},
					Set: "ERROR",
				},
			},
		},
	}

	log := New()
	entry, err := log.Add("alert", map[string]any{"asn": 64501}, "", "")
	if err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if entry.Level() != "ERROR" {
		t.Fatalf("expected ERROR level, got %q", entry.Level())
	}
}

func TestCallbackErrorAddsEntry(t *testing.T) {
	ResetConfig()

	log := New()
	log.Callback = func(_ *Entry) error {
		return fmt.Errorf("boom")
	}
	if _, err := log.Add("CALLBACK", nil, "", ""); err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if log.Callback != nil {
		t.Fatalf("expected callback to be cleared after error")
	}
	if len(log.Entries()) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(log.Entries()))
	}
	if log.Entries()[1].Tag != "LOGGER_CALLBACK_ERROR" {
		t.Fatalf("unexpected error tag %q", log.Entries()[1].Tag)
	}
}

func TestLevelUnknownPanics(t *testing.T) {
	defer profile.ResetEffective()
	ResetConfig()

	profile.Effective().TestLevels = map[string]map[string]string{
		"SYSTEM": {
			"ALERT": "bogus",
		},
	}

	entry, err := NewEntry("ALERT", nil, "Case", "System")
	if err != nil {
		t.Fatalf("new entry: %v", err)
	}

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("expected panic for unknown level")
		}
	}()
	_ = entry.Level()
}
