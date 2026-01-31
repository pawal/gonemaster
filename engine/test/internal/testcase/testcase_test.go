package testcase

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/util"
)

func TestRunBuffersAndFlushesEntries(t *testing.T) {
	parent := logger.New()
	util.SetLogger(parent)
	t.Cleanup(func() { util.SetLogger(nil) })

	entries, err := Run(context.Background(), func(ctx context.Context) ([]*logger.Entry, error) {
		if _, err := util.Logger().Add("TEST_CASE_START", map[string]any{"testcase": "Demo"}, "Demo", "Demo01"); err != nil {
			return nil, err
		}
		return util.Logger().Entries(), nil
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	parentEntries := parent.Entries()
	if len(parentEntries) != 1 {
		t.Fatalf("expected 1 parent entry, got %d", len(parentEntries))
	}
	if parentEntries[0].Tag != "TEST_CASE_START" {
		t.Fatalf("unexpected parent tag %q", parentEntries[0].Tag)
	}

	if util.Logger() != parent {
		t.Fatalf("expected logger to be restored to parent")
	}
}

func TestRunStreamsExtraEntriesWithCallback(t *testing.T) {
	parent := logger.New()
	var tags []string
	parent.Callback = func(entry *logger.Entry) error {
		if entry != nil {
			tags = append(tags, entry.Tag)
		}
		return nil
	}
	util.SetLogger(parent)
	t.Cleanup(func() { util.SetLogger(nil) })

	entries, err := Run(context.Background(), func(ctx context.Context) ([]*logger.Entry, error) {
		if _, err := util.Logger().Add("BUF_ENTRY", map[string]any{}, "Test", "Case"); err != nil {
			return nil, err
		}
		extraLogger := logger.New()
		extra, err := extraLogger.Add("EXTRA_ENTRY", map[string]any{}, "Test", "Case")
		if err != nil {
			return nil, err
		}
		return []*logger.Entry{extra}, nil
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	got := map[string]bool{}
	for _, tag := range tags {
		got[tag] = true
	}
	if !got["BUF_ENTRY"] || !got["EXTRA_ENTRY"] {
		t.Fatalf("expected callback tags BUF_ENTRY and EXTRA_ENTRY, got %#v", tags)
	}
}
