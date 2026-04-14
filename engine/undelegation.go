package engine

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/normalization"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

var undelegatedDigestHexPattern = regexp.MustCompile(`^[0-9A-Fa-f]+$`)

var undelegatedDigestLengths = map[int]bool{
	40: true,
	64: true,
	96: true,
}

// UndelegatedNameserver represents one undelegated nameserver input row.
// Name is required and IP is optional.
type UndelegatedNameserver struct {
	// Name is the authoritative nameserver host name.
	Name string
	// IP optionally pins the nameserver to a specific address.
	IP string
}

// UndelegatedDSInfo represents one undelegated DS input row.
type UndelegatedDSInfo struct {
	// KeyTag is the DS key tag value.
	KeyTag int
	// Algorithm is the DNSSEC algorithm number.
	Algorithm int
	// DigestType is the DS digest type number.
	DigestType int
	// Digest is the uppercase hexadecimal digest text.
	Digest string
}

// ParseUndelegatedNameserver parses "name[/ip]" into an undelegated nameserver.
func ParseUndelegatedNameserver(spec string) (UndelegatedNameserver, error) {
	trimmed := normalization.TrimSpace(spec)
	if trimmed == "" {
		return UndelegatedNameserver{}, fmt.Errorf("undelegated nameserver: value is empty")
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) > 2 {
		return UndelegatedNameserver{}, fmt.Errorf("undelegated nameserver: expected name or name/ip")
	}

	item := UndelegatedNameserver{
		Name: normalization.TrimSpace(parts[0]),
	}
	if len(parts) == 2 {
		item.IP = normalization.TrimSpace(parts[1])
		if item.IP == "" {
			return UndelegatedNameserver{}, fmt.Errorf("undelegated nameserver: IP is empty")
		}
	}

	normalized, err := normalizeUndelegatedNameserver(item, -1)
	if err != nil {
		return UndelegatedNameserver{}, err
	}
	return normalized, nil
}

// ParseUndelegatedDS parses "keytag,algorithm,digtype,digest" into DS info.
func ParseUndelegatedDS(spec string) (UndelegatedDSInfo, error) {
	trimmed := normalization.TrimSpace(spec)
	if trimmed == "" {
		return UndelegatedDSInfo{}, fmt.Errorf("undelegated DS: value is empty")
	}

	parts := strings.Split(trimmed, ",")
	if len(parts) != 4 {
		return UndelegatedDSInfo{}, fmt.Errorf("undelegated DS: expected keytag,algorithm,digtype,digest")
	}

	keyTag, err := parseBoundedInt(normalization.TrimSpace(parts[0]), "keytag", 0, 65535)
	if err != nil {
		return UndelegatedDSInfo{}, fmt.Errorf("undelegated DS: %w", err)
	}
	algorithm, err := parseBoundedInt(normalization.TrimSpace(parts[1]), "algorithm", 0, 255)
	if err != nil {
		return UndelegatedDSInfo{}, fmt.Errorf("undelegated DS: %w", err)
	}
	digestType, err := parseBoundedInt(normalization.TrimSpace(parts[2]), "digtype", 0, 255)
	if err != nil {
		return UndelegatedDSInfo{}, fmt.Errorf("undelegated DS: %w", err)
	}
	digest := strings.ToUpper(normalization.TrimSpace(parts[3]))
	if err := validateDigest(digest); err != nil {
		return UndelegatedDSInfo{}, fmt.Errorf("undelegated DS: %w", err)
	}

	return UndelegatedDSInfo{
		KeyTag:     keyTag,
		Algorithm:  algorithm,
		DigestType: digestType,
		Digest:     digest,
	}, nil
}

// NormalizeUndelegatedInputs validates and normalizes structured undelegated inputs.
func NormalizeUndelegatedInputs(nameservers []UndelegatedNameserver, dsInfo []UndelegatedDSInfo) ([]UndelegatedNameserver, []UndelegatedDSInfo, error) {
	normalizedNameservers := make([]UndelegatedNameserver, 0, len(nameservers))
	seenNameservers := map[string]bool{}
	for i, item := range nameservers {
		normalized, err := normalizeUndelegatedNameserver(item, i)
		if err != nil {
			return nil, nil, err
		}
		key := strings.ToLower(normalized.Name) + "|" + normalized.IP
		if seenNameservers[key] {
			continue
		}
		seenNameservers[key] = true
		normalizedNameservers = append(normalizedNameservers, normalized)
	}

	normalizedDSInfo := make([]UndelegatedDSInfo, 0, len(dsInfo))
	seenDS := map[string]bool{}
	for i, item := range dsInfo {
		normalized, err := normalizeUndelegatedDSInfo(item, i)
		if err != nil {
			return nil, nil, err
		}
		key := fmt.Sprintf("%d|%d|%d|%s", normalized.KeyTag, normalized.Algorithm, normalized.DigestType, normalized.Digest)
		if seenDS[key] {
			continue
		}
		seenDS[key] = true
		normalizedDSInfo = append(normalizedDSInfo, normalized)
	}

	return normalizedNameservers, normalizedDSInfo, nil
}

