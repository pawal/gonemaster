package server

import (
	"time"

	"codeberg.org/pawal/gonemaster/server/extdata"
)

// PublicAnalysisDomainRegistry is live registration data for a domain. It is
// not part of the snapshot: it carries its own fetch time and can change
// between requests for the same snapshot.
type PublicAnalysisDomainRegistry struct {
	// State is one of fresh, stale, pending or unavailable.
	State            string     `json:"state"`
	FetchedAt        *time.Time `json:"fetched_at,omitempty"`
	SourceURL        string     `json:"source_url,omitempty"`
	Handle           string     `json:"handle,omitempty"`
	Status           []string   `json:"status,omitempty"`
	Registrar        string     `json:"registrar,omitempty"`
	RegistryOrg      string     `json:"registry_org,omitempty"`
	RegisteredAt     *time.Time `json:"registered_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	ChangedAt        *time.Time `json:"changed_at,omitempty"`
	Nameservers      []string   `json:"nameservers,omitempty"`
	DelegationSigned *bool      `json:"delegation_signed,omitempty"`
}

// attachRegistryData adds the RDAP endpoint and the registration summary to
// a domain detail. Both are omitted entirely when the provider is disabled.
func (s *Server) attachRegistryData(detail *PublicAnalysisDomainDetail, domain string) {
	provider := s.registry
	if provider == nil || !provider.Enabled() {
		return
	}
	if url, state := provider.RDAPDomainURL(domain); state == extdata.StateFresh {
		detail.RDAPURL = url
	}
	summary, fetchedAt, state := provider.RDAPDomain(domain)
	registry := &PublicAnalysisDomainRegistry{State: string(state)}
	if summary != nil {
		registry.SourceURL = summary.SourceURL
		registry.Handle = summary.Handle
		registry.Status = summary.Status
		registry.Registrar = summary.Registrar
		registry.RegistryOrg = summary.RegistryOrg
		registry.RegisteredAt = copyTimePtr(summary.RegisteredAt)
		registry.ExpiresAt = copyTimePtr(summary.ExpiresAt)
		registry.ChangedAt = copyTimePtr(summary.ChangedAt)
		registry.Nameservers = summary.Nameservers
		if summary.DelegationSigned != nil {
			signed := *summary.DelegationSigned
			registry.DelegationSigned = &signed
		}
		if !fetchedAt.IsZero() {
			at := fetchedAt
			registry.FetchedAt = &at
		}
	}
	detail.Registry = registry
}

func copyTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	out := *t
	return &out
}
