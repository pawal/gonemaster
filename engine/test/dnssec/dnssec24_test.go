package dnssec

import (
	"context"
	"crypto"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// ds24Zone is one signed zone of the fake tree.
type ds24Zone struct {
	name   dnsname.Name
	key    *dns.DNSKEY
	signer crypto.Signer
	rrsets map[string][]dns.RR
	cuts   map[string]ds24Cut
}

// ds24Cut is one delegation with its NS RRset, glue and signed DS RRset.
type ds24Cut struct {
	name dnsname.Name
	ns   []dns.RR
	glue []dns.RR
	ds   []dns.RR
}

type ds24Mode int

const (
	ds24Signed ds24Mode = iota
	ds24NoDS
	ds24WrongDS
	ds24BadDSSig
)

// ds24World is a fake DNS tree below a root on the recursor's hints.
type ds24World struct {
	t       *testing.T
	ctx     context.Context
	rec     *recursor.Recursor
	zones   map[string]*ds24Zone
	servers map[string]nameserver.Nameserver
	mu      sync.Mutex
	counts  map[string]int
}

func newDS24World(t *testing.T) *ds24World {
	t.Helper()
	w := &ds24World{t: t, ctx: tctest.Context(t), zones: map[string]*ds24Zone{},
		servers: map[string]nameserver.Nameserver{}, counts: map[string]int{}}
	w.rec = tctest.Recursor(t, map[string]map[string][]string{".": {"a.root": {"192.0.2.1"}}})
	root := w.zone(".")
	anchor := ds24DS(t, ".", root.key)
	tctest.Stub(t, &rootTrustAnchors, func() []*dns.DS { return []*dns.DS{anchor} })
	return w
}

// ds24DS returns the SHA-256 DS of key at owner.
func ds24DS(t *testing.T, owner string, key *dns.DNSKEY) *dns.DS {
	t.Helper()
	ds := key.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatal("ToDS returned nil")
	}
	ds.Hdr = dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}
	return ds
}

// ds24CDS returns the CDS of key at owner.
func ds24CDS(t *testing.T, owner string, key *dns.DNSKEY) *dns.CDS {
	t.Helper()
	return &dns.CDS{DS: *ds24DS(t, owner, key)}
}

// ds24CDNSKEY returns the CDNSKEY of key at owner.
func ds24CDNSKEY(owner string, key *dns.DNSKEY) *dns.CDNSKEY {
	rr := &dns.CDNSKEY{DNSKEY: *key}
	rr.Hdr = dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}
	return rr
}

// zone creates a zone with a fresh key and publishes its signed DNSKEY RRset.
func (w *ds24World) zone(name string) *ds24Zone {
	w.t.Helper()
	key, signer := tctest.SignedKey(w.t, name, dns.ECDSAP256SHA256, tctest.SEP())
	z := &ds24Zone{name: dnsname.New(name), key: key, signer: signer,
		rrsets: map[string][]dns.RR{}, cuts: map[string]ds24Cut{}}
	w.zones[z.name.StringLower()] = z
	w.add(name, key)
	return z
}

// add publishes one RRset in zone, signed with its key.
func (w *ds24World) add(zoneName string, rrs ...dns.RR) {
	w.t.Helper()
	z := w.zones[dnsname.New(zoneName).StringLower()]
	w.put(zoneName, append(rrs, tctest.Sign(w.t, z.key, z.signer, dns.RRToType(rrs[0]), rrs))...)
}

// put publishes records in zone as given.
func (w *ds24World) put(zoneName string, rrs ...dns.RR) {
	z := w.zones[dnsname.New(zoneName).StringLower()]
	key := ds22Key(rrs[0].Header().Name, dns.TypeToString[dns.RRToType(rrs[0])])
	z.rrsets[key] = append(z.rrsets[key], rrs...)
}

// delegate publishes in parent a referral to child via "name/ip" servers, with glue below parent.
func (w *ds24World) delegate(parent string, child string, mode ds24Mode, servers ...string) {
	w.t.Helper()
	p := w.zones[dnsname.New(parent).StringLower()]
	cut := ds24Cut{name: dnsname.New(child)}
	for _, spec := range servers {
		name, ip, _ := strings.Cut(spec, "/")
		cut.ns = append(cut.ns, tctest.NSRR(child, name))
		if ip != "" && p.name.IsInBailiwick(dnsname.New(name)) {
			cut.glue = append(cut.glue, tctest.ARR(name, ip))
		}
	}
	signKey, signer := p.key, p.signer
	var ds *dns.DS
	switch mode {
	case ds24NoDS:
	case ds24WrongDS:
		other, _ := tctest.SignedKey(w.t, child, dns.ECDSAP256SHA256, tctest.SEP())
		ds = ds24DS(w.t, child, other)
	case ds24BadDSSig:
		signKey, signer = tctest.SignedKey(w.t, parent, dns.ECDSAP256SHA256, tctest.SEP())
		fallthrough
	default:
		ds = ds24DS(w.t, child, w.zones[cut.name.StringLower()].key)
	}
	if ds != nil {
		cut.ds = []dns.RR{ds, tctest.Sign(w.t, signKey, signer, dns.TypeDS, []dns.RR{ds})}
	}
	p.cuts[cut.name.StringLower()] = cut
}

