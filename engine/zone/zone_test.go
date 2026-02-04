package zone

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
)

func newHookedNameserver(ctx context.Context, t *testing.T, name string, addr string, hook func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error)) nameserver.Nameserver {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, addr, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(hook)
	return ns
}

func TestZoneQueryOneSkipsDisabledIP(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = false
	prof.Net.IPv6 = true

	ns4, err := nameserver.NewWithContext(ctx, "ns4.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns6, err := nameserver.NewWithContext(ctx, "ns6.example", "2001:db8::1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	var calls4 int
	ns4.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		calls4++
		return packet.Packet{}, nil
	})
	ns6.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "example.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: net.IPv4(192, 0, 2, 9),
			},
		}
		return packet.Packet{Msg: msg}, nil
	})

	z := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{ns4, ns6},
		nsSet: true,
	}

	resp, err := z.QueryOne(ctx, "example", "A", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if resp.Msg == nil {
		t.Fatalf("expected response from IPv6 nameserver")
	}
	if calls4 != 0 {
		t.Fatalf("expected IPv4 nameserver skipped, got %d calls", calls4)
	}
}

func TestZoneQueryAllParallel(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	prof.Resolver.Defaults.Parallel = 2
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, _ string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qtype != "DNSKEY" {
				return packet.Packet{}, nil
			}
			select {
			case started <- id:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			msg := new(dns.Msg)
			msg.SetQuestion("example.", dns.TypeDNSKEY)
			msg.Response = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg, AnswerFrom: id}, nil
		}
	}

	ns1 := newHookedNameserver(baseCtx, t, "ns1.example", "192.0.2.50", hook("ns1"))
	ns2 := newHookedNameserver(baseCtx, t, "ns2.example", "192.0.2.51", hook("ns2"))

	z := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{ns1, ns2},
		nsSet: true,
	}

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var res []packet.Packet
	var qerr error
	go func() {
		res, qerr = z.QueryAll(ctx, "example", "DNSKEY", nil)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel DNSKEY queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if qerr != nil {
			t.Fatalf("queryall: %v", qerr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("queryall did not finish")
	}

	if len(res) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(res))
	}
	if res[0].AnswerFrom != "ns1" || res[1].AnswerFrom != "ns2" {
		t.Fatalf("expected ordered responses, got %q and %q", res[0].AnswerFrom, res[1].AnswerFrom)
	}
}

func TestZoneGlueUsesFakeAddresses(t *testing.T) {
	r, err := recursor.New()
	if err != nil {
		t.Fatalf("new recursor: %v", err)
	}
	err = r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.55"},
	})
	if err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	z := Zone{
		Name:         dnsname.New("example"),
		recursor:     r,
		glueNames:    []dnsname.Name{dnsname.New("ns1.example")},
		glueNamesSet: true,
	}

	glue, err := z.Glue(context.Background())
	if err != nil {
		t.Fatalf("glue: %v", err)
	}
	if len(glue) != 1 {
		t.Fatalf("expected 1 glue nameserver, got %d", len(glue))
	}
	if glue[0].Address.String() != "192.0.2.55" {
		t.Fatalf("unexpected glue address %s", glue[0].Address.String())
	}
}

func TestZoneNewWithRecursorEmptyName(t *testing.T) {
	if _, err := NewWithRecursor("", nil); err == nil {
		t.Fatalf("expected error for empty zone name")
	}
}

func TestZoneParentRoot(t *testing.T) {
	z := Zone{Name: dnsname.New(".")}
	parent, err := z.Parent(context.Background())
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if parent != &z {
		t.Fatalf("expected parent to be root zone")
	}
}

func TestZoneParentMissingRecursor(t *testing.T) {
	z := Zone{Name: dnsname.New("example")}
	if _, err := z.Parent(context.Background()); err == nil {
		t.Fatalf("expected error for missing recursor")
	}
}

func TestZoneGlueNamesFromParent(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	parentNS := newHookedNameserver(context.Background(), t, "ns.parent.example", "192.0.2.10", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.NS{
				Hdr: dns.RR_Header{
					Name:   "child.example.",
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
				},
				Ns: "NS1.Child.Example.",
			},
			&dns.NS{
				Hdr: dns.RR_Header{
					Name:   "child.example.",
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
				},
				Ns: "ns2.child.example.",
			},
			&dns.NS{
				Hdr: dns.RR_Header{
					Name:   "child.example.",
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
				},
				Ns: "ns1.child.example.",
			},
		}
		return packet.Packet{Msg: msg}, nil
	})

	parent := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{parentNS},
		nsSet: true,
	}
	z := Zone{
		Name:      dnsname.New("child.example"),
		parent:    &parent,
		parentSet: true,
	}

	names, err := z.GlueNames(context.Background())
	if err != nil {
		t.Fatalf("glue names: %v", err)
	}
	want := []string{"ns1.child.example", "ns2.child.example"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d", len(want), len(names))
	}
	for i, name := range names {
		if name.String() != want[i] {
			t.Fatalf("unexpected name %q", name.String())
		}
	}
}

