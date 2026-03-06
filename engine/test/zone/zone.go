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

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logargs"
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
	}

	if hasEntryTag(results, "Z11_SPF_SYNTAX_OK") {
		if util.ShouldRunTest(ctx, "zone13") {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Zone13(ctx, z)
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
			"Z13_SPF_PTR_DEPRECATED",
			"Z13_SPF_RECURSIVE_ERROR",
			"Z13_UNABLE_TO_CHECK",
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

	nss, err := method4and5(ctx, z)
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
				if p2.HasRRsOfTypeForName("CNAME", dnsname.New(mx.Mx), "answer") {
					if err := appendLog(ctx, &results, testcase, "MX_RECORD_IS_CNAME", map[string]any{}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(ctx, &results, testcase, "MX_RECORD_IS_NOT_CNAME", map[string]any{}); err != nil {
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

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "SOA", "MX"); err != nil {
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
		setTypedAddresses(args, noResponseMX)
		if err := appendLog(ctx, &results, testcase, "Z09_NON_AUTH_MX_RESPONSE", args); err != nil {
			return results, err
		}
	}

	if len(noMXSet) > 0 && len(mxSet) > 0 {
		if err := appendLog(ctx, &results, testcase, "Z09_INCONSISTENT_MX", map[string]any{}); err != nil {
			return results, err
		}
		argsNoMX := map[string]any{}
		setTypedAddresses(argsNoMX, noMXSet)
		if err := appendLog(ctx, &results, testcase, "Z09_NO_MX_FOUND", argsNoMX); err != nil {
			return results, err
		}
		argsFound := map[string]any{}
		setTypedAddresses(argsFound, mapKeys(mxSet))
		if err := appendLog(ctx, &results, testcase, "Z09_MX_FOUND", argsFound); err != nil {
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
					if err := appendLog(ctx, &results, testcase, "Z09_INCONSISTENT_MX_DATA", map[string]any{}); err != nil {
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
						args := map[string]any{
							"mail_targets": mxExchangeList(records),
						}
						setTypedAddresses(args, ips)
						if err := appendLog(ctx, &results, testcase, "Z09_MX_DATA", args); err != nil {
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
						if err := appendLog(ctx, &results, testcase, "Z09_NULL_MX_WITH_OTHER_MX", map[string]any{}); err != nil {
							return results, err
						}
					}
					if mx.Preference > 0 && !hasEntryTag(results, "Z09_NULL_MX_NON_ZERO_PREF") {
						if err := appendLog(ctx, &results, testcase, "Z09_NULL_MX_NON_ZERO_PREF", map[string]any{}); err != nil {
							return results, err
						}
					}
					hasNullMX = true
				}
			}

			if !hasNullMX {
				if z.Name.String() == "." {
					if err := appendLog(ctx, &results, testcase, "Z09_ROOT_EMAIL_DOMAIN", map[string]any{}); err != nil {
						return results, err
					}
				} else if nextHigherIsRoot(z.Name) {
					if err := appendLog(ctx, &results, testcase, "Z09_TLD_EMAIL_DOMAIN", map[string]any{}); err != nil {
						return results, err
					}
				} else {
					args := map[string]any{
						"mail_targets": mxExchangeList(mxSet[firstIP]),
					}
					setTypedAddresses(args, mxSetOrder)
					if err := appendLog(ctx, &results, testcase, "Z09_MX_DATA", args); err != nil {
						return results, err
					}
				}
			}
		}
	} else if len(noMXSet) > 0 {
		name := strings.ToLower(z.Name.String())
		if z.Name.String() != "." && !nextHigherIsRoot(z.Name) && !strings.HasSuffix(name, ".arpa") {
			if err := appendLog(ctx, &results, testcase, "Z09_MISSING_MAIL_TARGET", map[string]any{}); err != nil {
				return results, err
			}
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

	nsItems, err := getDelNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}
	zoneItems, err := getZoneNSNamesAndIPs(ctx, z)
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

		if spfSyntaxOk(spfText) {
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

	nss, err := method4and5(ctx, z)
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
			i, ns := i, ns
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

	var hasCSYNC, noCSYNC int
	csyncKeys := map[string]struct{}{}

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
			if err := appendLog(ctx, &results, testcase, "Z12_CSYNC_FOUND", withNameserverArgs(ns, map[string]any{
				"serial":      csync.CSYNC.Serial,
				"flags":       csync.CSYNC.Flags,
				"type_bitmap": typeBitmap,
			})); err != nil {
				return results, err
			}
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
		} else {
			noCSYNC++
			if err := appendLog(ctx, &results, testcase, "Z12_NO_CSYNC", withNameserverArgs(ns, nil)); err != nil {
				return results, err
			}
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
	count, loopDomain, errorTarget, hasPtr, hasLoop, hasError := spfWalkLookups(ctx, z, spfRecord, visited)

	if hasPtr {
		if err := appendLog(ctx, &results, testcase, "Z13_SPF_PTR_DEPRECATED", map[string]any{
			"domain": z.Name.String(),
		}); err != nil {
			return results, err
		}
	}

	if hasLoop {
		if err := appendLog(ctx, &results, testcase, "Z13_SPF_LOOKUP_LOOP", map[string]any{
			"domain":      z.Name.String(),
			"loop_domain": loopDomain,
		}); err != nil {
			return results, err
		}
	}

	if hasError {
		if err := appendLog(ctx, &results, testcase, "Z13_SPF_RECURSIVE_ERROR", map[string]any{
			"domain": z.Name.String(),
			"target": errorTarget,
		}); err != nil {
			return results, err
		}
	}

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

// spfWalkLookups recursively walks an SPF record and counts DNS-resolving mechanisms.
// Returns (count, loopDomain, errorTarget, hasPtr, hasLoop, hasError).
func spfWalkLookups(ctx context.Context, z *zonepkg.Zone, spfRecord string, visited map[string]bool) (int, string, string, bool, bool, bool) {
	rest := strings.TrimSpace(spfRecord[len("v=spf1"):])
	if rest == "" {
		return 0, "", "", false, false, false
	}

	count := 0
	var loopDomain, errorTarget string
	var hasPtr, hasLoop, hasError bool

	for _, term := range strings.Fields(rest) {
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
			count++
			target := term[len("include:"):]
			subCount, subLoop, subErr, subPtr, subHasLoop, subHasErr := spfResolveLookups(ctx, z, target, visited)
			count += subCount
			if subPtr {
				hasPtr = true
			}
			if subHasLoop && !hasLoop {
				hasLoop = true
				loopDomain = subLoop
			}
			if subHasErr && !hasError {
				hasError = true
				errorTarget = subErr
			}

		case strings.HasPrefix(term, "redirect="):
			count++
			target := term[len("redirect="):]
			subCount, subLoop, subErr, subPtr, subHasLoop, subHasErr := spfResolveLookups(ctx, z, target, visited)
			count += subCount
			if subPtr {
				hasPtr = true
			}
			if subHasLoop && !hasLoop {
				hasLoop = true
				loopDomain = subLoop
			}
			if subHasErr && !hasError {
				hasError = true
				errorTarget = subErr
			}

		case term == "a" || strings.HasPrefix(term, "a:") || strings.HasPrefix(term, "a/"):
			count++
		case term == "mx" || strings.HasPrefix(term, "mx:") || strings.HasPrefix(term, "mx/"):
			count++
		case term == "ptr" || strings.HasPrefix(term, "ptr:") || strings.HasPrefix(term, "ptr/"):
			count++
			hasPtr = true
		case strings.HasPrefix(term, "exists:"):
			count++
		}
	}

	return count, loopDomain, errorTarget, hasPtr, hasLoop, hasError
}

// spfResolveLookups fetches the SPF record for a target domain and recursively walks it.
func spfResolveLookups(ctx context.Context, z *zonepkg.Zone, target string, visited map[string]bool) (int, string, string, bool, bool, bool) {
	target = strings.ToLower(strings.TrimRight(target, "."))
	if visited[target] {
		return 0, target, "", false, true, false
	}
	visited[target] = true

	// Ensure FQDN for DNS query.
	fqdn := target
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}

	resp, err := recurse(ctx, z, fqdn, "TXT")
	if err != nil || resp.Msg == nil {
		return 0, "", target, false, false, true
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

	// No SPF record found at target — treat as resolution error.
	return 0, "", target, false, false, true
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
	nss, err := method5(ctx, z)
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
	ok := dnsutil.IsName(value)
	return ok
}

func nameserversFromNSItems(ctx context.Context, z *zonepkg.Zone, items []methodsv2.NSItem) []nameserver.Nameserver {
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
		values = append(values, ns.NameString())
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
	seen := map[string]bool{}
	var out []string
	for _, rr := range records {
		mx, ok := rr.(*dns.MX)
		if !ok {
			continue
		}
		mxName := dnsname.New(mx.Mx)
		value := mxName.String()
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
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