// serve starts nameserver name/ip answering for the given zones.
func (w *ds24World) serve(name string, ip string, zones ...string) {
	w.t.Helper()
	var hosted []*ds24Zone
	for _, zoneName := range zones {
		hosted = append(hosted, w.zones[dnsname.New(zoneName).StringLower()])
	}
	w.serveWith(name, ip, func(q tctest.Query) packet.Packet { return ds24Answer(hosted, q) })
}

// serveWith starts nameserver name/ip answering through handler, counting questions.
func (w *ds24World) serveWith(name string, ip string, handler tctest.Handler) {
	w.t.Helper()
	w.servers[name+"/"+ip] = tctest.NSOn(w.t, w.ctx, w.rec, name, ip, func(q tctest.Query) packet.Packet {
		w.mu.Lock()
		w.counts[ip+" "+ds22Key(q.Name, q.Type)]++
		w.mu.Unlock()
		return handler(q)
	})
}

// asked returns how often ip was asked name/qtype.
func (w *ds24World) asked(ip string, name string, qtype string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.counts[ip+" "+ds22Key(name, qtype)]
}

// askedType returns how often any server was asked qtype.
func (w *ds24World) askedType(qtype string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	total := 0
	for key, n := range w.counts {
		if strings.HasSuffix(key, "/"+qtype) {
			total += n
		}
	}
	return total
}

// ds24Answer answers q as an authoritative server for hosted.
func ds24Answer(hosted []*ds24Zone, q tctest.Query) packet.Packet {
	qname := dnsname.New(q.Name)
	question := tctest.Question(q.Name, dns.StringToType[q.Type])
	var z *ds24Zone
	for _, candidate := range hosted {
		// The parent side answers DS for a zone apex.
		if !candidate.name.IsInBailiwick(qname) || (q.Type == "DS" && len(qname.Labels()) > 0 && candidate.name.Compare(qname) == 0) {
			continue
		}
		if z == nil || len(candidate.name.Labels()) > len(z.name.Labels()) {
			z = candidate
		}
	}
	if z == nil {
		return tctest.Response(question, tctest.Secure(), tctest.NotAuthoritative(), tctest.Rcode(dns.RcodeRefused))
	}
	var cut *ds24Cut
	for _, c := range z.cuts {
		if c.name.IsInBailiwick(qname) && (cut == nil || len(c.name.Labels()) > len(cut.name.Labels())) {
			cut = &c
		}
	}
	if cut != nil && (q.Type != "DS" || cut.name.Compare(qname) != 0) {
		return tctest.Response(question, tctest.Secure(), tctest.NotAuthoritative(),
			tctest.Authority(append(slices.Clone(cut.ns), cut.ds...)...), tctest.Additional(cut.glue...))
	}
	if cut != nil {
		return tctest.Response(question, tctest.Secure(), tctest.Answers(cut.ds...))
	}
	if rrs, ok := z.rrsets[ds22Key(q.Name, q.Type)]; ok {
		return tctest.Response(question, tctest.Secure(), tctest.Answers(rrs...))
	}
	for key := range z.rrsets {
		owner, _, _ := strings.Cut(key, "/")
		if qname.IsInBailiwick(dnsname.New(owner)) {
			return tctest.Response(question, tctest.Secure())
		}
	}
	return tctest.Response(question, tctest.Secure(), tctest.NXDOMAIN())
}

const (
	ds24TLDIP = "192.0.2.2"
	ds24NS1IP = "192.0.2.11"
	ds24NS2IP = "192.0.2.12"
	ds24Child = "child.test"
	ds24NS1   = "ns1.op.test"
	ds24NS2   = "ns2.op.test"
	ds24Owner = "_dsboot.child.test._signal.ns1.op.test"
	ds24Sig   = "sig.op.test"
	ds24SigIP = "192.0.2.21"
)

// ds24Op is the tree root, test, op.test, with child.test unsecured on the op.test servers.
type ds24Op struct {
	*ds24World
	cds      *dns.CDS
	cdnskey  *dns.CDNSKEY
	sigZones []string
}

