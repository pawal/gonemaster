package zone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/methods"
	methodsv2 "codeberg.org/pawal/gonemaster/engine/methodsv2"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/runner"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testcase"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testlogger"
	"codeberg.org/pawal/gonemaster/engine/util"
	zonepkg "codeberg.org/pawal/gonemaster/engine/zone"
)

const moduleName = "Zone"

var (
	method3              = methods.Method3
	method5              = methods.Method5
	method4and5          = methods.Method4and5
	getDelNSNamesAndIPs  = methodsv2.GetDelNSNamesAndIPs
	getZoneNSNamesAndIPs = methodsv2.GetZoneNSNamesAndIPs
	getAddressesFor      = defaultGetAddressesFor
	recurse              = defaultRecurse
	queryAuth            = defaultQueryAuth
)

var nullSpfRegex = regexp.MustCompile(`(?i)^v=spf1[ \t]+-all[ \t]*$`)

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
	return z.Recursor().Recurse(ctx, name, qtype, "IN")
}

func defaultQueryAuth(ctx context.Context, z *zonepkg.Zone, name string, qtype string) (packet.Packet, error) {
	if z == nil {
		return packet.Packet{}, fmt.Errorf("missing zone")
	}
	return z.QueryAuth(ctx, name, qtype, nil)
}

