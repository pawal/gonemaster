package consistency

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/constants"
	"github.com/pawal/gonemaster/engine/dnsname"
	"github.com/pawal/gonemaster/engine/logger"
	"github.com/pawal/gonemaster/engine/methods"
	"github.com/pawal/gonemaster/engine/nameserver"
	"github.com/pawal/gonemaster/engine/packet"
	"github.com/pawal/gonemaster/engine/profile"
	"github.com/pawal/gonemaster/engine/util"
	"github.com/pawal/gonemaster/engine/zone"
)

const moduleName = "Consistency"

var (
	method4        = methods.Method4
	method5        = methods.Method5
	method2and3    = methods.Method2and3
	method4and5    = methods.Method4and5
	queryParentAll = defaultQueryParentAll
	recurse        = defaultRecurse
)

// All runs the Consistency test cases in order, mirroring the Perl implementation.
func All(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest("consistency01") {
		entries, err := Consistency01(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("consistency02") {
		entries, err := Consistency02(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("consistency03") {
		entries, err := Consistency03(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("consistency04") {
		entries, err := Consistency04(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("consistency05") {
		entries, err := Consistency05(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest("consistency06") {
		entries, err := Consistency06(ctx, z)
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// Metadata returns the set of tags emitted by Consistency test cases.
func Metadata() map[string][]string {
	return map[string][]string{
		"consistency01": {
			"NO_RESPONSE",
			"NO_RESPONSE_SOA_QUERY",
			"ONE_SOA_SERIAL",
			"MULTIPLE_SOA_SERIALS",
			"SOA_SERIAL",
			"SOA_SERIAL_VARIATION",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"consistency02": {
			"NO_RESPONSE",
			"NO_RESPONSE_SOA_QUERY",
			"ONE_SOA_RNAME",
			"MULTIPLE_SOA_RNAMES",
			"SOA_RNAME",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"consistency03": {
			"NO_RESPONSE",
			"NO_RESPONSE_SOA_QUERY",
			"ONE_SOA_TIME_PARAMETER_SET",
			"MULTIPLE_SOA_TIME_PARAMETER_SET",
			"SOA_TIME_PARAMETER_SET",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"consistency04": {
			"NO_RESPONSE",
			"NO_RESPONSE_NS_QUERY",
			"ONE_NS_SET",
			"MULTIPLE_NS_SET",
			"NS_SET",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"consistency05": {
			"ADDRESSES_MATCH",
			"CHILD_NS_FAILED",
			"CHILD_ZONE_LAME",
			"EXTRA_ADDRESS_CHILD",
			"IN_BAILIWICK_ADDR_MISMATCH",
			"NO_RESPONSE",
			"OUT_OF_BAILIWICK_ADDR_MISMATCH",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"consistency06": {
			"NO_RESPONSE",
			"NO_RESPONSE_SOA_QUERY",
			"ONE_SOA_MNAME",
			"MULTIPLE_SOA_MNAMES",
			"SOA_MNAME",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// Consistency01 runs the CONSISTENCY01 test case.
func Consistency01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency01"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsnamesAndIP := map[string]bool{}
	serials := map[string][]string{}
	queryType := "SOA"

	list4, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	list5, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}

		disabled, err := ipDisabledMessage(&results, testcase, ns, queryType)
		if err != nil {
			return results, err
		}
		if disabled {
			continue
		}

		resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
		if err != nil || resp.Msg == nil {
			if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		records := resp.GetRecordsForName(queryType, z.Name)
		if len(records) == 0 {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}
		soa, ok := records[0].(*dns.SOA)
		if !ok {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		serial := fmt.Sprintf("%d", soa.Serial)
		serials[serial] = append(serials[serial], key)
		nsnamesAndIP[key] = true
	}

	serialKeys := make([]string, 0, len(serials))
	for key := range serials {
		serialKeys = append(serialKeys, key)
	}
	sort.Strings(serialKeys)

	for _, serial := range serialKeys {
		nsList := append([]string{}, serials[serial]...)
		sort.Strings(nsList)
		if err := appendLog(&results, testcase, "SOA_SERIAL", map[string]any{
			"serial":  serial,
			"ns_list": strings.Join(nsList, ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(serialKeys) == 1 {
		if err := appendLog(&results, testcase, "ONE_SOA_SERIAL", map[string]any{
			"serial": serialKeys[0],
		}); err != nil {
			return results, err
		}
	} else if len(serialKeys) > 0 {
		if err := appendLog(&results, testcase, "MULTIPLE_SOA_SERIALS", map[string]any{
			"count": len(serialKeys),
		}); err != nil {
			return results, err
		}

		minVal, errMin := strconv.ParseInt(serialKeys[0], 10, 64)
		maxVal, errMax := strconv.ParseInt(serialKeys[len(serialKeys)-1], 10, 64)
		if errMin == nil && errMax == nil {
			if maxVal-minVal > int64(constants.SerialMaxVariation) {
				if err := appendLog(&results, testcase, "SOA_SERIAL_VARIATION", map[string]any{
					"serial_min":    serialKeys[0],
					"serial_max":    serialKeys[len(serialKeys)-1],
					"max_variation": constants.SerialMaxVariation,
				}); err != nil {
					return results, err
				}
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Consistency02 runs the CONSISTENCY02 test case.
func Consistency02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency02"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsnamesAndIP := map[string]bool{}
	rnames := map[string][]string{}
	order := []string{}
	queryType := "SOA"

	list4, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	list5, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}

		disabled, err := ipDisabledMessage(&results, testcase, ns, queryType)
		if err != nil {
			return results, err
		}
		if disabled {
			continue
		}

		resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
		if err != nil || resp.Msg == nil {
			if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		records := resp.GetRecordsForName(queryType, z.Name)
		if len(records) == 0 {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}
		soa, ok := records[0].(*dns.SOA)
		if !ok {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		rname := strings.ToLower(soa.Mbox)
		if _, ok := rnames[rname]; !ok {
			order = append(order, rname)
		}
		rnames[rname] = append(rnames[rname], key)
		nsnamesAndIP[key] = true
	}

	if len(order) == 1 {
		if err := appendLog(&results, testcase, "ONE_SOA_RNAME", map[string]any{
			"rname": order[0],
		}); err != nil {
			return results, err
		}
	} else if len(order) > 0 {
		if err := appendLog(&results, testcase, "MULTIPLE_SOA_RNAMES", map[string]any{
			"count": len(order),
		}); err != nil {
			return results, err
		}
		for _, rname := range order {
			if err := appendLog(&results, testcase, "SOA_RNAME", map[string]any{
				"rname":   rname,
				"ns_list": strings.Join(rnames[rname], ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Consistency03 runs the CONSISTENCY03 test case.
func Consistency03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency03"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsnamesAndIP := map[string]bool{}
	timeSets := map[string][]string{}
	timeValues := map[string]timeParams{}
	order := []string{}
	queryType := "SOA"

	list4, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	list5, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}

		disabled, err := ipDisabledMessage(&results, testcase, ns, queryType)
		if err != nil {
			return results, err
		}
		if disabled {
			continue
		}

		resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
		if err != nil || resp.Msg == nil {
			if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		records := resp.GetRecordsForName(queryType, z.Name)
		if len(records) == 0 {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}
		soa, ok := records[0].(*dns.SOA)
		if !ok {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		params := timeParams{
			refresh: int(soa.Refresh),
			retry:   int(soa.Retry),
			expire:  int(soa.Expire),
			minimum: int(soa.Minttl),
		}
		setKey := fmt.Sprintf("%d;%d;%d;%d", params.refresh, params.retry, params.expire, params.minimum)
		if _, ok := timeSets[setKey]; !ok {
			order = append(order, setKey)
			timeValues[setKey] = params
		}
		timeSets[setKey] = append(timeSets[setKey], key)
		nsnamesAndIP[key] = true
	}

	if len(order) == 1 {
		params := timeValues[order[0]]
		if err := appendLog(&results, testcase, "ONE_SOA_TIME_PARAMETER_SET", map[string]any{
			"refresh": params.refresh,
			"retry":   params.retry,
			"expire":  params.expire,
			"minimum": params.minimum,
		}); err != nil {
			return results, err
		}
	} else if len(order) > 0 {
		if err := appendLog(&results, testcase, "MULTIPLE_SOA_TIME_PARAMETER_SET", map[string]any{
			"count": len(order),
		}); err != nil {
			return results, err
		}
		for _, setKey := range order {
			params := timeValues[setKey]
			nsList := append([]string{}, timeSets[setKey]...)
			sort.Strings(nsList)
			if err := appendLog(&results, testcase, "SOA_TIME_PARAMETER_SET", map[string]any{
				"refresh": params.refresh,
				"retry":   params.retry,
				"expire":  params.expire,
				"minimum": params.minimum,
				"ns_list": strings.Join(nsList, ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Consistency04 runs the CONSISTENCY04 test case.
func Consistency04(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency04"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsnamesAndIP := map[string]bool{}
	nsSets := map[string][]string{}
	order := []string{}
	queryType := "NS"

	list4, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	list5, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}

		disabled, err := ipDisabledMessage(&results, testcase, ns, queryType)
		if err != nil {
			return results, err
		}
		if disabled {
			continue
		}

		resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
		if err != nil || resp.Msg == nil {
			if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		records := resp.GetRecordsForName(queryType, z.Name)
		if len(records) == 0 {
			if err := appendLog(&results, testcase, "NO_RESPONSE_NS_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		var names []string
		for _, rr := range records {
			nsRR, ok := rr.(*dns.NS)
			if !ok {
				continue
			}
			names = append(names, strings.ToLower(nsRR.Ns))
		}
		if len(names) == 0 {
			if err := appendLog(&results, testcase, "NO_RESPONSE_NS_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}
		sort.Strings(names)

		setKey := strings.Join(names, ";")
		if _, ok := nsSets[setKey]; !ok {
			order = append(order, setKey)
		}
		nsSets[setKey] = append(nsSets[setKey], ns.String())
		nsnamesAndIP[key] = true
	}

	if len(order) == 1 {
		if err := appendLog(&results, testcase, "ONE_NS_SET", map[string]any{
			"nsname_list": order[0],
		}); err != nil {
			return results, err
		}
	} else if len(order) > 0 {
		if err := appendLog(&results, testcase, "MULTIPLE_NS_SET", map[string]any{
			"count": len(order),
		}); err != nil {
			return results, err
		}
		for _, setKey := range order {
			if err := appendLog(&results, testcase, "NS_SET", map[string]any{
				"nsname_list": setKey,
				"servers":     strings.Join(nsSets[setKey], ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Consistency05 runs the CONSISTENCY05 test case.
func Consistency05(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency05"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	strictGlue := map[string]bool{}
	extendedGlue := map[string][]string{}
	parentGlues := map[string]dnsname.Name{}

	nsResponses, err := queryParentAll(ctx, z, z.Name.String(), "NS")
	if err != nil {
		return results, err
	}

	var nsRecords []dns.RR
	for _, resp := range nsResponses {
		if resp.Msg == nil {
			continue
		}
		nsRecords = append(nsRecords, resp.GetRecordsForName("NS", z.Name)...)
	}

	childNSNames := map[string]dnsname.Name{}
	for _, rr := range nsRecords {
		nsRR, ok := rr.(*dns.NS)
		if !ok {
			continue
		}
		name := dnsname.New(strings.ToLower(nsRR.Ns))
		childNSNames[name.String()] = name
	}

	childKeys := make([]string, 0, len(childNSNames))
	for key := range childNSNames {
		childKeys = append(childKeys, key)
	}
	sort.Strings(childKeys)

	for _, key := range childKeys {
		nsName := childNSNames[key]
		aResponses, err := queryParentAll(ctx, z, nsName.String(), "A")
		if err != nil {
			return results, err
		}
		for _, resp := range aResponses {
			if resp.Msg == nil {
				continue
			}
			for _, rr := range resp.GetRecordsForName("A", nsName) {
				aRR, ok := rr.(*dns.A)
				if !ok {
					continue
				}
				glueKey := strings.ToLower(rr.Header().Name) + "/" + aRR.A.String()
				parentGlues[glueKey] = nsName
			}
		}

		aaaaResponses, err := queryParentAll(ctx, z, nsName.String(), "AAAA")
		if err != nil {
			return results, err
		}
		for _, resp := range aaaaResponses {
			if resp.Msg == nil {
				continue
			}
			for _, rr := range resp.GetRecordsForName("AAAA", nsName) {
				AAAArr, ok := rr.(*dns.AAAA)
				if !ok {
					continue
				}
				glueKey := strings.ToLower(rr.Header().Name) + "/" + AAAArr.AAAA.String()
				parentGlues[glueKey] = nsName
			}
		}
	}

	for nsString, nsName := range parentGlues {
		if z.Name.IsInBailiwick(nsName) {
			strictGlue[nsString] = true
			continue
		}
		nsKey := strings.ToLower(nsName.String())
		extendedGlue[nsKey] = append(extendedGlue[nsKey], nsString)
	}

	ibNames, err := method2and3(ctx, z)
	if err != nil {
		return results, err
	}
	var inBailiwickNames []dnsname.Name
	for _, name := range ibNames {
		if z.Name.IsInBailiwick(name) {
			inBailiwickNames = append(inBailiwickNames, name)
		}
	}

	ibNS, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}
	var inBailiwickServers []nameserver.Nameserver
	for _, ns := range ibNS {
		if ns.Address.Is4() && util.IPVersionOK(constants.IPVersion4) {
			inBailiwickServers = append(inBailiwickServers, ns)
			continue
		}
		if ns.Address.Is6() && util.IPVersionOK(constants.IPVersion6) {
			inBailiwickServers = append(inBailiwickServers, ns)
		}
	}

	childIBStrings := map[string]bool{}
	for _, nsName := range inBailiwickNames {
		isLame := true
		for _, ns := range inBailiwickServers {
			msgA, rrsA, err := getAddrRRs(ctx, ns, nsName, "A", z, testcase)
			if err != nil {
				return results, err
			}
			msgAAAA, rrsAAAA, err := getAddrRRs(ctx, ns, nsName, "AAAA", z, testcase)
			if err != nil {
				return results, err
			}

			if msgA != nil {
				results = append(results, msgA)
			}
			if msgAAAA != nil {
				results = append(results, msgAAAA)
			}
			if msgA == nil || msgAAAA == nil {
				isLame = false
			}

			for _, rr := range append(rrsA, rrsAAAA...) {
				childIBStrings[addrKey(rr)] = true
			}
		}

		if isLame {
			if err := appendLog(&results, testcase, "CHILD_ZONE_LAME", map[string]any{}); err != nil {
				return results, err
			}
			return appendTestCaseEnd(results, testcase)
		}
	}

	ibMismatch := []string{}
	for key := range strictGlue {
		if !childIBStrings[key] {
			ibMismatch = append(ibMismatch, key)
		}
	}
	ibExtraChild := []string{}
	for key := range childIBStrings {
		if !strictGlue[key] {
			ibExtraChild = append(ibExtraChild, key)
		}
	}

	if len(ibMismatch) > 0 {
		if err := appendLog(&results, testcase, "IN_BAILIWICK_ADDR_MISMATCH", map[string]any{
			"parent_addresses": strings.Join(sortedKeys(strictGlue), ";"),
			"zone_addresses":   strings.Join(sortedKeys(childIBStrings), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(ibExtraChild) > 0 {
		sort.Strings(ibExtraChild)
		if err := appendLog(&results, testcase, "EXTRA_ADDRESS_CHILD", map[string]any{
			"ns_ip_list": strings.Join(ibExtraChild, ";"),
		}); err != nil {
			return results, err
		}
	}

	oobMismatch := []string{}
	glueKeys := make([]string, 0, len(extendedGlue))
	for key := range extendedGlue {
		glueKeys = append(glueKeys, key)
	}
	sort.Strings(glueKeys)

	for _, glueName := range glueKeys {
		glueStrings := append([]string{}, extendedGlue[glueName]...)
		childOOB := map[string]bool{}

		respA, err := recurse(ctx, z, glueName, "A")
		if err == nil && respA.Msg != nil {
			for _, rr := range respA.GetRecordsForName("A", dnsname.New(glueName), "answer") {
				childOOB[addrKey(rr)] = true
			}
		}

		respAAAA, err := recurse(ctx, z, glueName, "AAAA")
		if err == nil && respAAAA.Msg != nil {
			for _, rr := range respAAAA.GetRecordsForName("AAAA", dnsname.New(glueName), "answer") {
				childOOB[addrKey(rr)] = true
			}
		}

		var mismatchForGlue []string
		for _, glueString := range glueStrings {
			if childOOB[glueString] {
				continue
			}
			mismatchForGlue = append(mismatchForGlue, glueString)
			oobMismatch = append(oobMismatch, glueString)
		}

		if len(mismatchForGlue) > 0 {
			sort.Strings(glueStrings)
			if err := appendLog(&results, testcase, "OUT_OF_BAILIWICK_ADDR_MISMATCH", map[string]any{
				"parent_addresses": strings.Join(glueStrings, ";"),
				"zone_addresses":   strings.Join(sortedKeys(childOOB), ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	if len(ibExtraChild) == 0 && len(ibMismatch) == 0 && len(oobMismatch) == 0 {
		if err := appendLog(&results, testcase, "ADDRESSES_MATCH", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Consistency06 runs the CONSISTENCY06 test case.
func Consistency06(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency06"
	var results []*logger.Entry

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsnamesAndIP := map[string]bool{}
	mnames := map[string][]string{}
	order := []string{}
	queryType := "SOA"

	list4, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	list5, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}

		disabled, err := ipDisabledMessage(&results, testcase, ns, queryType)
		if err != nil {
			return results, err
		}
		if disabled {
			continue
		}

		resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
		if err != nil || resp.Msg == nil {
			if err := appendLog(&results, testcase, "NO_RESPONSE", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		records := resp.GetRecordsForName(queryType, z.Name)
		if len(records) == 0 {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}
		soa, ok := records[0].(*dns.SOA)
		if !ok {
			if err := appendLog(&results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{"ns": ns.String()}); err != nil {
				return results, err
			}
			continue
		}

		mname := strings.ToLower(soa.Ns)
		if _, ok := mnames[mname]; !ok {
			order = append(order, mname)
		}
		mnames[mname] = append(mnames[mname], key)
		nsnamesAndIP[key] = true
	}

	if len(order) == 1 {
		if err := appendLog(&results, testcase, "ONE_SOA_MNAME", map[string]any{
			"mname": order[0],
		}); err != nil {
			return results, err
		}
	} else if len(order) > 0 {
		if err := appendLog(&results, testcase, "MULTIPLE_SOA_MNAMES", map[string]any{
			"count": len(order),
		}); err != nil {
			return results, err
		}
		for _, mname := range order {
			if err := appendLog(&results, testcase, "SOA_MNAME", map[string]any{
				"mname":   mname,
				"ns_list": strings.Join(mnames[mname], ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

type timeParams struct {
	refresh int
	retry   int
	expire  int
	minimum int
}

func defaultQueryParentAll(ctx context.Context, z *zone.Zone, name string, qtype string) ([]packet.Packet, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	parent, err := z.Parent(ctx)
	if err != nil || parent == nil {
		return nil, err
	}
	return parent.QueryAll(ctx, name, qtype, nil)
}

func defaultRecurse(ctx context.Context, z *zone.Zone, name string, qtype string) (packet.Packet, error) {
	if z == nil || z.Recursor() == nil {
		return packet.Packet{}, fmt.Errorf("missing recursor")
	}
	return z.Recursor().Recurse(ctx, name, qtype, "IN")
}

func getAddrRRs(ctx context.Context, ns nameserver.Nameserver, name dnsname.Name, qtype string, z *zone.Zone, testcase string) (*logger.Entry, []dns.RR, error) {
	recurseOff := false
	opts := &nameserver.QueryOptions{Recurse: &recurseOff}
	resp, err := ns.QueryWithOptions(ctx, name.String(), qtype, opts)
	if err != nil || resp.Msg == nil {
		entry, addErr := util.Logger().Add("NO_RESPONSE", map[string]any{"ns": ns.String()}, moduleName, testcase)
		if addErr != nil {
			return nil, nil, addErr
		}
		return entry, nil, nil
	}

	if resp.IsRedirect() {
		recResp, recErr := recurse(ctx, z, name.String(), qtype)
		if recErr == nil && recResp.Msg != nil {
			return nil, recResp.GetRecordsForName(qtype, name, "answer"), nil
		}
		return nil, nil, nil
	}

	if resp.AA() && resp.Rcode() == "NOERROR" {
		return nil, resp.GetRecordsForName(qtype, name, "answer"), nil
	}

	if !(resp.AA() && resp.Rcode() == "NXDOMAIN") {
		entry, addErr := util.Logger().Add("CHILD_NS_FAILED", map[string]any{"ns": ns.String()}, moduleName, testcase)
		if addErr != nil {
			return nil, nil, addErr
		}
		return entry, nil, nil
	}

	return nil, nil, nil
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

func addrKey(rr dns.RR) string {
	owner := strings.ToLower(rr.Header().Name)
	switch r := rr.(type) {
	case *dns.A:
		return owner + "/" + r.A.String()
	case *dns.AAAA:
		return owner + "/" + r.AAAA.String()
	default:
		return owner + "/" + rr.String()
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
