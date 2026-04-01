package asnlookup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/util"
)

// Resolver defines the DNS recursion interface used by ASN lookup.
type Resolver interface {
	Recurse(ctx context.Context, name string, qtype string, qclass string) (packet.Packet, error)
}

// Result captures ASN lookup data and status.
type Result struct {
	ASNs   []int
	Prefix *netip.Prefix
	Raw    string
	Code   string
}

const (
	// CodeFound indicates that one or more ASNs were found for the query IP.
	CodeFound = "AS_FOUND"
	// CodeEmpty indicates that the source returned no ASN mapping for the query IP.
	CodeEmpty = "EMPTY_ASN_SET"
	// CodeError indicates that ASN lookup failed across all configured sources.
	CodeError = "ERROR_ASN_DATABASE"
)

var errTryNext = errors.New("asn lookup: try next source")
var cymruSplit = regexp.MustCompile(`\s+\|\s*`)

// Get returns the list of ASNs for an IP address.
func Get(ctx context.Context, resolver Resolver, ip netip.Addr) ([]int, error) {
	result, err := GetWithPrefix(ctx, resolver, ip)
	if err != nil {
		return nil, err
	}
	if result.ASNs == nil {
		return nil, nil
	}
	return result.ASNs, nil
}

// GetWithPrefix returns ASN list, prefix, raw response, and status code.
func GetWithPrefix(ctx context.Context, resolver Resolver, ip netip.Addr) (Result, error) {
	if resolver == nil {
		return Result{}, fmt.Errorf("missing resolver")
	}
	if !ip.IsValid() {
		return Result{}, fmt.Errorf("invalid IP address")
	}

	prof := profile.FromContext(ctx)
	style := prof.ASNDB.Style
	if style == "" {
		return Result{}, fmt.Errorf("asn database style undefined")
	}
	sources := prof.ASNDB.Sources[style]
	if len(sources) == 0 {
		return Result{}, fmt.Errorf("asn database sources undefined")
	}

	for idx, source := range sources {
		var result Result
		var err error
		switch style {
		case "cymru":
			result, err = lookupCymru(ctx, resolver, ip, source)
		case "ripe":
			result, err = lookupRipe(ctx, ip, source)
		default:
			return Result{}, fmt.Errorf("asn database style value %q is illegal", style)
		}
		if errors.Is(err, errTryNext) {
			if idx == len(sources)-1 {
				return Result{Code: CodeError}, nil
			}
			continue
		}
		if err != nil {
			return Result{}, err
		}
		return result, nil
	}

	return Result{Code: CodeError}, nil
}