func normalizeUndelegatedNameserver(item UndelegatedNameserver, index int) (UndelegatedNameserver, error) {
	scope := undelegatedNameserverScope(index)

	name := normalization.TrimSpace(item.Name)
	if name == "" {
		return UndelegatedNameserver{}, fmt.Errorf("%s: name is required", scope)
	}
	errs, normalizedName := normalization.NormalizeName(name)
	if len(errs) > 0 {
		return UndelegatedNameserver{}, fmt.Errorf("%s: invalid name %q: %s", scope, name, errs[0].Message())
	}
	if normalizedName == "." {
		return UndelegatedNameserver{}, fmt.Errorf("%s: root label is not allowed", scope)
	}

	ip := normalization.TrimSpace(item.IP)
	if ip != "" {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return UndelegatedNameserver{}, fmt.Errorf("%s: invalid IP %q", scope, item.IP)
		}
		ip = addr.String()
	}

	return UndelegatedNameserver{Name: normalizedName, IP: ip}, nil
}

func normalizeUndelegatedDSInfo(item UndelegatedDSInfo, index int) (UndelegatedDSInfo, error) {
	scope := undelegatedDSScope(index)

	if item.KeyTag < 0 || item.KeyTag > 65535 {
		return UndelegatedDSInfo{}, fmt.Errorf("%s: keytag must be between 0 and 65535", scope)
	}
	if item.Algorithm < 0 || item.Algorithm > 255 {
		return UndelegatedDSInfo{}, fmt.Errorf("%s: algorithm must be between 0 and 255", scope)
	}
	if item.DigestType < 0 || item.DigestType > 255 {
		return UndelegatedDSInfo{}, fmt.Errorf("%s: digtype must be between 0 and 255", scope)
	}

	digest := strings.ToUpper(normalization.TrimSpace(item.Digest))
	if err := validateDigest(digest); err != nil {
		return UndelegatedDSInfo{}, fmt.Errorf("%s: %w", scope, err)
	}

	return UndelegatedDSInfo{
		KeyTag:     item.KeyTag,
		Algorithm:  item.Algorithm,
		DigestType: item.DigestType,
		Digest:     digest,
	}, nil
}

func validateDigest(digest string) error {
	if digest == "" {
		return fmt.Errorf("digest is required")
	}
	if !undelegatedDigestHexPattern.MatchString(digest) {
		return fmt.Errorf("digest must be hexadecimal")
	}
	if !undelegatedDigestLengths[len(digest)] {
		return fmt.Errorf("digest length must be 40, 64, or 96")
	}
	return nil
}

func parseBoundedInt(value string, field string, min int, max int) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("%s must be between %d and %d", field, min, max)
	}
	return parsed, nil
}

func undelegatedNameserverScope(index int) string {
	if index >= 0 {
		return fmt.Sprintf("undelegated nameserver[%d]", index)
	}
	return "undelegated nameserver"
}

func undelegatedDSScope(index int) string {
	if index >= 0 {
		return fmt.Sprintf("undelegated DS[%d]", index)
	}
	return "undelegated DS"
}

type undelegatedAddressLookup func(context.Context, string) ([]netip.Addr, error)

type undelegatedTagEmitter func(tag string, args map[string]any) error

