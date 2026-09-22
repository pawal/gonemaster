package zone

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logargs"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/test/internal/runner"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testcase"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testlogger"
	"codeberg.org/pawal/gonemaster/engine/util"
	zonepkg "codeberg.org/pawal/gonemaster/engine/zone"
)

const moduleName = "Zone"

var (
	apexNSNames = func(ctx context.Context, z *zonepkg.Zone) ([]dnsname.Name, error) {
		return z.ApexNSNames(ctx)
	}
	apexNameservers       = nsdiscovery.ApexNameservers
	delegationNameservers = nsdiscovery.DelegationNameservers
	zoneNameservers       = nsdiscovery.ZoneNameservers
	authoritativeNS       = func(ctx context.Context, z *zonepkg.Zone) ([]nameserver.Nameserver, error) {
		items, err := nsdiscovery.ZoneNameservers(ctx, z)
		if err != nil {
			return nil, err
		}
		return nameserversFromNSItems(ctx, z, items), nil
	}
	getAddressesFor = defaultGetAddressesFor
	recurse         = defaultRecurse
	queryAuth       = defaultQueryAuth
)

var nullSpfRegex = regexp.MustCompile(`(?i)^v=spf1[ \t]+-all[ \t]*$`)

const csyncFlagSoaMinimum uint16 = 0x0002

func defaultGetAddressesFor(ctx context.Context, z *zonepkg.Zone, name string) ([]netip.Addr, error) {
	if z == nil || z.Recursor() == nil {
		return nil, fmt.Errorf("missing recursor")
	}
	return z.Recursor().GetAddressesFor(ctx, name)
}

func defaultRecurse(ctx context.Context, z *zonepkg.Zone, name string, qtype string) (packet.Packet, error) {
	if z == nil || z.Recursor() == nil {
		return packet.Packet{}, fmt.Errorf("missing recursor")
	}
	resp, err := z.Recursor().Recurse(ctx, name, qtype, "IN")
	return resp, recursor.IgnoreCNAMEError(err)
}

func defaultQueryAuth(ctx context.Context, z *zonepkg.Zone, name string, qtype string) (packet.Packet, error) {
	if z == nil {
		return packet.Packet{}, fmt.Errorf("missing zone")
	}
	return z.QueryAuth(ctx, name, qtype, nil)
}

