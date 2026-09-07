package server

import (
	"log/slog"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/server/extdata"
)

// externalDataConfig maps the server configuration onto the provider's.
func externalDataConfig(cfg Config) extdata.Config {
	return extdata.Config{
		Enabled:              cfg.ExternalData.Enabled,
		RefreshInterval:      cfg.ExternalData.RefreshInterval.Duration,
		RecordTTL:            cfg.ExternalData.RecordTTL.Duration,
		NegativeTTL:          cfg.ExternalData.NegativeTTL.Duration,
		Timeout:              cfg.ExternalData.Timeout.Duration,
		MaxRequestsPerMinute: cfg.ExternalData.MaxRequestsPerMinute,
		MaxCachedRecords:     cfg.ExternalData.MaxCachedRecords,
		Sources: extdata.Sources{
			IANATLDs:      cfg.ExternalData.Sources.IANATLDs,
			RDAPBootstrap: cfg.ExternalData.Sources.RDAPBootstrap,
		},
		UserAgent: "gonemaster/" + engine.Version,
	}
}

// newExternalDataProvider builds the provider, or nil when the feature is
// off so no goroutine and no cache exist at all.
func newExternalDataProvider(cfg Config, logger *slog.Logger) *extdata.Provider {
	if !cfg.ExternalData.Enabled {
		return nil
	}
	pcfg := externalDataConfig(cfg)
	pcfg.Logger = logger
	return extdata.New(pcfg)
}

// ExternalData returns the reference-data provider. Nil when disabled; every
// provider method is nil-safe.
func (s *Server) ExternalData() *extdata.Provider {
	return s.extData
}

// registryLookup is the part of the provider the domain detail uses. Tests
// substitute a fake instead of reaching the network.
type registryLookup interface {
	Enabled() bool
	RDAPDomainURL(domain string) (string, extdata.State)
	RDAPDomain(domain string) (*extdata.RDAPDomainSummary, time.Time, extdata.State)
}
