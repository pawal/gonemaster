package server

import (
	"os"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func BenchmarkApplyProfileOverrides(b *testing.B) {
	baseFile, err := os.CreateTemp("", "gm-profile-*.json")
	if err != nil {
		b.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(`{"net":{"ipv4":false}}`); err != nil {
		_ = baseFile.Close()
		_ = os.Remove(baseFile.Name())
		b.Fatalf("write base profile: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		_ = os.Remove(baseFile.Name())
		b.Fatalf("close base profile: %v", err)
	}
	defer os.Remove(baseFile.Name())

	overrides := map[string]any{
		"resolver": map[string]any{
			"defaults": map[string]any{
				"timeout": 3,
				"retry":   2,
			},
		},
	}

	b.Run("uncached", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := engine.RunRequest{Domain: "example.com"}
			cleanup, err := applyProfileOverrides(&req, overrides, baseFile.Name())
			if err != nil {
				b.Fatalf("apply overrides: %v", err)
			}
			if cleanup == nil {
				b.Fatalf("expected cleanup function")
			}
			cleanup()
		}
	})

	b.Run("cached", func(b *testing.B) {
		cache := newProfileOverrideCache(64, time.Hour)
		defer cache.Close()

		warmupReq := engine.RunRequest{Domain: "example.com"}
		cleanup, err := applyProfileOverridesWithCache(&warmupReq, overrides, baseFile.Name(), cache)
		if err != nil {
			b.Fatalf("warmup apply overrides: %v", err)
		}
		if cleanup != nil {
			cleanup()
			b.Fatalf("expected nil cleanup on warmup cache insert")
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := engine.RunRequest{Domain: "example.com"}
			cleanup, err := applyProfileOverridesWithCache(&req, overrides, baseFile.Name(), cache)
			if err != nil {
				b.Fatalf("apply overrides: %v", err)
			}
			if cleanup != nil {
				cleanup()
				b.Fatalf("expected nil cleanup on cache hit")
			}
		}
	})
}
