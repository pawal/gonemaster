package tctest

import (
	"context"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/transport"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// rootHints is the single fake root nameserver RootZone serves.
var rootHints = map[string][]string{"a.root": {"192.0.2.1"}}

// Query is one query as a test nameserver handler sees it.
type Query struct {
	Name  string
	Type  string
	Class string
	Opts  *nameserver.QueryOptions
}

// Handler answers one query from a test nameserver.
type Handler func(Query) packet.Packet

// RawHandler is the nameserver query hook signature, for handlers that need the
// query context or must fail with an error.
type RawHandler func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error)

// Setup replaces the global logger for the test and resets the effective
// profile afterwards. Use it when the test builds its own context.
func Setup(t TB) {
	t.Helper()
	t.Cleanup(profile.ResetEffective)
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })
}

// Context returns a context carrying a fresh nameserver cache, set up as Setup.
func Context(t TB) context.Context {
	t.Helper()
	Setup(t)
	return nameserver.WithCache(context.Background(), nameserver.NewCacheStore())
}

// NS returns a nameserver answering every query through handler. A nil handler
// answers with an empty packet.
func NS(t TB, ctx context.Context, name string, ip string, handler Handler) nameserver.Nameserver {
	t.Helper()
	return newNS(t, ctx, name, ip, nil, handler)
}

// Recursor returns a recursor resolving the given fake addresses, keyed by
// domain and then by nameserver name.
func Recursor(t TB, fakes map[string]map[string][]string) *recursor.Recursor {
	t.Helper()
	r := &recursor.Recursor{}
	for domain, data := range fakes {
		if err := r.AddFakeAddresses(domain, data); err != nil {
			t.Fatalf("add fake addresses for %s: %v", domain, err)
		}
	}
	return r
}

// NSOn returns a nameserver on the recursor's client, answering via handler.
func NSOn(t TB, ctx context.Context, r *recursor.Recursor, name string, ip string, handler Handler) nameserver.Nameserver {
	t.Helper()
	return newNS(t, ctx, name, ip, r.Client(), handler)
}

// NSRaw returns a nameserver on the recursor's client, answering via a raw
// query hook.
func NSRaw(t TB, ctx context.Context, r *recursor.Recursor, name string, ip string, hook RawHandler) nameserver.Nameserver {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, ip, r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(hook)
	return ns
}

// Zone returns a zone served by the recursor.
func Zone(t TB, name string, r *recursor.Recursor) *zone.Zone {
	t.Helper()
	z, err := zone.NewWithRecursor(name, r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	return &z
}

// RootZone returns the root zone served by a single fake nameserver answering
// through handler.
func RootZone(t TB, ctx context.Context, handler Handler) *zone.Zone {
	t.Helper()
	r := Recursor(t, map[string]map[string][]string{".": rootHints})
	NSOn(t, ctx, r, "a.root", "192.0.2.1", handler)
	return Zone(t, ".", r)
}

// ZoneWithAddrs returns a zone whose recursor resolves the given fake addresses.
func ZoneWithAddrs(t TB, name string, data map[string][]string) *zone.Zone {
	t.Helper()
	return Zone(t, name, Recursor(t, map[string]map[string][]string{name: data}))
}

// Stub replaces a package seam for the duration of the test.
func Stub[T any](t TB, target *T, value T) {
	t.Helper()
	orig := *target
	t.Cleanup(func() { *target = orig })
	*target = value
}

func newNS(t TB, ctx context.Context, name string, ip string, client *transport.Client, handler Handler) nameserver.Nameserver {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, ip, client)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if handler == nil {
		handler = func(Query) packet.Packet { return packet.Packet{} }
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		return handler(Query{Name: qname, Type: qtype, Class: qclass, Opts: opts}), nil
	})
	return ns
}