func applyUndelegatedDelegation(ctx context.Context, r *recursor.Recursor, z *zone.Zone, nameservers []UndelegatedNameserver, dsInfo []UndelegatedDSInfo) error {
	if len(nameservers) == 0 && len(dsInfo) == 0 {
		return nil
	}
	if r == nil {
		return fmt.Errorf("undelegated: recursor is nil")
	}
	if z == nil {
		return fmt.Errorf("undelegated: zone is nil")
	}

	emit := func(tag string, args map[string]any) error {
		_, err := util.Info(ctx, tag, args)
		return err
	}
	var delegation map[string][]string
	if len(nameservers) > 0 {
		var err error
		delegation, err = buildUndelegatedFakeDelegation(ctx, z.Name, nameservers, r.GetAddressesFor, emit)
		if err != nil {
			return err
		}
		if err := r.AddFakeAddresses(z.Name.String(), delegation); err != nil {
			return err
		}
	}

	parentNS, err := undelegatedParentNameservers(ctx, r, z)
	if err != nil {
		return err
	}
	if len(parentNS) == 0 {
		return nil
	}

	if len(delegation) > 0 {
		for _, ns := range parentNS {
			if err := ns.AddFakeDelegation(z.Name.String(), delegation); err != nil {
				return err
			}
		}
	}

	if err := applyUndelegatedDS(parentNS, z.Name.String(), dsInfo); err != nil {
		return err
	}
	return nil
}

func undelegatedParentNameservers(ctx context.Context, r *recursor.Recursor, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if r == nil {
		return nil, fmt.Errorf("undelegated: recursor is nil")
	}
	if z == nil {
		return nil, fmt.Errorf("undelegated: zone is nil")
	}

	parentName, _, err := r.Parent(ctx, z.Name.String())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(parentName) == "" {
		return nil, nil
	}

	parentZone, err := zone.NewWithRecursor(parentName, r)
	if err != nil {
		return nil, err
	}
	return parentZone.NS(ctx)
}

func applyUndelegatedDS(parentNS []nameserver.Nameserver, domain string, dsInfo []UndelegatedDSInfo) error {
	if len(parentNS) == 0 || len(dsInfo) == 0 {
		return nil
	}

	dsRecords := buildUndelegatedDSData(dsInfo)
	for _, ns := range parentNS {
		if err := ns.AddFakeDS(domain, dsRecords); err != nil {
			return err
		}
	}
	return nil
}

func buildUndelegatedDSData(dsInfo []UndelegatedDSInfo) []nameserver.DSData {
	if len(dsInfo) == 0 {
		return nil
	}

	out := make([]nameserver.DSData, 0, len(dsInfo))
	for _, ds := range dsInfo {
		out = append(out, nameserver.DSData{
			KeyTag:     uint16(ds.KeyTag),
			Algorithm:  uint8(ds.Algorithm),
			DigestType: uint8(ds.DigestType),
			Digest:     ds.Digest,
		})
	}
	return out
}

func buildUndelegatedFakeDelegation(ctx context.Context, zoneName dnsname.Name, nameservers []UndelegatedNameserver, lookup undelegatedAddressLookup, emit undelegatedTagEmitter) (map[string][]string, error) {
	out := map[string][]string{}
	seen := map[string]map[string]bool{}

	for _, item := range nameservers {
		nameKey := strings.ToLower(item.Name)
		if seen[nameKey] == nil {
			seen[nameKey] = map[string]bool{}
		}
		if _, ok := out[nameKey]; !ok {
			out[nameKey] = []string{}
		}
		if item.IP == "" {
			continue
		}
		if seen[nameKey][item.IP] {
			continue
		}
		seen[nameKey][item.IP] = true
		out[nameKey] = append(out[nameKey], item.IP)
	}

	names := make([]string, 0, len(out))
	for name := range out {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if len(out[name]) > 0 {
			continue
		}
		nameObj := dnsname.New(name)
		if zoneName.IsInBailiwick(nameObj) {
			if emit != nil {
				if err := emit("FAKE_DELEGATION_IN_ZONE_NO_IP", map[string]any{
					"domain": zoneName.String(),
					"ns":     name,
				}); err != nil {
					return nil, err
				}
			}
			continue
		}
		if lookup != nil {
			addrs, err := lookup(ctx, name)
			if err == nil {
				for _, addr := range addrs {
					ip := addr.String()
					if seen[name][ip] {
						continue
					}
					seen[name][ip] = true
					out[name] = append(out[name], ip)
				}
			}
		}
		if len(out[name]) == 0 && emit != nil {
			if err := emit("FAKE_DELEGATION_NO_IP", map[string]any{
				"domain": zoneName.String(),
				"ns":     name,
			}); err != nil {
				return nil, err
			}
		}
	}

	return out, nil
}

func fakeDelegationToSelf(ns nameserver.Nameserver, delegation map[string][]string) bool {
	nsName := strings.ToLower(ns.Name.String())
	ips := delegation[nsName]
	if len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			continue
		}
		if addr == ns.Address {
			return true
		}
	}
	return false
}
