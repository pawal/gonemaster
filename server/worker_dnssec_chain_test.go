package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/dnssecchain"
)

// chainStubEngine simulates the engine invoking the chain sink when one is set.
func chainStubEngine(req engine.RunRequest) ([]engine.LogEntry, error) {
	if req.DNSSECChainSink != nil {
		req.DNSSECChainSink(&dnssecchain.Summary{
			Version: dnssecchain.Version,
			Zone:    "example.com",
			Status:  dnssecchain.StatusSecure,
		})
	}
	return nil, nil
}

func chainJob(id, origin string) Job {
	return Job{
		ID:        id,
		Domain:    "example.com",
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
		Origin:    origin,
	}
}

func TestRunEngineForJobCollectsChainForPublicOrigin(t *testing.T) {
	srv := New(DefaultConfig()) // ShowDNSSECChainPublic defaults true
	srv.engineRunner = chainStubEngine

	art, err := srv.runEngineForJob(chainJob("job-pub", JobOriginPublic), context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if art.dnssecChainJSON == "" {
		t.Fatal("expected a chain blob for a public-origin job")
	}
	if !strings.Contains(art.dnssecChainJSON, `"zone":"example.com"`) {
		t.Errorf("unexpected chain blob: %s", art.dnssecChainJSON)
	}
}

func TestRunEngineForJobSkipsChainForNonPublicOrigins(t *testing.T) {
	srv := New(DefaultConfig())
	srv.engineRunner = chainStubEngine

	for _, origin := range []string{JobOriginAdmin, JobOriginBatch, ""} {
		art, err := srv.runEngineForJob(chainJob("job-"+origin, origin), context.Background())
		if err != nil {
			t.Fatalf("origin %q: %v", origin, err)
		}
		if art.dnssecChainJSON != "" {
			t.Errorf("origin %q: expected no chain blob, got %q", origin, art.dnssecChainJSON)
		}
	}
}

func TestRunEngineForJobSkipsChainWhenFlagOff(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ShowDNSSECChainPublic = false
	srv := New(cfg)
	srv.engineRunner = chainStubEngine

	art, err := srv.runEngineForJob(chainJob("job-flagoff", JobOriginPublic), context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if art.dnssecChainJSON != "" {
		t.Errorf("flag off: expected no chain blob, got %q", art.dnssecChainJSON)
	}
}

func TestMarshalDNSSECChain(t *testing.T) {
	srv := New(DefaultConfig())

	if got := srv.marshalDNSSECChain(nil); got != "" {
		t.Errorf("nil summary: got %q, want empty", got)
	}

	small := &dnssecchain.Summary{Version: 1, Zone: "example.com", Status: dnssecchain.StatusSecure}
	if got := srv.marshalDNSSECChain(small); got == "" {
		t.Error("small summary: expected non-empty blob")
	}

	// A summary larger than the storage cap must be dropped, not truncated.
	oversized := &dnssecchain.Summary{Version: 1, Zone: "example.com"}
	for i := 0; i < 4000; i++ {
		oversized.Child.DNSKEYs = append(oversized.Child.DNSKEYs, dnssecchain.DNSKEY{
			KeyTag:  uint16(i),
			Servers: []string{"203.0.113.1", "203.0.113.2"},
		})
	}
	if got := srv.marshalDNSSECChain(oversized); got != "" {
		t.Errorf("oversized summary: expected drop, got %d bytes", len(got))
	}
}
