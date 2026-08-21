package tctest

import (
	"slices"
	"sync"
)

// Gate blocks query hooks so a test can assert that a testcase issues its
// queries in parallel. Inside a synctest bubble the block is durable, so
// synctest.Wait returns once every parallel query has arrived at the gate.
type Gate struct {
	mu      sync.Mutex
	arrived []string
	release chan struct{}
}

// NewGate returns a gate with nothing arrived and nothing released.
func NewGate() *Gate {
	return &Gate{release: make(chan struct{})}
}

// Arrive records id and blocks until Release. Call it from a query hook.
func (g *Gate) Arrive(id string) {
	g.Record(id)
	<-g.release
}

// Record records id without blocking, for a query the test lets through.
func (g *Gate) Record(id string) {
	g.mu.Lock()
	g.arrived = append(g.arrived, id)
	g.mu.Unlock()
}

// InFlight returns the recorded ids in arrival order, duplicates kept.
func (g *Gate) InFlight() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.arrived)
}

// Release unblocks every arrived and later caller.
func (g *Gate) Release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-g.release:
	default:
		close(g.release)
	}
}

// RequireInFlight fails unless the distinct arrived ids are exactly want.
func (g *Gate) RequireInFlight(t TB, want ...string) {
	t.Helper()
	got := g.InFlight()
	distinct := slices.Clone(got)
	slices.Sort(distinct)
	distinct = slices.Compact(distinct)
	sorted := slices.Clone(want)
	slices.Sort(sorted)
	if !slices.Equal(distinct, sorted) {
		t.Fatalf("expected parallel queries from %v, got %v", want, got)
	}
}
