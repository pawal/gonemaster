package extdata

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

const (
	datasetIANATLDs      = "iana_tlds"
	datasetRDAPBootstrap = "rdap_dns_bootstrap"
)

// dataset is a periodically refreshed file.
type dataset struct {
	name string
	url  func(Sources) string
	// parse returns the value and the file's own version marker.
	parse    func([]byte) (any, string, error)
	wantJSON bool
}

var datasetTable = []dataset{
	{
		name:  datasetIANATLDs,
		url:   func(s Sources) string { return s.IANATLDs },
		parse: parseTLDList,
	},
	{
		name:     datasetRDAPBootstrap,
		url:      func(s Sources) string { return s.RDAPBootstrap },
		parse:    parseRDAPBootstrap,
		wantJSON: true,
	},
}

func datasetNames() []string {
	out := make([]string, 0, len(datasetTable))
	for _, d := range datasetTable {
		out = append(out, d.name)
	}
	return out
}

func datasetByName(name string) (dataset, bool) {
	for _, d := range datasetTable {
		if d.name == name {
			return d, true
		}
	}
	return dataset{}, false
}

// datasetSize reports how many entries a parsed dataset holds.
func datasetSize(value any) int {
	switch v := value.(type) {
	case *TLDList:
		return v.Len()
	case *RDAPBootstrap:
		return v.Len()
	}
	return 0
}

// TLDList is the set of delegated top-level domains from IANA.
type TLDList struct {
	Version string
	names   map[string]struct{}
}

// Has reports whether name is in the list. Comparison is on the A-label.
func (l *TLDList) Has(name string) bool {
	if l == nil {
		return false
	}
	_, ok := l.names[normalizeDomain(name)]
	return ok
}

// Len returns the number of listed top-level domains.
func (l *TLDList) Len() int {
	if l == nil {
		return 0
	}
	return len(l.names)
}

// Names returns the listed top-level domains in ascending order.
func (l *TLDList) Names() []string {
	if l == nil {
		return nil
	}
	out := make([]string, 0, len(l.names))
	for n := range l.names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// parseTLDList reads the IANA tlds-alpha-by-domain.txt format: one name per
// line, with a leading comment carrying the version.
func parseTLDList(body []byte) (any, string, error) {
	list := &TLDList{names: map[string]struct{}{}}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if v := versionFromComment(line); v != "" && list.Version == "" {
				list.Version = v
			}
			continue
		}
		name := normalizeDomain(line)
		if name == "" || strings.Contains(name, ".") {
			continue
		}
		list.names[name] = struct{}{}
	}
	if len(list.names) == 0 {
		return nil, "", errors.New("tld list is empty")
	}
	return list, list.Version, nil
}

// versionFromComment extracts the serial from "# Version 2026090700, ...".
func versionFromComment(line string) string {
	fields := strings.Fields(strings.TrimPrefix(line, "#"))
	for i, f := range fields {
		if strings.EqualFold(f, "version") && i+1 < len(fields) {
			return strings.TrimSuffix(fields[i+1], ",")
		}
	}
	return ""
}

// RDAPBootstrap maps a top-level domain to the RDAP base URLs serving it.
type RDAPBootstrap struct {
	Version     string
	Publication string
	byTLD       map[string][]string
}

// BaseURLs returns the RDAP base URLs registered for tld.
func (b *RDAPBootstrap) BaseURLs(tld string) []string {
	if b == nil {
		return nil
	}
	return b.byTLD[normalizeDomain(tld)]
}

// Len returns the number of top-level domains with a base URL.
func (b *RDAPBootstrap) Len() int {
	if b == nil {
		return 0
	}
	return len(b.byTLD)
}

// parseRDAPBootstrap reads RFC 9224 DNS bootstrap JSON. Only HTTPS base URLs
// are kept, because only those are ever fetched or linked.
func parseRDAPBootstrap(body []byte) (any, string, error) {
	var raw struct {
		Version     string       `json:"version"`
		Publication string       `json:"publication"`
		Services    [][][]string `json:"services"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, "", fmt.Errorf("parse rdap bootstrap: %w", err)
	}
	bs := &RDAPBootstrap{Version: raw.Version, Publication: raw.Publication, byTLD: map[string][]string{}}
	for _, service := range raw.Services {
		if len(service) < 2 {
			continue
		}
		var bases []string
		for _, candidate := range service[1] {
			base := strings.TrimSpace(candidate)
			if base == "" {
				continue
			}
			u, err := url.Parse(base)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				continue
			}
			if blockedHostReason(u.Hostname()) != "" {
				continue
			}
			bases = append(bases, strings.TrimSuffix(base, "/")+"/")
		}
		if len(bases) == 0 {
			continue
		}
		for _, tld := range service[0] {
			name := normalizeDomain(tld)
			if name == "" || strings.Contains(name, ".") {
				continue
			}
			bs.byTLD[name] = append(bs.byTLD[name], bases...)
		}
	}
	if len(bs.byTLD) == 0 {
		return nil, "", errors.New("rdap bootstrap has no usable services")
	}
	version := bs.Publication
	if version == "" {
		version = bs.Version
	}
	return bs, version, nil
}

// normalizeDomain lowercases a name and drops one trailing root dot.
func normalizeDomain(name string) string {
	out := strings.ToLower(strings.TrimSpace(name))
	return strings.TrimSuffix(out, ".")
}

// TLDs returns the cached IANA top-level domain list and its state.
func (p *Provider) TLDs() (*TLDList, State) {
	item, state := p.Lookup(datasetKey(datasetIANATLDs))
	list, _ := item.Value.(*TLDList)
	return list, state
}

// Bootstrap returns the cached RDAP bootstrap table and its state.
func (p *Provider) Bootstrap() (*RDAPBootstrap, State) {
	item, state := p.Lookup(datasetKey(datasetRDAPBootstrap))
	bs, _ := item.Value.(*RDAPBootstrap)
	return bs, state
}
