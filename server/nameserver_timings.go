package server

import (
	"context"
	"sort"
	"strings"
	"time"

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

func summarizeNameserverTimings(queryTimings map[string][]time.Duration, targets []nameserverTimingTarget) []NameserverTiming {
	if len(queryTimings) == 0 || len(targets) == 0 {
		return nil
	}

	nameOnly := map[string]bool{}
	pairs := map[string]bool{}
	for _, target := range targets {
		if target.name == "" {
			continue
		}
		if target.address == "" {
			nameOnly[target.name] = true
			continue
		}
		pairs[target.name+"/"+target.address] = true
	}

	out := make([]NameserverTiming, 0, len(queryTimings))
	for key, samples := range queryTimings {
		name, address, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		name = normalizeNameserverName(name)
		address = strings.TrimSpace(address)
		if !pairs[name+"/"+address] && !nameOnly[name] {
			continue
		}
		stats := enginenameserver.ComputeTimingStats(samples)
		if stats.Count == 0 {
			continue
		}
		out = append(out, NameserverTiming{
			Nameserver: name,
			Address:    address,
			AvgMS:      stats.Avg,
			MinMS:      stats.Min,
			MaxMS:      stats.Max,
			MedianMS:   stats.Median,
			StddevMS:   stats.Stddev,
			Count:      stats.Count,
		})
	}

	sort.Slice(out, func(i, j int) bool {
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

func cloneNameserverTimings(items []NameserverTiming) []NameserverTiming {
	if len(items) == 0 {
		return nil
	}
	out := make([]NameserverTiming, len(items))
	copy(out, items)
	return out
}

func (s *Server) collectNameserverTimings(job Job, queryTimings map[string][]time.Duration) []NameserverTiming {
	if len(queryTimings) == 0 {
		return nil
	}

	targets := nameserverTimingTargets(job, DelegationInfo{})
	if len(targets) == 0 && s != nil && s.delegationLookup != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		targets = nameserverTimingTargets(job, s.delegationLookup(ctx, job.Domain))
	}
	return summarizeNameserverTimings(queryTimings, targets)
}
