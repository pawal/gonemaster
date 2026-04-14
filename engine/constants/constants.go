package constants

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strings"

	"codeberg.org/pawal/gonemaster/share"
)

const (
	// AlgoStatusDeprecated marks a DNSSEC algorithm as deprecated.
	AlgoStatusDeprecated = 1
	// AlgoStatusPrivate marks a DNSSEC algorithm as private-use.
	AlgoStatusPrivate = 4
	// AlgoStatusReserved marks a DNSSEC algorithm as reserved.
	AlgoStatusReserved = 2
	// AlgoStatusUnassigned marks a DNSSEC algorithm as unassigned.
	AlgoStatusUnassigned = 3
	// AlgoStatusOther marks a DNSSEC algorithm with an unspecified status.
	AlgoStatusOther = 5
	// AlgoStatusNotZoneSign marks a DNSSEC algorithm as not for zone signing.
	AlgoStatusNotZoneSign = 8
	// AlgoStatusNotRecommended marks a DNSSEC algorithm as not recommended.
	AlgoStatusNotRecommended = 9
)

const (
	// BlacklistingEnabled controls whether nameserver blacklisting is active.
	BlacklistingEnabled = true

	// CNAMEMaxChainLength is the maximum allowed CNAME chain depth.
	CNAMEMaxChainLength = 10
	// CNAMEMaxRecords is the maximum allowed number of CNAME records in a response.
	CNAMEMaxRecords = 9

	// Duration5MinutesInSeconds is 5 minutes in seconds.
	Duration5MinutesInSeconds = 5 * 60
	// Duration1HourInSeconds is 1 hour in seconds.
	Duration1HourInSeconds = 60 * 60
	// Duration4HoursInSeconds is 4 hours in seconds.
	Duration4HoursInSeconds = 4 * 60 * 60
	// Duration12HoursInSeconds is 12 hours in seconds.
	Duration12HoursInSeconds = 12 * 60 * 60
	// Duration1DayInSeconds is 1 day in seconds.
	Duration1DayInSeconds = 24 * 60 * 60
	// Duration1WeekInSeconds is 1 week in seconds.
	Duration1WeekInSeconds = 7 * 24 * 60 * 60
	// Duration180DaysInSeconds is 180 days in seconds.
	Duration180DaysInSeconds = 180 * 24 * 60 * 60

	// FQDNMaxLength is the maximum length of a fully qualified domain name.
	FQDNMaxLength = 254
	// LabelMaxLength is the maximum length of a single DNS label.
	LabelMaxLength = 63

	// IPVersion4 is the numeric identifier for IPv4.
	IPVersion4 = 4
	// IPVersion6 is the numeric identifier for IPv6.
	IPVersion6 = 6

	// MinimumNumberOfNameservers is the minimum expected number of nameservers.
	MinimumNumberOfNameservers = 2

	// SerialBits is the number of bits used by SOA serial arithmetic.
	SerialBits = 32
	// SerialMaxVariation controls permitted serial variation; zero means exact.
	SerialMaxVariation = 0

	// UDPPayloadLimit is the classic DNS UDP payload limit without EDNS.
	UDPPayloadLimit = 512
	// EDNSUDPPayloadDefault is the default EDNS UDP payload size.
	EDNSUDPPayloadDefault = 512
	// EDNSUDPPayloadCommonLimit is a commonly accepted EDNS UDP payload ceiling.
	EDNSUDPPayloadCommonLimit = 4096
	// EDNSUDPPayloadDNSSECDefault is the DNSSEC-friendly EDNS UDP payload default.
	EDNSUDPPayloadDNSSECDefault = 1232
)

// SpecialIPBlock describes one IANA special-purpose IP prefix.
type SpecialIPBlock struct {
	// Prefix is the special-purpose network prefix.
	Prefix netip.Prefix
	// Name is the IANA registry name for the prefix.
	Name string
	// Reference is the registry reference text.
	Reference string
	// GloballyReachable mirrors the IANA globally reachable flag.
	GloballyReachable string
}

var (
	// IPv4SpecialAddresses holds IANA IPv4 special-purpose address blocks.
	IPv4SpecialAddresses []SpecialIPBlock
	// IPv6SpecialAddresses holds IANA IPv6 special-purpose address blocks.
	IPv6SpecialAddresses []SpecialIPBlock
)

var ianaIPv4CSV = share.IanaIPv4CSV

var ianaIPv6CSV = share.IanaIPv6CSV

func init() {
	var err error
	IPv4SpecialAddresses, err = loadIanaSpecialBlocks(IPVersion4)
	if err != nil {
		panic(err)
	}
	IPv6SpecialAddresses, err = loadIanaSpecialBlocks(IPVersion6)
	if err != nil {
		panic(err)
	}
}

func loadIanaSpecialBlocks(ipVersion int) ([]SpecialIPBlock, error) {
	var data string
	if ipVersion == IPVersion4 {
		data = ianaIPv4CSV
	} else if ipVersion == IPVersion6 {
		data = ianaIPv6CSV
	} else {
		return nil, fmt.Errorf("unsupported IP version: %d", ipVersion)
	}

	r := csv.NewReader(strings.NewReader(data))
	r.FieldsPerRecord = -1

	var blocks []SpecialIPBlock
	first := true
	prefixRe := regexp.MustCompile(`^(.+/[0-9]+)`) // matches Perl's trimming behavior

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if first {
			first = false
			continue
		}
		if len(record) < 9 {
			return nil, fmt.Errorf("unexpected CSV row: %v", record)
		}

		addressData := strings.ReplaceAll(record[0], " ", "")
		for _, item := range strings.Split(addressData, ",") {
			if item == "" {
				continue
			}
			match := prefixRe.FindStringSubmatch(item)
			if len(match) != 2 {
				return nil, fmt.Errorf("invalid prefix data: %q", item)
			}
			prefixStr := match[1]
			prefix, err := netip.ParsePrefix(prefixStr)
			if err != nil {
				return nil, fmt.Errorf("invalid prefix %q: %w", prefixStr, err)
			}
			blocks = append(blocks, SpecialIPBlock{
				Prefix:            prefix,
				Name:              record[1],
				Reference:         record[2],
				GloballyReachable: record[8],
			})
		}
	}

	return blocks, nil
}
