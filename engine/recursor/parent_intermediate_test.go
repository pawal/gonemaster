package recursor

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// referral delegates zone to nsName with glue at nsAddr.
func referral(zone string, nsName string, nsAddr string) packet.Packet {
	return dnstest.Response(dnstest.NotAuthoritative(),
		dnstest.Authority(dnstest.TTL(0, dnstest.NSRR(zone, nsName))...),
		dnstest.Additional(dnstest.TTL(0, dnstest.ARR(nsName, nsAddr))...))
}

// soaAt is an authoritative SOA answer owned by zone.
func soaAt(zone string) packet.Packet {
	return dnstest.Response(dnstest.Answers(dnstest.TTL(0, dnstest.SOARR(zone))...))
}

// soaOnly rejects every question that is not SOA before calling hook.
func soaOnly(hook queryHook) queryHook {
	return func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		if !strings.EqualFold(qtype, "SOA") {
			return packet.Packet{}, errors.New("unexpected qtype")
		}
		return hook(ctx, name, qtype, qclass, opts)
	}
}

// One server serves the parent, the intermediate zone and the name, so no referral crosses the zone cut.
func TestParentFindsAnIntermediateZoneOnSharedServers(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	r := fakeRootRecursor(t, "a.root.test", "192.0.2.1")

	// The root delegates "test" to one server, which serves every zone below.
	hookedNS(t, ctx, r, "a.root.test", "192.0.2.1", soaOnly(func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return referral("test", "ns.test", "192.0.2.2"), nil
	}))

	// a.ns.test is a zone of its own that was never delegated.
	var asked []string
	hookedNS(t, ctx, r, "ns.test", "192.0.2.2", soaOnly(func(_ context.Context, name string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name = dnsname.New(name).String()
		asked = append(asked, name)
		switch name {
		case "a.ns.test", "ns.test", "test":
			return soaAt(name), nil
		}
		return packet.Packet{}, errors.New("unexpected name " + name)
	}))

	got, _, err := r.Parent(ctx, "a.ns.test")
	if err != nil {
		t.Fatalf("Parent: %v", err)
	}
	if got != "ns.test" {
		t.Errorf("Parent(a.ns.test) = %q, want ns.test", got)
	}
	if !slices.Contains(asked, "ns.test") {
		t.Errorf("the intermediate SOA was never asked for; asked %v", asked)
	}
}

// A name one label below the delegation needs no intermediate lookup.
func TestParentTakesTheDelegationWhenNoZoneIntervenes(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	r := fakeRootRecursor(t, "a.root.test", "192.0.2.1")

	hookedNS(t, ctx, r, "a.root.test", "192.0.2.1", soaOnly(func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return referral("test", "ns.test", "192.0.2.2"), nil
	}))
	hookedNS(t, ctx, r, "ns.test", "192.0.2.2", func(_ context.Context, name string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name = dnsname.New(name).String()
		if name != "child.test" && name != "test" {
			return packet.Packet{}, errors.New("unexpected name " + name)
		}
		return soaAt(name), nil
	})

	got, _, err := r.Parent(ctx, "child.test")
	if err != nil {
		t.Fatalf("Parent: %v", err)
	}
	if got != "test" {
		t.Errorf("Parent(child.test) = %q, want test", got)
	}
}

// The name is delegated to servers that hold nothing above it.
func TestParentFindsAnIntermediateZoneAboveADelegatedName(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	r := fakeRootRecursor(t, "a.root.test", "192.0.2.1")

	hookedNS(t, ctx, r, "a.root.test", "192.0.2.1", soaOnly(func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return referral("test", "ns.test", "192.0.2.2"), nil
	}))

	// One server holds "test" and "sub.test" and delegates "child.sub.test".
	var asked []string
	hookedNS(t, ctx, r, "ns.test", "192.0.2.2", soaOnly(func(_ context.Context, name string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name = dnsname.New(name).String()
		asked = append(asked, name)
		switch name {
		case "child.sub.test":
			return referral("child.sub.test", "ns.child.sub.test", "192.0.2.3"), nil
		case "sub.test", "test":
			return soaAt(name), nil
		}
		return packet.Packet{}, errors.New("unexpected name " + name)
	}))

	// The delegated servers serve the name and refuse everything else.
	hookedNS(t, ctx, r, "ns.child.sub.test", "192.0.2.3", func(_ context.Context, name string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name = dnsname.New(name).String()
		if name != "child.sub.test" {
			return refusedPacket(""), nil
		}
		return soaAt(name), nil
	})

	got, _, err := r.Parent(ctx, "child.sub.test")
	if err != nil {
		t.Fatalf("Parent: %v", err)
	}
	if got != "sub.test" {
		t.Errorf("Parent(child.sub.test) = %q, want sub.test", got)
	}
	if !slices.Contains(asked, "sub.test") {
		t.Errorf("the intermediate SOA was never asked of the delegating server; asked %v", asked)
	}
}