func newDS24Op(t *testing.T) *ds24Op {
	t.Helper()
	w := newDS24World(t)
	w.zone("test")
	w.delegate(".", "test", ds24Signed, "ns.test/"+ds24TLDIP)
	w.zone("op.test")
	w.delegate("test", "op.test", ds24Signed, ds24NS1+"/"+ds24NS1IP, ds24NS2+"/"+ds24NS2IP)
	child := w.zone(ds24Child)
	w.delegate("test", ds24Child, ds24NoDS, ds24NS1+"/"+ds24NS1IP, ds24NS2+"/"+ds24NS2IP)
	w.add(ds24Child, tctest.SOARR(ds24Child))
	o := &ds24Op{ds24World: w, cds: ds24CDS(t, ds24Child, child.key), cdnskey: ds24CDNSKEY(ds24Child, child.key)}
	w.add(ds24Child, o.cds)
	w.add(ds24Child, o.cdnskey)
	return o
}

// signalZone creates the _signal zone of host, delegated from op.test to sig.op.test.
func (o *ds24Op) signalZone(host string, mode ds24Mode) string {
	o.t.Helper()
	name := "_signal." + host
	o.zone(name)
	o.delegate("op.test", name, mode, ds24Sig+"/"+ds24SigIP)
	o.sigZones = append(o.sigZones, name)
	return name
}

// signal copublishes the apex CDS and CDNSKEY under host in zoneName.
func (o *ds24Op) signal(zoneName string, host string) {
	o.t.Helper()
	owner := "_dsboot." + ds24Child + "._signal." + host
	o.add(zoneName, ds24CDS(o.t, owner, o.zones[ds24Child].key))
	o.add(zoneName, ds24CDNSKEY(owner, o.zones[ds24Child].key))
}

// start serves the root, the TLD and the op.test servers with extra zones.
func (o *ds24Op) start(extra ...string) {
	o.t.Helper()
	o.serve("a.root", "192.0.2.1", ".")
	o.serve("ns.test", ds24TLDIP, "test")
	zones := append([]string{"op.test", ds24Child}, extra...)
	o.serve(ds24NS1, ds24NS1IP, zones...)
	o.serve(ds24NS2, ds24NS2IP, zones...)
	if len(o.sigZones) > 0 {
		o.serve(ds24Sig, ds24SigIP, o.sigZones...)
	}
}

// evaluate walks the signaling name of ns1.op.test against the child's apex.
func (o *ds24Op) evaluate(w *ds24Walker) ds24Domain {
	o.t.Helper()
	apex := ds24Apex{dns.TypeCDS: {{o.cds}}, dns.TypeCDNSKEY: {{o.cdnskey}}}
	return w.evaluate(o.ctx, dnsname.New(ds24Owner), apex)
}

func (o *ds24Op) walker() *ds24Walker {
	return newDS24Walker(o.ctx, o.rec, rootTrustAnchors())
}

func TestDNSSEC24SignalName(t *testing.T) {
	name, ok := ds24SignalName(dnsname.New("Example.CO.uk"), dnsname.New("NS1.example.net"))
	if !ok || name.String() != "_dsboot.example.co.uk._signal.ns1.example.net" {
		t.Fatalf("signal name = %q, %v", name.String(), ok)
	}
	// Wire length 1 + 8 + 2 x 64 + 8 + 64 + (n + 1) + 2: 255 octets at n = 43.
	label := strings.Repeat("b", 63)
	for _, tc := range []struct {
		middle int
		ok     bool
	}{{43, true}, {44, false}} {
		host := dnsname.New(label + "." + strings.Repeat("c", tc.middle) + ".x")
		if _, ok := ds24SignalName(dnsname.New(label+"."+label), host); ok != tc.ok {
			t.Errorf("middle label of %d octets: ok = %v, want %v", tc.middle, ok, tc.ok)
		}
	}
}

func TestDNSSEC24WalkerValidatesThroughReferrals(t *testing.T) {
	o := newDS24Op(t)
	zoneName := o.signalZone(ds24NS1, ds24Signed)
	o.signal(zoneName, ds24NS1)
	o.start()

	d := o.evaluate(o.walker())
	if !d.validated || d.zone != zoneName || d.fault != nil {
		t.Fatalf("domain = %+v, want validated in %s", d, zoneName)
	}
}

// One server serves op.test and the signaling zone, so no referral marks the cut.
func TestDNSSEC24WalkerLinksASignerBelowTheAnsweringCut(t *testing.T) {
	o := newDS24Op(t)
	zoneName := o.signalZone(ds24NS1, ds24Signed)
	o.signal(zoneName, ds24NS1)
	o.start(zoneName)

	d := o.evaluate(o.walker())
	if !d.validated || d.zone != zoneName {
		t.Fatalf("domain = %+v, want validated in %s", d, zoneName)
	}
	if got := o.asked(ds24NS1IP, zoneName, "DS"); got != 1 {
		t.Fatalf("%s DS questions = %d, want 1", zoneName, got)
	}
}

// Signals inside the operator zone, without a _signal cut.
func TestDNSSEC24WalkerValidatesInTheOperatorZone(t *testing.T) {
	o := newDS24Op(t)
	o.signal("op.test", ds24NS1)
	o.start()

	d := o.evaluate(o.walker())
	if !d.validated || d.zone != "op.test" {
		t.Fatalf("domain = %+v, want validated in op.test", d)
	}
}

