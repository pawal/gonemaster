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

	info := lookupDelegation(ctx, domain)
	writeJSON(w, http.StatusOK, info)
}

// lookupDelegation queries DNS for NS and DS records of a domain.
func lookupDelegation(ctx context.Context, domain string) DelegationInfo {
	info := DelegationInfo{
		Nameservers: []DelegationNS{},
		DSRecords:   []DelegationDS{},
	}

	// NS lookup via system resolver.
	nss, err := net.DefaultResolver.LookupNS(ctx, domain)
	if err == nil {
		for _, ns := range nss {
			nsName := strings.TrimSuffix(ns.Host, ".")
			ips, err := net.DefaultResolver.LookupHost(ctx, nsName)
			if err != nil || len(ips) == 0 {
				info.Nameservers = append(info.Nameservers, DelegationNS{NS: nsName})
			} else {
				for _, ip := range ips {
					info.Nameservers = append(info.Nameservers, DelegationNS{NS: nsName, IP: ip})
				}
			}
		}
	}

	// DS lookup via DNS wire query (system resolver, DNSSEC-enabled).
	info.DSRecords = lookupDS(ctx, domain)

	return info
}

// lookupDS queries the system resolver for DS records.
func lookupDS(ctx context.Context, domain string) []DelegationDS {
	msg := transport.BuildQuery(domain, dns.TypeDS)
	msg.RecursionDesired = true

	c := &transport.Client{
		Timeout:  5 * time.Second,
		Fallback: true,
		DNSSEC:   true,
		EDNSSize: 4096,
	}

	servers := resolverAddresses()
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
		if records == nil {
			return []DelegationDS{}
		}
		return records
	}

	return []DelegationDS{}
}

// resolverAddresses returns DNS server addresses from /etc/resolv.conf.
func resolverAddresses() []string {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return []string{"127.0.0.1:53"}
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
	if len(addrs) > 0 {
		return addrs
	}
	return []string{"127.0.0.1:53"}
}
