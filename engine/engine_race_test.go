package engine

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
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

	var wg sync.WaitGroup
	wg.Add(runs)
	for i := range runs {
		go func() {
			defer wg.Done()
			p, err := profile.Default()
			if err != nil {
				results[i] = result{index: i, err: err}
				return
			}
			// Offline: no traffic leaves the test, and every query fails the
			// same deterministic way in every run.
			p.NoNetwork = true

			log := logger.New()
			runner := &Runner{
				Profile:         p,
				Logger:          log,
				Limiter:         transport.NewLimiter(4),
				NameserverCache: base.SnapshotForRun(),
			}
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
	var out []string
	for _, entry := range log.Entries() {
		if entry == nil || entry.Module == "System" {
			continue
		}
		out = append(out, fmt.Sprintf("%s/%s/%s/%s", entry.Module, entry.Testcase, entry.Tag, entry.Level()))
	}
	sort.Strings(out)
	return out
}