func TestDNSSEC24WalkerChainFaults(t *testing.T) {
	cases := []struct {
		name   string
		mode   ds24Mode
		status ds24Status
	}{
		{"no DS", ds24NoDS, ds24Insecure},
		{"DS matching no DNSKEY", ds24WrongDS, ds24Broken},
		{"DS RRSIG by another key", ds24BadDSSig, ds24Broken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newDS24Op(t)
			zoneName := o.signalZone(ds24NS1, tc.mode)
			o.signal(zoneName, ds24NS1)
			o.start()

			d := o.evaluate(o.walker())
			if d.fault == nil || d.fault.status != tc.status || d.fault.zone != zoneName {
				t.Fatalf("fault = %+v, want status %d at %s", d.fault, tc.status, zoneName)
			}
		})
	}
}

func TestDNSSEC24WalkerDNSKEYSignedByAnotherKey(t *testing.T) {
	o := newDS24Op(t)
	zoneName := o.signalZone(ds24NS1, ds24Signed)
	o.signal(zoneName, ds24NS1)
	z := o.zones[zoneName]
	other, otherSigner := tctest.SignedKey(t, zoneName, dns.ECDSAP256SHA256, tctest.SEP())
	z.rrsets[ds22Key(zoneName, "DNSKEY")] = []dns.RR{z.key, tctest.Sign(t, other, otherSigner, dns.TypeDNSKEY, []dns.RR{z.key})}
	o.start()

	d := o.evaluate(o.walker())
	if d.fault == nil || d.fault.status != ds24Broken || d.fault.zone != zoneName {
		t.Fatalf("fault = %+v, want broken at %s", d.fault, zoneName)
	}
}

func TestDNSSEC24WalkerRootMatchesNoAnchor(t *testing.T) {
	o := newDS24Op(t)
	o.signal("op.test", ds24NS1)
	o.start()
	other, _ := tctest.SignedKey(t, ".", dns.ECDSAP256SHA256, tctest.SEP())

	d := o.evaluate(newDS24Walker(o.ctx, o.rec, []*dns.DS{ds24DS(t, ".", other)}))
	if d.fault == nil || d.fault.status != ds24Broken || d.fault.zone != "." {
		t.Fatalf("fault = %+v, want broken at the root", d.fault)
	}
}

func TestDNSSEC24WalkerUnsignedSignal(t *testing.T) {
	cases := []struct {
		name    string
		publish func(o *ds24Op, owner string)
	}{
		{"no RRSIG", func(o *ds24Op, owner string) {
			o.put("op.test", ds24CDS(o.t, owner, o.zones[ds24Child].key))
			o.put("op.test", ds24CDNSKEY(owner, o.zones[ds24Child].key))
		}},
		{"RRSIG by a key outside the signaling zone", func(o *ds24Op, owner string) {
			child := o.zones[ds24Child]
			for _, rr := range []dns.RR{ds24CDS(o.t, owner, child.key), ds24CDNSKEY(owner, child.key)} {
				o.put("op.test", rr, tctest.Sign(o.t, child.key, child.signer, dns.RRToType(rr), []dns.RR{rr}))
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newDS24Op(t)
			tc.publish(o, ds24Owner)
			o.start()

			d := o.evaluate(o.walker())
			if !slices.Equal(d.unsigned, []string{"CDS", "CDNSKEY"}) || d.validated {
				t.Fatalf("domain = %+v, want both types unsigned", d)
			}
		})
	}
}

// The listed server answers for the cut with a referral to itself.
func TestDNSSEC24WalkerLameSignalingZone(t *testing.T) {
	o := newDS24Op(t)
	zoneName := "_signal." + ds24NS1
	o.zone(zoneName)
	o.delegate("op.test", zoneName, ds24Signed, ds24NS1+"/"+ds24NS1IP)
	o.start()

	d := o.evaluate(o.walker())
	if d.kind != ds24Unreached || d.fault == nil || d.fault.zone != zoneName {
		t.Fatalf("domain = %+v, want unreachable at %s", d, zoneName)
	}
	if got := tctest.ServerEndpoints(t, map[string]any{"servers": ds24Servers(d.fault)}); !slices.Equal(got, []string{ds24NS1 + "/" + ds24NS1IP}) {
		t.Fatalf("servers = %v", got)
	}
	// The question repeats the one op.test was asked, so the cache answers it.
	if got := o.asked(ds24NS1IP, ds24Owner, "CDS"); got != 1 {
		t.Fatalf("CDS questions = %d, want 1", got)
	}
}

// ds24Servers renders the servers of a fault as the log argument.
func ds24Servers(v *ds24Verdict) any {
	args := map[string]any{}
	setTypedServersFromEndpoints(args, v.servers)
	return args["servers"]
}

func TestDNSSEC24WalkerUnreachable(t *testing.T) {
	cases := []struct {
		name   string
		answer packet.Packet
	}{
		{"no response", packet.Packet{}},
		{"SERVFAIL", tctest.Response(tctest.Secure(), tctest.Rcode(dns.RcodeServerFailure))},
		{"referral above the cut", tctest.Response(tctest.Secure(), tctest.NotAuthoritative(),
			tctest.Authority(tctest.NSRR("test", "ns.test")))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newDS24Op(t)
			zoneName := "_signal." + ds24NS1
			o.zone(zoneName)
			o.delegate("op.test", zoneName, ds24Signed, "a.op.test/192.0.2.31", "b.op.test/192.0.2.32")
			o.start()
			for _, spec := range []string{"a.op.test/192.0.2.31", "b.op.test/192.0.2.32"} {
				name, ip, _ := strings.Cut(spec, "/")
				o.serveWith(name, ip, func(tctest.Query) packet.Packet { return tc.answer })
			}

			d := o.evaluate(o.walker())
			if d.kind != ds24Unreached || d.fault.zone != zoneName || len(d.fault.servers) != 2 {
				t.Fatalf("domain = %+v, want unreachable at %s after two servers", d, zoneName)
			}
		})
	}
}

