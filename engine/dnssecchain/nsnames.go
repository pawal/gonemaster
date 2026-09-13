package dnssecchain

import (
	"context"
	"sort"
	"sync"
)

// NSName statuses, one per conclusion the run reaches about the signatures on
// the address records of an in-domain nameserver name.
const (
	NSNameValidates     = "validates"
	NSNameInsecure      = "insecure"
	NSNameUnsigned      = "unsigned"
	NSNameOrphan        = "orphan"
	NSNameChainBroken   = "chain_broken"
	NSNameRRSIGExpired  = "rrsig_expired"
	NSNameRRSIGInvalid  = "rrsig_invalid"
	NSNameIndeterminate = "indeterminate"
)

// nsNameRank orders the statuses so the worst observation across the
// nameservers is the one the document reports.
var nsNameRank = map[string]int{
	NSNameIndeterminate: 0,
	NSNameValidates:     1,
	NSNameInsecure:      2,
	NSNameRRSIGInvalid:  3,
	NSNameRRSIGExpired:  4,
	NSNameUnsigned:      5,
	NSNameChainBroken:   6,
	NSNameOrphan:        7,
}

// NSName is one in-domain nameserver name of the zone and what the run
// concluded about the signatures on its address records. Signer is the
// Signer's Name of the covering RRSIG where one was seen, or the zone cut whose
// chain is broken. Servers are the nameservers that showed the reported status.
type NSName struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Signer  string   `json:"signer,omitempty"`
	KeyTag  uint16   `json:"key_tag,omitempty"`
	Servers []string `json:"servers"`
}

// nsNameCollector gathers per-name observations for one run, keeping the worst
// status seen for each name. It also carries the run's undelegated verdict,
// which is a statement about the zone itself rather than about a name in it.
type nsNameCollector struct {
	mu          sync.Mutex
	byName      map[string]*NSName
	undelegated string
}

type nsNamesKey struct{}

// WithNSNames installs a per-run collector in ctx. Without it CollectNSName is
// a no-op and the summary carries no ns_names section.
func WithNSNames(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, nsNamesKey{}, &nsNameCollector{byName: map[string]*NSName{}})
}

// collectorFromContext returns the collector stored in ctx, or nil.
func collectorFromContext(ctx context.Context) *nsNameCollector {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(nsNamesKey{}).(*nsNameCollector)
	return c
}

// CollectNSName records one nameserver's observation of one name. A worse
// status replaces what was seen before; an equally ranked one merges its
// servers into the entry.
func CollectNSName(ctx context.Context, obs NSName) {
	c := collectorFromContext(ctx)
	if c == nil || obs.Name == "" || obs.Status == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cur, ok := c.byName[obs.Name]
	if !ok {
		entry := obs
		entry.Servers = append([]string(nil), obs.Servers...)
		c.byName[obs.Name] = &entry
		return
	}
	switch {
	case nsNameRank[obs.Status] > nsNameRank[cur.Status]:
		entry := obs
		entry.Servers = append([]string(nil), obs.Servers...)
		*cur = entry
	case obs.Status == cur.Status:
		cur.Servers = append(cur.Servers, obs.Servers...)
		if cur.Signer == "" {
			cur.Signer = obs.Signer
		}
		if cur.KeyTag == 0 {
			cur.KeyTag = obs.KeyTag
		}
	}
}

// CollectUndelegated records that the parent zone proves no delegation exists
// at the zone under test. parent is the zone that carries the proof.
func CollectUndelegated(ctx context.Context, parent string) {
	c := collectorFromContext(ctx)
	if c == nil || parent == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.undelegated = parent
}

// UndelegatedFromContext returns the parent zone that proved no delegation, or
// the empty string.
func UndelegatedFromContext(ctx context.Context) string {
	c := collectorFromContext(ctx)
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.undelegated
}

// NSNamesFromContext returns the collected names sorted by name, with each
// entry's servers deduplicated and sorted. It returns nil when no collector is
// installed or nothing was collected.
func NSNamesFromContext(ctx context.Context) []NSName {
	c := collectorFromContext(ctx)
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.byName) == 0 {
		return nil
	}
	out := make([]NSName, 0, len(c.byName))
	for _, entry := range c.byName {
		item := *entry
		item.Servers = sortUnique(item.Servers)
		if item.Servers == nil {
			item.Servers = []string{}
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
