package consistency

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	dns "codeberg.org/miekg/dns"

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
	"codeberg.org/pawal/gonemaster/engine/transport"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
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

	if util.ShouldRunTest(ctx, "consistency01") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Consistency01(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "consistency02") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Consistency02(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "consistency03") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Consistency03(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "consistency04") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Consistency04(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "consistency05") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Consistency05(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "consistency06") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Consistency06(ctx, z)
		})
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
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// Consistency01 runs the CONSISTENCY01 test case.
func Consistency01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency01"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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

	type serialOutcome struct {
		key    string
		serial string
		skip   bool
	}

	ordered := make([]nameserver.Nameserver, 0, len(list4)+len(list5))
	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}
		nsnamesAndIP[key] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		outcomes := make([]serialOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := serialOutcome{key: ns.String()}

				disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryType)
				if err != nil {
					return err
				}
				if disabled {
					outcome.skip = true
					outcomes[i] = outcome
					return nil
				}

				resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
				if err != nil || resp.Msg == nil {
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				records := resp.GetRecordsForName(queryType, z.Name)
				if len(records) == 0 {
					if _, err := buf.Add("NO_RESPONSE_SOA_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}
				soa, ok := records[0].(*dns.SOA)
				if !ok {
					if _, err := buf.Add("NO_RESPONSE_SOA_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				outcome.serial = fmt.Sprintf("%d", soa.Serial)
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

		for _, outcome := range outcomes {
			if outcome.skip || outcome.serial == "" {
				continue
			}
			serials[outcome.serial] = append(serials[outcome.serial], outcome.key)
		}
	}

	serialKeys := make([]string, 0, len(serials))
	for key := range serials {
		serialKeys = append(serialKeys, key)
	}
	sort.Strings(serialKeys)

	for _, serial := range serialKeys {
		nsList := uniqueSortedValues(serials[serial])
		args := map[string]any{
			"serial": serial,
		}
		setTypedServersFromNames(args, strings.Join(nsList, ";"))
		if err := appendLog(ctx, &results, testcase, "SOA_SERIAL", args); err != nil {
			return results, err
		}
	}

	if len(serialKeys) == 1 {
		if err := appendLog(ctx, &results, testcase, "ONE_SOA_SERIAL", map[string]any{
			"serial": serialKeys[0],
		}); err != nil {
			return results, err
		}
	} else if len(serialKeys) > 0 {
		if err := appendLog(ctx, &results, testcase, "MULTIPLE_SOA_SERIALS", map[string]any{
			"count": len(serialKeys),
		}); err != nil {
			return results, err
		}

		minVal, errMin := strconv.ParseInt(serialKeys[0], 10, 64)
		maxVal, errMax := strconv.ParseInt(serialKeys[len(serialKeys)-1], 10, 64)
		if errMin == nil && errMax == nil {
			if maxVal-minVal > int64(constants.SerialMaxVariation) {
				if err := appendLog(ctx, &results, testcase, "SOA_SERIAL_VARIATION", map[string]any{
					"serial_min":    serialKeys[0],
					"serial_max":    serialKeys[len(serialKeys)-1],
					"max_variation": constants.SerialMaxVariation,
				}); err != nil {
					return results, err
				}
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Consistency02 runs the CONSISTENCY02 test case.
func Consistency02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency02"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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

	type rnameOutcome struct {
		key   string
		rname string
		skip  bool
	}

	ordered := make([]nameserver.Nameserver, 0, len(list4)+len(list5))
	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}
		nsnamesAndIP[key] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		outcomes := make([]rnameOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := rnameOutcome{key: ns.String()}

				disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryType)
				if err != nil {
					return err
				}
				if disabled {
					outcome.skip = true
					outcomes[i] = outcome
					return nil
				}

				resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
				if err != nil || resp.Msg == nil {
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				records := resp.GetRecordsForName(queryType, z.Name)
				if len(records) == 0 {
					if _, err := buf.Add("NO_RESPONSE_SOA_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}
				soa, ok := records[0].(*dns.SOA)
				if !ok {
					if _, err := buf.Add("NO_RESPONSE_SOA_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				outcome.rname = strings.ToLower(soa.Mbox)
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

		for _, outcome := range outcomes {
			if outcome.skip || outcome.rname == "" {
				continue
			}
			if _, ok := rnames[outcome.rname]; !ok {
				order = append(order, outcome.rname)
			}
			rnames[outcome.rname] = append(rnames[outcome.rname], outcome.key)
		}
	}

	if len(order) == 1 {
		if err := appendLog(ctx, &results, testcase, "ONE_SOA_RNAME", map[string]any{
			"rname": order[0],
		}); err != nil {
			return results, err
		}
	} else if len(order) > 0 {
		if err := appendLog(ctx, &results, testcase, "MULTIPLE_SOA_RNAMES", map[string]any{
			"count": len(order),
		}); err != nil {
			return results, err
		}
		for _, rname := range order {
			args := map[string]any{
				"rname": rname,
			}
			setTypedServersFromNames(args, strings.Join(uniqueSortedValues(rnames[rname]), ";"))
			if err := appendLog(ctx, &results, testcase, "SOA_RNAME", args); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Consistency03 runs the CONSISTENCY03 test case.
func Consistency03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency03"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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

	type timeOutcome struct {
		key    string
		setKey string
		params timeParams
		skip   bool
	}

	ordered := make([]nameserver.Nameserver, 0, len(list4)+len(list5))
	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}
		nsnamesAndIP[key] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		outcomes := make([]timeOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := timeOutcome{key: ns.String()}

				disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryType)
				if err != nil {
					return err
				}
				if disabled {
					outcome.skip = true
					outcomes[i] = outcome
					return nil
				}

				resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
				if err != nil || resp.Msg == nil {
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				records := resp.GetRecordsForName(queryType, z.Name)
				if len(records) == 0 {
					if _, err := buf.Add("NO_RESPONSE_SOA_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}
				soa, ok := records[0].(*dns.SOA)
				if !ok {
					if _, err := buf.Add("NO_RESPONSE_SOA_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				params := timeParams{
					refresh: int(soa.Refresh),
					retry:   int(soa.Retry),
					expire:  int(soa.Expire),
					minimum: int(soa.Minttl),
				}
				outcome.params = params
				outcome.setKey = fmt.Sprintf("%d;%d;%d;%d", params.refresh, params.retry, params.expire, params.minimum)
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

		for _, outcome := range outcomes {
			if outcome.skip || outcome.setKey == "" {
				continue
			}
			if _, ok := timeSets[outcome.setKey]; !ok {
				order = append(order, outcome.setKey)
				timeValues[outcome.setKey] = outcome.params
			}
			timeSets[outcome.setKey] = append(timeSets[outcome.setKey], outcome.key)
		}
	}

	if len(order) == 1 {
		params := timeValues[order[0]]
		if err := appendLog(ctx, &results, testcase, "ONE_SOA_TIME_PARAMETER_SET", map[string]any{
			"refresh": params.refresh,
			"retry":   params.retry,
			"expire":  params.expire,
			"minimum": params.minimum,
		}); err != nil {
			return results, err
		}
	} else if len(order) > 0 {
		if err := appendLog(ctx, &results, testcase, "MULTIPLE_SOA_TIME_PARAMETER_SET", map[string]any{
			"count": len(order),
		}); err != nil {
			return results, err
		}
		for _, setKey := range order {
			params := timeValues[setKey]
			nsList := uniqueSortedValues(timeSets[setKey])
			args := map[string]any{
				"refresh": params.refresh,
				"retry":   params.retry,
				"expire":  params.expire,
				"minimum": params.minimum,
			}
			setTypedServersFromNames(args, strings.Join(nsList, ";"))
			if err := appendLog(ctx, &results, testcase, "SOA_TIME_PARAMETER_SET", args); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Consistency04 runs the CONSISTENCY04 test case.
func Consistency04(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency04"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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

	type nsSetOutcome struct {
		key    string
		setKey string
		skip   bool
	}

	ordered := make([]nameserver.Nameserver, 0, len(list4)+len(list5))
	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}
		nsnamesAndIP[key] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		outcomes := make([]nsSetOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsSetOutcome{key: ns.String()}

				disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryType)
				if err != nil {
					return err
				}
				if disabled {
					outcome.skip = true
					outcomes[i] = outcome
					return nil
				}

				resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
				if err != nil || resp.Msg == nil {
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				records := resp.GetRecordsForName(queryType, z.Name)
				if len(records) == 0 {
					if _, err := buf.Add("NO_RESPONSE_NS_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
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
					if _, err := buf.Add("NO_RESPONSE_NS_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}
				sort.Strings(names)

				outcome.setKey = strings.Join(names, ";")
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

		for _, outcome := range outcomes {
			if outcome.skip || outcome.setKey == "" {
				continue
			}
			if _, ok := nsSets[outcome.setKey]; !ok {
				order = append(order, outcome.setKey)
			}
			nsSets[outcome.setKey] = append(nsSets[outcome.setKey], outcome.key)
		}
	}

	if len(order) == 1 {
		args := map[string]any{}
		setTypedServersFromNames(args, order[0])
		if err := appendLog(ctx, &results, testcase, "ONE_NS_SET", args); err != nil {
			return results, err
		}
	} else if len(order) > 0 {
		if err := appendLog(ctx, &results, testcase, "MULTIPLE_NS_SET", map[string]any{
			"count": len(order),
		}); err != nil {
			return results, err
		}
		for _, setKey := range order {
			args := map[string]any{}
			setTypedServerListAtKey(args, "ns_set_servers", setKey)
			setTypedServersFromNames(args, strings.Join(uniqueSortedValues(nsSets[setKey]), ";"))
			if err := appendLog(ctx, &results, testcase, "NS_SET", args); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Consistency05 runs the CONSISTENCY05 test case.
func Consistency05(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency05"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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
				glueKey := strings.ToLower(rr.Header().Name) + "/" + aRR.Addr.String()
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
				glueKey := strings.ToLower(rr.Header().Name) + "/" + AAAArr.Addr.String()
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

	strictGlueServers := nameserversFromStrictGlue(ctx, z, strictGlue)

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
		if ns.Address.Is4() && util.IPVersionOK(ctx, constants.IPVersion4) {
			inBailiwickServers = append(inBailiwickServers, ns)
			continue
		}
		if ns.Address.Is6() && util.IPVersionOK(ctx, constants.IPVersion6) {
			inBailiwickServers = append(inBailiwickServers, ns)
		}
	}
	if len(inBailiwickServers) == 0 {
		inBailiwickServers = strictGlueServers
		inBailiwickNames = appendChildNSNamesFromServers(ctx, z, inBailiwickNames, strictGlueServers)
	}

	childIBStrings := map[string]bool{}
	allAddressLookupsFailed := len(inBailiwickNames) > 0
	for _, nsName := range inBailiwickNames {
		nameAddressLookupsFailed := true
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
				nameAddressLookupsFailed = false
			}

			for _, rr := range append(rrsA, rrsAAAA...) {
				childIBStrings[addrKey(rr)] = true
			}
		}

		if !nameAddressLookupsFailed {
			allAddressLookupsFailed = false
		}
	}
	if allAddressLookupsFailed {
		if err := appendLog(ctx, &results, testcase, "CHILD_ZONE_LAME", map[string]any{}); err != nil {
			return results, err
		}
		return appendTestCaseEnd(ctx, results, testcase)
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
		args := map[string]any{}
		setTypedServersFromAddrKeysAtKey(args, "parent_servers", sortedKeys(strictGlue))
		setTypedServersFromAddrKeysAtKey(args, "zone_servers", sortedKeys(childIBStrings))
		if err := appendLog(ctx, &results, testcase, "IN_BAILIWICK_ADDR_MISMATCH", args); err != nil {
			return results, err
		}
	}

	if len(ibExtraChild) > 0 {
		addresses := addressesFromAddrKeys(ibExtraChild)
		if len(addresses) == 0 {
			addresses = append([]string(nil), ibExtraChild...)
			sort.Strings(addresses)
		}
		if err := appendLog(ctx, &results, testcase, "EXTRA_ADDRESS_CHILD", map[string]any{
			"addresses": addresses,
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
			args := map[string]any{}
			setTypedServersFromAddrKeysAtKey(args, "parent_servers", glueStrings)
			setTypedServersFromAddrKeysAtKey(args, "zone_servers", sortedKeys(childOOB))
			if err := appendLog(ctx, &results, testcase, "OUT_OF_BAILIWICK_ADDR_MISMATCH", args); err != nil {
				return results, err
			}
		}
	}

	if len(ibExtraChild) == 0 && len(ibMismatch) == 0 && len(oobMismatch) == 0 {
		if err := appendLog(ctx, &results, testcase, "ADDRESSES_MATCH", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Consistency06 runs the CONSISTENCY06 test case.
func Consistency06(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Consistency06"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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

	type mnameOutcome struct {
		key   string
		mname string
		skip  bool
	}

	ordered := make([]nameserver.Nameserver, 0, len(list4)+len(list5))
	for _, ns := range append(list4, list5...) {
		key := ns.String()
		if nsnamesAndIP[key] {
			continue
		}
		nsnamesAndIP[key] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		outcomes := make([]mnameOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := mnameOutcome{key: ns.String()}

				disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryType)
				if err != nil {
					return err
				}
				if disabled {
					outcome.skip = true
					outcomes[i] = outcome
					return nil
				}

				resp, err := ns.QueryWithOptions(ctx, z.Name.String(), queryType, nil)
				if err != nil || resp.Msg == nil {
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				records := resp.GetRecordsForName(queryType, z.Name)
				if len(records) == 0 {
					if _, err := buf.Add("NO_RESPONSE_SOA_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}
				soa, ok := records[0].(*dns.SOA)
				if !ok {
					if _, err := buf.Add("NO_RESPONSE_SOA_QUERY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				outcome.mname = strings.ToLower(soa.Ns)
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

		for _, outcome := range outcomes {
			if outcome.skip || outcome.mname == "" {
				continue
			}
			if _, ok := mnames[outcome.mname]; !ok {
				order = append(order, outcome.mname)
			}
			mnames[outcome.mname] = append(mnames[outcome.mname], outcome.key)
		}
	}

	if len(order) == 1 {
		if err := appendLog(ctx, &results, testcase, "ONE_SOA_MNAME", map[string]any{
			"mname": order[0],
		}); err != nil {
			return results, err
		}
	} else if len(order) > 0 {
		if err := appendLog(ctx, &results, testcase, "MULTIPLE_SOA_MNAMES", map[string]any{
			"count": len(order),
		}); err != nil {
			return results, err
		}
		for _, mname := range order {
			args := map[string]any{
				"mname": mname,
			}
			setTypedServersFromNames(args, strings.Join(uniqueSortedValues(mnames[mname]), ";"))
			if err := appendLog(ctx, &results, testcase, "SOA_MNAME", args); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
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

func nameserversFromStrictGlue(ctx context.Context, z *zone.Zone, strictGlue map[string]bool) []nameserver.Nameserver {
	var client *transport.Client
	if z != nil && z.Recursor() != nil {
		client = z.Recursor().Client()
	}

	var out []nameserver.Nameserver
	seen := map[string]bool{}
	for _, key := range sortedKeys(strictGlue) {
		nsName, address := parseAddrKey(key)
		if nsName == "" || address == "" {
			continue
		}
		ns, err := nameserver.NewWithContext(ctx, nsName, address, client)
		if err != nil {
			continue
		}
		if ns.Address.Is4() && !util.IPVersionOK(ctx, constants.IPVersion4) {
			continue
		}
		if ns.Address.Is6() && !util.IPVersionOK(ctx, constants.IPVersion6) {
			continue
		}
		nsKey := strings.ToLower(ns.String())
		if seen[nsKey] {
			continue
		}
		seen[nsKey] = true
		out = append(out, ns)
	}
	return out
}

func appendChildNSNamesFromServers(ctx context.Context, z *zone.Zone, names []dnsname.Name, servers []nameserver.Nameserver) []dnsname.Name {
	if z == nil || len(servers) == 0 {
		return names
	}

	seen := map[string]dnsname.Name{}
	for _, name := range names {
		seen[strings.ToLower(name.String())] = name
	}

	for _, ns := range servers {
		resp, err := ns.QueryWithOptions(ctx, z.Name.String(), "NS", nil)
		if err != nil || resp.Msg == nil {
			continue
		}
		for _, rr := range resp.GetRecordsForName("NS", z.Name) {
			nsRR, ok := rr.(*dns.NS)
			if !ok {
				continue
			}
			name := dnsname.New(strings.ToLower(nsRR.Ns))
			if !z.Name.IsInBailiwick(name) {
				continue
			}
			seen[strings.ToLower(name.String())] = name
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]dnsname.Name, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func getAddrRRs(ctx context.Context, ns nameserver.Nameserver, name dnsname.Name, qtype string, z *zone.Zone, testcase string) (*logger.Entry, []dns.RR, error) {
	recurseOff := false
	opts := &nameserver.QueryOptions{Recurse: &recurseOff}
	resp, err := ns.QueryWithOptions(ctx, name.String(), qtype, opts)
	if err != nil || resp.Msg == nil {
		entry, addErr := util.LoggerFromContext(ctx).Add("NO_RESPONSE", withNameserverArgs(ns, nil), moduleName, testcase)
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
		entry, addErr := util.LoggerFromContext(ctx).Add("CHILD_NS_FAILED", withNameserverArgs(ns, nil), moduleName, testcase)
		if addErr != nil {
			return nil, nil, addErr
		}
		return entry, nil, nil
	}

	return nil, nil, nil
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

func setTypedServersFromNames(args map[string]any, namesList string) {
	setTypedServerListAtKey(args, "servers", namesList)
}

func setTypedServerListAtKey(args map[string]any, key string, namesList string) {
	if args == nil || strings.TrimSpace(namesList) == "" {
		return
	}

	parts := strings.Split(namesList, ";")
	raw, ok := logargs.ServersFromValues(parts)["servers"]
	if !ok {
		return
	}
	servers, ok := raw.([]map[string]any)
	if !ok || len(servers) == 0 {
		return
	}
	args[key] = servers
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

func addrKey(rr dns.RR) string {
	owner := strings.ToLower(rr.Header().Name)
	switch r := rr.(type) {
	case *dns.A:
		return owner + "/" + r.Addr.String()
	case *dns.AAAA:
		return owner + "/" + r.Addr.String()
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

func addressesFromAddrKeys(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		address := strings.TrimSpace(value)
		sep := strings.LastIndex(address, "/")
		if sep >= 0 && sep < len(address)-1 {
			address = strings.TrimSpace(address[sep+1:])
		}
		if address == "" || seen[address] {
			continue
		}
		seen[address] = true
		out = append(out, address)
	}
	sort.Strings(out)
	return out
}

func uniqueSortedValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func parseAddrKey(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	sep := strings.LastIndex(value, "/")
	if sep < 1 || sep >= len(value)-1 {
		return "", value
	}
	name := logargs.EndpointName(value[:sep])
	address := strings.TrimSpace(value[sep+1:])
	return name, address
}

func setTypedServersFromAddrKeysAtKey(args map[string]any, key string, values []string) {
	if args == nil || key == "" || len(values) == 0 {
		return
	}
	seen := map[string]bool{}
	servers := make([]logargs.Server, 0, len(values))
	for _, value := range values {
		ns, address := parseAddrKey(value)
		if ns == "" && address == "" {
			continue
		}
		dedupe := ns + "|" + address
		if seen[dedupe] {
			continue
		}
		seen[dedupe] = true
		servers = append(servers, logargs.Server{NS: ns, Address: address})
	}
	if len(servers) == 0 {
		return
	}
	sort.Slice(servers, func(i, j int) bool {
		left := servers[i].NS + "|" + servers[i].Address
		right := servers[j].NS + "|" + servers[j].Address
		return left < right
	})
	if typed, ok := logargs.Servers(servers)["servers"]; ok {
		args[key] = typed
	}
}
