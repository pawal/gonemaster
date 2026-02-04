package basic

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/methods"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/runner"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testcase"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testlogger"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

const moduleName = "Basic"

// All runs the Basic test cases in order, mirroring the Perl implementation.
func All(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest(ctx, "basic01") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Basic01(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
		if !hasTag(results, "B01_CHILD_FOUND") {
			return results, nil
		}
	}

	var authResponseSOA bool
	if util.ShouldRunTest(ctx, "basic02") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Basic02(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
		authResponseSOA = hasTag(results, "B02_AUTH_RESPONSE_SOA")
	}

	if util.ShouldRunTest(ctx, "basic03") {
		if authResponseSOA {
			if err := appendLog(&results, "Basic03", "HAS_NAMESERVER_NO_WWW_A_TEST", map[string]any{
				"zname": z.Name.String(),
			}); err != nil {
				return results, err
			}
		} else {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Basic03(ctx, z)
			})
			results = append(results, entries...)
			if err != nil {
				return results, err
			}
		}
	}

	return results, nil
}

// CanContinue reports whether later test cases can proceed based on Basic02.
func CanContinue(ctx context.Context, z *zone.Zone, results []*logger.Entry) bool {
	if util.ShouldRunTest(ctx, "basic02") {
		tags := map[string]bool{}
		for _, entry := range results {
			if entry == nil {
				continue
			}
			tags[entry.Tag] = true
		}
		return !tags["B02_NO_DELEGATION"] && tags["B02_AUTH_RESPONSE_SOA"]
	}
	return true
}