// All runs the Zone test cases in order, mirroring the Perl implementation.
func All(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest("zone01") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone01(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("zone02") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone02(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("zone03") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone03(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("zone04") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone04(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("zone05") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone05(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("zone06") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone06(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("zone07") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone07(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("zone08") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone08(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if util.ShouldRunTest("zone09") && !hasEntryTag(results, "NO_RESPONSE_MX_QUERY") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Zone09(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if !hasEntryTag(results, "NO_RESPONSE_SOA_QUERY") {
		if util.ShouldRunTest("zone10") {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Zone10(ctx, z)
			})
			results = append(results, entries...)
			if err != nil {
				return results, err
			}
		}
		if util.ShouldRunTest("zone11") {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Zone11(ctx, z)
			})
			results = append(results, entries...)
			if err != nil {
				return results, err
			}
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
			"Z09_INCONSISTENT_MX",
			"Z09_INCONSISTENT_MX_DATA",
			"Z09_MISSING_MAIL_TARGET",
			"Z09_MX_DATA",
			"Z09_MX_FOUND",
			"Z09_NON_AUTH_MX_RESPONSE",
			"Z09_NO_MX_FOUND",
			"Z09_NO_RESPONSE_MX_QUERY",
			"Z09_NULL_MX_NON_ZERO_PREF",
			"Z09_NULL_MX_WITH_OTHER_MX",
			"Z09_ROOT_EMAIL_DOMAIN",
			"Z09_TLD_EMAIL_DOMAIN",
			"Z09_UNEXPECTED_RCODE_MX",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"zone10": {
			"MULTIPLE_SOA",
			"NO_RESPONSE",
			"NO_SOA_IN_RESPONSE",
			"ONE_SOA",
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
			"Z11_UNABLE_TO_CHECK_FOR_SPF",
		},
	}
}

// Zone01 runs the Zone01 test case.
func Zone01(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone01"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	mnameNS := map[string]map[string]*uint32{}
	var serialNS []uint32
	mnameNotMaster := map[string]map[string]uint32{}
	var mnameMaster []string
	var mnameLocalhost []string
	var mnameDot []string

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, ns := range nss {
		disabled, err := ipDisabledMessage(&results, testcase, ns, "SOA")
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
			if soaMname == "localhost" {
				mnameLocalhost = append(mnameLocalhost, ns.Address.String())
			} else if soaMname == "" {
				mnameDot = append(mnameDot, ns.Address.String())
			} else {
				if _, ok := mnameNS[soaMname]; !ok {
					mnameNS[soaMname] = map[string]*uint32{}
				}
			}
			serialNS = append(serialNS, soa.Serial)
		}
	}

	if len(mnameLocalhost) > 0 {
		if err := appendLog(&results, testcase, "Z01_MNAME_IS_LOCALHOST", map[string]any{
			"ns_ip_list": strings.Join(mnameLocalhost, ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(mnameDot) > 0 {
		if err := appendLog(&results, testcase, "Z01_MNAME_IS_DOT", map[string]any{
			"ns_ip_list": strings.Join(mnameDot, ";"),
		}); err != nil {
			return results, err
		}
	}

	foundIP := 0
	foundSerial := 0

	method3Names, err := method3(ctx, z)
	if err != nil {
		return results, err
	}
	method3Set := map[string]bool{}
	for _, name := range method3Names {
		method3Set[strings.ToLower(name.String())] = true
	}

	for mname := range mnameNS {
		if !method3Set[strings.ToLower(mname)] {
			if err := appendLog(&results, testcase, "Z01_MNAME_NOT_IN_NS_LIST", map[string]any{
				"nsname": mname,
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
					if err := appendLog(&results, testcase, "Z01_MNAME_HAS_LOCALHOST_ADDR", map[string]any{
						"nsname": mname,
						"ns_ip":  ip,
					}); err != nil {
						return results, err
					}
					continue
				}

				ns, err := nameserver.New(mname, ip, z.Recursor().Client())
				if err != nil {
					continue
				}

				disabled, err := ipDisabledMessage(&results, testcase, ns, "SOA")
				if err != nil {
					return results, err
				}
				if disabled {
					continue
				}

				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if resp.Msg == nil {
					if err := appendLog(&results, testcase, "Z01_MNAME_NO_RESPONSE", map[string]any{
						"ns": ns.String(),
					}); err != nil {
						return results, err
					}
					continue
				}

				soaRecords := resp.GetRecordsForName("SOA", z.Name, "answer")
				if resp.Rcode() == "NOERROR" && len(soaRecords) > 0 {
					if !resp.AA() {
						if err := appendLog(&results, testcase, "Z01_MNAME_NOT_AUTHORITATIVE", map[string]any{
							"ns": ns.String(),
						}); err != nil {
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
					if err := appendLog(&results, testcase, "Z01_MNAME_UNEXPECTED_RCODE", map[string]any{
						"ns":    ns.String(),
						"rcode": resp.Rcode(),
					}); err != nil {
						return results, err
					}
				} else if len(soaRecords) == 0 {
					if err := appendLog(&results, testcase, "Z01_MNAME_MISSING_SOA_RECORD", map[string]any{
						"ns": ns.String(),
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(&results, testcase, "Z01_MNAME_NOT_RESOLVE", map[string]any{
				"nsname": mname,
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
					nsList = append(nsList, mname+"/"+ip)
					soaserials = append(soaserials, serial)
				}
			}
			sort.Strings(nsList)
			if err := appendLog(&results, testcase, "Z01_MNAME_NOT_MASTER", map[string]any{
				"ns_list":        strings.Join(nsList, ";"),
				"soaserial":      maxUint32(uniqueUint32(soaserials)),
				"soaserial_list": joinUint32(serials, ";"),
			}); err != nil {
				return results, err
			}
		}

		if len(mnameMaster) > 0 {
			sort.Strings(mnameMaster)
			if err := appendLog(&results, testcase, "Z01_MNAME_IS_MASTER", map[string]any{
				"ns_list": strings.Join(mnameMaster, ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone02 runs the Zone02 test case.
func Zone02(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone02"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	threshold := profile.Effective().TestCasesVars.Zone02.SOARefreshMinimumValue
	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				refresh := int(soa.Refresh)
				if refresh < threshold {
					if err := appendLog(&results, testcase, "REFRESH_MINIMUM_VALUE_LOWER", map[string]any{
						"refresh":          refresh,
						"required_refresh": threshold,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(&results, testcase, "REFRESH_MINIMUM_VALUE_OK", map[string]any{
						"refresh":          refresh,
						"required_refresh": threshold,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone03 runs the Zone03 test case.
func Zone03(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone03"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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
					if err := appendLog(&results, testcase, "REFRESH_LOWER_THAN_RETRY", map[string]any{
						"retry":   retry,
						"refresh": refresh,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(&results, testcase, "REFRESH_HIGHER_THAN_RETRY", map[string]any{
						"retry":   retry,
						"refresh": refresh,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone04 runs the Zone04 test case.
func Zone04(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone04"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	threshold := profile.Effective().TestCasesVars.Zone04.SOARetryMinimumValue
	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				retry := int(soa.Retry)
				if retry < threshold {
					if err := appendLog(&results, testcase, "RETRY_MINIMUM_VALUE_LOWER", map[string]any{
						"retry":          retry,
						"required_retry": threshold,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(&results, testcase, "RETRY_MINIMUM_VALUE_OK", map[string]any{
						"retry":          retry,
						"required_retry": threshold,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone05 runs the Zone05 test case.
func Zone05(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone05"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	threshold := profile.Effective().TestCasesVars.Zone05.SOAExpireMinimumValue
	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				expire := int(soa.Expire)
				refresh := int(soa.Refresh)
				if expire < threshold {
					if err := appendLog(&results, testcase, "EXPIRE_MINIMUM_VALUE_LOWER", map[string]any{
						"expire":          expire,
						"required_expire": threshold,
					}); err != nil {
						return results, err
					}
				}
				if expire < refresh {
					if err := appendLog(&results, testcase, "EXPIRE_LOWER_THAN_REFRESH", map[string]any{
						"expire":  expire,
						"refresh": refresh,
					}); err != nil {
						return results, err
					}
				}
				if !hasNonStartEntry(results) {
					if err := appendLog(&results, testcase, "EXPIRE_MINIMUM_VALUE_OK", map[string]any{
						"expire":          expire,
						"refresh":         refresh,
						"required_expire": threshold,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone06 runs the Zone06 test case.
func Zone06(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone06"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := retrieveRecordFromZone(ctx, &results, testcase, z, z.Name, "SOA")
	if err != nil {
		return results, err
	}

	maxValue := profile.Effective().TestCasesVars.Zone06.SOADefaultTTLMaximumValue
	minValue := profile.Effective().TestCasesVars.Zone06.SOADefaultTTLMinimumValue
	if resp.Msg != nil {
		records := resp.GetRecords("SOA", "answer")
		if len(records) > 0 {
			if soa, ok := records[0].(*dns.SOA); ok {
				minimum := int(soa.Minttl)
				if minimum > maxValue {
					if err := appendLog(&results, testcase, "SOA_DEFAULT_TTL_MAXIMUM_VALUE_HIGHER", map[string]any{
						"minimum":         minimum,
						"highest_minimum": maxValue,
					}); err != nil {
						return results, err
					}
				} else if minimum < minValue {
					if err := appendLog(&results, testcase, "SOA_DEFAULT_TTL_MAXIMUM_VALUE_LOWER", map[string]any{
						"minimum":        minimum,
						"lowest_minimum": minValue,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(&results, testcase, "SOA_DEFAULT_TTL_MAXIMUM_VALUE_OK", map[string]any{
						"minimum":         minimum,
						"highest_minimum": maxValue,
						"lowest_minimum":  minValue,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone07 runs the Zone07 test case.
func Zone07(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone07"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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
						questionName := dnsname.New(q[0].Name)
						finalName = questionName.String()
					}

					if pMname.HasRRsOfTypeForName(qtype, dnsname.New(soaMname), "answer") ||
						pMname.HasRRsOfTypeForName(qtype, dnsname.New(finalName), "answer") {
						addresses++
					}

					if pMname.HasRRsOfTypeForName("CNAME", dnsname.New(soaMname), "answer") ||
						!strings.EqualFold(finalName, soaMname) {
						if err := appendLog(&results, testcase, "MNAME_IS_CNAME", map[string]any{
							"mname": soaMname,
						}); err != nil {
							return results, err
						}
					} else {
						if err := appendLog(&results, testcase, "MNAME_IS_NOT_CNAME", map[string]any{
							"mname": soaMname,
						}); err != nil {
							return results, err
						}
					}
				}

				if addresses == 0 {
					if err := appendLog(&results, testcase, "MNAME_HAS_NO_ADDRESS", map[string]any{
						"mname": soaMname,
					}); err != nil {
						return results, err
					}
				}
			}
		} else {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone08 runs the Zone08 test case.
func Zone08(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone08"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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
				if p2.HasRRsOfTypeForName("CNAME", dnsname.New(mx.Mx), "answer") {
					if err := appendLog(&results, testcase, "MX_RECORD_IS_CNAME", map[string]any{}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(&results, testcase, "MX_RECORD_IS_NOT_CNAME", map[string]any{}); err != nil {
						return results, err
					}
				}
			}
		}
	} else {
		if err := appendLog(&results, testcase, "NO_RESPONSE_MX_QUERY", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone09 runs the Zone09 test case.
func Zone09(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone09"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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

	nss, err := method4and5(ctx, z)
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
	}

	var outcomes []mxOutcome
	if len(unique) > 0 {
		outcomes = make([]mxOutcome, len(unique))
		tasks := make([]runner.Task, len(unique))
		for i, ns := range unique {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := mxOutcome{ip: ns.Address.String()}

				if disabled, err := ipDisabledMessageWithLogger(buf, ns, "SOA", "MX"); err != nil {
					return err
				} else if disabled {
					outcome.disabled = true
					outcomes[i] = outcome
					return nil
				}

				p1, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if p1.Msg == nil || p1.Rcode() != "NOERROR" || !p1.AA() || !p1.HasRRsOfTypeForName("SOA", z.Name) {
					outcomes[i] = outcome
					return nil
				}

				outcome.checked = true
				usevc := false
				p2, _ := ns.QueryWithOptions(ctx, z.Name.String(), "MX", &nameserver.QueryOptions{
					UseVC:    &usevc,
				})
				if p2.Msg != nil && p2.TC() {
					usevc = true
					p2, _ = ns.QueryWithOptions(ctx, z.Name.String(), "MX", &nameserver.QueryOptions{
						UseVC:    &usevc,
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

		parallelism := profile.Effective().Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
	}

	for _, outcome := range outcomes {
		if outcome.disabled || !outcome.checked {
			continue
		}
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
		if err := appendLog(&results, testcase, "Z09_NO_RESPONSE_MX_QUERY", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(noResponseMX), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(unexpectedRcodeMX) > 0 {
		for _, rcode := range sortedKeys(unexpectedRcodeMX) {
			if err := appendLog(&results, testcase, "Z09_UNEXPECTED_RCODE_MX", map[string]any{
				"rcode":      rcode,
				"ns_ip_list": strings.Join(sortedStrings(unexpectedRcodeMX[rcode]), ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	if len(nonAuthoritativeMX) > 0 {
		if err := appendLog(&results, testcase, "Z09_NON_AUTH_MX_RESPONSE", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(noResponseMX), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(noMXSet) > 0 && len(mxSet) > 0 {
		if err := appendLog(&results, testcase, "Z09_INCONSISTENT_MX", map[string]any{}); err != nil {
			return results, err
		}
		if err := appendLog(&results, testcase, "Z09_NO_MX_FOUND", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(noMXSet), ";"),
		}); err != nil {
			return results, err
		}
		if err := appendLog(&results, testcase, "Z09_MX_FOUND", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(mapKeys(mxSet)), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(mxSet) > 0 {
		var dataJSON string
		first := true

		for _, ip := range sortedStrings(mxSetOrder) {
			records := mxSet[ip]
			if first {
				dataJSON = encodeLowercaseRRSet(records)
				first = false
			} else {
				nextData := encodeLowercaseRRSet(records)
				if nextData != dataJSON {
					if err := appendLog(&results, testcase, "Z09_INCONSISTENT_MX_DATA", map[string]any{}); err != nil {
						return results, err
					}
					for _, nsName := range allNSOrder {
						ips := allNS[nsName]
						if len(ips) == 0 {
							continue
						}
						records := mxSet[ips[0]]
						if len(records) == 0 {
							continue
						}
						if err := appendLog(&results, testcase, "Z09_MX_DATA", map[string]any{
							"mailtarget_list": strings.Join(mxExchangeList(records), ";"),
							"ns_ip_list":      strings.Join(ips, ";"),
						}); err != nil {
							return results, err
						}
					}
					break
				}
			}
		}

		if !hasEntryTag(results, "Z09_INCONSISTENT_MX_DATA") {
			hasNullMX := false
			firstIP := ""
			if len(mxSetOrder) > 0 {
				firstIP = mxSetOrder[0]
			}
			for _, rr := range mxSet[firstIP] {
				mx, ok := rr.(*dns.MX)
				if !ok {
					continue
				}
				if mx.Mx == "." {
					if len(mxSet[firstIP]) > 1 && !hasEntryTag(results, "Z09_NULL_MX_WITH_OTHER_MX") {
						if err := appendLog(&results, testcase, "Z09_NULL_MX_WITH_OTHER_MX", map[string]any{}); err != nil {
							return results, err
						}
					}
					if mx.Preference > 0 && !hasEntryTag(results, "Z09_NULL_MX_NON_ZERO_PREF") {
						if err := appendLog(&results, testcase, "Z09_NULL_MX_NON_ZERO_PREF", map[string]any{}); err != nil {
							return results, err
						}
					}
					hasNullMX = true
				}
			}

			if !hasNullMX {
				if z.Name.String() == "." {
					if err := appendLog(&results, testcase, "Z09_ROOT_EMAIL_DOMAIN", map[string]any{}); err != nil {
						return results, err
					}
				} else if nextHigherIsRoot(z.Name) {
					if err := appendLog(&results, testcase, "Z09_TLD_EMAIL_DOMAIN", map[string]any{}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(&results, testcase, "Z09_MX_DATA", map[string]any{
						"ns_ip_list":      strings.Join(mxSetOrder, ";"),
						"mailtarget_list": strings.Join(mxExchangeList(mxSet[firstIP]), ";"),
					}); err != nil {
						return results, err
					}
				}
			}
		}
	} else if len(noMXSet) > 0 {
		name := strings.ToLower(z.Name.String())
		if z.Name.String() != "." && !nextHigherIsRoot(z.Name) && !strings.HasSuffix(name, ".arpa") {
			if err := appendLog(&results, testcase, "Z09_MISSING_MAIL_TARGET", map[string]any{}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone10 runs the Zone10 test case.
func Zone10(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone10"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	if len(nss) > 0 {
		tasks := make([]runner.Task, len(nss))
		for i, ns := range nss {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(buf, ns, "SOA"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if resp.Msg == nil {
					if _, err := buf.Add("NO_RESPONSE", map[string]any{
						"ns": ns.String(),
					}); err != nil {
						return err
					}
					return nil
				}

				records := resp.GetRecords("SOA", "answer")
				if len(records) > 0 {
					if len(records) > 1 {
						if _, err := buf.Add("MULTIPLE_SOA", map[string]any{
							"ns":    ns.String(),
							"count": len(records),
						}); err != nil {
							return err
						}
					} else if soa, ok := records[0].(*dns.SOA); ok {
						owner := strings.ToLower(soa.Hdr.Name)
						expected := strings.ToLower(z.Name.FQDN())
						if owner != expected {
							if _, err := buf.Add("WRONG_SOA", map[string]any{
								"ns":    ns.String(),
								"owner": owner,
								"name":  expected,
							}); err != nil {
								return err
							}
						}
					}
				} else {
					if _, err := buf.Add("NO_SOA_IN_RESPONSE", map[string]any{
						"ns": ns.String(),
					}); err != nil {
						return err
					}
				}
				return nil
			}
		}

		parallelism := profile.Effective().Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
	}

	if !hasNonStartEntry(results) {
		if err := appendLog(&results, testcase, "ONE_SOA", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Zone11 runs the Zone11 test case.
func Zone11(ctx context.Context, z *zonepkg.Zone) ([]*logger.Entry, error) {
	const testcase = "Zone11"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsItems, err := getDelNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}
	zoneItems, err := getZoneNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}

	allNS := nameserversFromNSItems(z, append(nsItems, zoneItems...))
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
			i, group := i, group
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

				if disabled, err := ipDisabledMessageWithLogger(buf, ns, "TXT"); err != nil {
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

		parallelism := profile.Effective().Resolver.Defaults.Parallel
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
		if err := appendLog(&results, testcase, "Z11_UNABLE_TO_CHECK_FOR_SPF", map[string]any{}); err != nil {
			return results, err
		}
	} else if allEmptyKeys(spfNS) {
		if z.Name.String() == "." || nextHigherIsRoot(z.Name) || strings.HasSuffix(strings.ToLower(z.Name.String()), ".arpa") {
			if err := appendLog(&results, testcase, "Z11_NO_SPF_NON_MAIL_DOMAIN", map[string]any{}); err != nil {
				return results, err
			}
		} else {
			if err := appendLog(&results, testcase, "Z11_NO_SPF_FOUND", map[string]any{
				"domain": z.Name.String(),
			}); err != nil {
				return results, err
			}
		}
	} else if len(spfNS) > 1 {
		if err := appendLog(&results, testcase, "Z11_INCONSISTENT_SPF_POLICIES", map[string]any{}); err != nil {
			return results, err
		}
		for _, nsList := range spfNS {
			if err := appendLog(&results, testcase, "Z11_DIFFERENT_SPF_POLICIES_FOUND", map[string]any{
				"ns_list": strings.Join(sortedStrings(nsList), ";"),
			}); err != nil {
				return results, err
			}
		}
	} else if len(badSpfIPs(nsSpf)) > 0 {
		var nsList []string
		for _, ip := range badSpfIPs(nsSpf) {
			nsList = append(nsList, ipToNS[ip]...)
		}
		if err := appendLog(&results, testcase, "Z11_SPF_MULTIPLE_RECORDS", map[string]any{
			"ns_list": strings.Join(sortedStrings(nsList), ";"),
		}); err != nil {
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

		if spfSyntaxOk(spfText) {
			if z.Name.String() == "." || nextHigherIsRoot(z.Name) || strings.HasSuffix(strings.ToLower(z.Name.String()), ".arpa") {
				if nullSpfRegex.MatchString(spfText) {
					if err := appendLog(&results, testcase, "Z11_NULL_SPF_NON_MAIL_DOMAIN", map[string]any{
						"domain": z.Name.String(),
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(&results, testcase, "Z11_NON_NULL_SPF_NON_MAIL_DOMAIN", map[string]any{
						"domain": z.Name.String(),
					}); err != nil {
						return results, err
					}
				}
			} else {
				if err := appendLog(&results, testcase, "Z11_SPF_SYNTAX_OK", map[string]any{
					"domain": z.Name.String(),
				}); err != nil {
					return results, err
				}
			}
		} else {
			var nsList []string
			for ip := range nsSpf {
				nsList = append(nsList, ipToNS[ip]...)
			}
			if err := appendLog(&results, testcase, "Z11_SPF_SYNTAX_ERROR", map[string]any{
				"ns_list": strings.Join(sortedStrings(nsList), ";"),
				"domain":  z.Name.String(),
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

func appendTestCaseEnd(results []*logger.Entry, testcase string) ([]*logger.Entry, error) {
	if err := appendLog(&results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

func appendLog(results *[]*logger.Entry, testcase string, tag string, args map[string]any) error {
	entry, err := util.Logger().Add(tag, args, moduleName, testcase)
	if err != nil {
		return err
	}
	*results = append(*results, entry)
	return nil
}

func ipDisabledMessageWithLogger(buf *testlogger.Buffer, ns nameserver.Nameserver, rrtypes ...string) (bool, error) {
	if ns.Address.Is6() && !profile.Effective().Net.IPv6 {
		for _, rrtype := range rrtypes {
			if _, err := buf.Add("IPV6_DISABLED", map[string]any{
				"ns":     ns.String(),
				"rrtype": rrtype,
			}); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if ns.Address.Is4() && !profile.Effective().Net.IPv4 {
		for _, rrtype := range rrtypes {
			if _, err := buf.Add("IPV4_DISABLED", map[string]any{
				"ns":     ns.String(),
				"rrtype": rrtype,
			}); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}

func ipDisabledMessage(results *[]*logger.Entry, testcase string, ns nameserver.Nameserver, rrtypes ...string) (bool, error) {
	if !profile.Effective().Net.IPv6 && ns.Address.Is6() {
		for _, rrtype := range rrtypes {
			if err := appendLog(results, testcase, "IPV6_DISABLED", map[string]any{
				"ns":     ns.String(),
				"rrtype": rrtype,
			}); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if !profile.Effective().Net.IPv4 && ns.Address.Is4() {
		for _, rrtype := range rrtypes {
			if err := appendLog(results, testcase, "IPV4_DISABLED", map[string]any{
				"ns":     ns.String(),
				"rrtype": rrtype,
			}); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}

func retrieveRecordFromZone(ctx context.Context, results *[]*logger.Entry, testcase string, z *zonepkg.Zone, name dnsname.Name, qtype string) (packet.Packet, error) {
	nss, err := method5(ctx, z)
	if err != nil {
		return packet.Packet{}, err
	}

	for _, ns := range nss {
		if disabled, err := ipDisabledMessage(results, testcase, ns, qtype); err != nil {
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

func spfSyntaxOk(spf string) bool {
	spf = strings.TrimSpace(spf)
	if spf == "" {
		return false
	}
	for i := 0; i < len(spf); i++ {
		b := spf[i]
		if b < 32 || b > 126 {
			return false
		}
	}

	lower := strings.ToLower(spf)
	if !strings.HasPrefix(lower, "v=spf1") {
		return false
	}
	if len(lower) > len("v=spf1") {
		next := lower[len("v=spf1")]
		if next != ' ' && next != '\t' {
			return false
		}
	}

	rest := strings.TrimSpace(lower[len("v=spf1"):])
	if rest == "" {
		return true
	}

	for _, term := range strings.Fields(rest) {
		if !spfTermOk(term) {
			return false
		}
	}
	return true
}

func spfTermOk(term string) bool {
	if term == "" {
		return false
	}

	if term[0] == '+' || term[0] == '-' || term[0] == '~' || term[0] == '?' {
		term = term[1:]
		if term == "" {
			return false
		}
	}

	switch term {
	case "all", "a", "mx", "ptr":
		return true
	}

	if strings.HasPrefix(term, "ip4:") {
		return validIPTerm(term[len("ip4:"):], 4)
	}
	if strings.HasPrefix(term, "ip6:") {
		return validIPTerm(term[len("ip6:"):], 6)
	}
	if strings.HasPrefix(term, "include:") {
		return validDomain(term[len("include:"):])
	}
	if strings.HasPrefix(term, "exists:") {
		return validDomain(term[len("exists:"):])
	}
	if strings.HasPrefix(term, "redirect=") {
		return validDomain(term[len("redirect="):])
	}
	if strings.HasPrefix(term, "exp=") {
		return validDomain(term[len("exp="):])
	}
	if strings.HasPrefix(term, "a") {
		return validMechanismTerm(term, "a")
	}
	if strings.HasPrefix(term, "mx") {
		return validMechanismTerm(term, "mx")
	}
	if strings.HasPrefix(term, "ptr") {
		return validMechanismTerm(term, "ptr")
	}

	return false
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
		parts := strings.Split(rest[1:], "/")
		if len(parts) == 0 || !validDomain(parts[0]) {
			return false
		}
		return validCidrParts(parts[1:])
	}
	if strings.HasPrefix(rest, "/") {
		parts := strings.Split(rest[1:], "/")
		return validCidrParts(parts)
	}
	return false
}

func validCidrParts(parts []string) bool {
	if len(parts) == 0 {
		return true
	}
	if len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 || value > 128 {
			return false
		}
	}
	return true
}

func validIPTerm(value string, version int) bool {
	if value == "" {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) > 2 {
		return false
	}
	addr, err := netip.ParseAddr(parts[0])
	if err != nil {
		return false
	}
	if version == 4 && !addr.Is4() {
		return false
	}
	if version == 6 && !addr.Is6() {
		return false
	}
	if len(parts) == 2 {
		prefix, err := strconv.Atoi(parts[1])
		if err != nil {
			return false
		}
		if version == 4 && (prefix < 0 || prefix > 32) {
			return false
		}
		if version == 6 && (prefix < 0 || prefix > 128) {
			return false
		}
	}
	return true
}

func validDomain(value string) bool {
	if value == "" {
		return false
	}
	value = strings.TrimSuffix(value, ".")
	if value == "" {
		return false
	}
	_, ok := dns.IsDomainName(value)
	return ok
}

func nameserversFromNSItems(z *zonepkg.Zone, items []methodsv2.NSItem) []nameserver.Nameserver {
	if z == nil || z.Recursor() == nil {
		return nil
	}

	seen := map[string]nameserver.Nameserver{}
	for _, item := range items {
		if !item.HasAddress {
			continue
		}
		ns, err := nameserver.New(item.Name.String(), item.Address.String(), z.Recursor().Client())
		if err != nil {
			continue
		}
		seen[strings.ToLower(ns.String())] = ns
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

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
		values = append(values, ns.String())
	}
	return values
}

func encodeLowercaseRRSet(records []dns.RR) string {
	var data []string
	for _, rr := range records {
		data = append(data, strings.ToLower(rr.String()))
	}
	sort.Strings(data)
	encoded, _ := json.Marshal(data)
	return string(encoded)
}

func mxExchangeList(records []dns.RR) []string {
	var out []string
	for _, rr := range records {
		mx, ok := rr.(*dns.MX)
		if !ok {
			continue
		}
		mxName := dnsname.New(mx.Mx)
		out = append(out, mxName.String())
	}
	return out
}

func sortedStrings(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

func sortedKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
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
