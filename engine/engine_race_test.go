package engine

import (
	"slices"
	"sync"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

// Concurrent runs over one shared cache lineage must produce identical
// findings. This is the executable form of the promise in the architecture
// doc: a binary may call engine.Run from many goroutines and each run keeps
// its own state.
//
// System-module entries are excluded from the comparison on purpose. Under the
// hot cache a run store adopts the parent's per-address query cache by
// pointer, so whichever goroutine warms a key first turns the other runs'
// QUERY into CACHED_RETURN. That divergence is cache warmth, not a difference
// in findings, and comparing unfiltered would make this test fail for a reason
// it is not about.
func TestRunConcurrentIdenticalFindings(t *testing.T) {
	t.Parallel()

	const runs = 6
	base := nameserver.NewCacheStore()

	type result struct {
		index    int
		findings []string
		err      error
	}
	results := make([]result, runs)

	// Built on the test goroutine so the helpers may fail the test; the
	// goroutines below only run them. Offline profiles keep traffic inside the
	// test and make every query fail the same deterministic way in every run.
	runners := make([]*Runner, runs)
	for i := range runs {
		prof := dnstest.DefaultProfile(t)
		prof.NoNetwork = true
		runners[i] = newTestRunner(t, withProfile(prof), withRunLimits(4), withCache(base.SnapshotForRun()))
	}

	var wg sync.WaitGroup
	wg.Add(runs)
	for i := range runs {
		go func() {
			defer wg.Done()
			runner := runners[i]
			log := runner.Logger
			req := RunRequest{Domain: "example.com", Testcases: []string{"syntax01", "basic01"}}
			if _, err := RunWithRunner(req, runner); err != nil {
				results[i] = result{index: i, err: err}
				return
			}
			results[i] = result{index: i, findings: findingSignature(log)}
		}()
	}
	wg.Wait()

	for _, res := range results {
		if res.err != nil {
			t.Fatalf("run %d failed: %v", res.index, res.err)
		}
	}
	if len(results[0].findings) == 0 {
		t.Fatalf("expected the runs to produce findings; got none")
	}

	want := results[0].findings
	for _, res := range results[1:] {
		if len(res.findings) != len(want) {
			t.Fatalf("run %d produced %d findings, run 0 produced %d", res.index, len(res.findings), len(want))
		}
		for i := range want {
			if res.findings[i] != want[i] {
				t.Fatalf("run %d diverged from run 0 at %d: %q vs %q", res.index, i, res.findings[i], want[i])
			}
		}
	}
}

// findingSignature returns a sorted, comparable view of the run's findings,
// excluding the System module (framework diagnostics such as query and cache
// tracing, which legitimately differ with cache warmth).
func findingSignature(log *logger.Logger) []string {
	var findings []*logger.Entry
	for _, entry := range log.Entries() {
		if entry != nil && entry.Module != "System" {
			findings = append(findings, entry)
		}
	}
	out := dnstest.Signature(findings)
	slices.Sort(out)
	return out
}