// Metadata returns the set of tags emitted by Basic test cases.
func Metadata() map[string][]string {
	return map[string][]string{
		"basic01": {
			"B01_CHILD_IS_ALIAS",
			"B01_CHILD_FOUND",
			"B01_INCONSISTENT_ALIAS",
			"B01_INCONSISTENT_DELEGATION",
			"B01_NO_CHILD",
			"B01_PARENT_DISREGARDED",
			"B01_PARENT_FOUND",
			"B01_PARENT_NOT_FOUND",
			"B01_PARENT_UNDETERMINED",
			"B01_ROOT_HAS_NO_PARENT",
			"B01_SERVER_ZONE_ERROR",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"basic02": {
			"B02_AUTH_RESPONSE_SOA",
			"B02_NO_DELEGATION",
			"B02_NO_WORKING_NS",
			"B02_NS_BROKEN",
			"B02_NS_NOT_AUTH",
			"B02_NS_NO_IP_ADDR",
			"B02_NS_NO_RESPONSE",
			"B02_UNEXPECTED_RCODE",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"IPV4_ENABLED",
			"IPV6_ENABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"basic03": {
			"A_QUERY_NO_RESPONSES",
			"HAS_A_RECORDS",
			"IPV4_DISABLED",
			"IPV4_ENABLED",
			"IPV6_DISABLED",
			"IPV6_ENABLED",
			"NO_A_RECORDS",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// Basic01 runs the BASIC01 test case.
func Basic01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Basic01"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	if z.Name.String() == "." {
		if err := appendLog(&results, testcase, "B01_CHILD_FOUND", map[string]any{"domain": z.Name.String()}); err != nil {
			return results, err
		}
		if err := appendLog(&results, testcase, "B01_ROOT_HAS_NO_PARENT", map[string]any{}); err != nil {
			return results, err
		}
		return appendTestCaseEnd(results, testcase)
	}

	rec := z.Recursor()
	if rec == nil {
		return results, fmt.Errorf("missing recursor")
	}

	if rec.HasFakeAddresses(z.Name.String()) {
		if err := appendLog(&results, testcase, "B01_CHILD_FOUND", map[string]any{"domain": z.Name.String()}); err != nil {
			return results, err
		}
		if err := appendLog(&results, testcase, "B01_PARENT_DISREGARDED", map[string]any{}); err != nil {
			return results, err
		}
		return appendTestCaseEnd(results, testcase)
	}

	handledServers := map[string]map[string]bool{}
	parentFound := map[string]map[string]bool{}
	delegationFound := map[string]map[string]bool{}
	aaNXDomain := map[string]map[string]bool{}
	aaSOA := map[string]map[string]bool{}
	aaCNAME := map[string]map[string]bool{}
	cnameWithReferral := map[string]map[string]bool{}
	aaDname := map[string]map[string]map[string]bool{}
	aaNodata := map[string]map[string]bool{}

	root, err := rec.RootServers()
	if err != nil {
		return results, err
	}

	allServers := map[string][]nameserver.Nameserver{
		".": root,
	}
	allLabels := map[string]bool{".": true}
	remainingLabels := []string{"."}
	zoneLabels := z.Name.Labels()

	for len(remainingLabels) > 0 {
		zoneName := remainingLabels[0]
		remainingLabels = remainingLabels[1:]
		remainingServers := append([]nameserver.Nameserver{}, allServers[zoneName]...)

	serverLoop:
		for len(remainingServers) > 0 {
			ns := remainingServers[0]
			remainingServers = remainingServers[1:]
			addr := ns.Address.String()

			if markHandled(handledServers, zoneName, addr) {
				continue serverLoop
			}

			disabled, err := ipDisabledMessage(ctx, &results, testcase, ns, "SOA", "NS", "DNAME")
			if err != nil {
				return results, err
			}
			if disabled {
				continue serverLoop
			}
			if err := ipEnabledMessage(ctx, &results, testcase, ns, "SOA", "NS", "DNAME"); err != nil {
				return results, err
			}

			pSOA, err := ns.Query(ctx, zoneName, "SOA")
			if err != nil || pSOA.Msg == nil || pSOA.Rcode() != "NOERROR" || !pSOA.AA() || len(pSOA.GetRecordsForName("SOA", dnsname.New(zoneName), "answer")) != 1 {
				if err := appendLog(&results, testcase, "B01_SERVER_ZONE_ERROR", map[string]any{
					"query_name": zoneName,
					"rrtype":     "SOA",
					"ns":         ns.String(),
				}); err != nil {
					return results, err
				}
				continue serverLoop
			}

			pNS, err := ns.Query(ctx, zoneName, "NS")
			if err != nil || pNS.Msg == nil || pNS.Rcode() != "NOERROR" || !pNS.AA() || len(pNS.GetRecords("NS", "answer")) == 0 || len(pNS.GetRecords("NS", "answer")) != len(pNS.GetRecordsForName("NS", dnsname.New(zoneName), "answer")) {
				if err := appendLog(&results, testcase, "B01_SERVER_ZONE_ERROR", map[string]any{
					"query_name": zoneName,
					"rrtype":     "NS",
					"ns":         ns.String(),
				}); err != nil {
					return results, err
				}
				continue serverLoop
			}

			rrsNS := map[string][]netip.Addr{}
			for _, rr := range pNS.GetRecordsForName("NS", dnsname.New(zoneName), "answer") {
				nsRR, ok := rr.(*dns.NS)
				if !ok {
					continue
				}
				nsNameObj := dnsname.New(nsRR.Ns)
				nsName := strings.ToLower(nsNameObj.String())
				rrsNS[nsName] = []netip.Addr{}
			}
			for _, rr := range append(pNS.GetRecords("A", "additional"), pNS.GetRecords("AAAA", "additional")...) {
				ownerName := dnsname.New(rr.Header().Name)
				owner := strings.ToLower(ownerName.String())
				if _, ok := rrsNS[owner]; ok {
					if addr, ok := addrFromRR(rr); ok {
						rrsNS[owner] = append(rrsNS[owner], addr)
					}
				}
			}

			for nsName, addrs := range rrsNS {
				if len(addrs) == 0 {
					for _, qtype := range []string{"A", "AAAA"} {
						resp, err := rec.Recurse(ctx, nsName, qtype, "IN")
						if err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
							continue
						}
						for _, rr := range resp.GetRecordsForName(qtype, dnsname.New(nsName)) {
							if addr, ok := addrFromRR(rr); ok {
								addrs = append(addrs, addr)
							}
						}
					}
				}

				for _, addr := range addrs {
					if handledServers[zoneName] != nil && handledServers[zoneName][addr.String()] {
						continue
					}
					nsObj, err := nameserver.New(nsName, addr.String(), rec.Client())
					if err != nil {
						continue
					}
					allServers[zoneName] = append(allServers[zoneName], nsObj)
					if !allLabels[zoneName] {
						remainingLabels = append(remainingLabels, zoneName)
						allLabels[zoneName] = true
					}
				}
				rrsNS[nsName] = addrs
			}

			intermediate := dnsname.New(zoneName)
			loopZoneName := strings.ToLower(intermediate.String())
			loopCount := 0
			for {
				loopCount++
				if loopCount >= 1000 {
					_, _ = util.Logger().Add("LOOP_PROTECTION", map[string]any{
						"caller":                  "basic.Basic01",
						"child_zone_name":         z.Name.String(),
						"name":                    loopZoneName,
						"intermediate_query_name": intermediate.String(),
					}, "", testcase)
					return appendTestCaseEnd(results, testcase)
				}

				if len(intermediate.Labels()) >= len(zoneLabels) {
					break
				}
				idx := len(zoneLabels) - len(intermediate.Labels()) - 1
				intermediate = intermediate.Prepend(zoneLabels[idx])

				pSOA, err = ns.Query(ctx, intermediate.String(), "SOA")
				if err != nil || pSOA.Msg == nil {
					if err := appendLog(&results, testcase, "B01_SERVER_ZONE_ERROR", map[string]any{
						"query_name": intermediate.String(),
						"rrtype":     "SOA",
						"ns":         ns.String(),
					}); err != nil {
						return results, err
					}
					continue serverLoop
				}

				if pSOA.Rcode() == "NOERROR" && pSOA.AA() && len(pSOA.GetRecordsForName("SOA", intermediate, "answer")) == 1 {
					if strings.EqualFold(intermediate.String(), z.Name.String()) {
						addNS(parentFound, loopZoneName, ns.String())
						addNS(aaSOA, loopZoneName, ns.String())
					} else {
						pNS, err = ns.Query(ctx, intermediate.String(), "NS")
						if err != nil || pNS.Msg == nil || pNS.Rcode() != "NOERROR" || !pNS.AA() || len(pNS.GetRecords("NS", "answer")) == 0 || len(pNS.GetRecords("NS", "answer")) != len(pNS.GetRecordsForName("NS", intermediate, "answer")) {
							if err := appendLog(&results, testcase, "B01_SERVER_ZONE_ERROR", map[string]any{
								"query_name": intermediate.String(),
								"rrtype":     "NS",
								"ns":         ns.String(),
							}); err != nil {
								return results, err
							}
							continue serverLoop
						}

						rrsNSBis := map[string][]netip.Addr{}
						for _, rr := range pNS.GetRecordsForName("NS", intermediate, "answer") {
							nsRR, ok := rr.(*dns.NS)
							if !ok {
								continue
							}
							nsNameObj := dnsname.New(nsRR.Ns)
							nsName := strings.ToLower(nsNameObj.String())
							rrsNSBis[nsName] = []netip.Addr{}
						}
						for _, rr := range append(pNS.GetRecords("A", "additional"), pNS.GetRecords("AAAA", "additional")...) {
							ownerName := dnsname.New(rr.Header().Name)
							owner := strings.ToLower(ownerName.String())
							if _, ok := rrsNSBis[owner]; ok {
								if addr, ok := addrFromRR(rr); ok {
									rrsNSBis[owner] = append(rrsNSBis[owner], addr)
								}
							}
						}

						for nsName, addrs := range rrsNSBis {
							if len(addrs) == 0 {
								for _, qtype := range []string{"A", "AAAA"} {
									resp, err := rec.Recurse(ctx, nsName, qtype, "IN")
									if err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
										continue
									}
									for _, rr := range resp.GetRecordsForName(qtype, dnsname.New(nsName)) {
										if addr, ok := addrFromRR(rr); ok {
											addrs = append(addrs, addr)
										}
									}
								}
							}

							for _, addr := range addrs {
								if handledServers[intermediate.String()] != nil && handledServers[intermediate.String()][addr.String()] {
									continue
								}
								nsObj, err := nameserver.New(nsName, addr.String(), rec.Client())
								if err != nil {
									continue
								}
								allServers[intermediate.String()] = append(allServers[intermediate.String()], nsObj)
								if !allLabels[intermediate.String()] {
									remainingLabels = append(remainingLabels, intermediate.String())
									allLabels[intermediate.String()] = true
								}
							}
							rrsNSBis[nsName] = addrs
						}

						loopZoneName = strings.ToLower(intermediate.String())
						continue
					}
				} else if pSOA.Rcode() == "NXDOMAIN" && pSOA.AA() {
					addNS(parentFound, loopZoneName, ns.String())
					addNS(aaNXDomain, loopZoneName, ns.String())
				} else if pSOA.IsRedirect() && len(pSOA.GetRecordsForName("NS", intermediate, "authority")) > 0 {
					if strings.EqualFold(intermediate.String(), z.Name.String()) {
						addNS(parentFound, loopZoneName, ns.String())
						addNS(delegationFound, loopZoneName, ns.String())
					} else {
						rrsNSBis := map[string][]netip.Addr{}
						for _, rr := range pSOA.GetRecordsForName("NS", intermediate, "authority") {
							nsRR, ok := rr.(*dns.NS)
							if !ok {
								continue
							}
							nsNameObj := dnsname.New(nsRR.Ns)
							nsName := strings.ToLower(nsNameObj.String())
							rrsNSBis[nsName] = []netip.Addr{}
						}
						for _, rr := range append(pSOA.GetRecords("A", "additional"), pSOA.GetRecords("AAAA", "additional")...) {
							ownerName := dnsname.New(rr.Header().Name)
							owner := strings.ToLower(ownerName.String())
							if _, ok := rrsNSBis[owner]; ok {
								if addr, ok := addrFromRR(rr); ok {
									rrsNSBis[owner] = append(rrsNSBis[owner], addr)
								}
							}
						}

						for nsName, addrs := range rrsNSBis {
							if len(addrs) == 0 {
								for _, qtype := range []string{"A", "AAAA"} {
									resp, err := rec.Recurse(ctx, nsName, qtype, "IN")
									if err != nil || resp.Msg == nil || resp.Rcode() != "NOERROR" {
										continue
									}
									for _, rr := range resp.GetRecordsForName(qtype, dnsname.New(nsName)) {
										if addr, ok := addrFromRR(rr); ok {
											addrs = append(addrs, addr)
										}
									}
								}
							}

							for _, addr := range addrs {
								if handledServers[intermediate.String()] != nil && handledServers[intermediate.String()][addr.String()] {
									continue
								}
								nsObj, err := nameserver.New(nsName, addr.String(), rec.Client())
								if err != nil {
									continue
								}
								allServers[intermediate.String()] = append(allServers[intermediate.String()], nsObj)
								if !allLabels[intermediate.String()] {
									remainingLabels = append(remainingLabels, intermediate.String())
									allLabels[intermediate.String()] = true
								}
							}
							rrsNSBis[nsName] = addrs
						}
					}
				} else if pSOA.Rcode() == "NOERROR" && pSOA.AA() {
					if !strings.EqualFold(intermediate.String(), z.Name.String()) {
						continue
					}
					if len(pSOA.GetRecordsForName("CNAME", z.Name, "answer")) > 0 {
						addNS(parentFound, loopZoneName, ns.String())
						addNS(aaCNAME, loopZoneName, ns.String())
					} else {
						pDname, err := ns.Query(ctx, z.Name.String(), "DNAME")
						if err == nil && pDname.Msg != nil && pDname.AA() && pDname.Rcode() == "NOERROR" && len(pDname.GetRecordsForName("DNAME", z.Name, "answer")) == 1 {
							records := pDname.GetRecordsForName("DNAME", z.Name, "answer")
							dnameRR, ok := records[0].(*dns.DNAME)
							if ok {
								targetName := dnsname.New(dnameRR.Target)
								target := strings.ToLower(targetName.String())
								addNS(parentFound, loopZoneName, ns.String())
								addDname(aaDname, target, loopZoneName, ns.String())
							} else {
								addNS(parentFound, loopZoneName, ns.String())
								addNS(aaNodata, loopZoneName, ns.String())
							}
						} else {
							addNS(parentFound, loopZoneName, ns.String())
							addNS(aaNodata, loopZoneName, ns.String())
						}
					}
				} else if pSOA.IsRedirect() && len(pSOA.GetRecordsForName("CNAME", z.Name, "answer")) > 0 {
					addNS(parentFound, loopZoneName, ns.String())
					addNS(cnameWithReferral, loopZoneName, ns.String())
				} else {
					if err := appendLog(&results, testcase, "B01_SERVER_ZONE_ERROR", map[string]any{
						"query_name": intermediate.String(),
						"rrtype":     "SOA",
						"ns":         ns.String(),
					}); err != nil {
						return results, err
					}
				}

				continue serverLoop
			}
		}
	}

	if len(parentFound) > 0 {
		for domain, nsMap := range parentFound {
			if err := appendLog(&results, testcase, "B01_PARENT_FOUND", map[string]any{
				"domain":  domain,
				"ns_list": joinSorted(nsMap),
			}); err != nil {
				return results, err
			}
		}
		if len(parentFound) > 1 {
			nsSet := map[string]bool{}
			for _, nsMap := range parentFound {
				mergeSet(nsSet, nsMap)
			}
			if err := appendLog(&results, testcase, "B01_PARENT_UNDETERMINED", map[string]any{
				"ns_list": joinSorted(nsSet),
			}); err != nil {
				return results, err
			}
		}
	} else {
		if err := appendLog(&results, testcase, "B01_PARENT_NOT_FOUND", map[string]any{}); err != nil {
			return results, err
		}
	}

	if len(delegationFound) > 0 || len(aaSOA) > 0 {
		if err := appendLog(&results, testcase, "B01_CHILD_FOUND", map[string]any{
			"domain": z.Name.String(),
		}); err != nil {
			return results, err
		}

		if !rec.HasFakeAddresses(z.Name.String()) {
			parents := map[string]bool{}
			collectParents(parents, aaNXDomain)
			collectParents(parents, aaCNAME)
			collectParents(parents, cnameWithReferral)
			collectParents(parents, aaNodata)
			for _, perTarget := range aaDname {
				collectParents(parents, perTarget)
			}

			for parent := range parents {
				nsSet := map[string]bool{}
				mergeSet(nsSet, aaNXDomain[parent])
				mergeSet(nsSet, aaCNAME[parent])
				mergeSet(nsSet, cnameWithReferral[parent])
				mergeSet(nsSet, aaNodata[parent])
				for _, perTarget := range aaDname {
					mergeSet(nsSet, perTarget[parent])
				}
				if err := appendLog(&results, testcase, "B01_INCONSISTENT_DELEGATION", map[string]any{
					"domain_parent": parent,
					"domain_child":  z.Name.String(),
					"ns_list":       joinSorted(nsSet),
				}); err != nil {
					return results, err
				}
			}
		}
	}

	if len(delegationFound) == 0 && len(aaSOA) == 0 {
		if rec.HasFakeAddresses(z.Name.String()) {
			if err := appendLog(&results, testcase, "B01_CHILD_NOT_EXIST", map[string]any{
				"domain": z.Name.String(),
			}); err != nil {
				return results, err
			}
		} else {
			parent, _ := z.Name.NextHigher()
			if err := appendLog(&results, testcase, "B01_NO_CHILD", map[string]any{
				"domain_child": z.Name.String(),
				"domain_super": parent.String(),
			}); err != nil {
				return results, err
			}
		}
	}

	if len(aaDname) > 0 {
		for target, perParent := range aaDname {
			nsSet := map[string]bool{}
			for _, nsMap := range perParent {
				mergeSet(nsSet, nsMap)
			}
			if err := appendLog(&results, testcase, "B01_CHILD_IS_ALIAS", map[string]any{
				"domain_child":  z.Name.String(),
				"domain_target": target,
				"ns_list":       joinSorted(nsSet),
			}); err != nil {
				return results, err
			}
		}
		if len(aaDname) > 1 {
			if err := appendLog(&results, testcase, "B01_INCONSISTENT_ALIAS", map[string]any{
				"domain": z.Name.String(),
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Basic02 runs the BASIC02 test case.
func Basic02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Basic02"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsNames, err := methods.Method2(ctx, z)
	if err != nil {
		return results, err
	}
	nsServers, err := methods.Method4(ctx, z)
	if err != nil {
		return results, err
	}

	if len(nsNames) == 0 {
		if err := appendLog(&results, testcase, "B02_NO_DELEGATION", map[string]any{
			"domain": z.Name.String(),
		}); err != nil {
			return results, err
		}
		return appendTestCaseEnd(results, testcase)
	}

	nsBroken := map[string]bool{}
	nsNotAuth := map[string]bool{}
	nsCantResolve := map[string]bool{}
	nsNoResponse := map[string]bool{}
	unexpectedRcode := map[string]string{}
	authResponseSOA := map[string]bool{}

	if len(nsServers) == 0 {
		foundIP := map[string]bool{}
		glueAddrs, err := z.GlueAddresses(ctx)
		if err != nil {
			return results, err
		}

		for _, nsName := range nsNames {
			key := strings.ToLower(nsName.String())
			foundIP[key] = false
			for _, rr := range glueAddrs {
				ownerName := dnsname.New(rr.Header().Name)
				owner := strings.ToLower(ownerName.String())
				if owner != key {
					continue
				}
				if addr, ok := addrFromRR(rr); ok {
					nsObj, err := nameserver.New(nsName.String(), addr.String(), z.Recursor().Client())
					if err != nil {
						continue
					}
					nsServers = append(nsServers, nsObj)
					foundIP[key] = true
				}
			}
		}

		for key, ok := range foundIP {
			if !ok {
				nsCantResolve[key] = true
			}
		}
	}

	type nsOutcome struct {
		ns             nameserver.Nameserver
		skipped        bool
		noResponse     bool
		notAuth        bool
		broken         bool
		authResponse   bool
		unexpectedCode string
	}

	if len(nsServers) > 0 {
		outcomes := make([]nsOutcome, len(nsServers))
		tasks := make([]runner.Task, len(nsServers))
		for i, ns := range nsServers {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{ns: ns}

				if ns.Address.Is6() && !profile.FromContext(ctx).Net.IPv6 {
					if _, err := buf.Add("IPV6_DISABLED", map[string]any{"ns": ns.String(), "rrtype": "SOA"}); err != nil {
						return err
					}
					outcome.skipped = true
					outcomes[i] = outcome
					return nil
				}
				if ns.Address.Is4() && !profile.FromContext(ctx).Net.IPv4 {
					if _, err := buf.Add("IPV4_DISABLED", map[string]any{"ns": ns.String(), "rrtype": "SOA"}); err != nil {
						return err
					}
					outcome.skipped = true
					outcomes[i] = outcome
					return nil
				}

				if ns.Address.Is6() && profile.FromContext(ctx).Net.IPv6 {
					if _, err := buf.Add("IPV6_ENABLED", map[string]any{"ns": ns.String(), "rrtype": "SOA"}); err != nil {
						return err
					}
				}
				if ns.Address.Is4() && profile.FromContext(ctx).Net.IPv4 {
					if _, err := buf.Add("IPV4_ENABLED", map[string]any{"ns": ns.String(), "rrtype": "SOA"}); err != nil {
						return err
					}
				}

				resp, err := ns.Query(ctx, z.Name.String(), "SOA")
				if err != nil || resp.Msg == nil {
					outcome.noResponse = true
					outcomes[i] = outcome
					return nil
				}
				if resp.Rcode() != "NOERROR" {
					outcome.unexpectedCode = resp.Rcode()
					outcomes[i] = outcome
					return nil
				}
				if !resp.AA() {
					outcome.notAuth = true
					outcomes[i] = outcome
					return nil
				}
				if len(resp.GetRecordsForName("SOA", z.Name, "answer")) > 0 {
					outcome.authResponse = true
				} else {
					outcome.broken = true
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

		for _, outcome := range outcomes {
			if outcome.skipped {
				continue
			}
			key := outcome.ns.String()
			switch {
			case outcome.noResponse:
				nsNoResponse[key] = true
			case outcome.unexpectedCode != "":
				unexpectedRcode[key] = outcome.unexpectedCode
			case outcome.notAuth:
				nsNotAuth[key] = true
			case outcome.authResponse:
				authResponseSOA[key] = true
			case outcome.broken:
				nsBroken[key] = true
			}
		}
	}

	if len(authResponseSOA) > 0 {
		if err := appendLog(&results, testcase, "B02_AUTH_RESPONSE_SOA", map[string]any{
			"domain":  z.Name.String(),
			"ns_list": joinSorted(authResponseSOA),
		}); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(&results, testcase, "B02_NO_WORKING_NS", map[string]any{
			"domain": z.Name.String(),
		}); err != nil {
			return results, err
		}

		for ns := range nsBroken {
			if err := appendLog(&results, testcase, "B02_NS_BROKEN", map[string]any{"ns": ns}); err != nil {
				return results, err
			}
		}
		for ns := range nsNotAuth {
			if err := appendLog(&results, testcase, "B02_NS_NOT_AUTH", map[string]any{"ns": ns}); err != nil {
				return results, err
			}
		}
		for nsName := range nsCantResolve {
			if err := appendLog(&results, testcase, "B02_NS_NO_IP_ADDR", map[string]any{"nsname": nsName}); err != nil {
				return results, err
			}
		}
		for ns := range nsNoResponse {
			if err := appendLog(&results, testcase, "B02_NS_NO_RESPONSE", map[string]any{"ns": ns}); err != nil {
				return results, err
			}
		}
		for ns, rcode := range unexpectedRcode {
			if err := appendLog(&results, testcase, "B02_UNEXPECTED_RCODE", map[string]any{
				"rcode": rcode,
				"ns":    ns,
			}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(results, testcase)
}

// Basic03 runs the BASIC03 test case.
func Basic03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Basic03"
	var results []*logger.Entry
	logger.ModuleName = moduleName
	logger.TestCaseName = testcase

	if err := appendLog(&results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	queryName := "www." + z.Name.String()

	nsServers, err := methods.Method4(ctx, z)
	if err != nil {
		return results, err
	}

	if len(nsServers) > 0 {
		type nsOutcome struct {
			responded bool
		}

		outcomes := make([]nsOutcome, len(nsServers))
		tasks := make([]runner.Task, len(nsServers))
		for i, ns := range nsServers {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{}

				if ns.Address.Is6() && !profile.FromContext(ctx).Net.IPv6 {
					if _, err := buf.Add("IPV6_DISABLED", map[string]any{"ns": ns.String(), "rrtype": "A"}); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}
				if ns.Address.Is4() && !profile.FromContext(ctx).Net.IPv4 {
					if _, err := buf.Add("IPV4_DISABLED", map[string]any{"ns": ns.String(), "rrtype": "A"}); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				if ns.Address.Is6() && profile.FromContext(ctx).Net.IPv6 {
					if _, err := buf.Add("IPV6_ENABLED", map[string]any{"ns": ns.String(), "rrtype": "A"}); err != nil {
						return err
					}
				}
				if ns.Address.Is4() && profile.FromContext(ctx).Net.IPv4 {
					if _, err := buf.Add("IPV4_ENABLED", map[string]any{"ns": ns.String(), "rrtype": "A"}); err != nil {
						return err
					}
				}

				resp, err := ns.Query(ctx, queryName, "A")
				if err != nil || resp.Msg == nil {
					outcomes[i] = outcome
					return nil
				}
				outcome.responded = true
				target := dnsname.New(queryName)
				if resp.HasRRsOfTypeForName("A", target) {
					if _, err := buf.Add("HAS_A_RECORDS", map[string]any{
						"ns":     ns.String(),
						"domain": queryName,
					}); err != nil {
						return err
					}
				} else {
					if _, err := buf.Add("NO_A_RECORDS", map[string]any{
						"ns":     ns.String(),
						"domain": queryName,
					}); err != nil {
						return err
					}
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

		responseCount := 0
		for _, outcome := range outcomes {
			if outcome.responded {
				responseCount++
			}
		}

		if responseCount == 0 {
			if err := appendLog(&results, testcase, "A_QUERY_NO_RESPONSES", map[string]any{}); err != nil {
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

func ipDisabledMessage(ctx context.Context, results *[]*logger.Entry, testcase string, ns nameserver.Nameserver, rrtypes ...string) (bool, error) {
	if ns.Address.Is4() && !profile.FromContext(ctx).Net.IPv4 {
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
	if ns.Address.Is6() && !profile.FromContext(ctx).Net.IPv6 {
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
	return false, nil
}

func ipEnabledMessage(ctx context.Context, results *[]*logger.Entry, testcase string, ns nameserver.Nameserver, rrtypes ...string) error {
	if ns.Address.Is4() && profile.FromContext(ctx).Net.IPv4 {
		for _, rrtype := range rrtypes {
			if err := appendLog(results, testcase, "IPV4_ENABLED", map[string]any{
				"ns":     ns.String(),
				"rrtype": rrtype,
			}); err != nil {
				return err
			}
		}
	}
	if ns.Address.Is6() && profile.FromContext(ctx).Net.IPv6 {
		for _, rrtype := range rrtypes {
			if err := appendLog(results, testcase, "IPV6_ENABLED", map[string]any{
				"ns":     ns.String(),
				"rrtype": rrtype,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func markHandled(handled map[string]map[string]bool, zoneName string, addr string) bool {
	if handled[zoneName] == nil {
		handled[zoneName] = map[string]bool{}
	}
	if handled[zoneName][addr] {
		return true
	}
	handled[zoneName][addr] = true
	return false
}

func addNS(m map[string]map[string]bool, key string, ns string) {
	if m[key] == nil {
		m[key] = map[string]bool{}
	}
	m[key][ns] = true
}

func addDname(m map[string]map[string]map[string]bool, target string, parent string, ns string) {
	if m[target] == nil {
		m[target] = map[string]map[string]bool{}
	}
	if m[target][parent] == nil {
		m[target][parent] = map[string]bool{}
	}
	m[target][parent][ns] = true
}

func mergeSet(dst map[string]bool, src map[string]bool) {
	for key := range src {
		dst[key] = true
	}
}

func collectParents(dst map[string]bool, src map[string]map[string]bool) {
	for key := range src {
		dst[key] = true
	}
}

func joinSorted(items map[string]bool) string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ";")
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

func addrFromRR(rr dns.RR) (netip.Addr, bool) {
	switch v := rr.(type) {
	case *dns.A:
		addr, err := netip.ParseAddr(v.A.String())
		return addr, err == nil
	case *dns.AAAA:
		addr, err := netip.ParseAddr(v.AAAA.String())
		return addr, err == nil
	default:
		return netip.Addr{}, false
	}
}
