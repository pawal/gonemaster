package dsboot

import (
	"slices"
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
)

func rr(t *testing.T, text string) dns.RR {
	t.Helper()
	record, err := dns.New(text)
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	return record
}

const (
	liveCDS     = "child.test. 60 IN CDS 2371 13 2 1F987CC6583E92DF0890718C42"
	liveCDNSKEY = "child.test. 60 IN CDNSKEY 257 3 13 AAECAwQFBgcICQoLDA0ODw=="
	deleteCDS   = "child.test. 60 IN CDS 0 0 0 00"
	deleteKey   = "child.test. 60 IN CDNSKEY 0 3 0 AA=="
)

func TestSignalName(t *testing.T) {
	name, ok := SignalName(dnsname.New("Example.CO.uk"), dnsname.New("NS1.example.net"))
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
		if _, ok := SignalName(dnsname.New(label+"."+label), host); ok != tc.ok {
			t.Errorf("middle label of %d octets: ok = %v, want %v", tc.middle, ok, tc.ok)
		}
	}
}

func TestSignalingHosts(t *testing.T) {
	for _, tc := range []struct {
		name string
		ns   []string
		want []string
	}{
		{"outside kept", []string{"ns2.op.test", "Child.Test", "ns1.child.test", "NS1.Op.Test", "ns1.op.test", "a.child.test.example"}, []string{"a.child.test.example", "ns1.op.test", "ns2.op.test"}},
		{"none outside", []string{"ns1.child.test"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ns []dnsname.Name
			for _, name := range tc.ns {
				ns = append(ns, dnsname.New(name))
			}
			var got []string
			for _, host := range SignalingHosts(dnsname.New("child.test"), ns) {
				got = append(got, host.String())
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("hosts = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsDelete(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{deleteCDS, true},
		{deleteKey, true},
		{"Child.TEST. 60 IN CDS 0 0 0 00", true},
		{liveCDS, false},
		{liveCDNSKEY, false},
		{"child.test. 60 IN A 192.0.2.1", false},
	} {
		if got := IsDelete(rr(t, tc.text)); got != tc.want {
			t.Errorf("IsDelete(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestRequestsDS(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cds, cdnskey []string
		want         bool
	}{
		{"no records", nil, nil, false},
		{"delete records alone", []string{deleteCDS}, []string{deleteKey}, false},
		{"delete beside a live CDS", []string{deleteCDS, liveCDS}, nil, true},
		{"live CDNSKEY alone", nil, []string{liveCDNSKEY}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cds, cdnskey []dns.RR
			for _, text := range tc.cds {
				cds = append(cds, rr(t, text))
			}
			for _, text := range tc.cdnskey {
				cdnskey = append(cdnskey, rr(t, text))
			}
			if got := RequestsDS(cds, cdnskey); got != tc.want {
				t.Fatalf("RequestsDS = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestContent(t *testing.T) {
	got := Content([]dns.RR{rr(t, liveCDNSKEY), rr(t, liveCDS), rr(t, liveCDS), rr(t, "child.test. 60 IN A 192.0.2.1")})
	want := []string{"2371 13 2 1F987CC6583E92DF0890718C42", "257 3 13 AAECAwQFBgcICQoLDA0ODw=="}
	if !slices.Equal(got, want) {
		t.Fatalf("Content = %q, want %q", got, want)
	}
}

func TestEqual(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b []string
		want bool
	}{
		{"owner and TTL ignored", []string{liveCDS}, []string{"Other.Test. 3600 IN CDS 2371 13 2 1F987CC6583E92DF0890718C42"}, true},
		{"digest case ignored", []string{liveCDS}, []string{"child.test. 60 IN CDS 2371 13 2 1f987cc6583e92df0890718c42"}, true},
		{"order ignored", []string{liveCDS, deleteCDS}, []string{deleteCDS, liveCDS}, true},
		{"empty equals empty", nil, nil, true},
		{"empty differs from a record", nil, []string{liveCDS}, false},
		{"digest differs", []string{liveCDS}, []string{"child.test. 60 IN CDS 2371 13 2 1F987CC6583E92DF0890718C43"}, false},
		{"public key case kept", []string{liveCDNSKEY}, []string{"child.test. 60 IN CDNSKEY 257 3 13 aaecawqfbgcicqolda0odw=="}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var a, b []dns.RR
			for _, text := range tc.a {
				a = append(a, rr(t, text))
			}
			for _, text := range tc.b {
				b = append(b, rr(t, text))
			}
			if got := Equal(a, b); got != tc.want {
				t.Fatalf("Equal = %v, want %v", got, tc.want)
			}
		})
	}
}
