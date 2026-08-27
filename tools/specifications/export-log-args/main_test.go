package main

import (
	"bytes"
	"testing"
)

// scanRoot builds the inventory payload the same way main does.
func scanRoot(t *testing.T, root string) []byte {
	t.Helper()

	files, err := collectGoFiles(root)
	if err != nil {
		t.Fatalf("collect go files: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no files found under %s", root)
	}

	inv := newInventory()
	for _, file := range files {
		if err := scanFile(file, inv); err != nil {
			t.Fatalf("scan %s: %v", file, err)
		}
	}

	data := inv.asPayload(len(files))
	data.Summary.PackedListOnlyKeyCount = len(data.PackedListOnlyKeys)

	out, err := encodeJSON(data)
	if err != nil {
		t.Fatalf("encode json: %v", err)
	}
	return out
}

// The committed inventory is compared byte for byte by spec-check, so the
// generator must not let Go map iteration order leak into its output. Repeated
// scans of the same tree therefore have to agree exactly.
func TestExportIsDeterministic(t *testing.T) {
	const root = "../../../engine"

	first := scanRoot(t, root)
	for i := range 4 {
		if next := scanRoot(t, root); !bytes.Equal(first, next) {
			t.Fatalf("run %d differs from the first scan of %s", i+2, root)
		}
	}
}
