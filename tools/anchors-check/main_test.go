package main

import (
	"slices"
	"testing"
	"time"
)

// rootAnchorsXML is root-anchors.xml as IANA published it on 2026-10-04, public keys left out.
const rootAnchorsXML = `<?xml version="1.0" encoding="UTF-8"?>
<TrustAnchor id="0C05FDD6-422C-4910-8ED6-430ED15E11C2" source="http://data.iana.org/root-anchors/root-anchors.xml">
    <Zone>.</Zone>
    <KeyDigest id="Kjqmt7v" validFrom="2010-07-15T00:00:00+00:00" validUntil="2019-01-11T00:00:00+00:00">
        <KeyTag>19036</KeyTag>
        <Algorithm>8</Algorithm>
        <DigestType>2</DigestType>
        <Digest>49AAC11D7B6F6446702E54A1607371607A1A41855200FD2CE1CDDE32F24E8FB5</Digest>
    </KeyDigest>
    <KeyDigest id="Klajeyz" validFrom="2017-02-02T00:00:00+00:00">
        <KeyTag>20326</KeyTag>
        <Algorithm>8</Algorithm>
        <DigestType>2</DigestType>
        <Digest>e06d44b80b8f1d39a95c0b0d7c65d08458e880409bbc683457104237c7f8ec8d</Digest>
    </KeyDigest>
    <KeyDigest id="Kmyv6jo" validFrom="2024-07-18T00:00:00+00:00">
        <KeyTag>38696</KeyTag>
        <Algorithm>8</Algorithm>
        <DigestType>2</DigestType>
        <Digest>683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16</Digest>
    </KeyDigest>
</TrustAnchor>`

func keyTags(anchors []anchor) []uint16 {
	tags := make([]uint16, 0, len(anchors))
	for _, a := range anchors {
		tags = append(tags, a.keyTag)
	}
	return tags
}

func TestPublishedTakesTheDigestsValidAtTheTime(t *testing.T) {
	cases := []struct {
		name string
		at   time.Time
		want []uint16
	}{
		{"2011, KSK-2010 only", time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC), []uint16{19036}},
		{"2018, KSK-2010 and KSK-2017", time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), []uint16{19036, 20326}},
		{"2026, KSK-2017 and KSK-2024", time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC), []uint16{20326, 38696}},
		{"the instant KSK-2010 expired", time.Date(2019, 1, 11, 0, 0, 0, 0, time.UTC), []uint16{20326}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := published([]byte(rootAnchorsXML), c.at)
			if err != nil {
				t.Fatalf("published: %v", err)
			}
			if !slices.Equal(keyTags(got), c.want) {
				t.Errorf("key tags = %v, want %v", keyTags(got), c.want)
			}
		})
	}
}

func TestPublishedRejectsADocument(t *testing.T) {
	cases := []struct {
		name     string
		document string
		at       time.Time
	}{
		{"truncated", "<TrustAnchor>", time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)},
		{"no digest current", rootAnchorsXML, time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"another zone", `<TrustAnchor><Zone>example.</Zone></TrustAnchor>`, time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)},
		{"unreadable date", `<TrustAnchor><Zone>.</Zone><KeyDigest validFrom="2017-02-02"><KeyTag>20326</KeyTag></KeyDigest></TrustAnchor>`,
			time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, err := published([]byte(c.document), c.at); err == nil {
				t.Errorf("published = %v, want an error", keyTags(got))
			}
		})
	}
}

// The digest case of the document does not matter; 20326 is published in lower case above.
func TestBuiltInMatchesTheDocument(t *testing.T) {
	current, err := published([]byte(rootAnchorsXML), time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("published: %v", err)
	}
	if problems := compare(builtIn(), current); problems != nil {
		t.Errorf("compare = %v, want no difference", problems)
	}
}

func TestCompareNamesBothDirections(t *testing.T) {
	ksk2017 := anchor{20326, 8, 2, "E06D44B80B8F1D39A95C0B0D7C65D08458E880409BBC683457104237C7F8EC8D"}
	ksk2024 := anchor{38696, 8, 2, "683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16"}
	cases := []struct {
		name           string
		built, current []anchor
		want           []string
	}{
		{"same set", []anchor{ksk2017, ksk2024}, []anchor{ksk2024, ksk2017}, nil},
		{"one missing and one extra", []anchor{ksk2024}, []anchor{ksk2017}, []string{
			"IANA publishes key 20326, which engine/dnssecutil does not carry",
			"engine/dnssecutil carries key 38696, which IANA does not publish as current",
		}},
		{"same key tag, other digest", []anchor{{20326, 8, 2, "00"}}, []anchor{ksk2017}, []string{
			"IANA publishes key 20326, which engine/dnssecutil does not carry",
			"engine/dnssecutil carries key 20326, which IANA does not publish as current",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := compare(c.built, c.current); !slices.Equal(got, c.want) {
				t.Errorf("compare = %q, want %q", got, c.want)
			}
		})
	}
}
