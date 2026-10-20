package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/normalization"
)

// Request is one run request; its JSON form is the contract.
type Request struct {
	Domain    string   `json:"domain"`
	Module    string   `json:"module,omitempty"`
	Testcases []string `json:"testcases,omitempty"`
	Exclude   []string `json:"exclude,omitempty"`
	// Profile is a JSON profile document applied over the defaults.
	Profile                json.RawMessage         `json:"profile,omitempty"`
	MinLevel               string                  `json:"min_level,omitempty"`
	IPv4                   *bool                   `json:"ipv4,omitempty"`
	IPv6                   *bool                   `json:"ipv6,omitempty"`
	Resolver               Resolver                `json:"resolver,omitzero"`
	UndelegatedNameservers []UndelegatedNameserver `json:"undelegated_nameservers,omitempty"`
	UndelegatedDS          []UndelegatedDS         `json:"undelegated_ds,omitempty"`
	BadkeysPath            string                  `json:"badkeys_path,omitempty"`
	// IncludeTimings adds a timings event before done.
	IncludeTimings bool `json:"include_timings,omitempty"`
}

// Resolver overrides resolver settings of the profile.
type Resolver struct {
	Timeout          *int    `json:"timeout,omitempty"`
	Retry            *int    `json:"retry,omitempty"`
	Retrans          *int    `json:"retrans,omitempty"`
	Fallback         *bool   `json:"fallback,omitempty"`
	Parallel         *int    `json:"parallel,omitempty"`
	Unordered        *bool   `json:"unordered,omitempty"`
	Source4          *string `json:"source4,omitempty"`
	Source6          *string `json:"source6,omitempty"`
	ErrorCacheTTL    *int    `json:"error_cache_ttl,omitempty"`
	PositiveCacheTTL *int    `json:"positive_cache_ttl,omitempty"`
	NegativeCacheTTL *int    `json:"negative_cache_ttl,omitempty"`
}

// UndelegatedNameserver is one nameserver of an undelegated zone.
type UndelegatedNameserver struct {
	Name string `json:"name"`
	IP   string `json:"ip,omitempty"`
}

// UndelegatedDS is one DS record of an undelegated zone.
type UndelegatedDS struct {
	KeyTag     int    `json:"keytag"`
	Algorithm  int    `json:"algorithm"`
	DigestType int    `json:"digest_type"`
	Digest     string `json:"digest"`
}

// ParseRequest decodes a request; unknown fields are an error.
func ParseRequest(data []byte) (Request, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var req Request
	if err := dec.Decode(&req); err != nil {
		return Request{}, &Error{Code: CodeInvalidRequest, Message: err.Error()}
	}
	return req, nil
}

// runRequest validates the request and maps it onto the engine.
func (req Request) runRequest() (engine.RunRequest, error) {
	errs, domain := normalization.NormalizeTarget(strings.TrimSpace(req.Domain))
	if len(errs) > 0 {
		return engine.RunRequest{}, &Error{Code: CodeInvalidDomain, Message: errs[0].Message()}
	}
	minLevel := strings.ToUpper(strings.TrimSpace(req.MinLevel))
	if _, ok := logger.Levels()[minLevel]; minLevel != "" && !ok {
		return engine.RunRequest{}, &Error{Code: CodeInvalidRequest, Message: fmt.Sprintf("unknown min level %q", req.MinLevel)}
	}
	module := strings.ToLower(strings.TrimSpace(req.Module))
	if module != "" && !knownModule(module) {
		return engine.RunRequest{}, &Error{Code: CodeUnknownModule, Message: fmt.Sprintf("unknown module %q", req.Module)}
	}
	for _, pe := range CheckProfile(req.Profile) {
		if pe.Code != ProfileUnknownTestcase {
			return engine.RunRequest{}, &Error{Code: CodeInvalidProfile, Message: pe.Message}
		}
	}
	out := engine.RunRequest{
		Domain:           domain,
		Module:           module,
		Testcases:        req.Testcases,
		Exclude:          req.Exclude,
		ProfileData:      string(req.Profile),
		MinLevel:         minLevel,
		CaptureMinLevel:  minLevel,
		IPv4:             req.IPv4,
		IPv6:             req.IPv6,
		Timeout:          req.Resolver.Timeout,
		Retry:            req.Resolver.Retry,
		Retrans:          req.Resolver.Retrans,
		Fallback:         req.Resolver.Fallback,
		Parallel:         req.Resolver.Parallel,
		Unordered:        req.Resolver.Unordered,
		SourceAddr4:      req.Resolver.Source4,
		SourceAddr6:      req.Resolver.Source6,
		ErrorCacheTTL:    req.Resolver.ErrorCacheTTL,
		PositiveCacheTTL: req.Resolver.PositiveCacheTTL,
		NegativeCacheTTL: req.Resolver.NegativeCacheTTL,
	}
	if path := strings.TrimSpace(req.BadkeysPath); path != "" {
		out.BadkeysPath = &path
	}
	for _, ns := range req.UndelegatedNameservers {
		out.UndelegatedNameservers = append(out.UndelegatedNameservers, engine.UndelegatedNameserver{Name: ns.Name, IP: ns.IP})
	}
	for _, ds := range req.UndelegatedDS {
		out.UndelegatedDSInfo = append(out.UndelegatedDSInfo, engine.UndelegatedDSInfo{KeyTag: ds.KeyTag, Algorithm: ds.Algorithm, DigestType: ds.DigestType, Digest: ds.Digest})
	}
	if _, _, err := engine.NormalizeUndelegatedInputs(out.UndelegatedNameservers, out.UndelegatedDSInfo); err != nil {
		return engine.RunRequest{}, &Error{Code: CodeInvalidRequest, Message: err.Error()}
	}
	return out, nil
}
