package server

import (
	"context"
	"sort"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	enginenameserver "codeberg.org/pawal/gonemaster/engine/nameserver"
)

type nameserverTimingTarget struct {
	name    string
	address string
}

func normalizeNameserverName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

func nameserverTimingTargets(job Job, info DelegationInfo) []nameserverTimingTarget {
	if len(job.UndelegatedNS) > 0 {
		targets := make([]nameserverTimingTarget, 0, len(job.UndelegatedNS))
		for _, item := range job.UndelegatedNS {
			name := normalizeNameserverName(item.Name)
			if name == "" {
				continue
			}
			targets = append(targets, nameserverTimingTarget{
				name:    name,
				address: strings.TrimSpace(item.IP),
			})
		}
		return uniqueNameserverTimingTargets(targets)
	}

	targets := make([]nameserverTimingTarget, 0, len(info.Nameservers))
	for _, item := range info.Nameservers {
		name := normalizeNameserverName(item.NS)
		if name == "" {
			continue
		}
		targets = append(targets, nameserverTimingTarget{
			name:    name,
			address: strings.TrimSpace(item.IP),
		})
	}
	return uniqueNameserverTimingTargets(targets)
}

func uniqueNameserverTimingTargets(items []nameserverTimingTarget) []nameserverTimingTarget {
	seen := map[string]bool{}
	out := make([]nameserverTimingTarget, 0, len(items))
	for _, item := range items {
		key := item.name + "/" + item.address
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

// nsFailureCounts are the per-address failure counters that ride along with
// the timing rows.
type nsFailureCounts struct {
	timeouts int
	refused  int
}

func summarizeNameserverTimings(queryTimings map[string][]time.Duration, queryTimeouts, queryRefused map[string]int, targets []nameserverTimingTarget) []NameserverTiming {
	if len(targets) == 0 {
		return nil
	}

	// Index queryTimings by (name, address) and by name alone so a
	// name-only target can discover the addresses the engine actually
	// probed.
	type sampleEntry struct {
		name, address string
		samples       []time.Duration
	}
	samplesByKey := map[string]sampleEntry{}
	samplesByName := map[string][]sampleEntry{}
	for key, samples := range queryTimings {
		name, address, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		name = normalizeNameserverName(name)
		address = strings.TrimSpace(address)
		entry := sampleEntry{name: name, address: address, samples: samples}
		samplesByKey[name+"/"+address] = entry
		samplesByName[name] = append(samplesByName[name], entry)
	}

	// Indexed separately: an address that only ever timed out has no
	// samples entry, and its count must still reach a row.
	countsByKey := map[string]nsFailureCounts{}
	timeoutAddrsByName := map[string][]string{}
	for key, count := range queryTimeouts {
		name, address, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		name = normalizeNameserverName(name)
		address = strings.TrimSpace(address)
		entry := countsByKey[name+"/"+address]
		entry.timeouts += count
		countsByKey[name+"/"+address] = entry
		timeoutAddrsByName[name] = append(timeoutAddrsByName[name], address)
	}
	// REFUSED never creates a row of its own: it is a response, so the
	// address always has samples and a target row already.
	for key, count := range queryRefused {
		name, address, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		name = normalizeNameserverName(name)
		address = strings.TrimSpace(address)
		entry := countsByKey[name+"/"+address]
		entry.refused += count
		countsByKey[name+"/"+address] = entry
	}

	out := make([]NameserverTiming, 0, len(targets))
	seen := map[string]bool{}
	emit := func(t NameserverTiming) {
		key := t.Nameserver + "/" + t.Address
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, t)
	}

	for _, target := range targets {
		if target.name == "" {
			continue
		}
		if target.address == "" {
			// Name-only target: use whatever addresses the engine probed
			// for the name, including ones that only ever timed out. Zero
			// matches → the NS hostname never resolved, emit a single
			// "unresolved" marker row.
			matches := samplesByName[target.name]
			timedOut := timeoutAddrsByName[target.name]
			if len(matches) == 0 && len(timedOut) == 0 {
				emit(NameserverTiming{
					Nameserver: target.name,
					Status:     NameserverTimingStatusUnresolved,
				})
				continue
			}
			for _, m := range matches {
				emit(timingFromSamples(m.name, m.address, m.samples, countsByKey[m.name+"/"+m.address]))
			}
			for _, address := range timedOut {
				counts := countsByKey[target.name+"/"+address]
				emit(NameserverTiming{
					Nameserver:   target.name,
					Address:      address,
					Status:       NameserverTimingStatusUnreachable,
					TimeoutCount: counts.timeouts,
					RefusedCount: counts.refused,
				})
			}
			continue
		}

		key := target.name + "/" + target.address
		m, ok := samplesByKey[key]
		if !ok {
			counts := countsByKey[key]
			emit(NameserverTiming{
				Nameserver:   target.name,
				Address:      target.address,
				Status:       NameserverTimingStatusUnreachable,
				TimeoutCount: counts.timeouts,
				RefusedCount: counts.refused,
			})
			continue
		}
		emit(timingFromSamples(m.name, m.address, m.samples, countsByKey[key]))
	}

	sort.Slice(out, func(i, j int) bool {
		pi, pj := timingStatusPriority(out[i].Status), timingStatusPriority(out[j].Status)
		if pi != pj {
			return pi < pj
		}
		if out[i].AvgMS != out[j].AvgMS {
			return out[i].AvgMS > out[j].AvgMS
		}
		if out[i].Nameserver != out[j].Nameserver {
			return out[i].Nameserver < out[j].Nameserver
		}
		return out[i].Address < out[j].Address
	})
	return out
}

// timingFromSamples builds an "ok" row from samples, or downgrades to
// "unreachable" when stats computation yields zero count (defensive: the
// caller should already have filtered empty samples).
func timingFromSamples(name, address string, samples []time.Duration, counts nsFailureCounts) NameserverTiming {
	stats := enginenameserver.ComputeTimingStats(samples)
	if stats.Count == 0 {
		return NameserverTiming{
			Nameserver:   name,
			Address:      address,
			Status:       NameserverTimingStatusUnreachable,
			TimeoutCount: counts.timeouts,
			RefusedCount: counts.refused,
		}
	}
	return NameserverTiming{
		Nameserver:   name,
		Address:      address,
		AvgMS:        stats.Avg,
		MinMS:        stats.Min,
		MaxMS:        stats.Max,
		MedianMS:     stats.Median,
		StddevMS:     stats.Stddev,
		Count:        stats.Count,
		Status:       NameserverTimingStatusOK,
		TimeoutCount: counts.timeouts,
		RefusedCount: counts.refused,
	}
}

// timingStatusPriority orders "worst first" so operators see problems at
// the top: unresolved, unreachable, then ok rows (sorted slowest first).
func timingStatusPriority(status string) int {
	switch status {
	case NameserverTimingStatusUnresolved:
		return 0
	case NameserverTimingStatusUnreachable:
		return 1
	default:
		return 2
	}
}

func cloneNameserverTimings(items []NameserverTiming) []NameserverTiming {
	if len(items) == 0 {
		return nil
	}
	out := make([]NameserverTiming, len(items))
	copy(out, items)
	return out
}

func (s *Server) collectNameserverTimings(job Job, queryTimings map[string][]time.Duration, queryTimeouts, queryRefused map[string]int, entries []engine.LogEntry) []NameserverTiming {
	if len(queryTimings) == 0 && len(queryTimeouts) == 0 {
		return nil
	}

	targets := nameserverTimingTargets(job, DelegationInfo{})
	if len(targets) == 0 && s != nil && s.delegationLookup != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		targets = nameserverTimingTargets(job, s.delegationLookup(ctx, job.Domain))
	}
	if len(targets) == 0 {
		// Last-ditch fallback: pull the child zone's own NSes out of
		// the engine's own log entries. The engine already discovered
		// them during the test; the external delegation lookup can
		// then fail without silently dropping all timings. Fixes the
		// case where ~1.6% of TLDs had no timings because
		// lookupDelegation happened to hit a DNS hiccup at test time.
		targets = childNameserversFromEntries(entries)
	}
	return summarizeNameserverTimings(queryTimings, queryTimeouts, queryRefused, targets)
}

