package main

import (
	"testing"

	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

func TestRestoredCacheSummary(t *testing.T) {
	got := restoredCacheSummary(nameserver.CacheMetrics{Hits: 3, Misses: 5})
	want := "packet cache: 3 hits, 5 misses"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
