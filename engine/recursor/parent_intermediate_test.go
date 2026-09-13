package recursor

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// fqdn appends the root label when it is missing.
func fqdn(name string) string {
	if strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}

// referral builds a delegation response for zone to nsName at nsAddr.
func referral(zone string, nsName string, nsAddr string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	ns := &dns.NS{Hdr: dns.Header{Name: fqdn(zone), Class: dns.ClassINET}}
	ns.Ns = fqdn(nsName)
	msg.Ns = []dns.RR{ns}
	a := &dns.A{Hdr: dns.Header{Name: fqdn(nsName), Class: dns.ClassINET}}
	a.Addr = netip.MustParseAddr(nsAddr)
	msg.Extra = []dns.RR{a}
	return packet.Packet{Msg: msg}
}

// soaAt builds an authoritative SOA answer owned by zone.
func soaAt(zone string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	soa := &dns.SOA{Hdr: dns.Header{Name: fqdn(zone), Class: dns.ClassINET}}
	soa.Ns = "ns." + fqdn(zone)
	soa.Mbox = "hostmaster." + fqdn(zone)
	soa.Serial = 1
	soa.Refresh = 3600
	soa.Retry = 600
	soa.Expire = 1209600
	soa.Minttl = 300
	msg.Answer = []dns.RR{soa}
	return packet.Packet{Msg: msg}
}

// A zone between the last delegation and the name is invisible to the walk when
// one set of servers serves the parent, the intermediate zone and the name, so
// no referral is ever traversed. The intermediate SOA has to be asked of a
// server that holds it, not of the one that delegated towards it.
func TestParentFindsAnIntermediateZoneOnSharedServers(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	r := fakeRootRecursor(t, "a.root.test", "192.0.2.1")

	// The root delegates "test" to one server, which serves every zone below.
	hookedNS(t, ctx, r, "a.root.test", "192.0.2.1", func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name = dnsname.New(name).String()
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}, errors.New("unexpected qtype")
		}
		// The delegating server is not authoritative for anything below it.
		return referral("test", "ns.test", "192.0.2.2"), nil
	})

	var asked []string
	hookedNS(t, ctx, r, "ns.test", "192.0.2.2", func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name = dnsname.New(name).String()
		asked = append(asked, name)
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}, errors.New("unexpected qtype")
		}
		switch name {
		case "a.ns.test":
			// A zone of its own, loaded here and never delegated.
			return soaAt("a.ns.test"), nil
		case "ns.test":
			return soaAt("ns.test"), nil
		case "test":
			return soaAt("test"), nil
		}
		return packet.Packet{}, errors.New("unexpected name " + name)
	})

	got, _, err := r.Parent(ctx, "a.ns.test")
	if err != nil {
		t.Fatalf("Parent: %v", err)
	}
	if got != "ns.test" {
		t.Errorf("Parent(a.ns.test) = %q, want ns.test", got)
	}
	// The intermediate SOA must actually have been asked for.
	if !slicesContains(asked, "ns.test") {
		t.Errorf("the intermediate SOA was never asked for; asked %v", asked)
	}
}

// A name one label below the delegation needs no intermediate lookup.
func TestParentTakesTheDelegationWhenNoZoneIntervenes(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	r := fakeRootRecursor(t, "a.root.test", "192.0.2.1")

	hookedNS(t, ctx, r, "a.root.test", "192.0.2.1", func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}, errors.New("unexpected qtype")
		}
		return referral("test", "ns.test", "192.0.2.2"), nil
	})
	hookedNS(t, ctx, r, "ns.test", "192.0.2.2", func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
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

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// refused builds a REFUSED response.
func refused() packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeRefused
	return packet.Packet{Msg: msg}
}

// The name is delegated to servers of its own, so those servers hold nothing
// above it. The intermediate zone has to be asked of the server that delegated
// to the name.
func TestParentFindsAnIntermediateZoneAboveADelegatedName(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	r := fakeRootRecursor(t, "a.root.test", "192.0.2.1")

	hookedNS(t, ctx, r, "a.root.test", "192.0.2.1", func(_ context.Context, _ string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}, errors.New("unexpected qtype")
		}
		return referral("test", "ns.test", "192.0.2.2"), nil
	})

	// One server holds "test" and "sub.test" and delegates "child.sub.test".
	var asked []string
	hookedNS(t, ctx, r, "ns.test", "192.0.2.2", func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name = dnsname.New(name).String()
		asked = append(asked, name)
		if strings.ToUpper(qtype) != "SOA" {
			return packet.Packet{}, errors.New("unexpected qtype")
		}
		switch name {
		case "child.sub.test":
			return referral("child.sub.test", "ns.child.sub.test", "192.0.2.3"), nil
		case "sub.test", "test":
			return soaAt(name), nil
		}
		return packet.Packet{}, errors.New("unexpected name " + name)
	})

	// The delegated servers serve the name and refuse everything else.
	hookedNS(t, ctx, r, "ns.child.sub.test", "192.0.2.3", func(_ context.Context, name string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name = dnsname.New(name).String()
		if name != "child.sub.test" {
			return refused(), nil
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
	if !slicesContains(asked, "sub.test") {
		t.Errorf("the intermediate SOA was never asked of the delegating server; asked %v", asked)
	}
}