// childNameserversFromEntries collects (ns, address) pairs from engine
// log entries that unambiguously name the child zone's own authoritative
// NSes. Only the explicit child-side argument keys are trusted -
// generic `servers` / `parent_servers` lists are not, since they also
// carry parent-side data (root servers etc.) that would end up in the
// timings list otherwise.
func childNameserversFromEntries(entries []engine.LogEntry) []nameserverTimingTarget {
	if len(entries) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var targets []nameserverTimingTarget
	add := func(ns, addr string) {
		ns = normalizeNameserverName(ns)
		addr = strings.TrimSpace(addr)
		if ns == "" {
			return
		}
		key := ns + "/" + addr
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		targets = append(targets, nameserverTimingTarget{name: ns, address: addr})
	}
	for _, entry := range entries {
		for _, key := range []string{"child_servers", "zone_servers", "ns_set_servers"} {
			raw, ok := entry.Args[key]
			if !ok {
				continue
			}
			list, ok := raw.([]any)
			if !ok {
				// Some callers materialize the list as []map[string]any
				// instead of []any; handle that shape too.
				typed, ok := raw.([]map[string]any)
				if !ok {
					continue
				}
				for _, item := range typed {
					ns, _ := item["ns"].(string)
					addr, _ := item["address"].(string)
					add(ns, addr)
				}
				continue
			}
			for _, item := range list {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				ns, _ := m["ns"].(string)
				addr, _ := m["address"].(string)
				add(ns, addr)
			}
		}
	}
	return targets
}