func TestDNSSEC24WalkerZoneCutAtTheSignalingName(t *testing.T) {
	cases := []struct {
		name    string
		hostCut bool
	}{
		{"referral to the signaling name", false},
		{"signer is the signaling name", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newDS24Op(t)
			o.zone(ds24Owner)
			o.delegate("op.test", ds24Owner, ds24Signed, ds24NS1+"/"+ds24NS1IP)
			o.add(ds24Owner, ds24CDS(t, ds24Owner, o.zones[ds24Child].key))
			if tc.hostCut {
				o.start(ds24Owner)
			} else {
				o.start()
			}

			if d := o.evaluate(o.walker()); d.kind != ds24AtCut {
				t.Fatalf("domain = %+v, want a zone cut at the signaling name", d)
			}
		})
	}
}

// The signaling zone's server has no glue, so the recursor resolves it.
func TestDNSSEC24WalkerResolvesServersWithoutGlue(t *testing.T) {
	o := newDS24Op(t)
	o.zone("other.test")
	o.delegate("test", "other.test", ds24Signed, "ns.other.test/192.0.2.40")
	o.add("other.test", tctest.ARR("ns.other.test", "192.0.2.40"))
	zoneName := "_signal." + ds24NS1
	o.zone(zoneName)
	o.delegate("op.test", zoneName, ds24Signed, "ns.other.test")
	o.signal(zoneName, ds24NS1)
	o.start()
	o.serve("ns.other.test", "192.0.2.40", "other.test", zoneName)

	d := o.evaluate(o.walker())
	if !d.validated || d.zone != zoneName {
		t.Fatalf("domain = %+v, want validated in %s", d, zoneName)
	}
}

func TestDNSSEC24WalkerAbsent(t *testing.T) {
	cases := []struct {
		name    string
		publish func(o *ds24Op)
	}{
		{"NXDOMAIN", func(*ds24Op) {}},
		{"NODATA with compact denial", func(o *ds24Op) { o.add("op.test", tctest.TXTRR(ds24Owner, "x")) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newDS24Op(t)
			tc.publish(o)
			o.start()

			if d := o.evaluate(o.walker()); d.kind != ds24Absent || d.signaled {
				t.Fatalf("domain = %+v, want absent", d)
			}
			if got := o.askedType("DNSKEY"); got != 0 {
				t.Fatalf("DNSKEY questions = %d, want 0", got)
			}
		})
	}
}

// run runs DNSSEC24 on child.test with the given delegation NS names.
func (o *ds24Op) run(hosts ...string) []*logger.Entry {
	o.t.Helper()
	o.stubInputs(hosts...)
	entries, err := DNSSEC24(o.ctx, tctest.Zone(o.t, ds24Child, o.rec))
	if err != nil {
		o.t.Fatalf("DNSSEC24: %v", err)
	}
	return entries
}

// stubInputs points the discovery seams at the fake tree.
func (o *ds24Op) stubInputs(hosts ...string) {
	o.t.Helper()
	var names []dnsname.Name
	var glue []nameserver.Nameserver
	for _, host := range hosts {
		names = append(names, dnsname.New(host))
		for key, ns := range o.servers {
			if strings.HasPrefix(key, host+"/") {
				glue = append(glue, ns)
			}
		}
	}
	slices.SortFunc(glue, func(a, b nameserver.Nameserver) int { return strings.Compare(a.String(), b.String()) })
	tld := o.servers["ns.test/"+ds24TLDIP]
	tctest.Stub(o.t, &delegationNSNames, func(context.Context, *zone.Zone) ([]dnsname.Name, error) { return names, nil })
	tctest.Stub(o.t, &glueNameservers, func(context.Context, *zone.Zone) ([]nameserver.Nameserver, error) { return glue, nil })
	tctest.Stub(o.t, &parentNameservers, func(context.Context, *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{tld}, nil
	})
	tctest.Stub(o.t, &parentApexNameservers, func(context.Context, *zone.Zone) ([]nameserver.Nameserver, error) { return nil, nil })
}

