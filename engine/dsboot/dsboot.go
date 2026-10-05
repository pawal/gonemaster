// Package dsboot holds the RFC 9615 signaling name and the RFC 8078 tests on CDS and CDNSKEY records.
package dsboot

import (
	"slices"
	"strconv"
	"strings"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
)

// SignalName returns _dsboot.<child>._signal.<ns>, lowercased; false beyond 255 octets.
func SignalName(child, ns dnsname.Name) (dnsname.Name, bool) {
	labels := append([]string{"_dsboot"}, child.Labels()...)
	labels = append(labels, "_signal")
	labels = append(labels, ns.Labels()...)
	wire := 1
	for i, label := range labels {
		labels[i] = strings.ToLower(label)
		wire += len(label) + 1
	}
	return dnsname.NewFromLabels(labels), wire <= 255
}

// SignalingHosts returns the names in ns outside child, lowercased, deduplicated and sorted.
func SignalingHosts(child dnsname.Name, ns []dnsname.Name) []dnsname.Name {
	var hosts []string
	for _, name := range ns {
		if host := name.StringLower(); !child.IsInBailiwick(name) && !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}
	slices.Sort(hosts)
	out := make([]dnsname.Name, len(hosts))
	for i, host := range hosts {
		out[i] = dnsname.New(host)
	}
	return out
}

// IsDelete reports an RFC 8078 delete record: a CDS or CDNSKEY with algorithm zero.
func IsDelete(rr dns.RR) bool {
	switch record := rr.(type) {
	case *dns.CDS:
		return record.Algorithm == 0
	case *dns.CDNSKEY:
		return record.Algorithm == 0
	}
	return false
}

// RequestsDS reports at least one record that is not a delete record.
func RequestsDS(cds, cdnskey []dns.RR) bool {
	live := func(rr dns.RR) bool { return !IsDelete(rr) }
	return slices.ContainsFunc(cds, live) || slices.ContainsFunc(cdnskey, live)
}

// Content returns the rdata of CDS and CDNSKEY records, sorted and deduplicated.
func Content(rrs []dns.RR) []string {
	var out []string
	for _, rr := range rrs {
		switch record := rr.(type) {
		case *dns.CDS:
			out = append(out, strconv.Itoa(int(record.KeyTag))+" "+strconv.Itoa(int(record.Algorithm))+" "+
				strconv.Itoa(int(record.DigestType))+" "+strings.ToUpper(record.Digest))
		case *dns.CDNSKEY:
			out = append(out, strconv.Itoa(int(record.Flags))+" "+strconv.Itoa(int(record.Protocol))+" "+
				strconv.Itoa(int(record.Algorithm))+" "+record.PublicKey)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Equal compares two RRsets by Content, ignoring owner and TTL.
func Equal(a, b []dns.RR) bool {
	return slices.Equal(Content(a), Content(b))
}
