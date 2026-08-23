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
	srv := newTestServer(t) // ShowDNSSECChainPublic defaults true
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
	srv := newTestServer(t)
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
	srv := newTestServer(t, withConfig(func(c *Config) { c.ShowDNSSECChainPublic = false }))
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
	srv := newTestServer(t)

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

func TestRunEngineForJobKeepsChainWithMostEvidence(t *testing.T) {
	// A multi-testcase job invokes the sink once per engine run. A later run
	// without parent evidence (its testcase never resolved the parent zone)
	// must not overwrite an earlier summary that has it.
	srv := newTestServer(t)
	rich := &dnssecchain.Summary{
		Version: dnssecchain.Version,
		Zone:    "example.com",
		Status:  dnssecchain.StatusSecure,
		Parent:  dnssecchain.Parent{DSSource: dnssecchain.DSSourceParent, ServersQueried: []string{"192.0.2.1"}},
		Child:   dnssecchain.Child{ServersQueried: []string{"203.0.113.1"}},
	}
	poor := &dnssecchain.Summary{
		Version: dnssecchain.Version,
		Zone:    "example.com",
		Status:  dnssecchain.StatusIsland,
		Parent:  dnssecchain.Parent{DSSource: dnssecchain.DSSourceNone},
		Child:   dnssecchain.Child{ServersQueried: []string{"203.0.113.1"}},
	}
	summaries := []*dnssecchain.Summary{rich, poor}
	i := 0
	srv.engineRunner = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.DNSSECChainSink != nil && i < len(summaries) {
			req.DNSSECChainSink(summaries[i])
			i++
		}
		return nil, nil
	}

	job := chainJob("job-multi", JobOriginPublic)
	job.Tests = []string{"dnssec05", "zone01"}
	art, err := srv.runEngineForJob(job, context.Background())
	if err != nil {
		t.Fatalf("runEngineForJob: %v", err)
	}
	if !strings.Contains(art.dnssecChainJSON, `"status":"secure"`) {
		t.Errorf("expected the richer summary to be kept, got %s", art.dnssecChainJSON)
	}
}

func TestChainEvidenceScoreOrdering(t *testing.T) {
	full := &dnssecchain.Summary{
		Parent: dnssecchain.Parent{ServersQueried: []string{"192.0.2.1"}},
		Child:  dnssecchain.Child{ServersQueried: []string{"203.0.113.1"}},
	}
	childOnly := &dnssecchain.Summary{
		Child: dnssecchain.Child{ServersQueried: []string{"203.0.113.1"}},
	}
	input := &dnssecchain.Summary{
		Parent: dnssecchain.Parent{DSSource: dnssecchain.DSSourceInput},
	}
	if !(chainEvidenceScore(full) > chainEvidenceScore(childOnly)) {
		t.Error("full evidence must outrank child-only evidence")
	}
	if !(chainEvidenceScore(childOnly) > chainEvidenceScore(nil)) {
		t.Error("any summary must outrank nil")
	}
	if !(chainEvidenceScore(input) > chainEvidenceScore(childOnly)) {
		t.Error("undelegated input DS counts as parent evidence")
	}
	if !(chainEvidenceScore(nil) < 0) {
		t.Error("nil must never replace an existing summary")
	}
}
