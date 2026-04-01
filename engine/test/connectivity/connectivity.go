package connectivity

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/asnlookup"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logargs"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/methods"
	"codeberg.org/pawal/gonemaster/engine/methodsv2"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/runner"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testcase"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testlogger"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

const moduleName = "Connectivity"

var (
	method4and5          = methods.Method4and5
	getDelNSNamesAndIPs  = methodsv2.GetDelNSNamesAndIPs
	getZoneNSNamesAndIPs = methodsv2.GetZoneNSNamesAndIPs
	lookupASN            = asnlookup.GetWithPrefix
)

// All runs the Connectivity test cases in order, mirroring the Perl implementation.
func All(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest(ctx, "connectivity01") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Connectivity01(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "connectivity02") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Connectivity02(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "connectivity03") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Connectivity03(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "connectivity04") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Connectivity04(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// Metadata returns the set of tags emitted by Connectivity test cases.
func Metadata() map[string][]string {
	return map[string][]string{
		"connectivity01": {
			"CN01_IPV4_DISABLED",
			"CN01_IPV6_DISABLED",
			"CN01_MISSING_NS_RECORD_UDP",
			"CN01_MISSING_SOA_RECORD_UDP",
			"CN01_NO_RESPONSE_NS_QUERY_UDP",
			"CN01_NO_RESPONSE_SOA_QUERY_UDP",
			"CN01_NO_RESPONSE_UDP",
			"CN01_NS_RECORD_NOT_AA_UDP",
			"CN01_OK_UDP",
			"CN01_SOA_RECORD_NOT_AA_UDP",
			"CN01_UNEXPECTED_RCODE_NS_QUERY_UDP",
			"CN01_UNEXPECTED_RCODE_SOA_QUERY_UDP",
			"CN01_WRONG_NS_RECORD_UDP",
			"CN01_WRONG_SOA_RECORD_UDP",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"connectivity02": {
			"CN02_MISSING_NS_RECORD_TCP",
			"CN02_MISSING_SOA_RECORD_TCP",
			"CN02_NO_RESPONSE_NS_QUERY_TCP",
			"CN02_NO_RESPONSE_SOA_QUERY_TCP",
			"CN02_NO_RESPONSE_TCP",
			"CN02_NS_RECORD_NOT_AA_TCP",
			"CN02_OK_TCP",
			"CN02_SOA_RECORD_NOT_AA_TCP",
			"CN02_UNEXPECTED_RCODE_NS_QUERY_TCP",
			"CN02_UNEXPECTED_RCODE_SOA_QUERY_TCP",
			"CN02_WRONG_NS_RECORD_TCP",
			"CN02_WRONG_SOA_RECORD_TCP",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"connectivity03": {
			"ASN_INFOS_RAW",
			"ASN_INFOS_ANNOUNCE_BY",
			"ASN_INFOS_ANNOUNCE_IN",
			"EMPTY_ASN_SET",
			"ERROR_ASN_DATABASE",
			"IPV4_DIFFERENT_ASN",
			"IPV4_ONE_ASN",
			"IPV4_SAME_ASN",
			"IPV6_DIFFERENT_ASN",
			"IPV6_ONE_ASN",
			"IPV6_SAME_ASN",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"connectivity04": {
			"CN04_ASN_INFOS_RAW",
			"CN04_ASN_INFOS_ANNOUNCE_IN",
			"CN04_EMPTY_PREFIX_SET",
			"CN04_ERROR_PREFIX_DATABASE",
			"CN04_IPV4_DIFFERENT_PREFIX",
			"CN04_IPV4_SAME_PREFIX",
			"CN04_IPV4_SINGLE_PREFIX",
			"CN04_IPV6_DIFFERENT_PREFIX",
			"CN04_IPV6_SAME_PREFIX",
			"CN04_IPV6_SINGLE_PREFIX",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// Connectivity01 runs the CONNECTIVITY01 test case.
func Connectivity01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Connectivity01"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	if z == nil {
		return results, fmt.Errorf("zone is nil")
	}

	name := z.Name
	nsList, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	ipv4Disabled, ipv6Disabled := disabledNS(ctx, nsList)
	if len(ipv4Disabled) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, ipv4Disabled)
		if err := appendLog(ctx, &results, testcase, "CN01_IPV4_DISABLED", args); err != nil {
			return results, err
		}
	}
	if len(ipv6Disabled) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, ipv6Disabled)
		if err := appendLog(ctx, &results, testcase, "CN01_IPV6_DISABLED", args); err != nil {
			return results, err
		}
	}

	if err := connectivityLoop(ctx, testcase, name, nsList, &results); err != nil {
		return results, err
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Connectivity02 runs the CONNECTIVITY02 test case.
func Connectivity02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Connectivity02"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	if z == nil {
		return results, fmt.Errorf("zone is nil")
	}

	name := z.Name
	nsList, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	if err := connectivityLoop(ctx, testcase, name, nsList, &results); err != nil {
		return results, err
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Connectivity03 runs the CONNECTIVITY03 test case.
func Connectivity03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Connectivity03"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	if z == nil {
		return results, fmt.Errorf("zone is nil")
	}
	if z.Recursor() == nil {
		return results, fmt.Errorf("missing recursor")
	}

	nsList, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	v4ips, v6ips := uniqueIPs(nsList)
	var v4asns []int
	var v4asnsets []string
	var v6asns []int
	var v6asnsets []string

	type asnOutcome struct {
		asns   []int
		asnset string
	}

	parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
	if parallelism < 1 {
		parallelism = 1
	}

	if len(v4ips) > 0 {
		outcomes := make([]asnOutcome, len(v4ips))
		tasks := make([]runner.Task, len(v4ips))
		for i, ip := range v4ips {
			i, ip := i, ip
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)

				res, err := lookupASN(ctx, z.Recursor(), ip)
				if err != nil {
					return err
				}
				if res.Code == asnlookup.CodeError || res.Code == asnlookup.CodeEmpty {
					if _, err := buf.Add(res.Code, map[string]any{"address": ip.String()}); err != nil {
						return err
					}
					return nil
				}
				if res.Raw != "" {
					if _, err := buf.Add("ASN_INFOS_RAW", map[string]any{
						"address": ip.String(),
						"data":    res.Raw,
					}); err != nil {
						return err
					}
				}
				if len(res.ASNs) > 0 {
					asns := uniqueSortedInts(append([]int{}, res.ASNs...))
					if _, err := buf.Add("ASN_INFOS_ANNOUNCE_BY", map[string]any{
						"address": ip.String(),
						"asns":    asns,
					}); err != nil {
						return err
					}
					outcomes[i] = asnOutcome{
						asns:   asns,
						asnset: asnSetSignature(asns),
					}
				}
				if res.Prefix != nil {
					if _, err := buf.Add("ASN_INFOS_ANNOUNCE_IN", map[string]any{
						"address":  ip.String(),
						"prefixes": []string{res.Prefix.String()},
					}); err != nil {
						return err
					}
				}
				return nil
			}
		}

		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
		for _, outcome := range outcomes {
			if len(outcome.asns) > 0 {
				v4asns = append(v4asns, outcome.asns...)
			}
			if outcome.asnset != "" {
				v4asnsets = append(v4asnsets, outcome.asnset)
			}
		}
	}

	if len(v6ips) > 0 {
		outcomes := make([]asnOutcome, len(v6ips))
		tasks := make([]runner.Task, len(v6ips))
		for i, ip := range v6ips {
			i, ip := i, ip
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)

				res, err := lookupASN(ctx, z.Recursor(), ip)
				if err != nil {
					return err
				}
				if res.Code == asnlookup.CodeError || res.Code == asnlookup.CodeEmpty {
					if _, err := buf.Add(res.Code, map[string]any{"address": ip.String()}); err != nil {
						return err
					}
					return nil
				}
				if res.Raw != "" {
					if _, err := buf.Add("ASN_INFOS_RAW", map[string]any{
						"address": ip.String(),
						"data":    res.Raw,
					}); err != nil {
						return err
					}
				}
				if len(res.ASNs) > 0 {
					asns := uniqueSortedInts(append([]int{}, res.ASNs...))
					if _, err := buf.Add("ASN_INFOS_ANNOUNCE_BY", map[string]any{
						"address": ip.String(),
						"asns":    asns,
					}); err != nil {
						return err
					}
					outcomes[i] = asnOutcome{
						asns:   asns,
						asnset: asnSetSignature(asns),
					}
				}
				if res.Prefix != nil {
					if _, err := buf.Add("ASN_INFOS_ANNOUNCE_IN", map[string]any{
						"address":  ip.String(),
						"prefixes": []string{res.Prefix.String()},
					}); err != nil {
						return err
					}
				}
				return nil
			}
		}

		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}
		results = append(results, entries...)
		for _, outcome := range outcomes {
			if len(outcome.asns) > 0 {
				v6asns = append(v6asns, outcome.asns...)
			}
			if outcome.asnset != "" {
				v6asnsets = append(v6asnsets, outcome.asnset)
			}
		}
	}

	v4asns = uniqueSortedInts(v4asns)
	v4asnsets = uniqueSortedStrings(v4asnsets)
	v6asns = uniqueSortedInts(v6asns)
	v6asnsets = uniqueSortedStrings(v6asnsets)

	if len(v4asns) > 0 {
		if len(v4asns) == 1 {
			if err := appendLog(ctx, &results, testcase, "IPV4_ONE_ASN", map[string]any{
				"asn": v4asns[0],
			}); err != nil {
				return results, err
			}
		} else if len(v4asnsets) == 1 {
			if err := appendLog(ctx, &results, testcase, "IPV4_SAME_ASN", map[string]any{
				"asns": v4asns,
			}); err != nil {
				return results, err
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "IPV4_DIFFERENT_ASN", map[string]any{
				"asns": v4asns,
			}); err != nil {
				return results, err
			}
		}
	}

	if len(v6asns) > 0 {
		if len(v6asns) == 1 {
			if err := appendLog(ctx, &results, testcase, "IPV6_ONE_ASN", map[string]any{
				"asn": v6asns[0],
			}); err != nil {
				return results, err
			}
		} else if len(v6asnsets) == 1 {
			if err := appendLog(ctx, &results, testcase, "IPV6_SAME_ASN", map[string]any{
				"asns": v6asns,
			}); err != nil {
				return results, err
			}
		} else {
			if err := appendLog(ctx, &results, testcase, "IPV6_DIFFERENT_ASN", map[string]any{
				"asns": v6asns,
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Connectivity04 runs the CONNECTIVITY04 test case.
func Connectivity04(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Connectivity04"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	if z == nil {
		return results, fmt.Errorf("zone is nil")
	}
	if z.Recursor() == nil {
		return results, fmt.Errorf("missing recursor")
	}

	delItems, err := getDelNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}
	zoneItems, err := getZoneNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}

	prefixes := map[int]map[string][]string{}
	processed := map[int]map[string]bool{}

	type prefixItem struct {
		item    methodsv2.NSItem
		version int
	}

	items := append(delItems, zoneItems...)
	var ordered []prefixItem
	for _, item := range items {
		if !item.HasAddress {
			continue
		}
		ip := item.Address
		version := ipVersion(ip)
		if version == 0 {
			continue
		}
		if processed[version] == nil {
			processed[version] = map[string]bool{}
		}
		if processed[version][ip.String()] {
			continue
		}
		processed[version][ip.String()] = true
		ordered = append(ordered, prefixItem{item: item, version: version})
	}

	if len(ordered) > 0 {
		type prefixOutcome struct {
			version int
			prefix  string
			item    string
		}

		outcomes := make([]prefixOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, entry := range ordered {
			i, entry := i, entry
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				ip := entry.item.Address

				res, err := lookupASN(ctx, z.Recursor(), ip)
				if err != nil {
					return err
				}
				if res.Code == asnlookup.CodeError || res.Code == asnlookup.CodeEmpty {
					tag := res.Code
					if res.Code == asnlookup.CodeError {
						tag = "CN04_ERROR_PREFIX_DATABASE"
					} else if res.Code == asnlookup.CodeEmpty {
						tag = "CN04_EMPTY_PREFIX_SET"
					}
					if _, err := buf.Add(tag, map[string]any{"address": ip.String()}); err != nil {
						return err
					}
					return nil
				}

				if res.Raw != "" {
					if _, err := buf.Add("CN04_ASN_INFOS_RAW", map[string]any{
						"address": ip.String(),
						"data":    res.Raw,
					}); err != nil {
						return err
					}
				}

				if res.Prefix != nil {
					prefixStr := res.Prefix.String()
					if _, err := buf.Add("CN04_ASN_INFOS_ANNOUNCE_IN", map[string]any{
						"address":  ip.String(),
						"prefixes": []string{prefixStr},
					}); err != nil {
						return err
					}
					outcomes[i] = prefixOutcome{
						version: entry.version,
						prefix:  prefixStr,
						item:    entry.item.Name.String() + "/" + entry.item.Address.String(),
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

		for _, outcome := range outcomes {
			if outcome.prefix == "" {
				continue
			}
			if prefixes[outcome.version] == nil {
				prefixes[outcome.version] = map[string][]string{}
			}
			prefixes[outcome.version][outcome.prefix] = append(prefixes[outcome.version][outcome.prefix], outcome.item)
		}
	}

	versions := make([]int, 0, len(prefixes))
	for version := range prefixes {
		versions = append(versions, version)
	}
	sort.Ints(versions)

	for _, version := range versions {
		prefixMap := prefixes[version]
		if len(prefixMap) == 0 {
			continue
		}

		prefixKeys := make([]string, 0, len(prefixMap))
		for key := range prefixMap {
			prefixKeys = append(prefixKeys, key)
		}
		sort.Strings(prefixKeys)

		var combined []string
		for _, prefix := range prefixKeys {
			list := prefixMap[prefix]
			if len(list) == 1 {
				combined = append(combined, list[0])
				continue
			}
			if len(list) >= 2 {
				tag := fmt.Sprintf("CN04_IPV%d_SAME_PREFIX", version)
				args := map[string]any{
					"prefixes": []string{prefix},
				}
				setTypedServersFromNames(args, list)
				if err := appendLog(ctx, &results, testcase, tag, args); err != nil {
					return results, err
				}
			}
		}

		if len(combined) > 0 {
			tag := fmt.Sprintf("CN04_IPV%d_DIFFERENT_PREFIX", version)
			args := map[string]any{}
			setTypedServersFromNames(args, combined)
			if err := appendLog(ctx, &results, testcase, tag, args); err != nil {
				return results, err
			}
		}

		if len(prefixKeys) == 1 {
			list := prefixMap[prefixKeys[0]]
			if processed[version] != nil && len(list) == len(processed[version]) {
				tag := fmt.Sprintf("CN04_IPV%d_SINGLE_PREFIX", version)
				if err := appendLog(ctx, &results, testcase, tag, map[string]any{}); err != nil {
					return results, err
				}
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

func connectivityLoop(ctx context.Context, testcase string, name dnsname.Name, nsList []nameserver.Nameserver, results *[]*logger.Entry) error {
	prefix := ""
	useTCP := false
	protocol := ""
	canonical := testcase

	switch strings.ToLower(testcase) {
	case "connectivity01":
		prefix = "CN01"
		useTCP = false
		protocol = "UDP"
		canonical = "Connectivity01"
	case "connectivity02":
		prefix = "CN02"
		useTCP = true
		protocol = "TCP"
		canonical = "Connectivity02"
	default:
		return fmt.Errorf("unknown testcase %q", testcase)
	}

	testcase = canonical

	if len(nsList) > 0 {
		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		okNS := make([]string, len(nsList))
		tasks := make([]runner.Task, len(nsList))
		for i, ns := range nsList {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "SOA", "NS")
				if err != nil {
					return err
				}
				if disabled {
					return nil
				}

				useVC := useTCP
				opts := &nameserver.QueryOptions{UseVC: &useVC}
				soaResp, _ := ns.QueryWithOptions(ctx, name.String(), "SOA", opts)
				nsResp, _ := ns.QueryWithOptions(ctx, name.String(), "NS", opts)

				if soaResp.Msg == nil && nsResp.Msg == nil {
					if _, err := buf.Add(fmt.Sprintf("%s_NO_RESPONSE_%s", prefix, protocol), withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					return nil
				}

				ok := true
				for _, qtype := range []string{"SOA", "NS"} {
					resp := soaResp
					if qtype == "NS" {
						resp = nsResp
					}

					if resp.Msg == nil {
						if _, err := buf.Add(fmt.Sprintf("%s_NO_RESPONSE_%s_QUERY_%s", prefix, qtype, protocol), withNameserverArgs(ns, nil)); err != nil {
							return err
						}
						ok = false
						continue
					}

					if resp.Rcode() != "NOERROR" {
						if _, err := buf.Add(fmt.Sprintf("%s_UNEXPECTED_RCODE_%s_QUERY_%s", prefix, qtype, protocol), withNameserverArgs(ns, map[string]any{
							"rcode": resp.Rcode(),
						})); err != nil {
							return err
						}
						ok = false
						continue
					}

					rrs := resp.GetRecords(qtype, "answer")
					if len(rrs) == 0 {
						if _, err := buf.Add(fmt.Sprintf("%s_MISSING_%s_RECORD_%s", prefix, qtype, protocol), withNameserverArgs(ns, nil)); err != nil {
							return err
						}
						ok = false
						continue
					}

					rrOwner := dnsname.New(rrs[0].Header().Name).FQDN()
					expected := name.FQDN()
					if !strings.EqualFold(rrOwner, expected) {
						if _, err := buf.Add(fmt.Sprintf("%s_WRONG_%s_RECORD_%s", prefix, qtype, protocol), withNameserverArgs(ns, map[string]any{
							"domain_found":    strings.ToLower(rrOwner),
							"domain_expected": strings.ToLower(expected),
						})); err != nil {
							return err
						}
						ok = false
						continue
					}

					if !resp.AA() {
						if _, err := buf.Add(fmt.Sprintf("%s_%s_RECORD_NOT_AA_%s", prefix, qtype, protocol), withNameserverArgs(ns, nil)); err != nil {
							return err
						}
						ok = false
						continue
					}
				}

				if ok {
					okNS[i] = ns.NameString() + "/" + ns.AddressString()
				}

				return nil
			}
		}

		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return err
		}
		*results = append(*results, entries...)

		var okList []string
		for _, v := range okNS {
			if v != "" {
				okList = append(okList, v)
			}
		}
		if len(okList) > 0 {
			args := map[string]any{}
			setTypedServersFromNames(args, okList)
			if err := appendLog(ctx, results, testcase, fmt.Sprintf("%s_OK_%s", prefix, protocol), args); err != nil {
				return err
			}
		}
	}

	return nil
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

func disabledNS(ctx context.Context, nsList []nameserver.Nameserver) ([]string, []string) {
	var ipv4 []string
	var ipv6 []string
	for _, ns := range nsList {
		if ns.Address.Is4() && !profile.FromContext(ctx).Net.IPv4 {
			ipv4 = append(ipv4, ns.NameString())
			continue
		}
		if ns.Address.Is6() && !profile.FromContext(ctx).Net.IPv6 {
			ipv6 = append(ipv6, ns.NameString())
		}
	}
	return ipv4, ipv6
}

func uniqueIPs(nsList []nameserver.Nameserver) ([]netip.Addr, []netip.Addr) {
	seen4 := map[string]bool{}
	seen6 := map[string]bool{}
	var v4 []netip.Addr
	var v6 []netip.Addr
	for _, ns := range nsList {
		ip := ns.Address
		if ip.Is4() {
			key := ip.String()
			if !seen4[key] {
				seen4[key] = true
				v4 = append(v4, ip)
			}
			continue
		}
		if ip.Is6() {
			key := ip.String()
			if !seen6[key] {
				seen6[key] = true
				v6 = append(v6, ip)
			}
		}
	}
	return v4, v6
}

func ipVersion(ip netip.Addr) int {
	if ip.Is4() {
		return 4
	}
	if ip.Is6() {
		return 6
	}
	return 0
}

func uniqueSortedInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	sort.Ints(values)
	out := []int{values[0]}
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	out := []string{values[0]}
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func asnSetSignature(values []int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return strings.Join(parts, ",")
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