func lookupCymru(ctx context.Context, resolver Resolver, ip netip.Addr, source string) (Result, error) {
	if _, err := util.LoggerFromContext(ctx).Add("ASN_LOOKUP_SOURCE", map[string]any{"source": source}, "", ""); err != nil {
		return Result{}, err
	}

	reverse := strings.ToLower(dnsutil.ReverseAddr(ip))

	suffix := ""
	replacement := ""
	if ip.Is4() {
		suffix = "in-addr.arpa."
		replacement = "origin." + source
	} else if ip.Is6() {
		suffix = "ip6.arpa."
		replacement = "origin6." + source
	} else {
		return Result{}, fmt.Errorf("unsupported IP version")
	}

	if !strings.HasSuffix(reverse, suffix) {
		return Result{}, fmt.Errorf("unexpected reverse address %q", reverse)
	}

	query := strings.TrimSuffix(reverse, suffix) + replacement
	resp, err := resolver.Recurse(ctx, query, "TXT", "IN")
	if err != nil || resp.Msg == nil {
		return Result{}, errTryNext
	}

	rcode := strings.ToUpper(resp.Rcode())
	switch rcode {
	case "NXDOMAIN":
		soaRecords := resp.GetRecords("SOA", "authority")
		if len(soaRecords) == 1 {
			if soa, ok := soaRecords[0].(*dns.SOA); ok {
				ownerName := dnsname.New(soa.Hdr.Name)
				owner := ownerName.String()
				sourceNameObj := dnsname.New(source)
				sourceName := sourceNameObj.String()
				if strings.EqualFold(owner, sourceName) {
					return Result{Code: CodeEmpty}, nil
				}
			}
		}
		return Result{}, errTryNext
	case "NOERROR":
		if len(resp.Answer()) == 0 {
			return Result{Code: CodeEmpty}, nil
		}

		txtRecords := resp.GetRecords("TXT", "answer")
		if len(txtRecords) == 0 {
			return Result{Code: CodeError}, nil
		}

		maxPrefixLen := -1
		var selectedPrefix netip.Prefix
		var selectedASNs []int
		var selectedRaw string
		found := false

		for _, rr := range txtRecords {
			txt, ok := rr.(*dns.TXT)
			if !ok {
				continue
			}
			raw := strings.Join(txt.Txt, "")
			fields := cymruSplit.Split(raw, -1)
			if len(fields) <= 1 {
				continue
			}

			prefixStr := strings.TrimSpace(fields[1])
			prefix, err := netip.ParsePrefix(prefixStr)
			if err != nil {
				return Result{Code: CodeError}, nil
			}
			prefix = prefix.Masked()
			if !prefix.Contains(ip) {
				return Result{Code: CodeError}, nil
			}

			asns, err := parseASNList(fields[0])
			if err != nil {
				return Result{}, err
			}

			if prefix.Bits() > maxPrefixLen {
				maxPrefixLen = prefix.Bits()
				selectedPrefix = prefix
				selectedASNs = asns
				selectedRaw = raw
				found = true
			}
		}

		if !found {
			return Result{Code: CodeEmpty}, nil
		}
		if !selectedPrefix.Contains(ip) {
			return Result{Code: CodeError}, nil
		}

		return Result{ASNs: selectedASNs, Prefix: &selectedPrefix, Raw: selectedRaw, Code: CodeFound}, nil
	default:
		return Result{}, errTryNext
	}
}

func lookupRipe(ctx context.Context, ip netip.Addr, source string) (Result, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(source, "43"))
	if err != nil {
		return Result{}, errTryNext
	}
	defer conn.Close()

	if _, err := fmt.Fprintf(conn, "-F -M %s\n", ip.String()); err != nil {
		return Result{}, errTryNext
	}

	scanner := bufio.NewScanner(conn)
	hasAnswer := false
	line := ""
	for scanner.Scan() {
		hasAnswer = true
		text := scanner.Text()
		text = strings.TrimRight(text, "\r\n")
		if strings.HasPrefix(text, "%") || strings.TrimSpace(text) == "" {
			continue
		}
		line = text
		break
	}
	if err := scanner.Err(); err != nil {
		return Result{}, errTryNext
	}
	if !hasAnswer {
		return Result{}, errTryNext
	}
	if line == "" {
		return Result{Code: CodeEmpty}, nil
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return Result{Code: CodeError}, nil
	}

	asnFields := strings.Split(fields[0], "/")
	asns := make([]int, 0, len(asnFields))
	for _, field := range asnFields {
		asn, err := strconv.Atoi(field)
		if err != nil {
			return Result{}, fmt.Errorf("asn lookup value isn't numeric: %q", field)
		}
		asns = append(asns, asn)
	}

	prefix, err := netip.ParsePrefix(fields[1])
	if err != nil {
		return Result{}, err
	}
	prefix = prefix.Masked()

	return Result{ASNs: asns, Prefix: &prefix, Raw: line, Code: CodeFound}, nil
}

func parseASNList(value string) ([]int, error) {
	fields := strings.Fields(value)
	asns := make([]int, 0, len(fields))
	for _, field := range fields {
		asn, err := strconv.Atoi(field)
		if err != nil {
			return nil, fmt.Errorf("asn lookup value isn't numeric: %q", field)
		}
		asns = append(asns, asn)
	}
	return asns, nil
}
