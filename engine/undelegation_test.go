package engine

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
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestParseUndelegatedNameserver(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    UndelegatedNameserver
		wantErr bool
	}{
		{
			name: "name and ip",
			spec: "NS1.Example.COM/192.0.2.1",
			want: UndelegatedNameserver{Name: "ns1.example.com", IP: "192.0.2.1"},
		},
		{
			name: "name only",
			spec: "NS2.Example.COM",
			want: UndelegatedNameserver{Name: "ns2.example.com"},
		},
		{
			name:    "invalid name",
			spec:    "bad!name.example/192.0.2.1",
			wantErr: true,
		},
		{
			name:    "invalid ip",
			spec:    "ns1.example.com/not-an-ip",
			wantErr: true,
		},
		{
			name:    "invalid format",
			spec:    "ns1.example.com/192.0.2.1/extra",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseUndelegatedNameserver(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse undelegated nameserver: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected parse result: got=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestParseUndelegatedDS(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    UndelegatedDSInfo
		wantErr bool
	}{
		{
			name: "valid",
			spec: "12345,13,2," + strings.Repeat("a", 64),
			want: UndelegatedDSInfo{
				KeyTag:     12345,
				Algorithm:  13,
				DigestType: 2,
				Digest:     strings.Repeat("A", 64),
			},
		},
		{
			name:    "bad field count",
			spec:    "12345,13,2",
			wantErr: true,
		},
		{
			name:    "bad keytag",
			spec:    "-1,13,2," + strings.Repeat("a", 64),
			wantErr: true,
		},
		{
			name:    "bad digest",
			spec:    "12345,13,2,not-hex",
			wantErr: true,
		},
		{
			name:    "bad digest length",
			spec:    "12345,13,2,ABCD",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseUndelegatedDS(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse undelegated DS: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected parse result: got=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestNormalizeUndelegatedInputs(t *testing.T) {
	nameservers := []UndelegatedNameserver{
		{Name: "NS1.Example.COM", IP: "192.0.2.1"},
		{Name: "ns1.example.com", IP: "192.0.2.1"},
		{Name: "ns2.example.com"},
	}
	ds := []UndelegatedDSInfo{
		{
			KeyTag:     12345,
			Algorithm:  13,
			DigestType: 2,
			Digest:     strings.Repeat("a", 64),
		},
		{
			KeyTag:     12345,
			Algorithm:  13,
			DigestType: 2,
			Digest:     strings.Repeat("A", 64),
		},
	}

	normalizedNS, normalizedDS, err := NormalizeUndelegatedInputs(nameservers, ds)
	if err != nil {
		t.Fatalf("normalize undelegated inputs: %v", err)
	}
	if len(normalizedNS) != 2 {
		t.Fatalf("expected 2 unique nameservers, got %d", len(normalizedNS))
	}
	if normalizedNS[0].Name != "ns1.example.com" || normalizedNS[0].IP != "192.0.2.1" {
		t.Fatalf("unexpected first nameserver: %+v", normalizedNS[0])
	}
	if normalizedNS[1].Name != "ns2.example.com" || normalizedNS[1].IP != "" {
		t.Fatalf("unexpected second nameserver: %+v", normalizedNS[1])
	}

	if len(normalizedDS) != 1 {
		t.Fatalf("expected 1 unique DS record, got %d", len(normalizedDS))
	}
	if normalizedDS[0].Digest != strings.Repeat("A", 64) {
		t.Fatalf("expected uppercase digest, got %q", normalizedDS[0].Digest)
	}
}

func TestBuildUndelegatedDSData(t *testing.T) {
	ds := []UndelegatedDSInfo{{
		KeyTag:     12345,
		Algorithm:  13,
		DigestType: 2,
		Digest:     strings.Repeat("A", 64),
	}}

	records := buildUndelegatedDSData(ds)
	if len(records) != 1 {
		t.Fatalf("expected one DS record, got %d", len(records))
	}
	if records[0].KeyTag != 12345 {
		t.Fatalf("unexpected DS key tag: %d", records[0].KeyTag)
	}
	if records[0].Algorithm != 13 {
		t.Fatalf("unexpected DS algorithm: %d", records[0].Algorithm)
	}
	if records[0].DigestType != 2 {
		t.Fatalf("unexpected DS digest type: %d", records[0].DigestType)
	}
	if records[0].Digest != strings.Repeat("A", 64) {
		t.Fatalf("unexpected DS digest: %q", records[0].Digest)
	}
}

func TestApplyUndelegatedDS(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ns1, err := nameserver.NewWithContext(ctx, "a.gtld-servers.net", "192.5.6.30", nil)
	if err != nil {
		t.Fatalf("new nameserver 1: %v", err)
	}
	ns2, err := nameserver.NewWithContext(ctx, "b.gtld-servers.net", "192.33.14.30", nil)
	if err != nil {
		t.Fatalf("new nameserver 2: %v", err)
	}

	ds := []UndelegatedDSInfo{
		{
			KeyTag:     12345,
			Algorithm:  13,
			DigestType: 2,
			Digest:     strings.Repeat("A", 64),
		},
		{
			KeyTag:     54321,
			Algorithm:  8,
			DigestType: 2,
			Digest:     strings.Repeat("B", 64),
		},
	}

	if err := applyUndelegatedDS([]nameserver.Nameserver{ns1, ns2}, "example.com", ds); err != nil {
		t.Fatalf("apply undelegated DS: %v", err)
	}

	for idx, ns := range []nameserver.Nameserver{ns1, ns2} {
		records := ns.FakeDSRecords("example.com")
		if len(records) != len(ds) {
			t.Fatalf("nameserver %d: expected %d fake DS records, got %d", idx, len(ds), len(records))
		}
		first, ok := records[0].(*dns.DS)
		if !ok {
			t.Fatalf("nameserver %d: expected DS record, got %T", idx, records[0])
		}
		if first.KeyTag != uint16(ds[0].KeyTag) || first.Algorithm != uint8(ds[0].Algorithm) || first.DigestType != uint8(ds[0].DigestType) || first.Digest != ds[0].Digest {
			t.Fatalf("nameserver %d: unexpected first fake DS record: %+v", idx, first)
		}
	}
}

func TestRunRejectsInvalidUndelegatedInput(t *testing.T) {
	_, err := Run(RunRequest{
		Domain:    ".",
		Testcases: []string{"basic01"},
		UndelegatedNameservers: []UndelegatedNameserver{
			{Name: "bad!name.example"},
		},
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "undelegated nameserver") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildUndelegatedFakeDelegationInZoneMissingIP(t *testing.T) {
	var tags []string
	out, err := buildUndelegatedFakeDelegation(
		context.Background(),
		dnsname.New("example.com"),
		[]UndelegatedNameserver{{Name: "ns1.example.com"}},
		nil,
		func(tag string, _ map[string]any) error {
			tags = append(tags, tag)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("build fake delegation: %v", err)
	}
	if len(out["ns1.example.com"]) != 0 {
		t.Fatalf("expected empty in-zone glue list, got %v", out["ns1.example.com"])
	}
	if len(tags) != 1 || tags[0] != "FAKE_DELEGATION_IN_ZONE_NO_IP" {
		t.Fatalf("unexpected tags: %v", tags)
	}
}

func TestBuildUndelegatedFakeDelegationOutOfBailiwickFill(t *testing.T) {
	var lookedUp []string
	out, err := buildUndelegatedFakeDelegation(
		context.Background(),
		dnsname.New("example.com"),
		[]UndelegatedNameserver{{Name: "ns.other.test"}},
		func(_ context.Context, name string) ([]netip.Addr, error) {
			lookedUp = append(lookedUp, name)
			return []netip.Addr{netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("2001:db8::10")}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("build fake delegation: %v", err)
	}
	if len(lookedUp) != 1 || lookedUp[0] != "ns.other.test" {
		t.Fatalf("unexpected lookup sequence: %v", lookedUp)
	}
	if len(out["ns.other.test"]) != 2 {
		t.Fatalf("expected two filled addresses, got %v", out["ns.other.test"])
	}
}

func TestBuildUndelegatedFakeDelegationOutOfBailiwickNoFillEmitsNoIP(t *testing.T) {
	var tags []string
	out, err := buildUndelegatedFakeDelegation(
		context.Background(),
		dnsname.New("example.com"),
		[]UndelegatedNameserver{{Name: "ns.other.test"}},
		func(_ context.Context, _ string) ([]netip.Addr, error) {
			return nil, errors.New("lookup failed")
		},
		func(tag string, _ map[string]any) error {
			tags = append(tags, tag)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("build fake delegation: %v", err)
	}
	if len(out["ns.other.test"]) != 0 {
		t.Fatalf("expected unresolved out-of-bailiwick glue, got %v", out["ns.other.test"])
	}
	if len(tags) != 1 || tags[0] != "FAKE_DELEGATION_NO_IP" {
		t.Fatalf("unexpected tags: %v", tags)
	}
}

func TestFakeDelegationToSelf(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ns, err := nameserver.NewWithContext(ctx, "ns1.example.com", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	if !fakeDelegationToSelf(ns, map[string][]string{
		"ns1.example.com": {"192.0.2.1"},
	}) {
		t.Fatalf("expected self match")
	}

	if fakeDelegationToSelf(ns, map[string][]string{
		"ns1.example.com": {"192.0.2.2"},
	}) {
		t.Fatalf("did not expect self match")
	}
}

func TestApplyUndelegatedDelegationEmitsFakeDelegationToSelf(t *testing.T) {
	// Timeout-bound (TEST-NET queries); per-test ctx state, safe to overlap.
	t.Parallel()

	ctx, _, log := testhelpers.Context(t)

	r := recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"ns1.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root addresses: %v", err)
	}

	z, err := zone.NewWithRecursor("example", &r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	err = applyUndelegatedDelegation(
		ctx,
		&r,
		&z,
		[]UndelegatedNameserver{
			{Name: "ns1.root", IP: "192.0.2.1"},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("apply undelegated delegation: %v", err)
	}

	found := false
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		if entry.Tag == "FAKE_DELEGATION_TO_SELF" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected FAKE_DELEGATION_TO_SELF log entry")
	}
}
