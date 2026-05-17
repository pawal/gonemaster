package cache

import (
	"fmt"
	"strconv"
	"strings"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
)

// KeyParts defines the granularity for global DNS query caching.
// The key is intentionally sensitive to transport and EDNS settings,
// since they can affect response size, truncation, and DNSSEC behavior.
type KeyParts struct {
	// ServerAddr is the remote nameserver address.
	ServerAddr string
	// Name is the queried owner name.
	Name string
	// Qtype is the queried RR type.
	Qtype string
	// Qclass is the queried RR class.
	Qclass string
	// DNSSEC records whether DNSSEC behavior was enabled.
	DNSSEC bool
	// Recurse records whether the RD bit was set.
	Recurse bool
	// UseVC records whether TCP transport was used.
	UseVC bool

	// EDNSSize is the advertised EDNS UDP payload size.
	EDNSSize int
	// EDNSVersion is the EDNS version override.
	EDNSVersion *uint8
	// EDNSZ is the raw EDNS Z flag value.
	EDNSZ *uint16
	// EDNSRcode is the EDNS extended response code.
	EDNSRcode *uint8
	// EDNSData is the EDNS0 option list affecting the response shape.
	EDNSData []dns.EDNS0
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
		"NAME=" + nameObj.String(),
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
