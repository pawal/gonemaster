package delegation

import (
	"context"
	"fmt"
	"sort"
	"strings"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logargs"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/methods"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/runner"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testcase"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testlogger"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

const moduleName = "Delegation"

var (
	method1     = methods.Method1
	method2     = methods.Method2
	method3     = methods.Method3
	method4     = methods.Method4
	method5     = methods.Method5
	method2and3 = methods.Method2and3
	recurse     = defaultRecurse
)

// All runs the Delegation test cases in order, mirroring the Perl implementation.
func All(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest(ctx, "delegation01") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Delegation01(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "delegation02") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Delegation02(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "delegation03") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Delegation03(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "delegation04") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Delegation04(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "delegation05") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Delegation05(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "delegation06") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Delegation06(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "delegation07") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Delegation07(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// Metadata returns the set of tags emitted by Delegation test cases.
func Metadata() map[string][]string {
	return map[string][]string{
		"delegation01": {
			"ENOUGH_NS_CHILD",
			"ENOUGH_NS_DEL",
			"NOT_ENOUGH_NS_DEL",
			"NOT_ENOUGH_NS_CHILD",
			"ENOUGH_IPV4_NS_CHILD",
			"ENOUGH_IPV4_NS_DEL",
			"ENOUGH_IPV6_NS_CHILD",
			"ENOUGH_IPV6_NS_DEL",
			"NOT_ENOUGH_IPV4_NS_CHILD",
			"NOT_ENOUGH_IPV4_NS_DEL",
			"NOT_ENOUGH_IPV6_NS_CHILD",
			"NOT_ENOUGH_IPV6_NS_DEL",
			"NO_IPV4_NS_CHILD",
			"NO_IPV4_NS_DEL",
			"NO_IPV6_NS_CHILD",
			"NO_IPV6_NS_DEL",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"delegation02": {
			"CHILD_DISTINCT_NS_IP",
			"CHILD_NS_SAME_IP",
			"DEL_DISTINCT_NS_IP",
			"DEL_NS_SAME_IP",
			"SAME_IP_ADDRESS",
			"DISTINCT_IP_ADDRESS",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"delegation03": {
			"REFERRAL_SIZE_TOO_LARGE",
			"REFERRAL_SIZE_OK",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"delegation04": {
			"IS_NOT_AUTHORITATIVE",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"ARE_AUTHORITATIVE",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"delegation05": {
			"NO_NS_CNAME",
			"NO_RESPONSE",
			"NS_IS_CNAME",
			"UNEXPECTED_RCODE",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"delegation06": {
			"SOA_NOT_EXISTS",
			"SOA_EXISTS",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"delegation07": {
			"EXTRA_NAME_PARENT",
			"EXTRA_NAME_CHILD",
			"TOTAL_NAME_MISMATCH",
			"NAMES_MATCH",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// Delegation01 runs the DELEGATION01 test case.
func Delegation01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Delegation01"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	delNames, err := method2(ctx, z)
	if err != nil {
		return results, err
	}
	delNameStrings := namesToStrings(delNames)
	sort.Strings(delNameStrings)
	delArgs := map[string]any{
		"count":   len(delNameStrings),
		"minimum": constants.MinimumNumberOfNameservers,
	}
	setTypedServersFromNames(delArgs, delNameStrings)
	if len(delNameStrings) >= constants.MinimumNumberOfNameservers {
		if err := appendLog(ctx, &results, testcase, "ENOUGH_NS_DEL", delArgs); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NOT_ENOUGH_NS_DEL", delArgs); err != nil {
			return results, err
		}
	}

	childNames, err := method3(ctx, z)
	if err != nil {
		return results, err
	}
	childNameStrings := namesToStrings(childNames)
	sort.Strings(childNameStrings)
	childArgs := map[string]any{
		"count":   len(childNameStrings),
		"minimum": constants.MinimumNumberOfNameservers,
	}
	setTypedServersFromNames(childArgs, childNameStrings)
	if len(childNameStrings) >= constants.MinimumNumberOfNameservers {
		if err := appendLog(ctx, &results, testcase, "ENOUGH_NS_CHILD", childArgs); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NOT_ENOUGH_NS_CHILD", childArgs); err != nil {
			return results, err
		}
	}

	childNS, err := method5(ctx, z)
	if err != nil {
		return results, err
	}
	childIPv4 := filterByIPVersion(childNS, constants.IPVersion4)
	childIPv6 := filterByIPVersion(childNS, constants.IPVersion6)

	childIPv4Count := uniqueNamesCount(childIPv4)
	childIPv4Args := map[string]any{
		"count":   childIPv4Count,
		"minimum": constants.MinimumNumberOfNameservers,
	}
	setTypedEndpointsFromNameservers(childIPv4Args, childIPv4)
	if childIPv4Count >= constants.MinimumNumberOfNameservers {
		if err := appendLog(ctx, &results, testcase, "ENOUGH_IPV4_NS_CHILD", childIPv4Args); err != nil {
			return results, err
		}
	} else if childIPv4Count > 0 {
		if err := appendLog(ctx, &results, testcase, "NOT_ENOUGH_IPV4_NS_CHILD", childIPv4Args); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_IPV4_NS_CHILD", childIPv4Args); err != nil {
			return results, err
		}
	}

	childIPv6Count := uniqueNamesCount(childIPv6)
	childIPv6Args := map[string]any{
		"count":   childIPv6Count,
		"minimum": constants.MinimumNumberOfNameservers,
	}
	setTypedEndpointsFromNameservers(childIPv6Args, childIPv6)
	if childIPv6Count >= constants.MinimumNumberOfNameservers {
		if err := appendLog(ctx, &results, testcase, "ENOUGH_IPV6_NS_CHILD", childIPv6Args); err != nil {
			return results, err
		}
	} else if childIPv6Count > 0 {
		if err := appendLog(ctx, &results, testcase, "NOT_ENOUGH_IPV6_NS_CHILD", childIPv6Args); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_IPV6_NS_CHILD", childIPv6Args); err != nil {
			return results, err
		}
	}

	delNS, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	delIPv4 := filterByIPVersion(delNS, constants.IPVersion4)
	delIPv6 := filterByIPVersion(delNS, constants.IPVersion6)

	delIPv4Count := uniqueNamesCount(delIPv4)
	delIPv4Args := map[string]any{
		"count":   delIPv4Count,
		"minimum": constants.MinimumNumberOfNameservers,
	}
	setTypedEndpointsFromNameservers(delIPv4Args, delIPv4)
	if delIPv4Count >= constants.MinimumNumberOfNameservers {
		if err := appendLog(ctx, &results, testcase, "ENOUGH_IPV4_NS_DEL", delIPv4Args); err != nil {
			return results, err
		}
	} else if delIPv4Count > 0 {
		if err := appendLog(ctx, &results, testcase, "NOT_ENOUGH_IPV4_NS_DEL", delIPv4Args); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_IPV4_NS_DEL", delIPv4Args); err != nil {
			return results, err
		}
	}

	delIPv6Count := uniqueNamesCount(delIPv6)
	delIPv6Args := map[string]any{
		"count":   delIPv6Count,
		"minimum": constants.MinimumNumberOfNameservers,
	}
	setTypedEndpointsFromNameservers(delIPv6Args, delIPv6)
	if delIPv6Count >= constants.MinimumNumberOfNameservers {
		if err := appendLog(ctx, &results, testcase, "ENOUGH_IPV6_NS_DEL", delIPv6Args); err != nil {
			return results, err
		}
	} else if delIPv6Count > 0 {
		if err := appendLog(ctx, &results, testcase, "NOT_ENOUGH_IPV6_NS_DEL", delIPv6Args); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NO_IPV6_NS_DEL", delIPv6Args); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Delegation02 runs the DELEGATION02 test case.
func Delegation02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Delegation02"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	delNS, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	childNS, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	entries, err := findDupNS(ctx, testcase, "DEL_NS_SAME_IP", "DEL_DISTINCT_NS_IP", delNS)
	if err != nil {
		return results, err
	}
	results = append(results, entries...)

	entries, err = findDupNS(ctx, testcase, "CHILD_NS_SAME_IP", "CHILD_DISTINCT_NS_IP", childNS)
	if err != nil {
		return results, err
	}
	results = append(results, entries...)

	combined := append([]nameserver.Nameserver{}, delNS...)
	combined = append(combined, childNS...)
	entries, err = findDupNS(ctx, testcase, "SAME_IP_ADDRESS", "DISTINCT_IP_ADDRESS", combined)
	if err != nil {
		return results, err
	}
	results = append(results, entries...)

	return appendTestCaseEnd(ctx, results, testcase)
}

// Delegation03 runs the DELEGATION03 test case.
func Delegation03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Delegation03"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	longName := maxLengthNameFor(z.Name)
	nsNames, err := method2(ctx, z)
	if err != nil {
		return results, err
	}
	nss, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	parent, err := method1(ctx, z)
	if err != nil {
		return results, err
	}
	if parent == nil {
		return results, fmt.Errorf("missing parent")
	}

	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(longName), dns.TypeNS)
	for _, nsName := range nsNames {
		nsRR := &dns.NS{}
		nsRR.Hdr = dns.Header{Name: dnsutil.Fqdn(z.Name.String()), Class: dns.ClassINET}
		nsRR.Ns = dnsutil.Fqdn(nsName.String())
		msg.Ns = append(msg.Ns, nsRR)
	}

	nssV4 := filterByIPVersion(nss, constants.IPVersion4)
	if len(nssV4) > 0 && allInBailiwick(parent.Name, nssV4) {
		ns := nssV4[0]
		aRR := &dns.A{}
		aRR.Hdr = dns.Header{Name: dnsutil.Fqdn(ns.Name.String()), Class: dns.ClassINET}
		aRR.Addr = ns.Address
		msg.Extra = append(msg.Extra, aRR)
	}

	nssV6 := filterByIPVersion(nss, constants.IPVersion6)
	if len(nssV6) > 0 && allInBailiwick(parent.Name, nssV6) {
		ns := nssV6[0]
		aaaaRR := &dns.AAAA{}
		aaaaRR.Hdr = dns.Header{Name: dnsutil.Fqdn(ns.Name.String()), Class: dns.ClassINET}
		aaaaRR.Addr = ns.Address
		msg.Extra = append(msg.Extra, aaaaRR)
	}

	if err := msg.Pack(); err != nil {
		return results, err
	}
	size := len(msg.Data)
	if size > constants.UDPPayloadLimit {
		if err := appendLog(ctx, &results, testcase, "REFERRAL_SIZE_TOO_LARGE", map[string]any{"size": size}); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "REFERRAL_SIZE_OK", map[string]any{"size": size}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Delegation04 runs the DELEGATION04 test case.
func Delegation04(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Delegation04"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	list4, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	list5, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	var authoritatives []string
	queryType := "SOA"

	type nsAction int
	const (
		actionSkip nsAction = iota
		actionDisabled
		actionQuery
	)
	type nsTask struct {
		ns      nameserver.Nameserver
		action  nsAction
		nameKey string
	}

	seen := map[string]bool{}
	ordered := make([]nsTask, 0, len(list4)+len(list5))
	for _, ns := range append(list4, list5...) {
		nameKey := ns.Name.String()
		action := actionQuery
		if (ns.Address.Is6() && !profile.FromContext(ctx).Net.IPv6) || (ns.Address.Is4() && !profile.FromContext(ctx).Net.IPv4) {
			action = actionDisabled
		} else if seen[nameKey] {
			action = actionSkip
		} else {
			seen[nameKey] = true
		}
		ordered = append(ordered, nsTask{ns: ns, action: action, nameKey: nameKey})
	}

	if len(ordered) > 0 {
		outcomes := make([]bool, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, task := range ordered {
			i, task := i, task
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				if task.action == actionSkip {
					return nil
				}
				buf := testlogger.Wrap(log, moduleName, testcase)
				if task.action == actionDisabled {
					_, err := ipDisabledMessageWithLogger(ctx, buf, task.ns, queryType)
					return err
				}
				authoritative := false
				for _, useVC := range []bool{false, true} {
					useVC := useVC
					resp, err := task.ns.QueryWithOptions(ctx, z.Name.String(), queryType, &nameserver.QueryOptions{UseVC: &useVC})
					if err != nil || resp.Msg == nil {
						continue
					}
					if !resp.AA() {
						if _, err := buf.Add("IS_NOT_AUTHORITATIVE", withNameserverArgs(task.ns, map[string]any{
							"proto": protoLabel(useVC),
						})); err != nil {
							return err
						}
					} else {
						authoritative = true
					}
				}
				outcomes[i] = authoritative
				return nil
			}
		}

		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)

		for i, ok := range outcomes {
			if ok {
				authoritatives = append(authoritatives, ordered[i].nameKey)
			}
		}
	}

	if (len(list4) > 0 || len(list5) > 0) && !hasTag(results, "IS_NOT_AUTHORITATIVE") && len(authoritatives) > 0 {
		uniq := uniqueStrings(authoritatives)
		sort.Strings(uniq)
		args := map[string]any{}
		setTypedServersFromNames(args, uniq)
		if err := appendLog(ctx, &results, testcase, "ARE_AUTHORITATIVE", args); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Delegation05 runs the DELEGATION05 test case.
func Delegation05(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Delegation05"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsNames, err := method2and3(ctx, z)
	if err != nil {
		return results, err
	}

	list4, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	list5, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	allNS := map[string]nameserver.Nameserver{}
	for _, ns := range append(list4, list5...) {
		allNS[ns.String()] = ns
	}
	keys := make([]string, 0, len(allNS))
	for key := range allNS {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, nsName := range nsNames {
		if z.Name.IsInBailiwick(nsName) {
			if len(keys) > 0 {
				tasks := make([]runner.Task, len(keys))
				for i, key := range keys {
					i, key := i, key
					ns := allNS[key]
					tasks[i] = func(ctx context.Context, log *logger.Logger) error {
						buf := testlogger.Wrap(log, moduleName, testcase)
						args := withNameserverArgs(ns, map[string]any{
							"query_name": nsName.String(),
							"query_type": "A",
						})

						disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "A")
						if err != nil {
							return err
						}
						if disabled {
							return nil
						}

						recurseOff := false
						resp, err := ns.QueryWithOptions(ctx, nsName.String(), "A", &nameserver.QueryOptions{Recurse: &recurseOff})
						if err != nil || resp.Msg == nil {
							_, err := buf.Add("NO_RESPONSE", args)
							return err
						}
						if resp.Rcode() != "NOERROR" {
							args["rcode"] = resp.Rcode()
							_, err := buf.Add("UNEXPECTED_RCODE", args)
							return err
						}
						if len(resp.GetRecords("CNAME", "answer")) > 0 {
							_, err := buf.Add("NS_IS_CNAME", map[string]any{"ns": logargs.EndpointName(nsName.String())})
							return err
						}
						if resp.IsRedirect() {
							recurseOn := true
							recResp, err := ns.QueryWithOptions(ctx, nsName.String(), "A", &nameserver.QueryOptions{Recurse: &recurseOn})
							if err == nil && recResp.Msg != nil && len(recResp.GetRecords("CNAME", "answer")) > 0 {
								_, err := buf.Add("NS_IS_CNAME", map[string]any{"ns": logargs.EndpointName(nsName.String())})
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
		} else {
			resp, err := recurse(ctx, z, nsName.String(), "A")
			if err == nil && resp.Msg != nil && len(resp.GetRecords("CNAME", "answer")) > 0 {
				if err := appendLog(ctx, &results, testcase, "NS_IS_CNAME", map[string]any{"ns": logargs.EndpointName(nsName.String())}); err != nil {
					return results, err
				}
			}
		}
	}

	if !hasTag(results, "NS_IS_CNAME") {
		if err := appendLog(ctx, &results, testcase, "NO_NS_CNAME", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Delegation06 runs the DELEGATION06 test case.
func Delegation06(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Delegation06"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	list4, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	list5, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	queryType := "SOA"

	type nsAction int
	const (
		actionSkip nsAction = iota
		actionDisabled
		actionQuery
	)
	type nsTask struct {
		ns      nameserver.Nameserver
		action  nsAction
		nameKey string
	}

	seen := map[string]bool{}
	ordered := make([]nsTask, 0, len(list4)+len(list5))
	for _, ns := range append(list4, list5...) {
		nameKey := ns.Name.String()
		action := actionQuery
		if (ns.Address.Is6() && !profile.FromContext(ctx).Net.IPv6) || (ns.Address.Is4() && !profile.FromContext(ctx).Net.IPv4) {
			action = actionDisabled
		} else if seen[nameKey] {
			action = actionSkip
		} else {
			seen[nameKey] = true
		}
		ordered = append(ordered, nsTask{ns: ns, action: action, nameKey: nameKey})
	}

	if len(ordered) > 0 {
		tasks := make([]runner.Task, len(ordered))
		for i, task := range ordered {
			task := task
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				if task.action == actionSkip {
					return nil
				}
				buf := testlogger.Wrap(log, moduleName, testcase)
				if task.action == actionDisabled {
					_, err := ipDisabledMessageWithLogger(ctx, buf, task.ns, queryType)
					return err
				}
				resp, err := task.ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
				if err == nil && resp.Msg != nil && resp.Rcode() == "NOERROR" {
					if len(resp.GetRecords(queryType, "answer")) == 0 {
						_, err := buf.Add("SOA_NOT_EXISTS", withNameserverArgs(task.ns, nil))
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

	if (len(list4) > 0 || len(list5) > 0) && onlyTestCaseStart(results) {
		if err := appendLog(ctx, &results, testcase, "SOA_EXISTS", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Delegation07 runs the DELEGATION07 test case.
func Delegation07(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Delegation07"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	parentNames, err := method2(ctx, z)
	if err != nil {
		return results, err
	}
	childNames, err := method3(ctx, z)
	if err != nil {
		return results, err
	}

	nameCounts := map[string]int{}
	for _, name := range parentNames {
		nameCounts[name.String()]++
	}
	for _, name := range childNames {
		nameCounts[name.String()]--
	}

	var sameNames []string
	var extraParent []string
	var extraChild []string
	for name, count := range nameCounts {
		switch {
		case count == 0:
			sameNames = append(sameNames, name)
		case count > 0:
			extraParent = append(extraParent, name)
		default:
			extraChild = append(extraChild, name)
		}
	}
	sort.Strings(sameNames)
	sort.Strings(extraParent)
	sort.Strings(extraChild)

	if len(extraParent) > 0 {
		if err := appendLog(ctx, &results, testcase, "EXTRA_NAME_PARENT", map[string]any{"extra": strings.Join(extraParent, ";")}); err != nil {
			return results, err
		}
	}
	if len(extraChild) > 0 {
		if err := appendLog(ctx, &results, testcase, "EXTRA_NAME_CHILD", map[string]any{"extra": strings.Join(extraChild, ";")}); err != nil {
			return results, err
		}
	}
	if len(extraParent) == 0 && len(extraChild) == 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sameNames)
		if err := appendLog(ctx, &results, testcase, "NAMES_MATCH", args); err != nil {
			return results, err
		}
	}
	if len(sameNames) == 0 {
		if err := appendLog(ctx, &results, testcase, "TOTAL_NAME_MISMATCH", map[string]any{
			"glue":  strings.Join(extraParent, ";"),
			"child": strings.Join(extraChild, ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

func filterByIPVersion(nss []nameserver.Nameserver, version int) []nameserver.Nameserver {
	var out []nameserver.Nameserver
	for _, ns := range nss {
		if version == constants.IPVersion4 && ns.Address.Is4() {
			out = append(out, ns)
		}
		if version == constants.IPVersion6 && ns.Address.Is6() {
			out = append(out, ns)
		}
	}
	return out
}

func uniqueNamesCount(nss []nameserver.Nameserver) int {
	seen := map[string]bool{}
	for _, ns := range nss {
		seen[ns.Name.String()] = true
	}
	return len(seen)
}

func setTypedEndpointsFromNameservers(args map[string]any, nss []nameserver.Nameserver) {
	if args == nil || len(nss) == 0 {
		return
	}

	if typed, ok := logargs.ServersFromNameservers(nss)["servers"]; ok {
		args["servers"] = typed
	}

	addressSet := map[string]bool{}
	for _, ns := range nss {
		address := strings.TrimSpace(ns.AddressString())
		if address != "" {
			addressSet[address] = true
		}
	}
	if len(addressSet) == 0 {
		return
	}

	addresses := make([]string, 0, len(addressSet))
	for address := range addressSet {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	args["addresses"] = addresses
}

func namesToStrings(names []dnsname.Name) []string {
	values := make([]string, 0, len(names))
	for _, name := range names {
		values = append(values, name.String())
	}
	return values
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

func findDupNS(ctx context.Context, testcase string, duplicateTag string, distinctTag string, nsList []nameserver.Nameserver) ([]*logger.Entry, error) {
	nsnamesAndIP := map[string]bool{}
	ips := map[string][]string{}
	for _, ns := range nsList {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}
		ips[ns.Address.String()] = append(ips[ns.Address.String()], ns.Name.String())
		nsnamesAndIP[key] = true
	}

	var results []*logger.Entry
	keys := make([]string, 0, len(ips))
	for key := range ips {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, ip := range keys {
		if len(ips[ip]) > 1 {
			args := map[string]any{
				"address": ip,
			}
			setTypedServersFromNames(args, ips[ip])
			entry, err := util.LoggerFromContext(ctx).Add(duplicateTag, args, moduleName, testcase)
			if err != nil {
				return results, err
			}
			results = append(results, entry)
		}
	}

	if len(nsList) > 0 && len(results) == 0 {
		entry, err := util.LoggerFromContext(ctx).Add(distinctTag, map[string]any{}, moduleName, testcase)
		if err != nil {
			return results, err
		}
		results = append(results, entry)
	}
	return results, nil
}

func maxLengthNameFor(top dnsname.Name) string {
	name := top.FQDN()
	if name == "." {
		name = ""
	}
	for len(name) < constants.FQDNMaxLength-1 {
		remaining := constants.FQDNMaxLength - len(name) - 1
		if remaining > constants.LabelMaxLength {
			remaining = constants.LabelMaxLength
		}
		label := strings.Repeat("A", remaining)
		if name == "" {
			name = label + "."
		} else {
			name = label + "." + name
		}
	}
	return name
}

func allInBailiwick(parent dnsname.Name, nss []nameserver.Nameserver) bool {
	for _, ns := range nss {
		if !parent.IsInBailiwick(ns.Name) {
			return false
		}
	}
	return true
}

func protoLabel(useVC bool) string {
	if useVC {
		return "TCP"
	}
	return "UDP"
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func onlyTestCaseStart(results []*logger.Entry) bool {
	for _, entry := range results {
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

func defaultRecurse(ctx context.Context, z *zone.Zone, name string, qtype string) (packet.Packet, error) {
	if z == nil || z.Recursor() == nil {
		return packet.Packet{}, fmt.Errorf("missing recursor")
	}
	return z.Recursor().Recurse(ctx, name, qtype, "IN")
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
