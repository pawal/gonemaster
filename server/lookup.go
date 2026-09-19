package server

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/normalization"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

// DelegationInfo holds NS and DS records fetched from DNS.
type DelegationInfo struct {
	Nameservers []DelegationNS `json:"nameservers"`
	DSRecords   []DelegationDS `json:"ds_records"`
}

// DelegationNS is a nameserver with an optional resolved address.
type DelegationNS struct {
	NS string `json:"ns"`
	IP string `json:"ip,omitempty"`
}

// DelegationDS is a DS record from the parent zone.
type DelegationDS struct {
	KeyTag    int    `json:"keytag"`
	Algorithm int    `json:"algorithm"`
	DigType   int    `json:"digtype"`
	Digest    string `json:"digest"`
}

// lookupResolvers selects the DNS servers and host resolver a delegation
// lookup uses. The zero value uses the system configuration.
type lookupResolvers struct {
	servers []string
	host    *net.Resolver
}

func (l lookupResolvers) addresses() []string {
	if len(l.servers) > 0 {
		return l.servers
	}
	return resolverAddresses()
}

func (l lookupResolvers) hostResolver() *net.Resolver {
	if l.host != nil {
		return l.host
	}
	return net.DefaultResolver
}

// handlePublicLookupDomain handles GET /pub/api/v1/lookup/{domain}.
func (s *Server) handlePublicLookupDomain(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.PathValue("domain"))
	if domain == "" {
		writeError(w, http.StatusBadRequest, "missing_domain", "domain is required", nil)
		return
	}
	if errs, normalized := normalization.NormalizeName(domain); len(errs) > 0 {
		writeError(w, http.StatusBadRequest, "invalid_domain", errs[0].Message(), nil)
		return
	} else {
		domain = normalized
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	info := s.delegationLookup(ctx, domain)
	writeJSON(w, http.StatusOK, info)
}

// lookupDelegation resolves a delegation through the server's resolvers.
func (s *Server) lookupDelegation(ctx context.Context, domain string) DelegationInfo {
	return lookupDelegation(ctx, domain, s.lookup)
}

// lookupDelegation queries DNS for NS and DS records of a domain.
func lookupDelegation(ctx context.Context, domain string, res lookupResolvers) DelegationInfo {
	info := DelegationInfo{
		Nameservers: []DelegationNS{},
		DSRecords:   []DelegationDS{},
	}

	info.Nameservers = lookupNS(ctx, domain, res)
	info.DSRecords = lookupDS(ctx, domain, res)

	return info
}

// lookupNS queries for NS records via DNS wire protocol.
func lookupNS(ctx context.Context, domain string, res lookupResolvers) []DelegationNS {
	msg := transport.BuildQuery(domain, dns.TypeNS)
	msg.RecursionDesired = true

	c := &transport.Client{
		Timeout:  5 * time.Second,
		Fallback: true,
		EDNSSize: 4096,
	}

	servers := res.addresses()
	for _, server := range servers {
		pkt, err := c.Exchange(ctx, server, msg)
		if err != nil || pkt.Msg == nil {
			continue
		}
		var nameservers []DelegationNS
		for _, ans := range pkt.Msg.Answer {
			ns, ok := ans.(*dns.NS)
			if !ok {
				continue
			}
			nsName := strings.TrimSuffix(ns.Ns, ".")
			addrs, err := res.hostResolver().LookupHost(ctx, nsName)
			if err != nil || len(addrs) == 0 {
				nameservers = append(nameservers, DelegationNS{NS: nsName})
			} else {
				for _, ip := range addrs {
					nameservers = append(nameservers, DelegationNS{NS: nsName, IP: ip})
				}
			}
		}
		if len(nameservers) > 0 {
			return nameservers
		}
		// Empty answer - try the next resolver.
	}
	return []DelegationNS{}
}

// lookupDS queries the system resolver for DS records.
func lookupDS(ctx context.Context, domain string, res lookupResolvers) []DelegationDS {
	msg := transport.BuildQuery(domain, dns.TypeDS)
	msg.RecursionDesired = true

	c := &transport.Client{
		Timeout:  5 * time.Second,
		Fallback: true,
		DNSSEC:   true,
		EDNSSize: 4096,
	}

	servers := res.addresses()
	for _, server := range servers {
		pkt, err := c.Exchange(ctx, server, msg)
		if err != nil || pkt.Msg == nil {
			continue
		}
		var records []DelegationDS
		for _, ans := range pkt.Msg.Answer {
			if ds, ok := ans.(*dns.DS); ok {
				records = append(records, DelegationDS{
					KeyTag:    int(ds.KeyTag),
					Algorithm: int(ds.Algorithm),
					DigType:   int(ds.DigestType),
					Digest:    strings.ToLower(ds.Digest),
				})
			}
		}
		if len(records) > 0 {
			return records
		}
		// Empty answer - try the next resolver.
	}

	return []DelegationDS{}
}

// resolverAddresses returns DNS server addresses: local resolvers from
// /etc/resolv.conf followed by public fallbacks (8.8.8.8, 1.1.1.1).
func resolverAddresses() []string {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return []string{"8.8.8.8:53", "1.1.1.1:53"}
	}
	defer f.Close()

	var addrs []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "nameserver") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			addrs = append(addrs, net.JoinHostPort(fields[1], "53"))
		}
	}
	// Always append public resolvers as fallbacks.
	addrs = append(addrs, "8.8.8.8:53", "1.1.1.1:53")
	return addrs
}
