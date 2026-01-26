package constants

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strings"

	"github.com/pawal/gonemaster/share"
)

const (
	AlgoStatusDeprecated     = 1
	AlgoStatusPrivate        = 4
	AlgoStatusReserved       = 2
	AlgoStatusUnassigned     = 3
	AlgoStatusOther          = 5
	AlgoStatusNotZoneSign    = 8
	AlgoStatusNotRecommended = 9
)

const (
	BlacklistingEnabled = true

	CNAMEMaxChainLength = 10
	CNAMEMaxRecords     = 9

	Duration5MinutesInSeconds = 5 * 60
	Duration1HourInSeconds    = 60 * 60
	Duration4HoursInSeconds   = 4 * 60 * 60
	Duration12HoursInSeconds  = 12 * 60 * 60
	Duration1DayInSeconds     = 24 * 60 * 60
	Duration1WeekInSeconds    = 7 * 24 * 60 * 60
	Duration180DaysInSeconds  = 180 * 24 * 60 * 60

	FQDNMaxLength  = 254
	LabelMaxLength = 63

	IPVersion4 = 4
	IPVersion6 = 6

	MinimumNumberOfNameservers = 2

	SerialBits         = 32
	SerialMaxVariation = 0

	UDPPayloadLimit             = 512
	EDNSUDPPayloadDefault       = 512
	EDNSUDPPayloadCommonLimit   = 4096
	EDNSUDPPayloadDNSSECDefault = 1232
)

type SpecialIPBlock struct {
	Prefix            netip.Prefix
	Name              string
	Reference         string
	GloballyReachable string
}

var (
	IPv4SpecialAddresses []SpecialIPBlock
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
