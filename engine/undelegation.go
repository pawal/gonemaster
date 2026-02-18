package engine

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/normalization"
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
	Name string
	IP   string
}

// UndelegatedDSInfo represents one undelegated DS input row.
type UndelegatedDSInfo struct {
	KeyTag     int
	Algorithm  int
	DigestType int
	Digest     string
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