// All runs the Zone test cases in order.
func All(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest(ctx, "zone01") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone01(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "zone02") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone02(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "zone03") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone03(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "zone04") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone04(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "zone05") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone05(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "zone06") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone06(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "zone07") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone07(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "zone08") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone08(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if util.ShouldRunTest(ctx, "zone09") && !hasEntryTag(results, "NO_RESPONSE_MX_QUERY") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone09(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if !hasEntryTag(results, "NO_RESPONSE_SOA_QUERY") {
		if util.ShouldRunTest(ctx, "zone10") {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Zone10(ctx, z)
			})
			results = append(results, entries...)
			if err != nil {
				return results, err
			}
		}
		if util.ShouldRunTest(ctx, "zone11") {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Zone11(ctx, z)
			})
			results = append(results, entries...)
			if err != nil {
				return results, err
			}
		}

		if util.ShouldRunTest(ctx, "zone12") {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Zone12(ctx, z)
			})
			results = append(results, entries...)
			if err != nil {
				return results, err
			}
		}

		// Zone13 walks a policy Zone11 accepted, or retrieves its own when
		// Zone11 was not selected.
		if util.ShouldRunTest(ctx, "zone13") && (spfPolicyAccepted(results) || !util.ShouldRunTest(ctx, "zone11")) {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Zone13(ctx, z)
			})
			results = append(results, entries...)
			if err != nil {
				return results, err
			}
		}
	}

	if util.ShouldRunTest(ctx, "zone14") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone14(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if util.ShouldRunTest(ctx, "zone15") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone15(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// Metadata returns the set of tags emitted by Zone test cases.
func Metadata() map[string][]string {
	return map[string][]string{
		"zone01": {
			"Z01_MNAME_HAS_LOCALHOST_ADDR",
			"Z01_MNAME_IS_DOT",
			"Z01_MNAME_IS_LOCALHOST",
			"Z01_MNAME_IS_MASTER",
			"Z01_MNAME_MISSING_SOA_RECORD",
			"Z01_MNAME_NO_RESPONSE",
			"Z01_MNAME_NOT_AUTHORITATIVE",
			"Z01_MNAME_NOT_IN_NS_LIST",
			"Z01_MNAME_NOT_MASTER",
			"Z01_MNAME_NOT_RESOLVE",
			"Z01_MNAME_UNEXPECTED_RCODE",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone02": {
			"REFRESH_MINIMUM_VALUE_LOWER",
			"REFRESH_MINIMUM_VALUE_OK",
			"NO_RESPONSE_SOA_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone03": {
			"REFRESH_LOWER_THAN_RETRY",
			"REFRESH_HIGHER_THAN_RETRY",
			"NO_RESPONSE_SOA_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone04": {
			"RETRY_MINIMUM_VALUE_LOWER",
			"RETRY_MINIMUM_VALUE_OK",
			"NO_RESPONSE_SOA_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone05": {
			"EXPIRE_MINIMUM_VALUE_LOWER",
			"EXPIRE_LOWER_THAN_REFRESH",
			"EXPIRE_MINIMUM_VALUE_OK",
			"NO_RESPONSE_SOA_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone06": {
			"SOA_DEFAULT_TTL_MAXIMUM_VALUE_HIGHER",
			"SOA_DEFAULT_TTL_MAXIMUM_VALUE_LOWER",
			"SOA_DEFAULT_TTL_MAXIMUM_VALUE_OK",
			"NO_RESPONSE_SOA_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone07": {
			"MNAME_IS_CNAME",
			"MNAME_IS_NOT_CNAME",
			"NO_RESPONSE_SOA_QUERY",
			"MNAME_HAS_NO_ADDRESS",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone08": {
			"MX_RECORD_IS_CNAME",
			"MX_RECORD_IS_NOT_CNAME",
			"NO_RESPONSE_MX_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone09": {
			"Z09_ARPA_EMAIL_DOMAIN",
			"Z09_INCONSISTENT_MX",
			"Z09_INCONSISTENT_MX_DATA",
			"Z09_MISSING_MAIL_TARGET",
			"Z09_MX_DATA",
			"Z09_MX_FOUND",
			"Z09_NON_AUTH_MX_RESPONSE",
			"Z09_NO_MX_FOUND",
			"Z09_NO_MX_FOUND_OR_EXPECTED",
			"Z09_NO_RESPONSE_MX_QUERY",
			"Z09_NO_SERVERS_MX_RESPONSE",
			"Z09_NULL_MX_NON_ZERO_PREF",
			"Z09_NULL_MX_WITH_OTHER_MX",
			"Z09_ROOT_EMAIL_DOMAIN",
			"Z09_TLD_EMAIL_DOMAIN",
			"Z09_UNEXPECTED_RCODE_MX",
			"Z09_VALID_NULL_MX",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone10": {
			"APEX_DNAME",
			"MULTIPLE_SOA",
			"NO_RESPONSE",
			"NO_SOA_IN_RESPONSE",
			"ONE_SOA",
			"SOA_AND_CNAME",
			"WRONG_SOA",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone11": {
			"Z11_DIFFERENT_SPF_POLICIES_FOUND",
			"Z11_INCONSISTENT_SPF_POLICIES",
			"Z11_NO_SPF_FOUND",
			"Z11_NO_SPF_NON_MAIL_DOMAIN",
			"Z11_NON_NULL_SPF_NON_MAIL_DOMAIN",
			"Z11_NULL_SPF_NON_MAIL_DOMAIN",
			"Z11_SPF_MULTIPLE_RECORDS",
			"Z11_SPF_SYNTAX_ERROR",
			"Z11_SPF_SYNTAX_OK",
			"Z11_SPF_UNKNOWN_MODIFIER",
			"Z11_UNABLE_TO_CHECK_FOR_SPF",
		},
		"zone12": {
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"Z12_CSYNC_FOUND",
			"Z12_INCONSISTENT_CSYNC",
			"Z12_MIXED_PRESENCE",
			"Z12_MULTIPLE_CSYNC",
			"Z12_NO_CSYNC",
			"Z12_SERIAL_MISMATCH",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone13": {
			"Z13_NO_SPF_FOUND",
			"Z13_SPF_LOOKUP_COUNT_EXCEEDED",
			"Z13_SPF_LOOKUP_COUNT_OK",
			"Z13_SPF_LOOKUP_LOOP",
			"Z13_SPF_MACRO_TARGET",
			"Z13_SPF_PTR_DEPRECATED",
			"Z13_SPF_RECURSIVE_ERROR",
			"Z13_UNABLE_TO_CHECK",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone14": {
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"Z14_DUPLICATE_SCHEME_HASH",
			"Z14_INCONSISTENT_ZONEMD",
			"Z14_MIXED_PRESENCE",
			"Z14_NO_ZONEMD",
			"Z14_SERIAL_MISMATCH",
			"Z14_UNSUPPORTED_HASH",
			"Z14_ZONEMD_FOUND",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone15": {
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"Z15_CAA_FOUND",
			"Z15_INCONSISTENT_CAA",
			"Z15_INVALID_IODEF_VALUE",
			"Z15_INVALID_ISSUE_VALUE",
			"Z15_INVALID_PROPERTY_TAG",
			"Z15_ISSUANCE_FORBIDDEN",
			"Z15_ISSUE_CONTRADICTION",
			"Z15_MIXED_PRESENCE",
			"Z15_NO_CAA",
			"Z15_NO_CAA_TLD",
			"Z15_NO_RESPONSE_CAA_QUERY",
			"Z15_RESERVED_FLAGS",
			"Z15_UNEXPECTED_RCODE_CAA",
			"Z15_UNKNOWN_PROPERTY",
			"Z15_UNKNOWN_PROPERTY_CRITICAL",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// Zone01 runs the Zone01 test case.
func Zone01(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone01"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	mnameNS := map[string]map[string]*uint32{}
	var serialNS []uint32
	mnameNotMaster := map[string]map[string]uint32{}
	var mnameMaster []string
	var mnameLocalhost []string
	var mnameDot []string

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	for _, ns := range nss {
		disabled, err := ipDisabledMessage(ctx, &results, testcase, ns, "SOA")
		if err != nil {
			return results, err
		}
		if disabled {
			continue
		}

		resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
		if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() || len(resp.GetRecordsForName("SOA", z.Name)) == 0 {
			continue
		}

		for _, rr := range resp.GetRecordsForName("SOA", z.Name) {
			soa, ok := rr.(*dns.SOA)
			if !ok {
				continue
			}
			soaMname := strings.ToLower(strings.TrimSuffix(soa.Ns, "."))
			switch soaMname {
			case "localhost":
				mnameLocalhost = append(mnameLocalhost, ns.Address.String())
			case "":
				mnameDot = append(mnameDot, ns.Address.String())
			default:
				if _, ok := mnameNS[soaMname]; !ok {
					mnameNS[soaMname] = map[string]*uint32{}
				}
			}
			serialNS = append(serialNS, soa.Serial)
		}
	}

	if len(mnameLocalhost) > 0 {
		args := map[string]any{}
		setTypedAddresses(args, mnameLocalhost)
		if err := appendLog(ctx, &results, testcase, "Z01_MNAME_IS_LOCALHOST", args); err != nil {
			return results, err
		}
	}

	if len(mnameDot) > 0 {
		args := map[string]any{}
		setTypedAddresses(args, mnameDot)
		if err := appendLog(ctx, &results, testcase, "Z01_MNAME_IS_DOT", args); err != nil {
			return results, err
		}
	}

	foundIP := 0
	foundSerial := 0

	apexNames, err := apexNSNames(ctx, z)
	if err != nil {
		return results, err
	}
	apexNameSet := map[string]bool{}
	for _, name := range apexNames {
		apexNameSet[strings.ToLower(name.String())] = true
	}

	for mname := range mnameNS {
		if !apexNameSet[strings.ToLower(mname)] {
			if err := appendLog(ctx, &results, testcase, "Z01_MNAME_NOT_IN_NS_LIST", map[string]any{
				"ns": mname,
			}); err != nil {
				return results, err
			}
		}

		addrs, err := getAddressesFor(ctx, z, mname)
		if err != nil {
			return results, err
		}
		for _, addr := range addrs {
			foundIP++
			mnameNS[mname][addr.String()] = nil
		}

		if foundIP > 0 {
			for ip := range mnameNS[mname] {
				if ip == "127.0.0.1" || ip == "::1" {
					if err := appendLog(ctx, &results, testcase, "Z01_MNAME_HAS_LOCALHOST_ADDR", map[string]any{
						"ns":      mname,
						"address": ip,
					}); err != nil {
						return results, err
					}
					continue
				}

				ns, err := nameserver.NewWithContext(ctx, mname, ip, z.Recursor().Client())
				if err != nil {
					continue
				}

				disabled, err := ipDisabledMessage(ctx, &results, testcase, ns, "SOA")
				if err != nil {
					return results, err
				}
				if disabled {
					continue
				}

				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if resp.Msg == nil {
					if err := appendLog(ctx, &results, testcase, "Z01_MNAME_NO_RESPONSE", withNameserverArgs(ns, nil)); err != nil {
						return results, err
					}
					continue
				}

				soaRecords := resp.GetRecordsForName("SOA", z.Name, "answer")
				if resp.Rcode() == "NOERROR" && len(soaRecords) > 0 {
					if !resp.AA() {
						if err := appendLog(ctx, &results, testcase, "Z01_MNAME_NOT_AUTHORITATIVE", withNameserverArgs(ns, nil)); err != nil {
							return results, err
						}
					} else {
						foundSerial++
						if soa, ok := soaRecords[0].(*dns.SOA); ok {
							serial := soa.Serial
							mnameNS[mname][ip] = &serial
						}
					}
				} else if resp.Rcode() != "NOERROR" {
					if err := appendLog(ctx, &results, testcase, "Z01_MNAME_UNEXPECTED_RCODE", withNameserverArgs(ns, map[string]any{
						"rcode": resp.Rcode(),
					})); err != nil {
						return results, err
					}
				} else if len(soaRecords) == 0 {
					if err := appendLog(ctx, &results, testcase, "Z01_MNAME_MISSING_SOA_RECORD", withNameserverArgs(ns, nil)); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "Z01_MNAME_NOT_RESOLVE", map[string]any{
				"ns": mname,
			}); err != nil {
				return results, err
			}
		}
	}

	if foundSerial > 0 {
		serials := uniqueUint32(serialNS)
		for mname, ipMap := range mnameNS {
		ipLoop:
			for ip, serialPtr := range ipMap {
				if serialPtr == nil {
					continue
				}
				for _, serial := range serials {
					if util.SerialGT(serial, *serialPtr) {
						if mnameNotMaster[mname] == nil {
							mnameNotMaster[mname] = map[string]uint32{}
						}
						mnameNotMaster[mname][ip] = *serialPtr
						continue ipLoop
					}
				}
				mnameMaster = append(mnameMaster, mname+"/"+ip)
			}
		}

		if len(mnameNotMaster) > 0 {
			var nsList []string
			var soaserials []uint32
			for mname, ipMap := range mnameNotMaster {
				for ip, serial := range ipMap {
					if ip != "" {
						nsList = append(nsList, mname+"/"+ip)
					} else {
						nsList = append(nsList, mname)
					}
					soaserials = append(soaserials, serial)
				}
			}
			args := map[string]any{
				"soaserial":      maxUint32(uniqueUint32(soaserials)),
				"soaserial_list": joinUint32(serials, ";"),
			}
			setTypedServersFromEndpoints(args, nsList)
			if err := appendLog(ctx, &results, testcase, "Z01_MNAME_NOT_MASTER", args); err != nil {
				return results, err
			}
		}

		if len(mnameMaster) > 0 {
			args := map[string]any{}
			setTypedServersFromEndpoints(args, mnameMaster)
			if err := appendLog(ctx, &results, testcase, "Z01_MNAME_IS_MASTER", args); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone02 runs the Zone02 test case.
func Zone02(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone02"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	threshold := profile.FromContext(ctx).TestCasesVars.Zone02.SOARefreshMinimumValue
	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				refresh := int(soa.Refresh)
				if refresh < threshold {
					if err := appendLog(ctx, &results, testcase, "REFRESH_MINIMUM_VALUE_LOWER", map[string]any{
						"refresh":          refresh,
						"required_refresh": threshold,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(ctx, &results, testcase, "REFRESH_MINIMUM_VALUE_OK", map[string]any{
						"refresh":          refresh,
						"required_refresh": threshold,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone03 runs the Zone03 test case.
func Zone03(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone03"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				refresh := int(soa.Refresh)
				retry := int(soa.Retry)
				if retry >= refresh {
					if err := appendLog(ctx, &results, testcase, "REFRESH_LOWER_THAN_RETRY", map[string]any{
						"retry":   retry,
						"refresh": refresh,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(ctx, &results, testcase, "REFRESH_HIGHER_THAN_RETRY", map[string]any{
						"retry":   retry,
						"refresh": refresh,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone04 runs the Zone04 test case.
func Zone04(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone04"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	threshold := profile.FromContext(ctx).TestCasesVars.Zone04.SOARetryMinimumValue
	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				retry := int(soa.Retry)
				if retry < threshold {
					if err := appendLog(ctx, &results, testcase, "RETRY_MINIMUM_VALUE_LOWER", map[string]any{
						"retry":          retry,
						"required_retry": threshold,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(ctx, &results, testcase, "RETRY_MINIMUM_VALUE_OK", map[string]any{
						"retry":          retry,
						"required_retry": threshold,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone05 runs the Zone05 test case.
func Zone05(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone05"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	threshold := profile.FromContext(ctx).TestCasesVars.Zone05.SOAExpireMinimumValue
	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				expire := int(soa.Expire)
				refresh := int(soa.Refresh)
				if expire < threshold {
					if err := appendLog(ctx, &results, testcase, "EXPIRE_MINIMUM_VALUE_LOWER", map[string]any{
						"expire":          expire,
						"required_expire": threshold,
					}); err != nil {
						return results, err
					}
				}
				if expire < refresh {
					if err := appendLog(ctx, &results, testcase, "EXPIRE_LOWER_THAN_REFRESH", map[string]any{
						"expire":  expire,
						"refresh": refresh,
					}); err != nil {
						return results, err
					}
				}
				if !hasNonStartEntry(results) {
					if err := appendLog(ctx, &results, testcase, "EXPIRE_MINIMUM_VALUE_OK", map[string]any{
						"expire":          expire,
						"refresh":         refresh,
						"required_expire": threshold,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone06 runs the Zone06 test case.
func Zone06(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone06"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	maxValue := profile.FromContext(ctx).TestCasesVars.Zone06.SOADefaultTTLMaximumValue
	minValue := profile.FromContext(ctx).TestCasesVars.Zone06.SOADefaultTTLMinimumValue
	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				minimum := int(soa.Minttl)
				if minimum > maxValue {
					if err := appendLog(ctx, &results, testcase, "SOA_DEFAULT_TTL_MAXIMUM_VALUE_HIGHER", map[string]any{
						"minimum":         minimum,
						"highest_minimum": maxValue,
					}); err != nil {
						return results, err
					}
				} else if minimum < minValue {
					if err := appendLog(ctx, &results, testcase, "SOA_DEFAULT_TTL_MAXIMUM_VALUE_LOWER", map[string]any{
						"minimum":        minimum,
						"lowest_minimum": minValue,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(ctx, &results, testcase, "SOA_DEFAULT_TTL_MAXIMUM_VALUE_OK", map[string]any{
						"minimum":         minimum,
						"highest_minimum": maxValue,
						"lowest_minimum":  minValue,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone07 runs the Zone07 test case.
func Zone07(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone07"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				soaMname := strings.TrimSuffix(soa.Ns, ".")
				addresses := 0

				for _, qtype := range []string{"A", "AAAA"} {
					pMname, err := recurse(ctx, z, soaMname, qtype)
					if err != nil || pMname.Msg == nil {
						continue
					}

					finalName := soaMname
					if q := pMname.Question(); len(q) > 0 {
						questionName := dnsname.New(q[0].Header().Name)
						finalName = questionName.String()
					}

					if pMname.HasRRsOfTypeForName(qtype, dnsname.New(soaMname), "answer") ||
						pMname.HasRRsOfTypeForName(qtype, dnsname.New(finalName), "answer") {
						addresses++
					}

					if pMname.HasRRsOfTypeForName("CNAME", dnsname.New(soaMname), "answer") ||
						!strings.EqualFold(finalName, soaMname) {
						if err := appendLog(ctx, &results, testcase, "MNAME_IS_CNAME", map[string]any{
							"mname": soaMname,
						}); err != nil {
							return results, err
						}
					} else {
						if err := appendLog(ctx, &results, testcase, "MNAME_IS_NOT_CNAME", map[string]any{
							"mname": soaMname,
						}); err != nil {
							return results, err
						}
					}
				}

				if addresses == 0 {
					if err := appendLog(ctx, &results, testcase, "MNAME_HAS_NO_ADDRESS", map[string]any{
						"mname": soaMname,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone08 runs the Zone08 test case.
func Zone08(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone08"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := queryAuth(ctx, z, z.Name.String(), "MX")
	if err != nil {
		return results, err
	}
	if resp.Msg != nil {
		mxRecords := resp.GetRecordsForName("MX", z.Name)
		for _, rr := range mxRecords {
			mx, ok := rr.(*dns.MX)
			if !ok {
				continue
			}
			p2, err := queryAuth(ctx, z, mx.Mx, "CNAME")
			if err != nil {
				return results, err
			}
			if p2.Msg != nil {
				// Name the exchange so several MX records stay distinguishable.
				exchange := dnsname.New(mx.Mx).String()
				if p2.HasRRsOfTypeForName("CNAME", dnsname.New(mx.Mx), "answer") {
					if err := appendLog(ctx, &results, testcase, "MX_RECORD_IS_CNAME", map[string]any{
						"mx": exchange,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(ctx, &results, testcase, "MX_RECORD_IS_NOT_CNAME", map[string]any{
						"mx": exchange,
					}); err != nil {
						return results, err
					}
				}
			}
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_MX_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone09 runs the Zone09 test case.
func Zone09(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone09"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var noResponseMX []string
	unexpectedRcodeMX := map[string][]string{}
	var nonAuthoritativeMX []string
	var noMXSet []string
	mxSet := map[string][]dns.RR{}
	var mxSetOrder []string

	allNS := map[string][]string{}
	var allNSOrder []string
	ipToNS := map[string]string{}

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type mxOutcome struct {
		ip               string
		checked          bool
		disabled         bool
		noResponse       bool
		unexpectedRcode  string
		nonAuthoritative bool
		noMX             bool
		records          []dns.RR
	}

	unique := uniqueServersByIP(nss)
	for _, ns := range unique {
		nsName := ns.Name.String()
		if _, ok := allNS[nsName]; !ok {
			allNSOrder = append(allNSOrder, nsName)
		}
		allNS[nsName] = append(allNS[nsName], ns.Address.String())
		ipToNS[ns.Address.String()] = nsName
	}

	// Map IPs to "ns-name/ip" endpoints for name-server reporting.
	endpointsFor := func(ips []string) []string {
		out := make([]string, 0, len(ips))
		for _, ip := range sortedStrings(ips) {
			if name := ipToNS[ip]; name != "" {
				out = append(out, name+"/"+ip)
			} else {
				out = append(out, ip)
			}
		}
		return out
	}

	var outcomes []mxOutcome
	if len(unique) > 0 {
		outcomes = make([]mxOutcome, len(unique))
		tasks := make([]runner.Task, len(unique))
		for i, ns := range unique {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := mxOutcome{ip: ns.Address.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "MX"); err != nil {
					return err
				} else if disabled {
					outcome.disabled = true
					outcomes[i] = outcome
					return nil
				}

				// Query MX directly; there is no SOA precondition.
				outcome.checked = true
				usevc := false
				fallback := false
				p2, _ := ns.QueryWithOptions(ctx, z.Name.String(), "MX", &nameserver.QueryOptions{
					UseVC:    &usevc,
					Fallback: &fallback,
				})
				if p2.Msg != nil && p2.TC() {
					usevc = true
					p2, _ = ns.QueryWithOptions(ctx, z.Name.String(), "MX", &nameserver.QueryOptions{
						UseVC:    &usevc,
						Fallback: &fallback,
					})
				}

				if p2.Msg == nil {
					outcome.noResponse = true
				} else if p2.Rcode() != "NOERROR" {
					outcome.unexpectedRcode = p2.Rcode()
				} else if !p2.AA() {
					outcome.nonAuthoritative = true
				} else if len(p2.GetRecordsForName("MX", z.Name, "answer")) == 0 {
					outcome.noMX = true
				} else {
					outcome.records = append(outcome.records, p2.GetRecordsForName("MX", z.Name, "answer")...)
				}

				outcomes[i] = outcome
				return nil
			}
		}

		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
	}

	checkedCount := 0
	for _, outcome := range outcomes {
		if outcome.disabled || !outcome.checked {
			continue
		}
		checkedCount++
		switch {
		case outcome.noResponse:
			noResponseMX = append(noResponseMX, outcome.ip)
		case outcome.unexpectedRcode != "":
			unexpectedRcodeMX[outcome.unexpectedRcode] = append(unexpectedRcodeMX[outcome.unexpectedRcode], outcome.ip)
		case outcome.nonAuthoritative:
			nonAuthoritativeMX = append(nonAuthoritativeMX, outcome.ip)
		case outcome.noMX:
			noMXSet = append(noMXSet, outcome.ip)
		case len(outcome.records) > 0:
			if _, ok := mxSet[outcome.ip]; !ok {
				mxSetOrder = append(mxSetOrder, outcome.ip)
			}
			mxSet[outcome.ip] = append(mxSet[outcome.ip], outcome.records...)
		}
	}

	if len(noResponseMX) > 0 {
		args := map[string]any{}
		setTypedAddresses(args, noResponseMX)
		if err := appendLog(ctx, &results, testcase, "Z09_NO_RESPONSE_MX_QUERY", args); err != nil {
			return results, err
		}
	}

	if len(unexpectedRcodeMX) > 0 {
		for _, rcode := range sortedKeys(unexpectedRcodeMX) {
			args := map[string]any{
				"rcode": rcode,
			}
			setTypedAddresses(args, unexpectedRcodeMX[rcode])
			if err := appendLog(ctx, &results, testcase, "Z09_UNEXPECTED_RCODE_MX", args); err != nil {
				return results, err
			}
		}
	}

	if len(nonAuthoritativeMX) > 0 {
		args := map[string]any{}
		setTypedAddresses(args, nonAuthoritativeMX)
		if err := appendLog(ctx, &results, testcase, "Z09_NON_AUTH_MX_RESPONSE", args); err != nil {
			return results, err
		}
	}

	if len(noMXSet) > 0 && len(mxSet) > 0 {
		if err := appendLog(ctx, &results, testcase, "Z09_INCONSISTENT_MX", map[string]any{}); err != nil {
			return results, err
		}
		argsNoMX := map[string]any{}
		setTypedServersFromEndpoints(argsNoMX, endpointsFor(noMXSet))
		if err := appendLog(ctx, &results, testcase, "Z09_NO_MX_FOUND", argsNoMX); err != nil {
			return results, err
		}
		argsFound := map[string]any{}
		setTypedServersFromEndpoints(argsFound, endpointsFor(mapKeys(mxSet)))
		if err := appendLog(ctx, &results, testcase, "Z09_MX_FOUND", argsFound); err != nil {
			return results, err
		}
	}

	if len(mxSet) > 0 {
		// Group servers by MX RDATA, ignoring TTL, record order and duplicate records.
		variants := map[string][]string{}
		variantRDATA := map[string][]mxRDATA{}
		normalized := map[string][]mxRDATA{}
		var variantOrder []string
		for _, ip := range sortedStrings(mxSetOrder) {
			list := normalizeMXRDATA(mxSet[ip])
			normalized[ip] = list
			key := mxRDATAKey(list)
			if _, ok := variants[key]; !ok {
				variantOrder = append(variantOrder, key)
				variantRDATA[key] = list
			}
			variants[key] = append(variants[key], ip)
		}
		sort.Strings(variantOrder)

		if len(variantOrder) > 1 {
			// One self-contained WARNING per RDATA variant.
			for _, key := range variantOrder {
				args := map[string]any{
					"mail_targets": mxExchanges(variantRDATA[key]),
					"mx_rdata":     mxRDATAStrings(variantRDATA[key]),
				}
				setTypedServersFromEndpoints(args, endpointsFor(variants[key]))
				if err := appendLog(ctx, &results, testcase, "Z09_INCONSISTENT_MX_DATA", args); err != nil {
					return results, err
				}
			}
		} else {
			list := normalized[mxSetOrder[0]]
			hasNullMX := false
			for _, entry := range list {
				if entry.exchange != "." {
					continue
				}
				if len(list) > 1 && !hasEntryTag(results, "Z09_NULL_MX_WITH_OTHER_MX") {
					if err := appendLog(ctx, &results, testcase, "Z09_NULL_MX_WITH_OTHER_MX", map[string]any{}); err != nil {
						return results, err
					}
				}
				if entry.pref > 0 && !hasEntryTag(results, "Z09_NULL_MX_NON_ZERO_PREF") {
					if err := appendLog(ctx, &results, testcase, "Z09_NULL_MX_NON_ZERO_PREF", map[string]any{}); err != nil {
						return results, err
					}
				}
				hasNullMX = true
			}

			if !hasNullMX {
				mailTargets := mxExchanges(list)
				if z.Name.String() == "." {
					if err := appendLog(ctx, &results, testcase, "Z09_ROOT_EMAIL_DOMAIN", map[string]any{"mail_targets": mailTargets}); err != nil {
						return results, err
					}
				} else if nextHigherIsRoot(z.Name) {
					if err := appendLog(ctx, &results, testcase, "Z09_TLD_EMAIL_DOMAIN", map[string]any{"mail_targets": mailTargets}); err != nil {
						return results, err
					}
				} else if isArpaTree(z.Name) {
					if err := appendLog(ctx, &results, testcase, "Z09_ARPA_EMAIL_DOMAIN", map[string]any{"mail_targets": mailTargets}); err != nil {
						return results, err
					}
				} else {
					args := map[string]any{"mail_targets": mailTargets, "mx_rdata": mxRDATAStrings(list)}
					setTypedServersFromEndpoints(args, endpointsFor(mxSetOrder))
					if err := appendLog(ctx, &results, testcase, "Z09_MX_DATA", args); err != nil {
						return results, err
					}
				}
			} else if !hasEntryTag(results, "Z09_NULL_MX_WITH_OTHER_MX") && !hasEntryTag(results, "Z09_NULL_MX_NON_ZERO_PREF") {
				// A single Null MX with preference 0 is a valid "no mail" statement.
				if err := appendLog(ctx, &results, testcase, "Z09_VALID_NULL_MX", map[string]any{}); err != nil {
					return results, err
				}
			}
		}
	} else if len(noMXSet) > 0 {
		if isNonMailDomain(z.Name) {
			if err := appendLog(ctx, &results, testcase, "Z09_NO_MX_FOUND_OR_EXPECTED", map[string]any{}); err != nil {
				return results, err
			}
		} else if err := appendLog(ctx, &results, testcase, "Z09_MISSING_MAIL_TARGET", map[string]any{}); err != nil {
			return results, err
		}
	} else if checkedCount > 0 {
		// Servers answered SOA but none returned a usable MX response.
		if err := appendLog(ctx, &results, testcase, "Z09_NO_SERVERS_MX_RESPONSE", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone10 runs the Zone10 test case.
func Zone10(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone10"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	if len(nss) > 0 {
		tasks := make([]runner.Task, len(nss))
		for i, ns := range nss {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "SOA"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if resp.Msg == nil {
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					return nil
				}

				records := resp.GetRecords("SOA", "answer")
				if len(records) > 0 {
					if len(records) > 1 {
						if _, err := buf.Add("MULTIPLE_SOA", withNameserverArgs(ns, map[string]any{
							"count": len(records),
						})); err != nil {
							return err
						}
					} else if soa, ok := records[0].(*dns.SOA); ok {
						owner := strings.ToLower(soa.Hdr.Name)
						expected := strings.ToLower(z.Name.FQDN())
						if owner != expected {
							if _, err := buf.Add("WRONG_SOA", withNameserverArgs(ns, map[string]any{
								"owner":      owner,
								"query_name": expected,
							})); err != nil {
								return err
							}
						}
						// RFC 1034 s3.6.2: CNAME may not coexist with other data at the same owner.
						cnameResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "CNAME", nil)
						if cnameResp.Msg != nil && len(cnameResp.GetRecordsForName("CNAME", z.Name, "answer")) > 0 {
							if _, err := buf.Add("SOA_AND_CNAME", withNameserverArgs(ns, nil)); err != nil {
								return err
							}
						}
						// RFC 6672: DNAME at apex legally coexists with SOA/NS (redirects only names below owner).
						dnameResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNAME", nil)
						if dnameResp.Msg != nil && len(dnameResp.GetRecordsForName("DNAME", z.Name, "answer")) > 0 {
							if _, err := buf.Add("APEX_DNAME", withNameserverArgs(ns, nil)); err != nil {
								return err
							}
						}
					}
				} else {
					if _, err := buf.Add("NO_SOA_IN_RESPONSE", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
				}
				return nil
			}
		}

		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
	}

	if !hasNonStartEntry(results) {
		if err := appendLog(ctx, &results, testcase, "ONE_SOA", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone11 runs the Zone11 test case.
func Zone11(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone11"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsItems, err := delegationNameservers(ctx, z)
	if err != nil {
		return results, err
	}
	zoneItems, err := zoneNameservers(ctx, z)
	if err != nil {
		return results, err
	}

	allNS := nameserversFromNSItems(ctx, z, append(nsItems, zoneItems...))
	groups := nameserversByIP(allNS)

	nsSpf := map[string][]string{}
	ipToNS := map[string][]string{}

	type spfOutcome struct {
		ip       string
		nsList   []string
		checked  bool
		policies []string
	}

	var outcomes []spfOutcome
	if len(groups) > 0 {
		outcomes = make([]spfOutcome, len(groups))
		tasks := make([]runner.Task, len(groups))
		for i, group := range groups {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				if len(group) == 0 {
					return nil
				}
				buf := testlogger.Wrap(log, moduleName, testcase)
				ns := group[0]
				outcome := spfOutcome{
					ip:     ns.Address.String(),
					nsList: nsStrings(group),
				}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "TXT"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "TXT", nil)
				if resp.Msg != nil && resp.Rcode() == "NOERROR" && resp.AA() {
					txtRecords := resp.GetRecordsForName("TXT", z.Name)
					var txtData []string
					for _, rr := range txtRecords {
						txt, ok := rr.(*dns.TXT)
						if !ok {
							continue
						}
						txtData = append(txtData, strings.ToLower(strings.Join(txt.Txt, "")))
					}
					var spfPolicies []string
					for _, txt := range txtData {
						if strings.HasPrefix(txt, "v=spf1") && (len(txt) == len("v=spf1") || txt[len("v=spf1")] == ' ' || txt[len("v=spf1")] == '\t') {
							spfPolicies = append(spfPolicies, txt)
						}
					}
					outcome.checked = true
					outcome.policies = spfPolicies
				}

				outcomes[i] = outcome
				return nil
			}
		}

		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
	}

	for _, outcome := range outcomes {
		if outcome.ip == "" {
			continue
		}
		ipToNS[outcome.ip] = outcome.nsList
		if outcome.checked {
			nsSpf[outcome.ip] = outcome.policies
		}
	}

	spfNS := map[string][]string{}
	for ip, policies := range nsSpf {
		policyList := append([]string{}, policies...)
		sort.Strings(policyList)
		mangled := ""
		for _, policy := range policyList {
			mangled += fmt.Sprintf("<%d>%s", len(policy), policy)
		}
		spfNS[mangled] = append(spfNS[mangled], ipToNS[ip]...)
	}

	if len(nsSpf) == 0 {
		if err := appendLog(ctx, &results, testcase, "Z11_UNABLE_TO_CHECK_FOR_SPF", map[string]any{}); err != nil {
			return results, err
		}
	} else if allEmptyKeys(spfNS) {
		if z.Name.String() == "." || nextHigherIsRoot(z.Name) || strings.HasSuffix(strings.ToLower(z.Name.String()), ".arpa") {
			if err := appendLog(ctx, &results, testcase, "Z11_NO_SPF_NON_MAIL_DOMAIN", map[string]any{
				"domain": z.Name.String(),
			}); err != nil {
				return results, err
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "Z11_NO_SPF_FOUND", map[string]any{
				"domain": z.Name.String(),
			}); err != nil {
				return results, err
			}
		}
	} else if len(spfNS) > 1 {
		if err := appendLog(ctx, &results, testcase, "Z11_INCONSISTENT_SPF_POLICIES", map[string]any{}); err != nil {
			return results, err
		}
		for _, nsList := range spfNS {
			args := map[string]any{}
			setTypedServersFromNames(args, nsList)
			if err := appendLog(ctx, &results, testcase, "Z11_DIFFERENT_SPF_POLICIES_FOUND", args); err != nil {
				return results, err
			}
		}
	} else if len(badSpfIPs(nsSpf)) > 0 {
		var nsList []string
		for _, ip := range badSpfIPs(nsSpf) {
			nsList = append(nsList, ipToNS[ip]...)
		}
		args := map[string]any{}
		setTypedServersFromNames(args, nsList)
		if err := appendLog(ctx, &results, testcase, "Z11_SPF_MULTIPLE_RECORDS", args); err != nil {
			return results, err
		}
	} else {
		spfText := ""
		for _, policies := range nsSpf {
			if len(policies) > 0 {
				spfText = policies[0]
				break
			}
		}

		check := spfCheckSyntax(spfText)
		if check.ok {
			if z.Name.String() == "." || nextHigherIsRoot(z.Name) || strings.HasSuffix(strings.ToLower(z.Name.String()), ".arpa") {
				if nullSpfRegex.MatchString(spfText) {
					if err := appendLog(ctx, &results, testcase, "Z11_NULL_SPF_NON_MAIL_DOMAIN", map[string]any{
						"domain": z.Name.String(),
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(ctx, &results, testcase, "Z11_NON_NULL_SPF_NON_MAIL_DOMAIN", map[string]any{
						"domain": z.Name.String(),
					}); err != nil {
						return results, err
					}
				}
			} else {
				if err := appendLog(ctx, &results, testcase, "Z11_SPF_SYNTAX_OK", map[string]any{
					"domain": z.Name.String(),
				}); err != nil {
					return results, err
				}
			}
			for _, modifier := range check.modifiers {
				if err := appendLog(ctx, &results, testcase, "Z11_SPF_UNKNOWN_MODIFIER", map[string]any{
					"domain":       z.Name.String(),
					"spf_modifier": modifier,
				}); err != nil {
					return results, err
				}
			}
		} else {
			var nsList []string
			for ip := range nsSpf {
				nsList = append(nsList, ipToNS[ip]...)
			}
			args := map[string]any{
				"domain": z.Name.String(),
			}
			setTypedServersFromNames(args, nsList)
			if err := appendLog(ctx, &results, testcase, "Z11_SPF_SYNTAX_ERROR", args); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Zone12 runs the Zone12 test case.
func Zone12(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone12"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type csyncOutcome struct {
		ns        nameserver.Nameserver
		checked   bool
		csyncRRs  []dns.RR
		soaSerial uint32
		soaOK     bool
	}

	var outcomes []csyncOutcome
	if len(nss) > 0 {
		outcomes = make([]csyncOutcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, ns := range nss {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := csyncOutcome{ns: ns}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "CSYNC"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "CSYNC", nil)
				if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() {
					outcomes[i] = outcome
					return nil
				}

				outcome.csyncRRs = resp.GetRecordsForName("CSYNC", z.Name)

				// Query SOA from the same NS to get the current serial for comparison.
				soaResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if soaResp.Msg != nil {
					for _, rr := range soaResp.GetRecordsForName("SOA", z.Name) {
						if soa, ok := rr.(*dns.SOA); ok {
							outcome.soaSerial = soa.Serial
							outcome.soaOK = true
							break
						}
					}
				}

				outcome.checked = true
				outcomes[i] = outcome
				return nil
			}
		}

		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
	}

	type csyncGroup struct {
		serial     uint32
		flags      uint16
		typeBitmap string
		endpoints  []string
	}

	var hasCSYNC, noCSYNC int
	csyncKeys := map[string]struct{}{}
	csyncGroups := map[string]*csyncGroup{}
	var csyncGroupOrder []string
	var noCSYNCNames []string

	for _, outcome := range outcomes {
		if !outcome.checked {
			continue
		}
		ns := outcome.ns
		if len(outcome.csyncRRs) > 1 {
			hasCSYNC++
			if err := appendLog(ctx, &results, testcase, "Z12_MULTIPLE_CSYNC", withNameserverArgs(ns, map[string]any{
				"count": len(outcome.csyncRRs),
			})); err != nil {
				return results, err
			}
		} else if len(outcome.csyncRRs) == 1 {
			hasCSYNC++
			csync, ok := outcome.csyncRRs[0].(*dns.CSYNC)
			if !ok {
				continue
			}
			typeBitmap := csyncTypeBitmap(csync.CSYNC.TypeBitMap)
			if outcome.soaOK && csyncSerialMismatch(csync.CSYNC.Serial, csync.CSYNC.Flags, outcome.soaSerial) {
				if err := appendLog(ctx, &results, testcase, "Z12_SERIAL_MISMATCH", withNameserverArgs(ns, map[string]any{
					"csync_serial": csync.CSYNC.Serial,
					"soa_serial":   outcome.soaSerial,
				})); err != nil {
					return results, err
				}
			}
			key := fmt.Sprintf("%d/%d/%v", csync.CSYNC.Serial, csync.CSYNC.Flags, csync.CSYNC.TypeBitMap)
			csyncKeys[key] = struct{}{}
			endpoint := ns.NameString() + "/" + ns.AddressString()
			if g, ok := csyncGroups[key]; ok {
				g.endpoints = append(g.endpoints, endpoint)
			} else {
				csyncGroups[key] = &csyncGroup{
					serial:     csync.CSYNC.Serial,
					flags:      csync.CSYNC.Flags,
					typeBitmap: typeBitmap,
					endpoints:  []string{endpoint},
				}
				csyncGroupOrder = append(csyncGroupOrder, key)
			}
		} else {
			noCSYNC++
			noCSYNCNames = append(noCSYNCNames, ns.NameString()+"/"+ns.AddressString())
		}
	}

	for _, key := range csyncGroupOrder {
		g := csyncGroups[key]
		args := map[string]any{
			"serial":      g.serial,
			"flags":       g.flags,
			"type_bitmap": g.typeBitmap,
		}
		setTypedServersFromEndpoints(args, g.endpoints)
		if err := appendLog(ctx, &results, testcase, "Z12_CSYNC_FOUND", args); err != nil {
			return results, err
		}
	}

	if noCSYNC > 0 {
		args := map[string]any{}
		setTypedServersFromEndpoints(args, noCSYNCNames)
		if err := appendLog(ctx, &results, testcase, "Z12_NO_CSYNC", args); err != nil {
			return results, err
		}
	}

	if hasCSYNC > 0 && noCSYNC > 0 {
		if err := appendLog(ctx, &results, testcase, "Z12_MIXED_PRESENCE", map[string]any{}); err != nil {
			return results, err
		}
	}
	if hasCSYNC > 1 && len(csyncKeys) > 1 {
		if err := appendLog(ctx, &results, testcase, "Z12_INCONSISTENT_CSYNC", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

func csyncSerialMismatch(csyncSerial uint32, flags uint16, soaSerial uint32) bool {
	// RFC 7477: with soaminimum set, CSYNC serial is a lower-bound gate.
	if flags&csyncFlagSoaMinimum != 0 {
		return util.SerialGT(csyncSerial, soaSerial)
	}
	return csyncSerial != soaSerial
}

// csyncTypeBitmap formats a CSYNC TypeBitMap as a semicolon-separated list of DNS type names.
func csyncTypeBitmap(types []uint16) string {
	var names []string
	for _, t := range types {
		if name, ok := dns.TypeToString[t]; ok {
			names = append(names, name)
		} else {
			names = append(names, fmt.Sprintf("TYPE%d", t))
		}
	}
	return strings.Join(names, ";")
}

// Zone13 runs the Zone13 test case (SPF DNS lookup count per RFC 7208 Section 4.6.4).
func Zone13(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone13"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	// Retrieve SPF record from zone apex.
	resp, _ := queryAuth(ctx, z, z.Name.String(), "TXT")
	if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() {
		if err := appendLog(ctx, &results, testcase, "Z13_UNABLE_TO_CHECK", map[string]any{}); err != nil {
			return results, err
		}
		return appendTestCaseEnd(ctx, results, testcase)
	}

	txtRecords := resp.GetRecordsForName("TXT", z.Name)
	var spfRecord string
	for _, rr := range txtRecords {
		txt, ok := rr.(*dns.TXT)
		if !ok {
			continue
		}
		joined := strings.Join(txt.Txt, "")
		lower := strings.ToLower(joined)
		if strings.HasPrefix(lower, "v=spf1") && (len(lower) == len("v=spf1") || lower[len("v=spf1")] == ' ' || lower[len("v=spf1")] == '\t') {
			spfRecord = lower
			break
		}
	}

	if spfRecord == "" {
		if err := appendLog(ctx, &results, testcase, "Z13_NO_SPF_FOUND", map[string]any{
			"domain": z.Name.String(),
		}); err != nil {
			return results, err
		}
		return appendTestCaseEnd(ctx, results, testcase)
	}

	limit := profile.FromContext(ctx).TestCasesVars.Zone13.SPFLookupLimit
	visited := map[string]bool{}
	walk := spfWalkLookups(ctx, z, spfRecord, visited)

	if walk.hasPtr {
		if err := appendLog(ctx, &results, testcase, "Z13_SPF_PTR_DEPRECATED", map[string]any{
			"domain": z.Name.String(),
		}); err != nil {
			return results, err
		}
	}

	if walk.hasLoop {
		if err := appendLog(ctx, &results, testcase, "Z13_SPF_LOOKUP_LOOP", map[string]any{
			"domain":      z.Name.String(),
			"loop_domain": walk.loopDomain,
		}); err != nil {
			return results, err
		}
	}

	if walk.hasMacro {
		if err := appendLog(ctx, &results, testcase, "Z13_SPF_MACRO_TARGET", map[string]any{
			"domain": z.Name.String(),
			"target": walk.macroTarget,
		}); err != nil {
			return results, err
		}
	}

	if walk.hasError {
		if err := appendLog(ctx, &results, testcase, "Z13_SPF_RECURSIVE_ERROR", map[string]any{
			"domain": z.Name.String(),
			"target": walk.errorTarget,
		}); err != nil {
			return results, err
		}
	}

	count := walk.count
	if count > limit {
		if err := appendLog(ctx, &results, testcase, "Z13_SPF_LOOKUP_COUNT_EXCEEDED", map[string]any{
			"domain": z.Name.String(),
			"count":  count,
			"limit":  limit,
		}); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "Z13_SPF_LOOKUP_COUNT_OK", map[string]any{
			"domain": z.Name.String(),
			"count":  count,
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// spfLookupResult is the aggregated outcome of walking an SPF record's DNS-resolving mechanisms.
type spfLookupResult struct {
	count       int
	loopDomain  string
	errorTarget string
	macroTarget string
	hasPtr      bool
	hasLoop     bool
	hasError    bool
	hasMacro    bool
}

func (r *spfLookupResult) merge(sub spfLookupResult) {
	r.count += sub.count
	if sub.hasPtr {
		r.hasPtr = true
	}
	if sub.hasLoop && !r.hasLoop {
		r.hasLoop = true
		r.loopDomain = sub.loopDomain
	}
	if sub.hasError && !r.hasError {
		r.hasError = true
		r.errorTarget = sub.errorTarget
	}
	if sub.hasMacro && !r.hasMacro {
		r.hasMacro = true
		r.macroTarget = sub.macroTarget
	}
}

// spfWalkLookups recursively walks an SPF record and counts DNS-resolving mechanisms.
func spfWalkLookups(ctx context.Context, z *zonepkg.Zone, spfRecord string, visited map[string]bool) spfLookupResult {
	rest := strings.TrimSpace(spfRecord[len("v=spf1"):])
	if rest == "" {
		return spfLookupResult{}
	}

	var res spfLookupResult

	for term := range strings.FieldsSeq(rest) {
		// Strip qualifier
		if len(term) > 0 && (term[0] == '+' || term[0] == '-' || term[0] == '~' || term[0] == '?') {
			term = term[1:]
		}
		if term == "" {
			continue
		}

		switch {
		case term == "all" || strings.HasPrefix(term, "ip4:") || strings.HasPrefix(term, "ip6:"):
			// No DNS lookup needed.
		case strings.HasPrefix(term, "exp="):
			// exp modifier does not count toward the limit.

		case strings.HasPrefix(term, "include:"):
			res.count++
			target := term[len("include:"):]
			if strings.Contains(target, "%") {
				// Macro-laden target cannot be statically resolved; record it and stop recursing this branch.
				if !res.hasMacro {
					res.hasMacro = true
					res.macroTarget = target
				}
				continue
			}
			res.merge(spfResolveLookups(ctx, z, target, visited))

		case strings.HasPrefix(term, "redirect="):
			res.count++
			target := term[len("redirect="):]
			if strings.Contains(target, "%") {
				if !res.hasMacro {
					res.hasMacro = true
					res.macroTarget = target
				}
				continue
			}
			res.merge(spfResolveLookups(ctx, z, target, visited))

		case term == "a" || strings.HasPrefix(term, "a:") || strings.HasPrefix(term, "a/"):
			res.count++
		case term == "mx" || strings.HasPrefix(term, "mx:") || strings.HasPrefix(term, "mx/"):
			res.count++
		case term == "ptr" || strings.HasPrefix(term, "ptr:") || strings.HasPrefix(term, "ptr/"):
			res.count++
			res.hasPtr = true
		case strings.HasPrefix(term, "exists:"):
			res.count++
		}
	}

	return res
}

// spfResolveLookups fetches the SPF record for a target domain and recursively walks it.
func spfResolveLookups(ctx context.Context, z *zonepkg.Zone, target string, visited map[string]bool) spfLookupResult {
	target = strings.ToLower(strings.TrimRight(target, "."))
	if visited[target] {
		return spfLookupResult{hasLoop: true, loopDomain: target}
	}
	visited[target] = true

	// Ensure FQDN for DNS query.
	fqdn := target
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}

	resp, err := recurse(ctx, z, fqdn, "TXT")
	if err != nil || resp.Msg == nil {
		return spfLookupResult{hasError: true, errorTarget: target}
	}

	txtRecords := resp.GetRecords("TXT")
	for _, rr := range txtRecords {
		txt, ok := rr.(*dns.TXT)
		if !ok {
			continue
		}
		joined := strings.ToLower(strings.Join(txt.Txt, ""))
		if strings.HasPrefix(joined, "v=spf1") && (len(joined) == len("v=spf1") || joined[len("v=spf1")] == ' ' || joined[len("v=spf1")] == '\t') {
			return spfWalkLookups(ctx, z, joined, visited)
		}
	}

	// No SPF record found at target - treat as resolution error.
	return spfLookupResult{hasError: true, errorTarget: target}
}

func appendTestCaseEnd(ctx context.Context, results []*logger.Entry, testcase string) ([]*logger.Entry, error) {
	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

func appendLog(ctx context.Context, results *[]*logger.Entry, testcase string, tag string, args map[string]any) error {
	entry, err := util.LoggerFromContext(ctx).Add(tag, args, moduleName, testcase)
	if err != nil {
		return err
	}
	*results = append(*results, entry)
	return nil
}

func withNameserverArgs(ns nameserver.Nameserver, args map[string]any) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	logargs.NormalizeQueryIdentity(args)
	logargs.SetNS(args, ns.NameString(), ns.AddressString())
	return args
}

func ipDisabledMessageWithLogger(ctx context.Context, buf *testlogger.Buffer, ns nameserver.Nameserver, rrtypes ...string) (bool, error) {
	if ns.Address.Is6() && !profile.FromContext(ctx).Net.IPv6 {
		for _, rrtype := range rrtypes {
			if _, err := buf.Add("IPV6_DISABLED", withNameserverArgs(ns, map[string]any{
				"query_type": rrtype,
			})); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if ns.Address.Is4() && !profile.FromContext(ctx).Net.IPv4 {
		for _, rrtype := range rrtypes {
			if _, err := buf.Add("IPV4_DISABLED", withNameserverArgs(ns, map[string]any{
				"query_type": rrtype,
			})); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}

func ipDisabledMessage(ctx context.Context, results *[]*logger.Entry, testcase string, ns nameserver.Nameserver, rrtypes ...string) (bool, error) {
	if !profile.FromContext(ctx).Net.IPv6 && ns.Address.Is6() {
		for _, rrtype := range rrtypes {
			if err := appendLog(ctx, results, testcase, "IPV6_DISABLED", withNameserverArgs(ns, map[string]any{
				"query_type": rrtype,
			})); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if !profile.FromContext(ctx).Net.IPv4 && ns.Address.Is4() {
		for _, rrtype := range rrtypes {
			if err := appendLog(ctx, results, testcase, "IPV4_DISABLED", withNameserverArgs(ns, map[string]any{
				"query_type": rrtype,
			})); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}

func retrieveRecordFromZone(ctx context.Context, results *[]*logger.Entry, testcase string, z *zonepkg.Zone, name dnsname.Name, qtype string) (packet.Packet, error) {
	nss, err := apexNameservers(ctx, z)
	if err != nil {
		return packet.Packet{}, err
	}

	for _, ns := range nss {
		if disabled, err := ipDisabledMessage(ctx, results, testcase, ns, qtype); err != nil {
			return packet.Packet{}, err
		} else if disabled {
			continue
		}

		resp, _ := ns.QueryWithOptions(ctx, name.String(), qtype, nil)
		if resp.Msg != nil && len(resp.GetRecords(qtype, "answer")) > 0 {
			if resp.AA() {
				return resp, nil
			}
		}
	}

	return packet.Packet{}, nil
}

// spfCheck is the outcome of an SPF record syntax check.
type spfCheck struct {
	ok        bool
	modifiers []string // unknown modifier names, record order, deduplicated
}

func spfCheckSyntax(spf string) spfCheck {
	spf = strings.TrimSpace(spf)
	if spf == "" {
		return spfCheck{}
	}
	for i := 0; i < len(spf); i++ {
		b := spf[i]
		if b < 32 || b > 126 {
			return spfCheck{}
		}
	}

	lower := strings.ToLower(spf)
	if !strings.HasPrefix(lower, "v=spf1") {
		return spfCheck{}
	}
	if len(lower) > len("v=spf1") {
		next := lower[len("v=spf1")]
		if next != ' ' && next != '\t' {
			return spfCheck{}
		}
	}

	rest := strings.TrimSpace(lower[len("v=spf1"):])
	if rest == "" {
		return spfCheck{ok: true}
	}

	check := spfCheck{ok: true}
	seen := map[string]bool{}
	for term := range strings.FieldsSeq(rest) {
		ok, modifier := spfTermOk(term)
		if !ok {
			return spfCheck{}
		}
		if modifier != "" && !seen[modifier] {
			seen[modifier] = true
			check.modifiers = append(check.modifiers, modifier)
		}
	}
	return check
}

// spfTermOk reports whether a term is valid, and names it when it is an
// unknown modifier.
func spfTermOk(term string) (bool, string) {
	if term == "" {
		return false, ""
	}

	// A modifier takes no qualifier, so match it on the raw term.
	if eq := strings.IndexByte(term, '='); eq > 0 {
		if colon := strings.IndexByte(term, ':'); colon < 0 || eq < colon {
			name, value := term[:eq], term[eq+1:]
			if validModifierName(name) {
				switch name {
				case "redirect", "exp":
					return validDomain(value), ""
				case "v":
					// A repeated version token means two merged policies.
					return false, ""
				}
				return validMacroString(value), name
			}
		}
	}

	if term[0] == '+' || term[0] == '-' || term[0] == '~' || term[0] == '?' {
		term = term[1:]
		if term == "" {
			return false, ""
		}
	}

	switch term {
	case "all", "a", "mx", "ptr":
		return true, ""
	}

	if strings.HasPrefix(term, "ip4:") {
		return validIPTerm(term[len("ip4:"):], 4), ""
	}
	if strings.HasPrefix(term, "ip6:") {
		return validIPTerm(term[len("ip6:"):], 6), ""
	}
	if strings.HasPrefix(term, "include:") {
		return validDomain(term[len("include:"):]), ""
	}
	if strings.HasPrefix(term, "exists:") {
		return validDomain(term[len("exists:"):]), ""
	}
	if strings.HasPrefix(term, "a") {
		return validMechanismTerm(term, "a"), ""
	}
	if strings.HasPrefix(term, "mx") {
		return validMechanismTerm(term, "mx"), ""
	}
	if strings.HasPrefix(term, "ptr") {
		return validMechanismTerm(term, "ptr"), ""
	}

	return false, ""
}

// validModifierName matches ALPHA *( ALPHA / DIGIT / "-" / "_" / "." ).
func validModifierName(name string) bool {
	if name == "" || !isASCIILetter(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		b := name[i]
		if isASCIILetter(b) || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' {
			continue
		}
		return false
	}
	return true
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// validMacroString matches *( macro-expand / macro-literal ).
func validMacroString(value string) bool {
	for i := 0; i < len(value); {
		b := value[i]
		if b != '%' {
			// macro-literal is any visible character except "%".
			if b < 0x21 || b > 0x7e {
				return false
			}
			i++
			continue
		}
		if i+1 >= len(value) {
			return false
		}
		switch value[i+1] {
		case '%', '_', '-':
			i += 2
			continue
		case '{':
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 || !validMacroExpand(value[i+2:i+2+end]) {
				return false
			}
			i += 2 + end + 1
			continue
		}
		return false
	}
	return true
}

// validMacroExpand matches macro-letter transformers *delimiter.
func validMacroExpand(body string) bool {
	if body == "" || strings.IndexByte("slodiphcrtv", body[0]) < 0 {
		return false
	}
	rest := body[1:]
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i < len(rest) && rest[i] == 'r' {
		i++
	}
	for ; i < len(rest); i++ {
		if strings.IndexByte(".-+,/_=", rest[i]) < 0 {
			return false
		}
	}
	return true
}

func validMechanismTerm(term string, prefix string) bool {
	if term == prefix {
		return true
	}
	if !strings.HasPrefix(term, prefix) {
		return false
	}
	rest := term[len(prefix):]
	if strings.HasPrefix(rest, ":") {
		spec := rest[1:]
		if i := strings.IndexByte(spec, '/'); i >= 0 {
			return validDomain(spec[:i]) && validDualCIDR(spec[i:])
		}
		return validDomain(spec)
	}
	return validDualCIDR(rest)
}

// validDualCIDR matches [ ip4-cidr-length ] [ "/" ip6-cidr-length ].
func validDualCIDR(value string) bool {
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, "/") {
		return false
	}
	rest := value[1:]
	if strings.HasPrefix(rest, "/") {
		return validCIDRNumber(rest[1:], 128)
	}
	v4, v6, hasV6 := strings.Cut(rest, "//")
	if !validCIDRNumber(v4, 32) {
		return false
	}
	if !hasV6 {
		return true
	}
	return validCIDRNumber(v6, 128)
}

// validCIDRNumber matches "0" / %x31-39 0*2DIGIT bounded by max.
func validCIDRNumber(s string, max int) bool {
	if s == "" || len(s) > 3 {
		return false
	}
	if len(s) > 1 && s[0] == '0' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	value, err := strconv.Atoi(s)
	return err == nil && value <= max
}

func validIPTerm(value string, version int) bool {
	if value == "" {
		return false
	}
	addrText, prefixText, hasPrefix := strings.Cut(value, "/")
	addr, err := netip.ParseAddr(addrText)
	if err != nil {
		return false
	}
	if version == 4 && !addr.Is4() {
		return false
	}
	if version == 6 && !addr.Is6() {
		return false
	}
	if !hasPrefix {
		return true
	}
	if version == 4 {
		return validCIDRNumber(prefixText, 32)
	}
	return validCIDRNumber(prefixText, 128)
}

func validDomain(value string) bool {
	if value == "" {
		return false
	}
	value = strings.TrimSuffix(value, ".")
	if value == "" {
		return false
	}
	ok := dnsutil.IsName(value)
	return ok
}

func nameserversFromNSItems(ctx context.Context, z *zonepkg.Zone, items []nsdiscovery.NSItem) []nameserver.Nameserver {
	if z == nil || z.Recursor() == nil {
		return nil
	}

	seen := map[string]nameserver.Nameserver{}
	for _, item := range items {
		if !item.HasAddress {
			continue
		}
		ns, err := nameserver.NewWithContext(ctx, item.Name.String(), item.Address.String(), z.Recursor().Client())
		if err != nil {
			continue
		}
		seen[strings.ToLower(ns.String())] = ns
	}

	keys := slices.Sorted(maps.Keys(seen))

	out := make([]nameserver.Nameserver, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func nameserversByIP(servers []nameserver.Nameserver) [][]nameserver.Nameserver {
	if len(servers) == 0 {
		return nil
	}
	seen := map[string]int{}
	var grouped [][]nameserver.Nameserver
	for _, ns := range servers {
		ip := ns.Address.String()
		if idx, ok := seen[ip]; ok {
			grouped[idx] = append(grouped[idx], ns)
			continue
		}
		seen[ip] = len(grouped)
		grouped = append(grouped, []nameserver.Nameserver{ns})
	}
	return grouped
}

func nsStrings(servers []nameserver.Nameserver) []string {
	values := make([]string, 0, len(servers))
	for _, ns := range servers {
		values = append(values, ns.NameString())
	}
	return values
}

// One MX record reduced to its RDATA, for comparison and reporting.
type mxRDATA struct {
	pref     uint16
	exchange string
}

// normalizeMXRDATA reduces MX records to deduplicated RDATA sorted by preference then exchange.
func normalizeMXRDATA(records []dns.RR) []mxRDATA {
	seen := map[mxRDATA]bool{}
	var out []mxRDATA
	for _, rr := range records {
		mx, ok := rr.(*dns.MX)
		if !ok {
			continue
		}
		entry := mxRDATA{pref: mx.Preference, exchange: dnsname.New(mx.Mx).StringLower()}
		if seen[entry] {
			continue
		}
		seen[entry] = true
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].pref != out[j].pref {
			return out[i].pref < out[j].pref
		}
		return out[i].exchange < out[j].exchange
	})
	return out
}

// mxRDATAStrings renders each RDATA element as "preference SP exchange".
func mxRDATAStrings(list []mxRDATA) []string {
	out := make([]string, 0, len(list))
	for _, entry := range list {
		out = append(out, fmt.Sprintf("%d %s", entry.pref, entry.exchange))
	}
	return out
}

// mxRDATAKey keys an MX RRset by RDATA only; TTL, record order and case are excluded.
func mxRDATAKey(list []mxRDATA) string {
	return strings.Join(mxRDATAStrings(list), "\n")
}

// mxExchanges lists the exchange names of an RDATA list, deduplicated and sorted.
func mxExchanges(list []mxRDATA) []string {
	seen := map[string]bool{}
	var out []string
	for _, entry := range list {
		if entry.exchange == "" || seen[entry.exchange] {
			continue
		}
		seen[entry.exchange] = true
		out = append(out, entry.exchange)
	}
	sort.Strings(out)
	return out
}

func sortedStrings(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

func setTypedAddresses(args map[string]any, values []string) {
	if args == nil || len(values) == 0 {
		return
	}
	seen := map[string]bool{}
	addresses := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		addresses = append(addresses, value)
	}
	if len(addresses) == 0 {
		return
	}
	sort.Strings(addresses)
	args["addresses"] = addresses
}

func setTypedServersFromNames(args map[string]any, values []string) {
	if args == nil || len(values) == 0 {
		return
	}
	raw, ok := logargs.ServersFromValues(values)["servers"]
	if !ok {
		return
	}
	servers, ok := raw.([]map[string]any)
	if !ok || len(servers) == 0 {
		return
	}
	args["servers"] = servers
}

func setTypedServersFromEndpoints(args map[string]any, values []string) {
	if args == nil || len(values) == 0 {
		return
	}
	seen := map[string]bool{}
	servers := make([]logargs.Server, 0, len(values))
	for _, value := range values {
		ns, address := splitEndpoint(value)
		if ns == "" && address == "" {
			continue
		}
		key := ns + "|" + address
		if seen[key] {
			continue
		}
		seen[key] = true
		servers = append(servers, logargs.Server{NS: ns, Address: address})
	}
	if len(servers) == 0 {
		return
	}
	if typed, ok := logargs.Servers(servers)["servers"]; ok {
		args["servers"] = typed
	}
}

func splitEndpoint(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if ip, err := netip.ParseAddr(value); err == nil {
		return "", ip.String()
	}
	sep := strings.LastIndex(value, "/")
	if sep > 0 && sep < len(value)-1 {
		name := strings.TrimSpace(value[:sep])
		ipText := strings.TrimSpace(value[sep+1:])
		if ip, err := netip.ParseAddr(ipText); err == nil {
			return logargs.EndpointName(name), ip.String()
		}
	}
	return logargs.EndpointName(value), ""
}

func sortedKeys(values map[string][]string) []string {
	keys := slices.Sorted(maps.Keys(values))
	return keys
}

func mapKeys(values map[string][]dns.RR) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func uniqueServersByIP(servers []nameserver.Nameserver) []nameserver.Nameserver {
	seen := map[string]bool{}
	unique := make([]nameserver.Nameserver, 0, len(servers))
	for _, server := range servers {
		ip := server.Address.String()
		if seen[ip] {
			continue
		}
		seen[ip] = true
		unique = append(unique, server)
	}
	return unique
}

func uniqueUint32(values []uint32) []uint32 {
	seen := map[uint32]bool{}
	out := make([]uint32, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func joinUint32(values []uint32, sep string) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatUint(uint64(value), 10))
	}
	return strings.Join(parts, sep)
}

func maxUint32(values []uint32) uint32 {
	var max uint32
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

// spfPolicyAccepted reports the Zone11 verdicts that accept the apex policy.
func spfPolicyAccepted(entries []*logger.Entry) bool {
	return hasEntryTag(entries, "Z11_SPF_SYNTAX_OK") ||
		hasEntryTag(entries, "Z11_NULL_SPF_NON_MAIL_DOMAIN") ||
		hasEntryTag(entries, "Z11_NON_NULL_SPF_NON_MAIL_DOMAIN")
}

func hasEntryTag(entries []*logger.Entry, tag string) bool {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag == tag {
			return true
		}
	}
	return false
}

func hasNonStartEntry(entries []*logger.Entry) bool {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag != "TEST_CASE_START" {
			return true
		}
	}
	return false
}

func allEmptyKeys(values map[string][]string) bool {
	for key := range values {
		if key != "" {
			return false
		}
	}
	return true
}

func badSpfIPs(nsSpf map[string][]string) []string {
	var bad []string
	for ip, policies := range nsSpf {
		if len(policies) > 1 {
			bad = append(bad, ip)
		}
	}
	return bad
}

func nextHigherIsRoot(name dnsname.Name) bool {
	parent, ok := name.NextHigher()
	return ok && parent.String() == "."
}

// isArpaTree reports whether name is inside the .arpa tree (excluding the arpa TLD itself).
func isArpaTree(name dnsname.Name) bool {
	return strings.HasSuffix(strings.ToLower(name.String()), ".arpa")
}

// isNonMailDomain reports whether a zone is not expected to host mail: the root,
// a TLD, or a domain in the .arpa tree.
func isNonMailDomain(name dnsname.Name) bool {
	return name.String() == "." || nextHigherIsRoot(name) || isArpaTree(name)
}

// Zone14 runs the Zone14 test case (ZONEMD presence and RFC 8976 compliance).
func Zone14(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone14"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type zonemdOutcome struct {
		ns        nameserver.Nameserver
		checked   bool
		zonemdRRs []dns.RR
		soaSerial uint32
		soaOK     bool
	}

	var outcomes []zonemdOutcome
	if len(nss) > 0 {
		outcomes = make([]zonemdOutcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, ns := range nss {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := zonemdOutcome{ns: ns}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "ZONEMD"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "ZONEMD", nil)
				if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() {
					outcomes[i] = outcome
					return nil
				}

				outcome.zonemdRRs = resp.GetRecordsForName("ZONEMD", z.Name)

				soaResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if soaResp.Msg != nil {
					for _, rr := range soaResp.GetRecordsForName("SOA", z.Name) {
						if soa, ok := rr.(*dns.SOA); ok {
							outcome.soaSerial = soa.Serial
							outcome.soaOK = true
							break
						}
					}
				}

				outcome.checked = true
				outcomes[i] = outcome
				return nil
			}
		}

		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
	}

	type zonemdGroup struct {
		serial    uint32
		scheme    uint8
		hash      uint8
		digest    string
		endpoints []string
	}

	type schemeHashKey struct{ scheme, hash uint8 }

	var hasZONEMD, noZONEMD int
	nsKeys := map[string]struct{}{}
	zonemdGroups := map[string]*zonemdGroup{}
	var zonemdGroupOrder []string
	var noZONEMDNames []string

	for _, outcome := range outcomes {
		if !outcome.checked {
			continue
		}
		ns := outcome.ns
		if len(outcome.zonemdRRs) == 0 {
			noZONEMD++
			noZONEMDNames = append(noZONEMDNames, ns.NameString()+"/"+ns.AddressString())
			continue
		}

		hasZONEMD++
		endpoint := ns.NameString() + "/" + ns.AddressString()

		// Extract typed records in response order for deterministic processing.
		type zmRec struct {
			serial uint32
			scheme uint8
			hash   uint8
			digest string
		}
		var recs []zmRec
		for _, rr := range outcome.zonemdRRs {
			zm, ok := rr.(*dns.ZONEMD)
			if !ok {
				continue
			}
			recs = append(recs, zmRec{
				serial: zm.ZONEMD.Serial,
				scheme: zm.ZONEMD.Scheme,
				hash:   zm.ZONEMD.Hash,
				digest: strings.ToLower(zm.ZONEMD.Digest),
			})
		}

		// Detect duplicate (Scheme, Hash) pairs.
		schemeHashCount := map[schemeHashKey]int{}
		var schemeHashOrder []schemeHashKey
		for _, r := range recs {
			k := schemeHashKey{r.scheme, r.hash}
			if schemeHashCount[k] == 0 {
				schemeHashOrder = append(schemeHashOrder, k)
			}
			schemeHashCount[k]++
		}
		for _, k := range schemeHashOrder {
			if schemeHashCount[k] > 1 {
				if err := appendLog(ctx, &results, testcase, "Z14_DUPLICATE_SCHEME_HASH", withNameserverArgs(ns, map[string]any{
					"scheme": k.scheme,
					"hash":   k.hash,
				})); err != nil {
					return results, err
				}
			}
		}

		// Detect unsupported hash algorithms (once per distinct (ns, hash) pair).
		seenUnsupportedHash := map[uint8]struct{}{}
		for _, r := range recs {
			if r.hash != dns.ZONEMDHashSHA384 && r.hash != dns.ZONEMDHashSHA512 {
				if _, already := seenUnsupportedHash[r.hash]; !already {
					seenUnsupportedHash[r.hash] = struct{}{}
					if err := appendLog(ctx, &results, testcase, "Z14_UNSUPPORTED_HASH", withNameserverArgs(ns, map[string]any{
						"hash": r.hash,
					})); err != nil {
						return results, err
					}
				}
			}
		}

		// Group records by content for consolidated Z14_ZONEMD_FOUND;
		// emit Z14_SERIAL_MISMATCH per record; build per-NS consistency key.
		var nsKeyParts []string
		for _, r := range recs {
			recordKey := fmt.Sprintf("%d/%d/%d/%s", r.serial, r.scheme, r.hash, r.digest)
			nsKeyParts = append(nsKeyParts, recordKey)
			if outcome.soaOK && r.serial != outcome.soaSerial {
				if err := appendLog(ctx, &results, testcase, "Z14_SERIAL_MISMATCH", withNameserverArgs(ns, map[string]any{
					"zonemd_serial": r.serial,
					"soa_serial":    outcome.soaSerial,
				})); err != nil {
					return results, err
				}
			}
			if g, ok := zonemdGroups[recordKey]; ok {
				g.endpoints = append(g.endpoints, endpoint)
			} else {
				zonemdGroups[recordKey] = &zonemdGroup{
					serial:    r.serial,
					scheme:    r.scheme,
					hash:      r.hash,
					digest:    r.digest,
					endpoints: []string{endpoint},
				}
				zonemdGroupOrder = append(zonemdGroupOrder, recordKey)
			}
		}
		sort.Strings(nsKeyParts)
		nsKeys[strings.Join(nsKeyParts, "|")] = struct{}{}
	}

	for _, key := range zonemdGroupOrder {
		g := zonemdGroups[key]
		args := map[string]any{
			"serial": g.serial,
			"scheme": g.scheme,
			"hash":   g.hash,
			"digest": g.digest,
		}
		setTypedServersFromEndpoints(args, g.endpoints)
		if err := appendLog(ctx, &results, testcase, "Z14_ZONEMD_FOUND", args); err != nil {
			return results, err
		}
	}

	if noZONEMD > 0 {
		args := map[string]any{}
		setTypedServersFromEndpoints(args, noZONEMDNames)
		if err := appendLog(ctx, &results, testcase, "Z14_NO_ZONEMD", args); err != nil {
			return results, err
		}
	}

	if hasZONEMD > 0 && noZONEMD > 0 {
		if err := appendLog(ctx, &results, testcase, "Z14_MIXED_PRESENCE", map[string]any{}); err != nil {
			return results, err
		}
	}
	if hasZONEMD > 1 && len(nsKeys) > 1 {
		if err := appendLog(ctx, &results, testcase, "Z14_INCONSISTENT_ZONEMD", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

const (
	caaFlagIssuerCritical uint8 = 0x80
	caaFlagReservedMask   uint8 = 0x7F
)

// caaKnownProperties holds the assignable entries in the IANA "Certification
// Authority Restriction Properties" registry. The Reserved entries auth, path
// and policy are left out on purpose: they can never be assigned a meaning, so
// no certificate authority will ever act on them.
var caaKnownProperties = map[string]struct{}{
	"issue":        {},
	"issuewild":    {},
	"iodef":        {},
	"issuemail":    {},
	"contactemail": {},
	"contactphone": {},
	"issuevmc":     {},
}

// caaIssueValue is the outcome of parsing an issue-value.
type caaIssueValue struct {
	valid   bool
	forbids bool
}

// parseCAAIssueValue parses an issue-value per RFC 8659 section 4.2. A value
// that does not match the grammar forbids issuance just like one with an empty
// issuer-domain-name.
func parseCAAIssueValue(value string) caaIssueValue {
	rest := strings.TrimLeft(value, " \t")

	var domain string
	semicolon := strings.IndexByte(rest, ';')
	if semicolon < 0 {
		domain = strings.TrimRight(rest, " \t")
	} else {
		domain = strings.TrimRight(rest[:semicolon], " \t")
	}

	if domain != "" && !caaIssuerDomainValid(domain) {
		return caaIssueValue{forbids: true}
	}
	if semicolon >= 0 && !caaParametersValid(rest[semicolon+1:]) {
		return caaIssueValue{forbids: true}
	}
	return caaIssueValue{valid: true, forbids: domain == ""}
}

// caaIssuerDomainValid checks issuer-domain-name = label *("." label).
func caaIssuerDomainValid(domain string) bool {
	for _, label := range strings.Split(domain, ".") {
		if !caaLabelValid(label) {
			return false
		}
	}
	return true
}

// caaLabelValid checks label = (ALPHA / DIGIT) *( *("-") (ALPHA / DIGIT)),
// which allows hyphens only between alphanumerics.
func caaLabelValid(label string) bool {
	if label == "" {
		return false
	}
	if !caaAlphaNum(label[0]) || !caaAlphaNum(label[len(label)-1]) {
		return false
	}
	for i := 0; i < len(label); i++ {
		if !caaAlphaNum(label[i]) && label[i] != '-' {
			return false
		}
	}
	return true
}

// caaParametersValid checks *WSP [parameters *WSP] following the first ";".
func caaParametersValid(rest string) bool {
	rest = strings.Trim(rest, " \t")
	if rest == "" {
		return true
	}
	for _, parameter := range strings.Split(rest, ";") {
		parameter = strings.Trim(parameter, " \t")
		equals := strings.IndexByte(parameter, '=')
		if equals < 0 {
			return false
		}
		tag := strings.TrimRight(parameter[:equals], " \t")
		value := strings.TrimLeft(parameter[equals+1:], " \t")
		if !caaLabelValid(tag) || !caaParameterValueValid(value) {
			return false
		}
	}
	return true
}

// caaParameterValueValid checks value = *(%x21-3A / %x3C-7E).
func caaParameterValueValid(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < 0x21 || c > 0x3A) && (c < 0x3C || c > 0x7E) {
			return false
		}
	}
	return true
}

// caaPropertyTagValid checks RFC 8659 section 4.1: at least one character,
// ASCII letters and digits only.
func caaPropertyTagValid(tag string) bool {
	if tag == "" {
		return false
	}
	for i := 0; i < len(tag); i++ {
		if !caaAlphaNum(tag[i]) {
			return false
		}
	}
	return true
}

func caaAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// caaIodefValueValid reports whether value is one of the URL schemes RFC 8659
// section 4.4 supports for iodef.
func caaIodefValueValid(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "mailto":
		return parsed.Opaque != ""
	case "http", "https":
		return parsed.Host != ""
	}
	return false
}

// isRootOrTLD reports whether name is the root zone or a top-level domain.
// Neither can hold a publicly trusted certificate for its own name.
func isRootOrTLD(name dnsname.Name) bool {
	return name.String() == "." || nextHigherIsRoot(name)
}

// Zone15 runs the Zone15 test case (CAA presence and syntax at the zone apex).
func Zone15(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone15"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type caaOutcome struct {
		ns              nameserver.Nameserver
		checked         bool
		noResponse      bool
		unexpectedRcode string
		caaRRs          []dns.RR
	}

	var outcomes []caaOutcome
	if len(nss) > 0 {
		outcomes = make([]caaOutcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, ns := range nss {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := caaOutcome{ns: ns}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "CAA"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "CAA", nil)
				switch {
				case resp.Msg == nil:
					outcome.noResponse = true
				case resp.Rcode() != "NOERROR":
					outcome.unexpectedRcode = resp.Rcode()
				case !resp.AA():
					// Lameness is reported by the Nameserver module.
				default:
					outcome.checked = true
					outcome.caaRRs = resp.GetRecordsForName("CAA", z.Name)
				}

				outcomes[i] = outcome
				return nil
			}
		}

		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
	}

	var noResponseCAA []string
	unexpectedRcodeCAA := map[string][]string{}
	for _, outcome := range outcomes {
		switch {
		case outcome.noResponse:
			noResponseCAA = append(noResponseCAA, outcome.ns.AddressString())
		case outcome.unexpectedRcode != "":
			rcode := outcome.unexpectedRcode
			unexpectedRcodeCAA[rcode] = append(unexpectedRcodeCAA[rcode], outcome.ns.AddressString())
		}
	}

	if len(noResponseCAA) > 0 {
		args := map[string]any{}
		setTypedAddresses(args, noResponseCAA)
		if err := appendLog(ctx, &results, testcase, "Z15_NO_RESPONSE_CAA_QUERY", args); err != nil {
			return results, err
		}
	}
	for _, rcode := range sortedKeys(unexpectedRcodeCAA) {
		args := map[string]any{"rcode": rcode}
		setTypedAddresses(args, unexpectedRcodeCAA[rcode])
		if err := appendLog(ctx, &results, testcase, "Z15_UNEXPECTED_RCODE_CAA", args); err != nil {
			return results, err
		}
	}

	type caaRec struct {
		flags uint8
		tag   string
		value string
	}
	type caaGroup struct {
		rec       caaRec
		endpoints []string
	}

	var hasCAA, noCAA int
	nsKeys := map[string]struct{}{}
	caaGroups := map[string]*caaGroup{}
	var caaGroupOrder []string
	var noCAANames []string
	var distinct []caaRec
	seen := map[string]struct{}{}

	for _, outcome := range outcomes {
		if !outcome.checked {
			continue
		}
		ns := outcome.ns

		// Extract typed records in response order for deterministic processing.
		var recs []caaRec
		for _, rr := range outcome.caaRRs {
			caa, ok := rr.(*dns.CAA)
			if !ok {
				continue
			}
			recs = append(recs, caaRec{flags: caa.CAA.Flag, tag: caa.CAA.Tag, value: caa.CAA.Value})
		}

		if len(recs) == 0 {
			noCAA++
			noCAANames = append(noCAANames, ns.NameString()+"/"+ns.AddressString())
			continue
		}

		hasCAA++
		endpoint := ns.NameString() + "/" + ns.AddressString()

		// Group records by byte-exact content for consolidated Z15_CAA_FOUND;
		// build the per-NS consistency key and the deduplicated union.
		var nsKeyParts []string
		for _, r := range recs {
			recordKey := fmt.Sprintf("%d/%s/%s", r.flags, r.tag, r.value)
			nsKeyParts = append(nsKeyParts, recordKey)
			if g, ok := caaGroups[recordKey]; ok {
				g.endpoints = append(g.endpoints, endpoint)
			} else {
				caaGroups[recordKey] = &caaGroup{rec: r, endpoints: []string{endpoint}}
				caaGroupOrder = append(caaGroupOrder, recordKey)
			}
			if _, ok := seen[recordKey]; !ok {
				seen[recordKey] = struct{}{}
				distinct = append(distinct, r)
			}
		}
		sort.Strings(nsKeyParts)
		nsKeys[strings.Join(nsKeyParts, "|")] = struct{}{}
	}

	for _, key := range caaGroupOrder {
		g := caaGroups[key]
		args := map[string]any{
			"caa_flags":    g.rec.flags,
			"caa_property": g.rec.tag,
			"caa_value":    g.rec.value,
		}
		setTypedServersFromEndpoints(args, g.endpoints)
		if err := appendLog(ctx, &results, testcase, "Z15_CAA_FOUND", args); err != nil {
			return results, err
		}
	}

	if noCAA > 0 {
		args := map[string]any{}
		setTypedServersFromEndpoints(args, noCAANames)
		tag := "Z15_NO_CAA"
		if isRootOrTLD(z.Name) {
			tag = "Z15_NO_CAA_TLD"
		}
		if err := appendLog(ctx, &results, testcase, tag, args); err != nil {
			return results, err
		}
	}

	if hasCAA > 0 && noCAA > 0 {
		if err := appendLog(ctx, &results, testcase, "Z15_MIXED_PRESENCE", map[string]any{}); err != nil {
			return results, err
		}
	}
	inconsistent := hasCAA > 1 && len(nsKeys) > 1
	if inconsistent {
		if err := appendLog(ctx, &results, testcase, "Z15_INCONSISTENT_CAA", map[string]any{}); err != nil {
			return results, err
		}
	}

	// Content validation runs once over the deduplicated union: a CAA defect is
	// a property of the zone content, not of an individual nameserver.
	type caaPolicy struct{ forbid, permit int }
	policies := map[string]*caaPolicy{}
	var policyOrder []string

	for _, r := range distinct {
		if r.flags&caaFlagReservedMask != 0 {
			if err := appendLog(ctx, &results, testcase, "Z15_RESERVED_FLAGS", map[string]any{
				"caa_flags":    r.flags,
				"caa_property": r.tag,
				"caa_value":    r.value,
			}); err != nil {
				return results, err
			}
		}

		if !caaPropertyTagValid(r.tag) {
			if err := appendLog(ctx, &results, testcase, "Z15_INVALID_PROPERTY_TAG", map[string]any{
				"caa_property": r.tag,
				"caa_value":    r.value,
			}); err != nil {
				return results, err
			}
			// An unusable tag blocks issuance when critical: RFC 8659 section
			// 4.1 covers properties that are unknown or unsupported.
			if r.flags&caaFlagIssuerCritical != 0 {
				if err := appendLog(ctx, &results, testcase, "Z15_UNKNOWN_PROPERTY_CRITICAL", map[string]any{
					"caa_property": r.tag,
					"caa_flags":    r.flags,
				}); err != nil {
					return results, err
				}
			}
			continue
		}

		property := strings.ToLower(r.tag)
		if _, known := caaKnownProperties[property]; !known {
			tag := "Z15_UNKNOWN_PROPERTY"
			args := map[string]any{"caa_property": r.tag}
			if r.flags&caaFlagIssuerCritical != 0 {
				tag = "Z15_UNKNOWN_PROPERTY_CRITICAL"
				args["caa_flags"] = r.flags
			}
			if err := appendLog(ctx, &results, testcase, tag, args); err != nil {
				return results, err
			}
			continue
		}

		switch property {
		case "issue", "issuewild", "issuemail":
			parsed := parseCAAIssueValue(r.value)
			if !parsed.valid {
				if err := appendLog(ctx, &results, testcase, "Z15_INVALID_ISSUE_VALUE", map[string]any{
					"caa_property": r.tag,
					"caa_value":    r.value,
				}); err != nil {
					return results, err
				}
			}
			policy, ok := policies[property]
			if !ok {
				policy = &caaPolicy{}
				policies[property] = policy
				policyOrder = append(policyOrder, property)
			}
			if parsed.forbids {
				policy.forbid++
			} else {
				policy.permit++
			}
		case "iodef":
			if !caaIodefValueValid(r.value) {
				if err := appendLog(ctx, &results, testcase, "Z15_INVALID_IODEF_VALUE", map[string]any{
					"caa_value": r.value,
				}); err != nil {
					return results, err
				}
			}
		}
	}

	// Policy verdicts need one agreed policy that a certificate authority would
	// actually consult, so they are skipped when the servers disagree and at the
	// root, which RFC 8659 tree-climbing never reaches.
	if inconsistent || z.Name.String() == "." {
		return appendTestCaseEnd(ctx, results, testcase)
	}

	// issuewild takes precedence over issue for wildcard names, so a permitting
	// issuewild keeps issuance open even when every issue value forbids.
	issue := policies["issue"]
	issuewild := policies["issuewild"]
	if issue != nil && issue.permit == 0 && (issuewild == nil || issuewild.permit == 0) {
		if err := appendLog(ctx, &results, testcase, "Z15_ISSUANCE_FORBIDDEN", map[string]any{}); err != nil {
			return results, err
		}
	}

	for _, property := range policyOrder {
		policy := policies[property]
		if policy.forbid > 0 && policy.permit > 0 {
			if err := appendLog(ctx, &results, testcase, "Z15_ISSUE_CONTRADICTION", map[string]any{
				"caa_property": property,
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}
