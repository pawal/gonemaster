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
