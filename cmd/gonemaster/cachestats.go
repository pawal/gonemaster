package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"

	"codeberg.org/pawal/gonemaster/engine/cachefile"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

// printCacheStats writes a human-readable summary of a parsed cache file.
func printCacheStats(out io.Writer, path string, file cachefile.File) error {
	size, gzipped := fileSizeAndCompression(path)
	compression := "plain"
	if gzipped {
		compression = "gzip"
	}
	s := file.Stats()

	if _, err := fmt.Fprintf(out, "file:     %s (%d bytes, %s)\n", path, size, compression); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "entries:  %d total\n", s.Total); err != nil {
		return err
	}
	for _, kind := range sortedKeys(s.ByKind) {
		if _, err := fmt.Fprintf(out, "  %-11s %d\n", kind, s.ByKind[kind]); err != nil {
			return err
		}
	}
	if len(s.ByAddress) > 0 {
		if _, err := fmt.Fprintln(out, "by address (nameserver):"); err != nil {
			return err
		}
		for _, addr := range sortedKeys(s.ByAddress) {
			if _, err := fmt.Fprintf(out, "  %-20s %d\n", addr, s.ByAddress[addr]); err != nil {
				return err
			}
		}
	}
	return nil
}

// restoredCacheSummary renders the packet-cache hit/miss line for a restored run.
func restoredCacheSummary(m nameserver.CacheMetrics) string {
	return fmt.Sprintf("packet cache: %d hits, %d misses", m.Hits, m.Misses)
}

// fileSizeAndCompression returns the on-disk size and whether the file starts
// with the gzip magic bytes. Errors yield a zero size and false.
func fileSizeAndCompression(path string) (int64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	f, err := os.Open(path)
	if err != nil {
		return info.Size(), false
	}
	defer f.Close()
	magic := make([]byte, 2)
	n, _ := io.ReadFull(f, magic)
	return info.Size(), n == 2 && bytes.Equal(magic, []byte{0x1f, 0x8b})
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
