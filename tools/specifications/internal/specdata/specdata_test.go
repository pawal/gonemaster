package specdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTestLevels(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "profile.json")
	if err := os.WriteFile(good, []byte(`{"test_levels":{"ZONE":{"A_TAG":"WARNING"}}}`), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	levels, err := TestLevels(good)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if levels["ZONE"]["A_TAG"] != "WARNING" {
		t.Fatalf("levels = %v, want ZONE A_TAG WARNING", levels)
	}

	if _, err := TestLevels(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("load of a missing file returned no error")
	}
}
