package extdata

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ianaRDAPBase serves RDAP records for top-level domains.
const ianaRDAPBase = "https://rdap.iana.org/"

// urlState says whether a record's fetch URL could be resolved.
type urlState int

const (
	urlReady urlState = iota
	// urlPending: the bootstrap file is not loaded yet.
	urlPending
	// urlUnavailable: no registry serves RDAP for this name.
	urlUnavailable
)

// RDAPDomainSummary is the subset of an RDAP domain object the dashboard
// renders. The raw response is not kept.
type RDAPDomainSummary struct {
	Handle           string     `json:"handle,omitempty"`
	LDHName          string     `json:"ldh_name,omitempty"`
	Status           []string   `json:"status,omitempty"`
	Registrar        string     `json:"registrar,omitempty"`
	RegistryOrg      string     `json:"registry_org,omitempty"`
	RegisteredAt     *time.Time `json:"registered_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	ChangedAt        *time.Time `json:"changed_at,omitempty"`
	Nameservers      []string   `json:"nameservers,omitempty"`
	DelegationSigned *bool      `json:"delegation_signed,omitempty"`
	SourceURL        string     `json:"source_url,omitempty"`
}

// RDAPDomainURL returns the RDAP endpoint for domain and the state of the
// lookup that resolved it. The URL is safe to hand to a browser.
func (p *Provider) RDAPDomainURL(domain string) (string, State) {
	if !p.Enabled() {
		return "", StateDisabled
	}
	key := RDAPDomainKey(domain)
	if strings.TrimPrefix(key, recordPrefix) == "" {
		return "", StateUnavailable
	}
	raw, st := p.recordURL(key)
	switch st {
	case urlReady:
		return raw, StateFresh
	case urlPending:
		// Schedule the bootstrap through Lookup so its negative TTL applies.
		p.Lookup(datasetKey(datasetRDAPBootstrap))
		return "", StatePending
	default:
		return "", StateUnavailable
	}
}

// RDAPDomain returns the cached RDAP summary for domain, when it was
// fetched, and the lookup state. A miss or a stale entry schedules a fetch.
func (p *Provider) RDAPDomain(domain string) (*RDAPDomainSummary, time.Time, State) {
	item, state := p.Lookup(RDAPDomainKey(domain))
	summary, _ := item.Value.(*RDAPDomainSummary)
	return summary, item.FetchedAt, state
}

// recordURL resolves the fetch URL for a record key: IANA for a top-level
// domain, the bootstrap base URL otherwise.
func (p *Provider) recordURL(key string) (string, urlState) {
	name := strings.TrimPrefix(key, recordPrefix)
	if name == "" {
		return "", urlUnavailable
	}
	if !strings.Contains(name, ".") {
		return ianaRDAPBase + "domain/" + url.PathEscape(name), urlReady
	}
	value, loaded := p.cachedValue(datasetKey(datasetRDAPBootstrap))
	if !loaded {
		return "", urlPending
	}
	bs, ok := value.(*RDAPBootstrap)
	if !ok {
		return "", urlUnavailable
	}
	tld := name[strings.LastIndex(name, ".")+1:]
	bases := bs.BaseURLs(tld)
	if len(bases) == 0 {
		return "", urlUnavailable
	}
	return bases[0] + "domain/" + url.PathEscape(name), urlReady
}

// parseRDAPDomain builds a summary from an RDAP domain response.
func parseRDAPDomain(body []byte, sourceURL string) (any, error) {
	var raw struct {
		Handle  string   `json:"handle"`
		LDHName string   `json:"ldhName"`
		Status  []string `json:"status"`
		Events  []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
		Entities []struct {
			Roles      []string `json:"roles"`
			VCardArray []any    `json:"vcardArray"`
		} `json:"entities"`
		Nameservers []struct {
			LDHName string `json:"ldhName"`
		} `json:"nameservers"`
		SecureDNS *struct {
			DelegationSigned *bool `json:"delegationSigned"`
		} `json:"secureDNS"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse rdap domain: %w", err)
	}
	out := &RDAPDomainSummary{
		Handle:    cleanText(raw.Handle),
		LDHName:   cleanText(raw.LDHName),
		SourceURL: sourceURL,
	}
	for _, s := range raw.Status {
		if v := cleanText(s); v != "" && len(out.Status) < maxListItems {
			out.Status = append(out.Status, v)
		}
	}
	for _, ev := range raw.Events {
		at, err := parseRDAPDate(ev.Date)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(ev.Action)) {
		case "registration":
			out.RegisteredAt = at
		case "expiration":
			out.ExpiresAt = at
		case "last changed":
			out.ChangedAt = at
		}
	}
	var registrant string
	for _, ent := range raw.Entities {
		name := entityName(ent.VCardArray)
		if name == "" {
			continue
		}
		for _, role := range ent.Roles {
			switch strings.ToLower(strings.TrimSpace(role)) {
			case "registrar":
				if out.Registrar == "" {
					out.Registrar = name
				}
			case "registry", "sponsor":
				if out.RegistryOrg == "" {
					out.RegistryOrg = name
				}
			case "registrant":
				if registrant == "" {
					registrant = name
				}
			}
		}
	}
	// IANA's TLD records carry the sponsoring organisation as registrant.
	if out.RegistryOrg == "" {
		out.RegistryOrg = registrant
	}
	for _, ns := range raw.Nameservers {
		if v := cleanText(normalizeDomain(ns.LDHName)); v != "" && len(out.Nameservers) < maxListItems {
			out.Nameservers = append(out.Nameservers, v)
		}
	}
	if raw.SecureDNS != nil && raw.SecureDNS.DelegationSigned != nil {
		signed := *raw.SecureDNS.DelegationSigned
		out.DelegationSigned = &signed
	}
	return out, nil
}

// parseRDAPDate accepts the RFC 3339 timestamps RDAP servers publish.
func parseRDAPDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("empty date")
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			utc := t.UTC()
			return &utc, nil
		}
	}
	return nil, fmt.Errorf("unrecognized date %q", value)
}

// entityName returns an entity's display name from its vCard, preferring
// the formatted name over the organisation.
func entityName(vcard []any) string {
	if fn := vcardField(vcard, "fn"); fn != "" {
		return fn
	}
	return vcardField(vcard, "org")
}

// vcardField reads one jCard property value.
func vcardField(vcard []any, field string) string {
	if len(vcard) < 2 {
		return ""
	}
	props, ok := vcard[1].([]any)
	if !ok {
		return ""
	}
	for _, prop := range props {
		entry, ok := prop.([]any)
		if !ok || len(entry) < 4 {
			continue
		}
		name, _ := entry[0].(string)
		if !strings.EqualFold(name, field) {
			continue
		}
		if v := vcardValue(entry[3]); v != "" {
			return v
		}
	}
	return ""
}

func vcardValue(value any) string {
	switch v := value.(type) {
	case string:
		return cleanText(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		return cleanText(strings.Join(parts, ", "))
	}
	return ""
}

const (
	maxTextLength = 200
	maxListItems  = 20
)

// cleanText bounds and strips control characters from third-party text.
func cleanText(value string) string {
	out := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if len(out) > maxTextLength {
		out = strings.TrimSpace(out[:maxTextLength])
	}
	return out
}
