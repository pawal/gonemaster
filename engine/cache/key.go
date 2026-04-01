package cache

import (
	"fmt"
	"strconv"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	dns "codeberg.org/miekg/dns"
)

// KeyParts defines the granularity for global DNS query caching.
// The key is intentionally sensitive to transport and EDNS settings,
// since they can affect response size, truncation, and DNSSEC behavior.
type KeyParts struct {
	ServerAddr string
	Name       string
	Qtype      string
	Qclass     string
	DNSSEC     bool
	Recurse    bool
	UseVC      bool

	EDNSSize    int
	EDNSVersion *uint8
	EDNSZ       *uint16
	EDNSRcode   *uint8
	EDNSData    []dns.EDNS0
}

// BuildKey returns a stable cache key string for the given parts.
func BuildKey(parts KeyParts) (string, error) {
	if parts.EDNSSize < 0 || parts.EDNSSize > 65535 {
		return "", fmt.Errorf("edns_size must be between 0 and 65535")
	}

	nameObj := dnsname.New(parts.Name)

	transport := "udp"
	if parts.UseVC {
		transport = "tcp"
	}

	qtype := strings.ToUpper(parts.Qtype)
	qclass := strings.ToUpper(parts.Qclass)
	partsList := []string{
		"SERVER=" + strings.ToLower(strings.TrimSpace(parts.ServerAddr)),
		"TRANSPORT=" + transport,
		"NAME=" + strings.ToLower(nameObj.String()),
		"TYPE=" + qtype,
		"CLASS=" + qclass,
		"DNSSEC=" + strconv.FormatBool(parts.DNSSEC),
		"RECURSE=" + strconv.FormatBool(parts.Recurse),
	}

	if parts.EDNSVersion != nil || parts.EDNSZ != nil || parts.EDNSRcode != nil || len(parts.EDNSData) > 0 {
		partsList = append(partsList,
			"EDNS_VERSION="+formatUint8(parts.EDNSVersion),
			"EDNS_Z="+formatUint16(parts.EDNSZ),
			"EDNS_RCODE="+formatUint8(parts.EDNSRcode),
			"EDNS_DATA="+formatEDNSData(parts.EDNSData),
		)
	}

	partsList = append(partsList, "EDNS_SIZE="+strconv.FormatUint(uint64(parts.EDNSSize), 10))
	return strings.Join(partsList, "|"), nil
}

func formatUint8(value *uint8) string {
	if value == nil {
		return "0"
	}
	return strconv.FormatUint(uint64(*value), 10)
}

func formatUint16(value *uint16) string {
	if value == nil {
		return "0"
	}
	return strconv.FormatUint(uint64(*value), 10)
}

func formatEDNSData(data []dns.EDNS0) string {
	if len(data) == 0 {
		return ""
	}
	return fmt.Sprint(data)
}