// ready publishes a signaling zone with signals for both op.test servers.
func (o *ds24Op) ready() {
	o.t.Helper()
	for _, host := range []string{ds24NS1, ds24NS2} {
		o.signal(o.signalZone(host, ds24Signed), host)
	}
}

func TestDNSSEC24BootstrapReady(t *testing.T) {
	o := newDS24Op(t)
	o.ready()
	o.start()

	entries := o.run(ds24NS1, ds24NS2)
	tctest.RequireTags(t, entries, "DS24_BOOTSTRAP_READY")
	tctest.RequireCount(t, entries, "DS24_SIGNAL_VALIDATED", 2)
	entry := tctest.First(entries, "DS24_SIGNAL_VALIDATED")
	tctest.RequireArg(t, entry, "ns", ds24NS1)
	tctest.RequireArg(t, entry, "query_name", ds24Owner)
	tctest.RequireArg(t, entry, "zone", "_signal."+ds24NS1)
	tctest.RequireNoTag(t, entries, "DS24_SIGNAL_MISMATCH", "DS24_SIGNAL_MISSING", "DS24_APEX_UNAVAILABLE")
	// The root, test and op.test are walked once; the second name starts at op.test.
	if got := o.asked("192.0.2.1", "_dsboot.child.test._signal.ns2.op.test", "CDS"); got != 0 {
		t.Fatalf("root CDS questions for the second name = %d, want 0", got)
	}
	for _, cut := range []struct{ ip, zone string }{
		{"192.0.2.1", "."}, {ds24TLDIP, "test"}, {ds24NS1IP, "op.test"},
		{ds24SigIP, "_signal." + ds24NS1}, {ds24SigIP, "_signal." + ds24NS2},
	} {
		if got := o.asked(cut.ip, cut.zone, "DNSKEY"); got != 1 {
			t.Fatalf("%s DNSKEY questions = %d, want 1", cut.zone, got)
		}
	}
	if got := o.askedType("DNSKEY"); got != 5 {
		t.Fatalf("DNSKEY questions = %d, want 5", got)
	}
}

func TestDNSSEC24GateCostsNoWalk(t *testing.T) {
	cases := []struct {
		name  string
		setup func(o *ds24Op)
		tag   string
	}{
		{"DS at the parent", func(o *ds24Op) { o.delegate("test", ds24Child, ds24Signed, ds24NS1+"/"+ds24NS1IP) },
			"DS24_DELEGATION_SECURE"},
		{"no CDS and no CDNSKEY", func(o *ds24Op) {
			for _, qtype := range []string{"CDS", "CDNSKEY"} {
				delete(o.zones[ds24Child].rrsets, ds22Key(ds24Child, qtype))
			}
		}, "DS24_NO_CDS_CDNSKEY"},
		{"delete records only", func(o *ds24Op) {
			cds, _ := dns.New(ds24Child + ". 60 IN CDS 0 0 0 00")
			key, _ := dns.New(ds24Child + ". 60 IN CDNSKEY 0 3 0 AA==")
			o.zones[ds24Child].rrsets[ds22Key(ds24Child, "CDS")] = []dns.RR{cds}
			o.zones[ds24Child].rrsets[ds22Key(ds24Child, "CDNSKEY")] = []dns.RR{key}
		}, "DS24_DELETE_REQUESTED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newDS24Op(t)
			tc.setup(o)
			o.start()

			entries := o.run(ds24NS1, ds24NS2)
			tctest.RequireTags(t, entries, tc.tag)
			if got := tctest.TagsWithPrefix(entries, "DS24_"); !slices.Equal(got, []string{tc.tag}) {
				t.Fatalf("DS24 tags = %v, want only %s", got, tc.tag)
			}
			if got := o.askedType("CDS") - o.asked(ds24NS1IP, ds24Child, "CDS") - o.asked(ds24NS2IP, ds24Child, "CDS"); got != 0 {
				t.Fatalf("signaling CDS questions = %d, want 0", got)
			}
		})
	}
}

func TestDNSSEC24MixedDeleteAndKeyRecordsWalk(t *testing.T) {
	o := newDS24Op(t)
	del, _ := dns.New(ds24Child + ". 60 IN CDS 0 0 0 00")
	o.zones[ds24Child].rrsets[ds22Key(ds24Child, "CDS")] = []dns.RR{o.cds, del}
	o.start()

	entries := o.run(ds24NS1)
	tctest.RequireNoTag(t, entries, "DS24_DELETE_REQUESTED")
	tctest.RequireTags(t, entries, "DS24_NO_SIGNAL")
}

