package nameserver

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"sort"
	"strings"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/methods"
	ns "codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

const moduleName = "Nameserver"

var nonExistentNames = []string{
	"xn--nameservertest.iis.se",
	"xn--nameservertest.icann.org",
	"xn--nameservertest.ripe.net",
}

var (
	method2     = methods.Method2
	method3     = methods.Method3
	method4and5 = methods.Method4and5
)

// All runs the Nameserver test cases in order, mirroring the Perl implementation.
func All(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest("nameserver01") {
		entries, err := Nameserver01(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver02") {
		entries, err := Nameserver02(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver03") {
		entries, err := Nameserver03(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver04") {
		entries, err := Nameserver04(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver05") {
		entries, err := Nameserver05(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver06") {
		entries, err := Nameserver06(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver07") {
		entries, err := Nameserver07(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver08") {
		entries, err := Nameserver08(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver09") {
		entries, err := Nameserver09(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver10") {
		entries, err := Nameserver10(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver11") {
		entries, err := Nameserver11(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver12") {
		entries, err := Nameserver12(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver13") {
		entries, err := Nameserver13(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("nameserver15") {
		entries, err := Nameserver15(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// Metadata returns the set of tags emitted by Nameserver test cases.
func Metadata() map[string][]string {
	return map[string][]string{
		"nameserver01": {
			"IS_A_RECURSOR",
			"NO_RECURSOR",
			"NO_RESPONSE",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver02": {
			"BREAKS_ON_EDNS",
			"EDNS_RESPONSE_WITHOUT_EDNS",
			"EDNS_VERSION_ERROR",
			"EDNS0_SUPPORT",
			"NO_EDNS_SUPPORT",
			"NO_RESPONSE",
			"NS_ERROR",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver03": {
			"AXFR_FAILURE",
			"AXFR_AVAILABLE",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver04": {
			"DIFFERENT_SOURCE_IP",
			"SAME_SOURCE_IP",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver05": {
			"AAAA_BAD_RDATA",
			"AAAA_QUERY_DROPPED",
			"AAAA_UNEXPECTED_RCODE",
			"AAAA_WELL_PROCESSED",
			"A_UNEXPECTED_RCODE",
			"NO_RESPONSE",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver06": {
			"CAN_NOT_BE_RESOLVED",
			"CAN_BE_RESOLVED",
			"NO_RESOLUTION",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver07": {
			"UPWARD_REFERRAL_IRRELEVANT",
			"UPWARD_REFERRAL",
			"NO_UPWARD_REFERRAL",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver08": {
			"QNAME_CASE_INSENSITIVE",
			"QNAME_CASE_SENSITIVE",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver09": {
			"CASE_QUERY_SAME_ANSWER",
			"CASE_QUERY_DIFFERENT_ANSWER",
			"CASE_QUERY_SAME_RC",
			"CASE_QUERY_DIFFERENT_RC",
			"CASE_QUERY_NO_ANSWER",
			"CASE_QUERIES_RESULTS_OK",
			"CASE_QUERIES_RESULTS_DIFFER",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver10": {
			"N10_NO_RESPONSE_EDNS1_QUERY",
			"N10_UNEXPECTED_RCODE",
			"N10_EDNS_RESPONSE_ERROR",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver11": {
			"N11_NO_EDNS",
			"N11_NO_RESPONSE",
			"N11_RETURNS_UNKNOWN_OPTION_CODE",
			"N11_UNEXPECTED_ANSWER_SECTION",
			"N11_UNEXPECTED_RCODE",
			"N11_UNSET_AA",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver12": {
			"NO_RESPONSE",
			"NO_EDNS_SUPPORT",
			"Z_FLAGS_NOTCLEAR",
			"NS_ERROR",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver13": {
			"NO_RESPONSE",
			"NO_EDNS_SUPPORT",
			"NS_ERROR",
			"MISSING_OPT_IN_TRUNCATED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver15": {
			"N15_ERROR_ON_VERSION_QUERY",
			"N15_NO_VERSION_REVEALED",
			"N15_SOFTWARE_VERSION",
			"N15_WRONG_CLASS",
		},
	}
}

// Nameserver01 runs the NAMESERVER01 test case.
func Nameserver01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver01"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "A"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		responseCount := 0
		nxdomainCount := 0
		isNoRecursor := true
		hasSeenRA := false

		for _, name := range nonExistentNames {
			resp, err := server.QueryWithOptions(ctx, name, "A", nil)
			if err != nil || resp.Msg == nil {
				if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{
					"ns":     server.String(),
					"domain": name,
				}); err != nil {
					return results, err
				}
				isNoRecursor = false
				continue
			}

			responseCount++
			if resp.RA() {
				hasSeenRA = true
			}
			if resp.Rcode() == "NXDOMAIN" {
				nxdomainCount++
			}
		}

		if hasSeenRA || (responseCount > 0 && nxdomainCount == responseCount) {
			if err := appendLog(&results, testcase, "IS_A_RECURSOR", map[string]any{"ns": server.String()}); err != nil {
				return results, err
			}
			isNoRecursor = false
		}
		if isNoRecursor {
			if err := appendLog(&results, testcase, "NO_RECURSOR", map[string]any{"ns": server.String()}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver02 runs the NAMESERVER02 test case.
func Nameserver02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver02"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsnamesAndIP := map[string]bool{}
	nErrors := 0
	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "SOA"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		key := server.String()
		if nsnamesAndIP[key] {
			continue
		}

		ver := uint8(0)
		resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver}})
		if err == nil && resp.Msg != nil {
			if resp.Rcode() == "FORMERR" && !resp.HasEdns() {
				if err := appendLog(&results, testcase, "NO_EDNS_SUPPORT", map[string]any{"ns": server.String()}); err != nil {
					return results, err
				}
				nErrors++
			} else if resp.Rcode() == "NOERROR" && resp.EdnsRcode() == 0 && len(resp.GetRecords("SOA", "answer")) > 0 && resp.EdnsVersion() == 0 {
				nsnamesAndIP[key] = true
				continue
			} else if resp.Rcode() == "NOERROR" && !resp.HasEdns() {
				if err := appendLog(&results, testcase, "EDNS_RESPONSE_WITHOUT_EDNS", map[string]any{
					"ns":     server.String(),
					"domain": z.Name.String(),
				}); err != nil {
					return results, err
				}
				nErrors++
			} else if resp.Rcode() == "NOERROR" && resp.HasEdns() && resp.EdnsVersion() != 0 {
				if err := appendLog(&results, testcase, "EDNS_VERSION_ERROR", map[string]any{
					"ns":     server.String(),
					"domain": z.Name.String(),
				}); err != nil {
					return results, err
				}
				nErrors++
			} else {
				if err := appendLog(&results, testcase, "NS_ERROR", map[string]any{"ns": server.String()}); err != nil {
					return results, err
				}
				nErrors++
			}
		} else {
			resp2, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
			if err == nil && resp2.Msg != nil {
				if err := appendLog(&results, testcase, "BREAKS_ON_EDNS", map[string]any{
					"ns":     server.String(),
					"domain": z.Name.String(),
				}); err != nil {
					return results, err
				}
				nErrors++
			} else {
				if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{
					"ns":     server.String(),
					"domain": z.Name.String(),
				}); err != nil {
					return results, err
				}
				nErrors++
			}
		}

		nsnamesAndIP[key] = true
	}

	if len(nsnamesAndIP) > 0 && nErrors == 0 {
		keys := sortedKeys(nsnamesAndIP)
		if err := appendLog(&results, testcase, "EDNS0_SUPPORT", map[string]any{
			"ns_list": strings.Join(keys, ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver03 runs the NAMESERVER03 test case.
func Nameserver03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver03"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsnamesAndIP := map[string]bool{}
	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "AXFR"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		key := server.String()
		if nsnamesAndIP[key] {
			continue
		}

		var firstRR dns.RR
		err := server.AXFR(ctx, z.Name.String(), func(rr dns.RR) bool {
			firstRR = rr
			return false
		}, "")
		if err != nil {
			if err := appendLog(&results, testcase, "AXFR_FAILURE", map[string]any{"ns": server.String()}); err != nil {
				return results, err
			}
		} else if soa, ok := firstRR.(*dns.SOA); ok && soa != nil {
			if err := appendLog(&results, testcase, "AXFR_AVAILABLE", map[string]any{"ns": server.String()}); err != nil {
				return results, err
			}
		}
		nsnamesAndIP[key] = true
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver04 runs the NAMESERVER04 test case.
func Nameserver04(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver04"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsnamesAndIP := map[string]bool{}
	nErrors := 0
	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "SOA"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		key := server.String()
		if nsnamesAndIP[key] {
			continue
		}

		resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
		if err == nil && resp.Msg != nil {
			if addr, ok := parseAnswerFrom(resp.AnswerFrom); ok && addr != server.Address {
				if err := appendLog(&results, testcase, "DIFFERENT_SOURCE_IP", map[string]any{
					"ns":     server.String(),
					"source": resp.AnswerFrom,
				}); err != nil {
					return results, err
				}
				nErrors++
			}
		}
		nsnamesAndIP[key] = true
	}

	if len(nsnamesAndIP) > 0 && nErrors == 0 {
		if err := appendLog(&results, testcase, "SAME_SOURCE_IP", map[string]any{
			"names": strings.Join(sortedKeys(nsnamesAndIP), ","),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver05 runs the NAMESERVER05 test case.
func Nameserver05(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver05"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsnamesAndIP := map[string]bool{}
	aaaaIssue := 0
	var aaaaOK []string
	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "A"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		key := server.String()
		if nsnamesAndIP[key] {
			continue
		}
		nsnamesAndIP[key] = true

		useVC := false
		resp, err := server.QueryWithOptions(ctx, z.Name.String(), "A", &ns.QueryOptions{UseVC: &useVC})
		if err != nil || resp.Msg == nil {
			if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{
				"ns":     server.String(),
				"domain": z.Name.String(),
			}); err != nil {
				return results, err
			}
			continue
		}
		if resp.Rcode() != "NOERROR" {
			if err := appendLog(&results, testcase, "A_UNEXPECTED_RCODE", map[string]any{
				"ns":    server.String(),
				"rcode": resp.Rcode(),
			}); err != nil {
				return results, err
			}
			continue
		}

		resp, err = server.QueryWithOptions(ctx, z.Name.String(), "AAAA", &ns.QueryOptions{UseVC: &useVC})
		if err != nil || resp.Msg == nil {
			if err := appendLog(&results, testcase, "AAAA_QUERY_DROPPED", map[string]any{"ns": server.String()}); err != nil {
				return results, err
			}
			aaaaIssue++
			continue
		}
		if resp.Rcode() != "NOERROR" {
			if err := appendLog(&results, testcase, "AAAA_UNEXPECTED_RCODE", map[string]any{
				"ns":    server.String(),
				"rcode": resp.Rcode(),
			}); err != nil {
				return results, err
			}
			aaaaIssue++
			continue
		}

		for _, rr := range resp.GetRecords("AAAA", "answer") {
			if aaaa, ok := rr.(*dns.AAAA); ok {
				if len(aaaa.AAAA) != net.IPv6len {
					if err := appendLog(&results, testcase, "AAAA_BAD_RDATA", map[string]any{
						"ns":     server.String(),
						"length": len(aaaa.AAAA),
					}); err != nil {
						return results, err
					}
					aaaaIssue++
				} else {
					aaaaOK = append(aaaaOK, aaaa.AAAA.String())
				}
			}
		}
	}

	if len(aaaaOK) > 0 && aaaaIssue == 0 {
		if err := appendLog(&results, testcase, "AAAA_WELL_PROCESSED", map[string]any{
			"ns_list": strings.Join(sortedKeys(nsnamesAndIP), ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver06 runs the NAMESERVER06 test case.
func Nameserver06(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver06"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	glueNames, err := method2(ctx, z)
	if err != nil {
		return results, err
	}
	childNames, err := method3(ctx, z)
	if err != nil {
		return results, err
	}
	allNames := map[string]bool{}
	for _, name := range glueNames {
		allNames[strings.ToLower(name.String())] = true
	}
	for _, name := range childNames {
		allNames[strings.ToLower(name.String())] = true
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}
	withIP := map[string]bool{}
	for _, server := range nss {
		withIP[strings.ToLower(server.Name.String())] = true
	}

	var withoutIP []string
	for name := range allNames {
		if !withIP[name] {
			withoutIP = append(withoutIP, name)
		}
	}
	sort.Strings(withoutIP)

	if len(withoutIP) > 0 && len(withIP) > 0 {
		if err := appendLog(&results, testcase, "CAN_NOT_BE_RESOLVED", map[string]any{
			"nsname_list": strings.Join(withoutIP, ";"),
		}); err != nil {
			return results, err
		}
	} else if len(withIP) == 0 {
		if err := appendLog(&results, testcase, "NO_RESOLUTION", map[string]any{
			"names": strings.Join(withoutIP, ","),
		}); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(&results, testcase, "CAN_BE_RESOLVED", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver07 runs the NAMESERVER07 test case.
func Nameserver07(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver07"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	if z != nil && z.Name.String() == "." {
		if err := appendLog(&results, testcase, "UPWARD_REFERRAL_IRRELEVANT", map[string]any{}); err != nil {
			return results, err
		}
		return appendTestCaseEnd(results, testcase)
	}

	nsnamesAndIP := map[string]bool{}
	nsnames := map[string]bool{}
	nErrors := 0
	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "NS"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		key := server.String()
		if nsnamesAndIP[key] {
			continue
		}

		resp, err := server.QueryWithOptions(ctx, ".", "NS", nil)
		if err == nil && resp.Msg != nil {
			if len(resp.GetRecords("NS", "authority")) > 0 {
				if err := appendLog(&results, testcase, "UPWARD_REFERRAL", map[string]any{"ns": server.String()}); err != nil {
					return results, err
				}
				nErrors++
			}
		}
		nsnames[server.Name.String()] = true
		nsnamesAndIP[key] = true
	}

	if len(nsnamesAndIP) > 0 && nErrors == 0 {
		keys := make([]string, 0, len(nsnames))
		for name := range nsnames {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		if err := appendLog(&results, testcase, "NO_UPWARD_REFERRAL", map[string]any{
			"nsname_list": strings.Join(keys, ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver08 runs the NAMESERVER08 test case.
func Nameserver08(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver08"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	original := strings.TrimRight("www."+z.Name.String(), ".")
	randomized := util.ScrambleCase(original)
	for randomized == original {
		randomized = util.ScrambleCase(original)
	}

	nsnamesAndIP := map[string]bool{}
	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "SOA"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		key := server.String()
		if nsnamesAndIP[key] {
			continue
		}

		resp, err := server.QueryWithOptions(ctx, randomized, "SOA", nil)
		if err == nil && resp.Msg != nil {
			questions := resp.Question()
			if len(questions) > 0 {
				qname := strings.TrimRight(questions[0].Name, ".")
				if qname == randomized {
					if err := appendLog(&results, testcase, "QNAME_CASE_SENSITIVE", map[string]any{
						"ns":     server.String(),
						"domain": randomized,
					}); err != nil {
						return results, err
					}
				} else {
					if err := appendLog(&results, testcase, "QNAME_CASE_INSENSITIVE", map[string]any{
						"ns":     server.String(),
						"domain": randomized,
					}); err != nil {
						return results, err
					}
				}
			}
		}
		nsnamesAndIP[key] = true
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver09 runs the NAMESERVER09 test case.
func Nameserver09(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver09"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	original := strings.TrimRight("www."+z.Name.String(), ".")
	recordType := "SOA"
	random1 := util.ScrambleCase(original)
	for random1 == original {
		random1 = util.ScrambleCase(original)
	}
	random2 := util.ScrambleCase(original)
	for random2 == original || random2 == random1 {
		random2 = util.ScrambleCase(original)
	}

	nsnamesAndIP := map[string]bool{}
	allResultsMatch := true
	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, recordType); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		key := server.String()
		if nsnamesAndIP[key] {
			continue
		}

		p1, _ := server.QueryWithOptions(ctx, random1, recordType, nil)
		p2, _ := server.QueryWithOptions(ctx, random2, recordType, nil)

		answer1 := normalizedAnswer(p1)
		answer2 := ""
		if len(p1.Answer()) > 0 {
			answer2 = normalizedAnswer(p2)
			if answer1 == answer2 {
				if err := appendLog(&results, testcase, "CASE_QUERY_SAME_ANSWER", map[string]any{
					"ns":     server.String(),
					"type":   recordType,
					"query1": random1,
					"query2": random2,
				}); err != nil {
					return results, err
				}
			} else {
				allResultsMatch = false
				if err := appendLog(&results, testcase, "CASE_QUERY_DIFFERENT_ANSWER", map[string]any{
					"ns":     server.String(),
					"type":   recordType,
					"query1": random1,
					"query2": random2,
				}); err != nil {
					return results, err
				}
			}
		} else if p1.Msg != nil && p2.Msg != nil {
			if p1.Rcode() == p2.Rcode() {
				if err := appendLog(&results, testcase, "CASE_QUERY_SAME_RC", map[string]any{
					"ns":     server.String(),
					"type":   recordType,
					"query1": random1,
					"query2": random2,
					"rcode":  p1.Rcode(),
				}); err != nil {
					return results, err
				}
			} else {
				allResultsMatch = false
				if err := appendLog(&results, testcase, "CASE_QUERY_DIFFERENT_RC", map[string]any{
					"ns":     server.String(),
					"type":   recordType,
					"query1": random1,
					"query2": random2,
					"rcode1": p1.Rcode(),
					"rcode2": p2.Rcode(),
				}); err != nil {
					return results, err
				}
			}
		} else if p1.Msg != nil || p2.Msg != nil {
			allResultsMatch = false
			if err := appendLog(&results, testcase, "CASE_QUERY_NO_ANSWER", map[string]any{
				"ns":     server.String(),
				"type":   recordType,
				"domain": firstNonEmpty(random1, p1, random2, p2),
			}); err != nil {
				return results, err
			}
		}

		nsnamesAndIP[key] = true
	}

	if allResultsMatch {
		if err := appendLog(&results, testcase, "CASE_QUERIES_RESULTS_OK", map[string]any{
			"type":   recordType,
			"domain": original,
		}); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(&results, testcase, "CASE_QUERIES_RESULTS_DIFFER", map[string]any{
			"type":   recordType,
			"domain": original,
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver10 runs the NAMESERVER10 test case.
func Nameserver10(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver10"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var noResponseEDNS1 []string
	unexpectedRcode := map[string][]string{}
	var ednsResponseError []string

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "SOA"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		ver0 := uint8(0)
		resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver0}})
		if err == nil && resp.Msg != nil && resp.Rcode() == "NOERROR" {
			ver1 := uint8(1)
			resp2, err2 := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver1}})
			if err2 == nil && resp2.Msg != nil {
				rcode := resp2.Msg.Rcode
				isBadvers := rcode == dns.RcodeBadVers || ((rcode&0xF) == dns.RcodeSuccess && resp2.EdnsRcode() == 1)
				if !isBadvers {
					unexpectedRcode[resp2.Rcode()] = append(unexpectedRcode[resp2.Rcode()], server.Address.String())
				} else if resp2.EdnsVersion() == 0 && len(resp2.Answer()) == 0 {
					continue
				} else {
					ednsResponseError = append(ednsResponseError, server.Address.String())
				}
			} else {
				noResponseEDNS1 = append(noResponseEDNS1, server.Address.String())
			}
		}
	}

	if len(noResponseEDNS1) > 0 {
		if err := appendLog(&results, testcase, "N10_NO_RESPONSE_EDNS1_QUERY", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(noResponseEDNS1), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(unexpectedRcode) > 0 {
		keys := make([]string, 0, len(unexpectedRcode))
		for key := range unexpectedRcode {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, rcode := range keys {
			list := sortedStrings(unexpectedRcode[rcode])
			if err := appendLog(&results, testcase, "N10_UNEXPECTED_RCODE", map[string]any{
				"rcode":      rcode,
				"ns_ip_list": strings.Join(list, ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	if len(ednsResponseError) > 0 {
		if err := appendLog(&results, testcase, "N10_EDNS_RESPONSE_ERROR", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(ednsResponseError), ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver11 runs the NAMESERVER11 test case.
func Nameserver11(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver11"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var noResponse []string
	unexpectedRcode := map[string][]string{}
	var noEdns []string
	var unexpectedAnswer []string
	var unsetAA []string
	var unknownOpt []string

	optCode := uint16(137)
	unknownOptData := &dns.EDNS0_LOCAL{Code: optCode, Data: []byte{}}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "SOA"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		ver0 := uint8(0)
		resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver0}})
		if err != nil || resp.Msg == nil || !resp.HasEdns() || resp.Rcode() != "NOERROR" || !resp.AA() || len(resp.GetRecordsForName("SOA", z.Name, "answer")) == 0 {
			continue
		}

		resp, err = server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver0, Data: []dns.EDNS0{unknownOptData}}})
		if err != nil || resp.Msg == nil {
			noResponse = append(noResponse, server.Address.String())
			continue
		}

		if resp.Rcode() != "NOERROR" {
			unexpectedRcode[resp.Rcode()] = append(unexpectedRcode[resp.Rcode()], server.Address.String())
			continue
		}
		if !resp.HasEdns() {
			noEdns = append(noEdns, server.Address.String())
			continue
		}
		if len(resp.GetRecordsForName("SOA", z.Name, "answer")) == 0 {
			unexpectedAnswer = append(unexpectedAnswer, server.Address.String())
			continue
		}
		if !resp.AA() {
			unsetAA = append(unsetAA, server.Address.String())
			continue
		}
		for _, opt := range resp.EdnsData() {
			if opt.Option() == optCode {
				unknownOpt = append(unknownOpt, server.Address.String())
				break
			}
		}
	}

	if len(noResponse) > 0 {
		if err := appendLog(&results, testcase, "N11_NO_RESPONSE", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(noResponse), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(unexpectedRcode) > 0 {
		keys := make([]string, 0, len(unexpectedRcode))
		for key := range unexpectedRcode {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, rcode := range keys {
			list := sortedStrings(unexpectedRcode[rcode])
			if err := appendLog(&results, testcase, "N11_UNEXPECTED_RCODE", map[string]any{
				"rcode":      rcode,
				"ns_ip_list": strings.Join(list, ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	if len(noEdns) > 0 {
		if err := appendLog(&results, testcase, "N11_NO_EDNS", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(noEdns), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(unexpectedAnswer) > 0 {
		if err := appendLog(&results, testcase, "N11_UNEXPECTED_ANSWER_SECTION", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(unexpectedAnswer), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(unsetAA) > 0 {
		if err := appendLog(&results, testcase, "N11_UNSET_AA", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(unsetAA), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(unknownOpt) > 0 {
		if err := appendLog(&results, testcase, "N11_RETURNS_UNKNOWN_OPTION_CODE", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(unknownOpt), ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver12 runs the NAMESERVER12 test case.
func Nameserver12(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver12"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "SOA"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		ver0 := uint8(0)
		zFlag := uint16(3)
		resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver0, Z: &zFlag}})
		if err == nil && resp.Msg != nil {
			if resp.Rcode() == "FORMERR" && resp.EdnsRcode() == 0 {
				if err := appendLog(&results, testcase, "NO_EDNS_SUPPORT", map[string]any{"ns": server.String()}); err != nil {
					return results, err
				}
			} else if resp.EdnsZ() != 0 {
				if err := appendLog(&results, testcase, "Z_FLAGS_NOTCLEAR", map[string]any{"ns": server.String()}); err != nil {
					return results, err
				}
			} else if resp.Rcode() == "NOERROR" && resp.EdnsRcode() == 0 && resp.EdnsVersion() == 0 && resp.EdnsZ() == 0 && len(resp.GetRecords("SOA", "answer")) > 0 {
				continue
			} else {
				if err := appendLog(&results, testcase, "NS_ERROR", map[string]any{"ns": server.String()}); err != nil {
					return results, err
				}
			}
		} else {
			if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{
				"ns":     server.String(),
				"domain": z.Name.String(),
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver13 runs the NAMESERVER13 test case.
func Nameserver13(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver13"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "SOA"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		ver0 := uint8(0)
		doBit := true
		size := uint16(512)
		useVC := false
		fallback := false
		resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{
			UseVC:    &useVC,
			Fallback: &fallback,
			EDNSDetails: &transport.EDNSDetails{
				Version: &ver0,
				Do:      &doBit,
				Size:    &size,
			},
		})
		if err == nil && resp.Msg != nil {
			if resp.Rcode() == "FORMERR" && resp.EdnsRcode() == 0 {
				if err := appendLog(&results, testcase, "NO_EDNS_SUPPORT", map[string]any{"ns": server.String()}); err != nil {
					return results, err
				}
			} else if resp.TC() && !resp.HasEdns() {
				if err := appendLog(&results, testcase, "MISSING_OPT_IN_TRUNCATED", map[string]any{"ns": server.String()}); err != nil {
					return results, err
				}
			} else if resp.Rcode() == "NOERROR" && resp.EdnsRcode() == 0 && resp.EdnsVersion() == 0 {
				continue
			} else {
				if err := appendLog(&results, testcase, "NS_ERROR", map[string]any{"ns": server.String()}); err != nil {
					return results, err
				}
			}
		} else {
			if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{
				"ns":     server.String(),
				"domain": z.Name.String(),
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Nameserver15 runs the NAMESERVER15 test case.
func Nameserver15(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver15"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	txtData := map[string]map[string][]string{}
	errorOnVersionQuery := map[string][]string{}
	sendingVersionQuery := map[string]bool{}
	wrongRecordClass := map[string]bool{}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, server := range nss {
		if disabled, err := ipDisabledMessage(&results, testcase, server, "SOA TXT"); err != nil {
			return results, err
		} else if disabled {
			continue
		}

		resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
		if err != nil || resp.Msg == nil {
			continue
		}

		sendingVersionQuery[server.String()] = true

		for _, queryName := range []string{"version.bind", "version.server"} {
			class := "CH"
			respTXT, err := server.QueryWithOptions(ctx, queryName, "TXT", &ns.QueryOptions{Class: class})
			if err != nil || respTXT.Msg == nil || respTXT.Rcode() == "SERVFAIL" {
				errorOnVersionQuery[queryName] = append(errorOnVersionQuery[queryName], server.String())
				continue
			}

			rrs := respTXT.GetRecordsForName("TXT", dnsname.New(queryName), "answer")
			if len(rrs) == 0 {
				continue
			}

			for _, rr := range rrs {
				txt, ok := rr.(*dns.TXT)
				if !ok {
					continue
				}
				if txt.Header().Class != dns.ClassCHAOS {
					wrongRecordClass[server.String()] = true
				}
				stringValue := strings.TrimSpace(strings.Join(txt.Txt, ""))
				if stringValue != "" {
					if txtData[stringValue] == nil {
						txtData[stringValue] = map[string][]string{}
					}
					txtData[stringValue][queryName] = append(txtData[stringValue][queryName], server.String())
					delete(sendingVersionQuery, server.String())
				}
			}
		}
	}

	if len(txtData) > 0 {
		stringsList := make([]string, 0, len(txtData))
		for value := range txtData {
			stringsList = append(stringsList, value)
		}
		sort.Strings(stringsList)
		for _, value := range stringsList {
			queries := txtData[value]
			queryNames := make([]string, 0, len(queries))
			for queryName := range queries {
				queryNames = append(queryNames, queryName)
			}
			sort.Strings(queryNames)
			for _, queryName := range queryNames {
				list := sortedStrings(queries[queryName])
				if err := appendLog(&results, testcase, "N15_SOFTWARE_VERSION", map[string]any{
					"string":     value,
					"query_name": queryName,
					"ns_list":    strings.Join(list, ";"),
				}); err != nil {
					return results, err
				}
			}
		}
	}

	if len(errorOnVersionQuery) > 0 {
		queryNames := make([]string, 0, len(errorOnVersionQuery))
		for queryName := range errorOnVersionQuery {
			queryNames = append(queryNames, queryName)
		}
		sort.Strings(queryNames)
		for _, queryName := range queryNames {
			list := sortedStrings(errorOnVersionQuery[queryName])
			if err := appendLog(&results, testcase, "N15_ERROR_ON_VERSION_QUERY", map[string]any{
				"query_name": queryName,
				"ns_list":    strings.Join(list, ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	if len(sendingVersionQuery) > 0 {
		if err := appendLog(&results, testcase, "N15_NO_VERSION_REVEALED", map[string]any{
			"ns_list": strings.Join(sortedKeys(sendingVersionQuery), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(wrongRecordClass) > 0 {
		if err := appendLog(&results, testcase, "N15_WRONG_CLASS", map[string]any{
			"ns_list": strings.Join(sortedKeys(wrongRecordClass), ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

func normalizedAnswer(resp packet.Packet) string {
	if resp.Msg == nil || len(resp.Answer()) == 0 {
		return ""
	}
	values := make([]string, 0, len(resp.Answer()))
	for _, rr := range resp.Answer() {
		values = append(values, strings.ToLower(rr.String()))
	}
	sort.Strings(values)
	data, _ := json.Marshal(values)
	return string(data)
}

func firstNonEmpty(query1 string, resp1 packet.Packet, query2 string, resp2 packet.Packet) string {
	if resp1.Msg != nil {
		return query1
	}
	return query2
}

func parseAnswerFrom(value string) (netip.Addr, bool) {
	if value == "" {
		return netip.Addr{}, false
	}
	host := value
	if strings.Contains(value, ":") {
		if h, _, err := net.SplitHostPort(value); err == nil {
			host = h
		}
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr, true
}

func sortedStrings(values []string) []string {
	unique := map[string]bool{}
	for _, value := range values {
		if value != "" {
			unique[value] = true
		}
	}
	keys := make([]string, 0, len(unique))
	for value := range unique {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
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

func ipDisabledMessage(results *[]*logger.Entry, testcase string, server ns.Nameserver, rrtypes ...string) (bool, error) {
	if !profile.Effective().Net.IPv6 && server.Address.Is6() {
		for _, rrtype := range rrtypes {
			if err := appendLog(results, testcase, "IPV6_DISABLED", map[string]any{
				"ns":     server.String(),
				"rrtype": rrtype,
			}); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if !profile.Effective().Net.IPv4 && server.Address.Is4() {
		for _, rrtype := range rrtypes {
			if err := appendLog(results, testcase, "IPV4_DISABLED", map[string]any{
				"ns":     server.String(),
				"rrtype": rrtype,
			}); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}
