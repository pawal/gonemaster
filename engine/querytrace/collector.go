package querytrace

import (
	"sort"
	"sync"
	"time"
)

// Collector is the default QueryTrace: a concurrency-safe per-nameserver
// aggregator of attempts, timeouts, and control decisions.
type Collector struct {
	mu sync.Mutex
	ns map[string]*NSStats
}

// NSStats aggregates one nameserver address's traced activity.
type NSStats struct {
	Name         string
	Addr         string
	Attempts     int
	Timeouts     int
	Errors       int
	TotalElapsed time.Duration
	Decisions    map[DecisionKind]int
}

// NewCollector returns an empty Collector.
func NewCollector() *Collector {
	return &Collector{ns: map[string]*NSStats{}}
}

// statsFor returns the per-address stats, creating them on first use. Must be
// called with c.mu held.
func (c *Collector) statsFor(addr, name string) *NSStats {
	s := c.ns[addr]
	if s == nil {
		s = &NSStats{Addr: addr, Decisions: map[DecisionKind]int{}}
		c.ns[addr] = s
	}
	if s.Name == "" && name != "" {
		s.Name = name
	}
	return s
}

// AttemptDone records one transport attempt.
func (c *Collector) AttemptDone(ev AttemptEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.statsFor(ev.NSAddr, ev.NSName)
	s.Attempts++
	s.TotalElapsed += ev.Elapsed
	switch ev.Outcome {
	case OutcomeTimeout:
		s.Timeouts++
	case OutcomeError:
		s.Errors++
	}
}

// Decision records one control decision.
func (c *Collector) Decision(ev DecisionEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.statsFor(ev.NSAddr, ev.NSName)
	s.Decisions[ev.Kind]++
}

// Totals returns run-wide counts: attempts, timeouts, and nameservers seen.
func (c *Collector) Totals() (attempts, timeouts, nameservers int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.ns {
		attempts += s.Attempts
		timeouts += s.Timeouts
	}
	return attempts, timeouts, len(c.ns)
}

// Stats returns a copy of the per-nameserver aggregates, sorted by attributable
// time (slowest first).
func (c *Collector) Stats() []NSStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]NSStats, 0, len(c.ns))
	for _, s := range c.ns {
		cp := *s
		cp.Decisions = make(map[DecisionKind]int, len(s.Decisions))
		for k, v := range s.Decisions {
			cp.Decisions[k] = v
		}
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalElapsed != out[j].TotalElapsed {
			return out[i].TotalElapsed > out[j].TotalElapsed
		}
		if out[i].Attempts != out[j].Attempts {
			return out[i].Attempts > out[j].Attempts
		}
		return out[i].Addr < out[j].Addr
	})
	return out
}
