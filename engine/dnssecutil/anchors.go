package dnssecutil

import dns "codeberg.org/miekg/dns"

// rootAnchor is one IANA root trust anchor, algorithm 8 with a SHA-256 digest.
type rootAnchor struct {
	keyTag uint16
	digest string
}

// rootAnchors are KSK-2017 and KSK-2024 from root-anchors.xml; make anchors-check verifies them.
var rootAnchors = []rootAnchor{
	{20326, "E06D44B80B8F1D39A95C0B0D7C65D08458E880409BBC683457104237C7F8EC8D"},
	{38696, "683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16"},
}

// RootTrustAnchors returns the IANA root trust anchors as fresh DS records.
func RootTrustAnchors() []*dns.DS {
	out := make([]*dns.DS, 0, len(rootAnchors))
	for _, anchor := range rootAnchors {
		ds := &dns.DS{Hdr: dns.Header{Name: ".", Class: dns.ClassINET}}
		ds.KeyTag = anchor.keyTag
		ds.Algorithm = dns.RSASHA256
		ds.DigestType = dns.SHA256
		ds.Digest = anchor.digest
		out = append(out, ds)
	}
	return out
}