func TestDNSSEC24OnlyInDomainNS(t *testing.T) {
	o := newDS24Op(t)
	o.start()
	o.serve("ns1.child.test", "192.0.2.51", ds24Child)
	o.serve("ns2.child.test", "192.0.2.52", ds24Child)

	entries := o.run("ns1.child.test", "ns2.child.test")
	entry := tctest.RequireTag(t, entries, "DS24_ONLY_IN_DOMAIN_NS")
	if got := tctest.ServerNames(t, entry.Args); !slices.Equal(got, []string{"ns1.child.test", "ns2.child.test"}) {
		t.Fatalf("servers = %v", got)
	}
	if got := o.asked("192.0.2.1", "_dsboot.child.test._signal.ns1.child.test", "CDS"); got != 0 {
		t.Fatalf("root CDS questions = %d, want 0", got)
	}
}

func TestDNSSEC24SignalNameTooLong(t *testing.T) {
	o := newDS24Op(t)
	o.start()
	host := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 50) + ".op.test"
	o.serve(host, "192.0.2.53", ds24Child)

	entries := o.run(host)
	entry := tctest.RequireTag(t, entries, "DS24_SIGNAL_NAME_TOO_LONG")
	tctest.RequireArg(t, entry, "ns", host)
	tctest.RequireNoTag(t, entries, "DS24_NO_SIGNAL", "DS24_BOOTSTRAP_READY")
}

func TestDNSSEC24NoSignal(t *testing.T) {
	o := newDS24Op(t)
	o.start()

	entries := o.run(ds24NS1, ds24NS2)
	entry := tctest.RequireTag(t, entries, "DS24_NO_SIGNAL")
	if got := tctest.ServerNames(t, entry.Args); !slices.Equal(got, []string{ds24NS1, ds24NS2}) {
		t.Fatalf("servers = %v", got)
	}
	tctest.RequireNoTag(t, entries, "DS24_SIGNAL_MISSING", "DS24_BOOTSTRAP_READY")
	if got := o.askedType("DNSKEY"); got != 0 {
		t.Fatalf("DNSKEY questions = %d, want 0", got)
	}
	// Root, test and op.test for the first name, op.test alone for the second.
	if got := o.askedType("CDS") - o.asked(ds24NS1IP, ds24Child, "CDS") - o.asked(ds24NS2IP, ds24Child, "CDS"); got != 4 {
		t.Fatalf("signaling CDS questions = %d, want 4", got)
	}
}

func TestDNSSEC24SignalMissing(t *testing.T) {
	o := newDS24Op(t)
	o.signal(o.signalZone(ds24NS1, ds24Signed), ds24NS1)
	o.start()

	entries := o.run(ds24NS1, ds24NS2)
	entry := tctest.RequireTag(t, entries, "DS24_SIGNAL_MISSING")
	tctest.RequireArg(t, entry, "ns", ds24NS2)
	tctest.RequireArg(t, entry, "query_name", "_dsboot.child.test._signal.ns2.op.test")
	tctest.RequireCount(t, entries, "DS24_SIGNAL_VALIDATED", 1)
	tctest.RequireNoTag(t, entries, "DS24_NO_SIGNAL", "DS24_BOOTSTRAP_READY")
}

func TestDNSSEC24Mismatch(t *testing.T) {
	cases := []struct {
		name    string
		publish func(o *ds24Op, zoneName string, other *dns.DNSKEY)
		want    []string
	}{
		{"CDS differs", func(o *ds24Op, zoneName string, other *dns.DNSKEY) {
			o.add(zoneName, ds24CDS(o.t, ds24Owner, other))
			o.add(zoneName, ds24CDNSKEY(ds24Owner, o.zones[ds24Child].key))
		}, []string{"CDS"}},
		{"CDNSKEY differs", func(o *ds24Op, zoneName string, other *dns.DNSKEY) {
			o.add(zoneName, ds24CDS(o.t, ds24Owner, o.zones[ds24Child].key))
			o.add(zoneName, ds24CDNSKEY(ds24Owner, other))
		}, []string{"CDNSKEY"}},
		{"CDNSKEY empty against non-empty", func(o *ds24Op, zoneName string, _ *dns.DNSKEY) {
			o.add(zoneName, ds24CDS(o.t, ds24Owner, o.zones[ds24Child].key))
		}, []string{"CDNSKEY"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newDS24Op(t)
			zoneName := o.signalZone(ds24NS1, ds24Signed)
			other, _ := tctest.SignedKey(t, ds24Child, dns.ECDSAP256SHA256, tctest.SEP())
			tc.publish(o, zoneName, other)
			o.start()

			entries := o.run(ds24NS1)
			if got := tctest.ArgValues(entries, "DS24_SIGNAL_MISMATCH", "query_type"); !slices.Equal(got, tc.want) {
				t.Fatalf("mismatched types = %v, want %v", got, tc.want)
			}
			tctest.RequireNoTag(t, entries, "DS24_SIGNAL_VALIDATED", "DS24_BOOTSTRAP_READY")
		})
	}
}

