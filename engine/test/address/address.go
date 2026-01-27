package address

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/methods"
	methodsv2 "codeberg.org/pawal/gonemaster/engine/methodsv2"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

const addressModuleName = "Address"

// AddressAll runs the Address test cases in order, mirroring the Perl implementation.
func AddressAll(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest("address01") {
		entries, err := Address01(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	nsWithReverse := true
	if util.ShouldRunTest("address02") {
		entries, err := Address02(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
		nsWithReverse = hasTag(results, "NAMESERVERS_IP_WITH_REVERSE")
	}

	if nsWithReverse && util.ShouldRunTest("address03") {
		entries, err := Address03(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// AddressMetadata returns the set of tags emitted by Address test cases.
func AddressMetadata() map[string][]string {
	return map[string][]string{
		"address01": {
			"A01_ADDR_NOT_GLOBALLY_REACHABLE",
			"A01_DOCUMENTATION_ADDR",
			"A01_GLOBALLY_REACHABLE_ADDR",
			"A01_LOCAL_USE_ADDR",
			"A01_NO_GLOBALLY_REACHABLE_ADDR",
			"A01_NO_NAME_SERVERS_FOUND",
		},
		"address02": {
			"NAMESERVER_IP_WITHOUT_REVERSE",
			"NAMESERVERS_IP_WITH_REVERSE",
			"NO_RESPONSE_PTR_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"address03": {
			"NAMESERVER_IP_WITHOUT_REVERSE",
			"NAMESERVER_IP_PTR_MISMATCH",
			"NAMESERVER_IP_PTR_MATCH",
			"NO_RESPONSE_PTR_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// Address01 runs the ADDRESS01 test case.
func Address01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Address01"
	var results []*logger.Entry
	logger.ModuleName = addressModuleName
	logger.TestCaseName = testcase

	if err := appendAddressLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	delItems, err := methodsv2.GetDelNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}
	zoneItems, err := methodsv2.GetZoneNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}

	seen := map[string]methodsv2.NSItem{}
	for _, item := range append(delItems, zoneItems...) {
		if !item.HasAddress {
			continue
		}
		key := strings.ToLower(item.String())
		if _, ok := seen[key]; !ok {
			seen[key] = item
		}
	}

	if len(seen) == 0 {
		if err := appendAddressLog(&results, testcase, "A01_NO_NAME_SERVERS_FOUND", map[string]any{}); err != nil {
			return results, err
		}
		return appendAddressTestCaseEnd(results, testcase)
	}

	ipGroups := map[string][]methodsv2.NSItem{}
	for _, item := range seen {
		ipGroups[item.Address.String()] = append(ipGroups[item.Address.String()], item)
	}

	ipKeys := make([]string, 0, len(ipGroups))
	for key := range ipGroups {
		ipKeys = append(ipKeys, key)
	}
	sort.Strings(ipKeys)

	var documentationAddr []string
	var localUseAddr []string
	var notGloballyReachable []string
	var globallyReachable []string

	for _, ip := range ipKeys {
		group := ipGroups[ip]
		if len(group) == 0 {
			continue
		}

		block := constants.FindSpecialAddress(group[0].Address)
		if block != nil {
			category := block.Name
			if strings.Contains(category, "Documentation") {
				documentationAddr = append(documentationAddr, nsItemStrings(group)...)
				continue
			}
			if isLocalUseCategory(category) {
				localUseAddr = append(localUseAddr, nsItemStrings(group)...)
				continue
			}
			if !constants.IsGloballyReachable(block) {
				notGloballyReachable = append(notGloballyReachable, nsItemStrings(group)...)
				continue
			}
		}

		globallyReachable = append(globallyReachable, nsItemStrings(group)...)
	}

	if len(globallyReachable) > 0 {
		if err := appendAddressLog(&results, testcase, "A01_GLOBALLY_REACHABLE_ADDR", map[string]any{
			"ns_list": joinNSList(globallyReachable),
		}); err != nil {
			return results, err
		}
	} else {
		if err := appendAddressLog(&results, testcase, "A01_NO_GLOBALLY_REACHABLE_ADDR", map[string]any{}); err != nil {
			return results, err
		}
	}

	if len(documentationAddr) > 0 {
		if err := appendAddressLog(&results, testcase, "A01_DOCUMENTATION_ADDR", map[string]any{
			"ns_list": joinNSList(documentationAddr),
		}); err != nil {
			return results, err
		}
	}

	if len(localUseAddr) > 0 {
		if err := appendAddressLog(&results, testcase, "A01_LOCAL_USE_ADDR", map[string]any{
			"ns_list": joinNSList(localUseAddr),
		}); err != nil {
			return results, err
		}
	}

	if len(notGloballyReachable) > 0 {
		if err := appendAddressLog(&results, testcase, "A01_ADDR_NOT_GLOBALLY_REACHABLE", map[string]any{
			"ns_list": joinNSList(notGloballyReachable),
		}); err != nil {
			return results, err
		}
	}

	return appendAddressTestCaseEnd(results, testcase)
}

// Address02 runs the ADDRESS02 test case.
func Address02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Address02"
	var results []*logger.Entry
	logger.ModuleName = addressModuleName
	logger.TestCaseName = testcase

	if err := appendAddressLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	rec := z.Recursor()
	if rec == nil {
		return results, fmt.Errorf("missing recursor")
	}

	method4, err := methods.Method4(ctx, z)
	if err != nil {
		return results, err
	}
	method5, err := methods.Method5(ctx, z)
	if err != nil {
		return results, err
	}

	ips := map[string]bool{}
	for _, ns := range append(method4, method5...) {
		ip := ns.Address.String()
		if ips[ip] {
			continue
		}
		ips[ip] = true

		ptrQuery, err := dns.ReverseAddr(ip)
		if err != nil {
			return results, err
		}

		resp, err := rec.Recurse(ctx, ptrQuery, "PTR", "IN")
		if err != nil {
			return results, err
		}

		if resp.Msg != nil && resp.Rcode() == "NOERROR" && len(resp.GetRecords("CNAME", "answer")) > 0 {
			if cname, ok := resp.GetRecords("CNAME", "answer")[0].(*dns.CNAME); ok {
				ptrQuery = cname.Target
				resp, err = rec.Recurse(ctx, ptrQuery, "PTR", "IN")
				if err != nil {
					return results, err
				}
			}
		}

		if resp.Msg != nil {
			if resp.Rcode() != "NOERROR" || len(resp.GetRecords("PTR", "answer")) == 0 {
				if err := appendAddressLog(&results, testcase, "NAMESERVER_IP_WITHOUT_REVERSE", map[string]any{
					"nsname": ns.Name.String(),
					"ns_ip":  ip,
				}); err != nil {
					return results, err
				}
			}
		} else {
			if err := appendAddressLog(&results, testcase, "NO_RESPONSE_PTR_QUERY", map[string]any{
				"domain": ptrQuery,
			}); err != nil {
				return results, err
			}
		}
	}

	if len(ips) > 0 && onlyTestCaseStart(results) {
		if err := appendAddressLog(&results, testcase, "NAMESERVERS_IP_WITH_REVERSE", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendAddressTestCaseEnd(results, testcase)
}

// Address03 runs the ADDRESS03 test case.
func Address03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Address03"
	var results []*logger.Entry
	logger.ModuleName = addressModuleName
	logger.TestCaseName = testcase

	if err := appendAddressLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	rec := z.Recursor()
	if rec == nil {
		return results, fmt.Errorf("missing recursor")
	}

	method5, err := methods.Method5(ctx, z)
	if err != nil {
		return results, err
	}

	ips := map[string]bool{}
	for _, ns := range method5 {
		ip := ns.Address.String()
		if ips[ip] {
			continue
		}
		ips[ip] = true

		ptrQuery, err := dns.ReverseAddr(ip)
		if err != nil {
			return results, err
		}

		resp, err := rec.Recurse(ctx, ptrQuery, "PTR", "IN")
		if err != nil {
			return results, err
		}

		if resp.Msg != nil {
			ptrRecords := resp.GetRecords("PTR", "answer")
			if resp.Rcode() == "NOERROR" && len(ptrRecords) > 0 {
				names := make([]string, 0, len(ptrRecords))
				matched := false
				for _, rr := range ptrRecords {
					ptr, ok := rr.(*dns.PTR)
					if !ok {
						continue
					}
					names = append(names, ptr.Ptr)
					ptrName := dnsname.New(ptr.Ptr)
					if strings.EqualFold(ptrName.String(), ns.Name.String()) {
						matched = true
					}
				}

				if !matched {
					if err := appendAddressLog(&results, testcase, "NAMESERVER_IP_PTR_MISMATCH", map[string]any{
						"nsname": ns.Name.String(),
						"ns_ip":  ip,
						"names":  strings.Join(names, "/"),
					}); err != nil {
						return results, err
					}
				}
			} else {
				if err := appendAddressLog(&results, testcase, "NAMESERVER_IP_WITHOUT_REVERSE", map[string]any{
					"nsname": ns.Name.String(),
					"ns_ip":  ip,
				}); err != nil {
					return results, err
				}
			}
		} else {
			if err := appendAddressLog(&results, testcase, "NO_RESPONSE_PTR_QUERY", map[string]any{
				"domain": ptrQuery,
			}); err != nil {
				return results, err
			}
		}
	}

	if len(ips) > 0 && onlyTestCaseStart(results) {
		if err := appendAddressLog(&results, testcase, "NAMESERVER_IP_PTR_MATCH", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendAddressTestCaseEnd(results, testcase)
}

func appendAddressTestCaseEnd(results []*logger.Entry, testcase string) ([]*logger.Entry, error) {
	if err := appendAddressLog(&results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

func appendAddressLog(results *[]*logger.Entry, testcase string, tag string, args map[string]any) error {
	entry, err := util.Logger().Add(tag, args, addressModuleName, testcase)
	if err != nil {
		return err
	}
	*results = append(*results, entry)
	return nil
}

func onlyTestCaseStart(entries []*logger.Entry) bool {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag != "TEST_CASE_START" {
			return false
		}
	}
	return true
}

func hasTag(entries []*logger.Entry, tag string) bool {
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

func nsItemStrings(items []methodsv2.NSItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.String())
	}
	return out
}

func joinNSList(items []string) string {
	unique := uniqueSortedStrings(items)
	return strings.Join(unique, ";")
}

func uniqueSortedStrings(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func isLocalUseCategory(category string) bool {
	localCategories := []string{
		"Private-Use",
		"Loopback",
		"Link Local",
		"Link-Local",
		"Unique-Local",
		"Shared Address Space",
	}

	for _, item := range localCategories {
		if strings.Contains(category, item) {
			return true
		}
	}
	return false
}