func TestZoneGlueAddressesFromParent(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	parentNS := newHookedNameserver(context.Background(), t, "ns.parent.example", "192.0.2.11", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Extra = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "ns1.child.example.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
				},
				A: net.IPv4(192, 0, 2, 100),
			},
			&dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   "ns1.child.example.",
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
				},
				AAAA: net.ParseIP("2001:db8::100"),
			},
		}
		return packet.Packet{Msg: msg}, nil
	})

	parent := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{parentNS},
		nsSet: true,
	}
	z := Zone{
		Name:      dnsname.New("child.example"),
		parent:    &parent,
		parentSet: true,
	}

	records, err := z.GlueAddresses(context.Background())
	if err != nil {
		t.Fatalf("glue addresses: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 glue records, got %d", len(records))
	}
}

func TestZoneNSNamesRootSorted(t *testing.T) {
	r, err := recursor.New()
	if err != nil {
		t.Fatalf("new recursor: %v", err)
	}
	z := Zone{Name: dnsname.New("."), recursor: r}

	names, err := z.NSNames(context.Background())
	if err != nil {
		t.Fatalf("ns names: %v", err)
	}
	if len(names) == 0 {
		t.Fatalf("expected root names")
	}
	for i := 1; i < len(names); i++ {
		prev := strings.ToLower(names[i-1].String())
		curr := strings.ToLower(names[i].String())
		if prev > curr {
			t.Fatalf("expected names sorted, got %q then %q", prev, curr)
		}
	}
}

func TestZoneQueryPersistentSelectsAnswer(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	ns1 := newHookedNameserver(context.Background(), t, "ns1.example", "192.0.2.20", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.NS{
				Hdr: dns.RR_Header{
					Name:   "other.example.",
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
				},
				Ns: "ns.other.example.",
			},
		}
		return packet.Packet{Msg: msg}, nil
	})
	ns2 := newHookedNameserver(context.Background(), t, "ns2.example", "192.0.2.21", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.NS{
				Hdr: dns.RR_Header{
					Name:   "example.",
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
				},
				Ns: "ns2.example.",
			},
		}
		return packet.Packet{Msg: msg}, nil
	})

	z := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{ns1, ns2},
		nsSet: true,
	}

	resp, err := z.QueryPersistent(context.Background(), "example", "NS", nil)
	if err != nil {
		t.Fatalf("query persistent: %v", err)
	}
	if resp.Msg == nil {
		t.Fatalf("expected response")
	}
	if len(resp.GetRecordsForName("NS", dnsname.New("example"), "answer")) == 0 {
		t.Fatalf("expected NS record for example")
	}
}

func TestZoneQueryPersistentAcceptsAuthority(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	ns := newHookedNameserver(context.Background(), t, "ns1.example", "192.0.2.31", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Ns = []dns.RR{
			&dns.NS{
				Hdr: dns.RR_Header{
					Name:   "child.example.",
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
				},
				Ns: "ns1.child.example.",
			},
		}
		return packet.Packet{Msg: msg}, nil
	})

	z := Zone{
		Name:  dnsname.New("child.example"),
		ns:    []nameserver.Nameserver{ns},
		nsSet: true,
	}

	resp, err := z.QueryPersistent(context.Background(), "child.example", "NS", nil)
	if err != nil {
		t.Fatalf("query persistent: %v", err)
	}
	if resp.Msg == nil {
		t.Fatalf("expected response")
	}
	if len(resp.GetRecordsForName("NS", dnsname.New("child.example"), "authority")) == 0 {
		t.Fatalf("expected NS record in authority section")
	}
}

func TestZoneIsInZone(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	ns := newHookedNameserver(context.Background(), t, "ns1.example", "192.0.2.30", func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Authoritative = true
		msg.Answer = []dns.RR{
			&dns.SOA{
				Hdr: dns.RR_Header{
					Name:   "example.",
					Rrtype: dns.TypeSOA,
					Class:  dns.ClassINET,
				},
				Ns:      "ns1.example.",
				Mbox:    "hostmaster.example.",
				Serial:  1,
				Refresh: 3600,
				Retry:   600,
				Expire:  86400,
				Minttl:  300,
			},
		}
		return packet.Packet{Msg: msg}, nil
	})

	z := Zone{
		Name:  dnsname.New("example"),
		ns:    []nameserver.Nameserver{ns},
		nsSet: true,
	}

	inZone, err := z.IsInZone(context.Background(), "www.example")
	if err != nil {
		t.Fatalf("is in zone: %v", err)
	}
	if !inZone {
		t.Fatalf("expected name to be in zone")
	}
}