// An apex server without an authoritative answer withholds the verdict.
func TestDNSSEC24ApexUnavailable(t *testing.T) {
	o := newDS24Op(t)
	o.ready()
	o.start()
	o.serveWith(ds24NS2, ds24NS2IP, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(strings.TrimSuffix(q.Name, "."), ds24Child) && q.Type == "CDNSKEY" {
			return tctest.Response(tctest.Secure(), tctest.NotAuthoritative())
		}
		return ds24Answer([]*ds24Zone{o.zones["op.test"], o.zones[ds24Child]}, q)
	})

	entries := o.run(ds24NS1, ds24NS2)
	entry := tctest.RequireTag(t, entries, "DS24_APEX_UNAVAILABLE")
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{ds24NS2 + "/" + ds24NS2IP}) {
		t.Fatalf("servers = %v", got)
	}
	tctest.RequireCount(t, entries, "DS24_SIGNAL_VALIDATED", 2)
	tctest.RequireNoTag(t, entries, "DS24_BOOTSTRAP_READY")
}

func TestDNSSEC24TagsOfAFault(t *testing.T) {
	o := newDS24Op(t)
	o.signal(o.signalZone(ds24NS1, ds24NoDS), ds24NS1)
	o.start()

	entries := o.run(ds24NS1)
	entry := tctest.RequireTag(t, entries, "DS24_SIGNAL_ZONE_INSECURE")
	tctest.RequireArg(t, entry, "ns", ds24NS1)
	tctest.RequireArg(t, entry, "zone", "_signal."+ds24NS1)
	tctest.RequireNoTag(t, entries, "DS24_SIGNAL_VALIDATED", "DS24_SIGNAL_MISSING", "DS24_NO_SIGNAL")
}

func TestDNSSEC24TransportDisabled(t *testing.T) {
	o := newDS24Op(t)
	o.start()
	o.serve(ds24NS1, "2001:db8::11", "op.test", ds24Child)
	if err := profile.Effective().Set("net.ipv6", false); err != nil {
		t.Fatalf("set net.ipv6: %v", err)
	}

	entries := o.run(ds24NS1)
	if got := tctest.ArgValues(entries, "IPV6_DISABLED", "query_type"); !slices.Equal(got, []string{"CDS", "CDNSKEY"}) {
		t.Fatalf("IPV6_DISABLED query types = %v", got)
	}
	tctest.RequireArg(t, tctest.First(entries, "IPV6_DISABLED"), "address", "2001:db8::11")
	if got := o.asked("2001:db8::11", ds24Child, "CDS"); got != 0 {
		t.Fatalf("CDS questions over IPv6 = %d, want 0", got)
	}
}

// The parent DS and apex questions of DNSSEC07 and DNSSEC15 are answered from the cache.
func TestDNSSEC24ReusesDNSSEC07AndDNSSEC15Queries(t *testing.T) {
	o := newDS24Op(t)
	o.start()
	o.stubInputs(ds24NS1, ds24NS2)
	items := tctest.NSItems(ds24NS1+"/"+ds24NS1IP, ds24NS2+"/"+ds24NS2IP)
	tctest.Stub(t, &delegationNameservers, func(context.Context, *zone.Zone) ([]nsdiscovery.NSItem, error) { return items, nil })
	tctest.Stub(t, &zoneNameservers, func(context.Context, *zone.Zone) ([]nsdiscovery.NSItem, error) { return items, nil })
	tctest.Stub(t, &apexNameservers, func(context.Context, *zone.Zone) ([]nameserver.Nameserver, error) { return nil, nil })
	tctest.Stub(t, &zoneParent, func(context.Context, *zone.Zone) (*zone.Zone, error) { return nil, errors.New("no parent") })

	z := tctest.Zone(t, ds24Child, o.rec)
	for _, run := range []func(context.Context, *zone.Zone) ([]*logger.Entry, error){DNSSEC07, DNSSEC15, DNSSEC24} {
		if _, err := run(o.ctx, z); err != nil {
			t.Fatalf("run: %v", err)
		}
	}
	for _, ip := range []string{ds24NS1IP, ds24NS2IP} {
		for _, qtype := range []string{"CDS", "CDNSKEY"} {
			if got := o.asked(ip, ds24Child, qtype); got != 1 {
				t.Fatalf("%s %s questions = %d, want 1", ip, qtype, got)
			}
		}
	}
	if got := o.asked(ds24TLDIP, ds24Child, "DS"); got != 1 {
		t.Fatalf("parent DS questions = %d, want 1", got)
	}
}
