package hints

import (
	"fmt"
	"regexp"
	"strings"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

var forbiddenDirectiveRe = regexp.MustCompile(`(?m)^\$(TTL|INCLUDE|ORIGIN|GENERATE)\b`)

// ParseHints parses a root hints zone file into a map of names to IP addresses.
func ParseHints(text string) (map[string][]string, error) {
	if match := forbiddenDirectiveRe.FindStringSubmatch(text); len(match) == 2 {
		return nil, fmt.Errorf("forbidden directive $%s", match[1])
	}

	parser := dns.NewZoneParser(strings.NewReader(text), ".", "named.root")

	ns := map[string]bool{}
	glue := map[string]string{}
	var records []dns.RR

	for rr, ok := parser.Next(); ok; rr, ok = parser.Next() {
		records = append(records, rr)
		hdr := rr.Header()

		if hdr.Class != dns.ClassINET {
			return nil, fmt.Errorf("forbidden RR class %s", dns.ClassToString[hdr.Class])
		}

		switch rr := rr.(type) {
		case *dns.NS:
			if hdr.Name != "." {
				return nil, fmt.Errorf("owner name for NS record must be \".\"")
			}
			name := strings.ToLower(rr.Ns)
			ns[name] = false
		case *dns.A:
			owner := strings.ToLower(hdr.Name)
			glue[owner] = "A"
		case *dns.AAAA:
			owner := strings.ToLower(hdr.Name)
			glue[owner] = "AAAA"
		default:
			return nil, fmt.Errorf("forbidden RR type %s", dnsutil.TypeToString(dns.RRToType(rr)))
		}
	}
	if err := parser.Err(); err != nil {
		return nil, fmt.Errorf("unable to parse root hints")
	}

	for owner, rrtype := range glue {
		if _, ok := ns[owner]; ok {
			ns[owner] = true
			continue
		}
		return nil, fmt.Errorf("owner name of %s record does not match any NS RDATA", rrtype)
	}

	for nsdname, ok := range ns {
		if !ok {
			return nil, fmt.Errorf("no address record found for NS %s", nsdname)
		}
	}

	if len(ns) == 0 {
		return nil, fmt.Errorf("no NS record found")
	}

	hints := map[string][]string{}
	for _, rr := range records {
		switch typed := rr.(type) {
		case *dns.A:
			owner := rr.Header().Name
			hints[owner] = append(hints[owner], typed.Addr.String())
		case *dns.AAAA:
			owner := rr.Header().Name
			hints[owner] = append(hints[owner], typed.Addr.String())
		}
	}

	return hints, nil
}
