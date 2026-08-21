package nstest

import (
	"context"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

// Hook is the nameserver query-hook signature.
type Hook func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error)

// Recursor returns a recursor resolving the given fake addresses, keyed by zone
// and then by nameserver name.
func Recursor(t testing.TB, fakes map[string]map[string][]string) *recursor.Recursor {
	t.Helper()
	r := &recursor.Recursor{}
	for zoneName, data := range fakes {
		if err := r.AddFakeAddresses(zoneName, data); err != nil {
			t.Fatalf("add fake addresses for %s: %v", zoneName, err)
		}
	}
	return r
}

// RootRecursor returns a recursor with fake addresses at the root only.
func RootRecursor(t testing.TB, data map[string][]string) *recursor.Recursor {
	t.Helper()
	return Recursor(t, map[string]map[string][]string{".": data})
}

// NS returns a nameserver on the recursor's client. A nil recursor gives a
// nameserver with no client.
func NS(t testing.TB, ctx context.Context, r *recursor.Recursor, name string, addr string) nameserver.Nameserver {
	t.Helper()
	var client = nameserverClient(r)
	ns, err := nameserver.NewWithContext(ctx, name, addr, client)
	if err != nil {
		t.Fatalf("new nameserver %s: %v", name, err)
	}
	return ns
}

// HookedNS returns a nameserver on the recursor's client answering via hook.
func HookedNS(t testing.TB, ctx context.Context, r *recursor.Recursor, name string, addr string, hook Hook) nameserver.Nameserver {
	t.Helper()
	ns := NS(t, ctx, r, name, addr)
	ns.SetQueryHook(hook)
	return ns
}

// PacketHook answers every query with p.
func PacketHook(p packet.Packet) Hook {
	return func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return p, nil
	}
}

// AnswerHook answers queries for qtype at zoneName with p; anything else gets
// an empty packet, as a server that holds no such data would.
func AnswerHook(zoneName string, qtype string, p packet.Packet) Hook {
	return func(_ context.Context, name string, gotType string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if !strings.EqualFold(name, zoneName) || !strings.EqualFold(gotType, qtype) {
			return packet.Packet{}, nil
		}
		return p, nil
	}
}

// RandomNameSet returns n pseudo-random label names under suffix. Names may
// repeat and mix case, so callers can check deduplication and case folding.
func RandomNameSet(rng *rand.Rand, n int, suffix string) []string {
	letters := []byte("abcdefghijklmnopqrstuvwxyz")
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		label := make([]byte, 1+rng.Intn(4))
		for j := range label {
			c := letters[rng.Intn(len(letters))]
			if rng.Intn(10) < 3 {
				c -= 32
			}
			label[j] = c
		}
		out = append(out, string(label)+"."+suffix)
	}
	return out
}

// IsSortedLowercase reports whether names are sorted by their lowercase form.
func IsSortedLowercase(names []string) bool {
	cmp := make([]string, len(names))
	for i, name := range names {
		cmp[i] = strings.ToLower(name)
	}
	return slices.IsSorted(cmp)
}

// HasNoDuplicates reports whether names are unique, case-insensitively.
func HasNoDuplicates(names []string) bool {
	seen := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

// nameserverClient returns the recursor's client, or nil for a nil recursor.
func nameserverClient(r *recursor.Recursor) *transport.Client {
	if r == nil {
		return nil
	}
	return r.Client()
}
