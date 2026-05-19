package nsdiscovery

import (
	"context"
	"net/netip"
	"sort"
	"strings"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

func validSOA(resp packet.Packet, name dnsname.Name) bool {
	return resp.Msg != nil && resp.Rcode() == "NOERROR" && resp.AA() &&
		len(resp.GetRecordsForName("SOA", name, "answer")) == 1
}

func validNS(resp packet.Packet, name dnsname.Name) bool {
	if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() {
		return false
	}
	answer := resp.GetRecords("NS", "answer")
	if len(answer) == 0 {
		return false
	}
	return len(answer) == len(resp.GetRecordsForName("NS", name, "answer"))
}

func nsMapFromResponse(resp packet.Packet, owner dnsname.Name, section string) map[string][]netip.Addr {
	out := map[string][]netip.Addr{}
	for _, rr := range resp.GetRecordsForName("NS", owner, section) {
		if nsRR, ok := rr.(*dns.NS); ok {
			nsNameObj := dnsname.New(nsRR.Ns)
			key := strings.ToLower(nsNameObj.String())
			out[key] = []netip.Addr{}
		}
	}
	for _, rr := range append(resp.GetRecords("A", "additional"), resp.GetRecords("AAAA", "additional")...) {
		ownerName := dnsname.New(rr.Header().Name)
		key := strings.ToLower(ownerName.String())
		if _, ok := out[key]; ok {
			if addr, ok := addrFromRR(rr); ok {
				out[key] = append(out[key], addr)
			}
		}
	}
	return out
}

func addrFromRR(rr dns.RR) (netip.Addr, bool) {
	switch v := rr.(type) {
	case *dns.A:
		return v.Addr, v.Addr.IsValid()
	case *dns.AAAA:
		return v.Addr, v.Addr.IsValid()
	default:
		return netip.Addr{}, false
	}
}

func collectAddrs(resp packet.Packet, qtype string, target dnsname.Name) []netip.Addr {
	var out []netip.Addr
	for _, rr := range resp.GetRecordsForName(qtype, target) {
		if addr, ok := addrFromRR(rr); ok {
			out = append(out, addr)
		}
	}
	return out
}

func collectResolvedAddrs(resp packet.Packet, qtype string, nsName dnsname.Name) []netip.Addr {
	if resp.HasRRsOfTypeForName("CNAME", nsName, "answer") {
		target := followCNAME(resp, nsName)
		return collectAddrs(resp, qtype, target)
	}

	if cnameFollowed(resp, nsName) {
		target := cnameTargetFromQuestion(resp)
		if resp.HasRRsOfTypeForName("CNAME", target, "answer") {
			target = followCNAME(resp, target)
		}
		return collectAddrs(resp, qtype, target)
	}

	if resp.HasRRsOfTypeForName(qtype, nsName, "answer") {
		return collectAddrs(resp, qtype, nsName)
	}
	return nil
}

func cnameFollowed(resp packet.Packet, nsName dnsname.Name) bool {
	questions := resp.Question()
	if len(questions) == 0 {
		return false
	}
	owner := dnsname.New(questions[0].Header().Name)
	return !strings.EqualFold(owner.String(), nsName.String())
}

func cnameTargetFromQuestion(resp packet.Packet) dnsname.Name {
	questions := resp.Question()
	if len(questions) == 0 {
		return dnsname.Name{}
	}
	return dnsname.New(questions[0].Header().Name)
}

func followCNAME(resp packet.Packet, start dnsname.Name) dnsname.Name {
	cnames := map[string]dnsname.Name{}
	for _, rr := range resp.GetRecords("CNAME", "answer") {
		if cname, ok := rr.(*dns.CNAME); ok {
			owner := dnsname.New(cname.Hdr.Name)
			target := dnsname.New(cname.Target)
			cnames[strings.ToLower(owner.String())] = target
		}
	}

	key := strings.ToLower(start.String())
	for {
		next, ok := cnames[key]
		if !ok {
			break
		}
		key = strings.ToLower(next.String())
	}
	return dnsname.New(key)
}

func queryPacket(ctx context.Context, ns nameserver.Nameserver, name string, qtype string) packet.Packet {
	resp, err := ns.Query(ctx, name, qtype)
	if err != nil {
		return packet.Packet{}
	}
	return resp
}

func uniqueAddrs(addrs []netip.Addr) []netip.Addr {
	seen := map[string]netip.Addr{}
	for _, addr := range addrs {
		seen[addr.String()] = addr
	}
	out := make([]netip.Addr, 0, len(seen))
	for _, addr := range seen {
		out = append(out, addr)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

func uniqueSortedItems(items []NSItem) []NSItem {
	seen := map[string]NSItem{}
	for _, item := range items {
		seen[item.String()] = item
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]NSItem, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func uniqueSortedNameservers(items []nameserver.Nameserver) []nameserver.Nameserver {
	seen := map[string]nameserver.Nameserver{}
	for _, item := range items {
		seen[strings.ToLower(item.String())] = item
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]nameserver.Nameserver, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func firstKey(m map[string][]nameserver.Nameserver) string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys[0]
}

func cloneNameservers(list []nameserver.Nameserver) []nameserver.Nameserver {
	if list == nil {
		return nil
	}
	out := make([]nameserver.Nameserver, len(list))
	copy(out, list)
	return out
}
