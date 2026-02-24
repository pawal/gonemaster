package nameserver

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"sort"
	"strings"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/methods"
	ns "codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/runner"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testcase"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testlogger"
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
	method2       = methods.Method2
	method3       = methods.Method3
	method4and5   = methods.Method4and5
	scrambleCaseFunc = util.ScrambleCase
)

// All runs the Nameserver test cases in order, mirroring the Perl implementation.
func All(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest(ctx, "nameserver01") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver01(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver02") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver02(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver03") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver03(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver04") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver04(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver05") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver05(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver06") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver06(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver07") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver07(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver08") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver08(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver09") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver09(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver10") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver10(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver11") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver11(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver12") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver12(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver13") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver13(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver15") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver15(ctx, z)
		})
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
			"IPV4_DISABLED",
			"IPV6_DISABLED",
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
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver03": {
			"AXFR_FAILURE",
			"AXFR_AVAILABLE",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver04": {
			"DIFFERENT_SOURCE_IP",
			"SAME_SOURCE_IP",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
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
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver08": {
			"QNAME_CASE_INSENSITIVE",
			"QNAME_CASE_SENSITIVE",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
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
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver10": {
			"N10_NO_RESPONSE_EDNS1_QUERY",
			"N10_UNEXPECTED_RCODE",
			"N10_EDNS_RESPONSE_ERROR",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
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
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver12": {
			"NO_RESPONSE",
			"NO_EDNS_SUPPORT",
			"Z_FLAGS_NOTCLEAR",
			"NS_ERROR",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver13": {
			"NO_RESPONSE",
			"NO_EDNS_SUPPORT",
			"NS_ERROR",
			"MISSING_OPT_IN_TRUNCATED",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver15": {
			"N15_ERROR_ON_VERSION_QUERY",
			"N15_NO_VERSION_REVEALED",
			"N15_SOFTWARE_VERSION",
			"N15_WRONG_CLASS",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// Nameserver01 runs the NAMESERVER01 test case.
func Nameserver01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver01"
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
		for i, server := range nss {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "A"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				responseCount := 0
				nxdomainCount := 0
				isNoRecursor := true
				hasSeenRA := false

				for _, name := range nonExistentNames {
					resp, err := server.QueryWithOptions(ctx, name, "A", nil)
					if err != nil || resp.Msg == nil {
						if _, err := buf.Add("NO_RESPONSE", map[string]any{
							"ns":     server.String(),
							"domain": name,
						}); err != nil {
							return err
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
					if _, err := buf.Add("IS_A_RECURSOR", map[string]any{"ns": server.String()}); err != nil {
						return err
					}
					isNoRecursor = false
				}
				if isNoRecursor {
					if _, err := buf.Add("NO_RECURSOR", map[string]any{"ns": server.String()}); err != nil {
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

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver02 runs the NAMESERVER02 test case.
func Nameserver02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver02"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	type ednsOutcome struct {
		key      string
		included bool
		hasError bool
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}
	ordered := uniqueServersByKey(nss)

	var outcomes []ednsOutcome
	if len(ordered) > 0 {
		outcomes = make([]ednsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := ednsOutcome{key: server.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				outcome.included = true
				ver := uint8(0)
				resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver}})
				if err == nil && resp.Msg != nil {
					if resp.Rcode() == "FORMERR" && !resp.HasEdns() {
						if _, err := buf.Add("NO_EDNS_SUPPORT", map[string]any{"ns": server.String()}); err != nil {
							return err
						}
						outcome.hasError = true
					} else if resp.Rcode() == "NOERROR" && resp.EdnsRcode() == 0 && len(resp.GetRecords("SOA", "answer")) > 0 && resp.EdnsVersion() == 0 {
						outcomes[i] = outcome
						return nil
					} else if resp.Rcode() == "NOERROR" && !resp.HasEdns() {
						if _, err := buf.Add("EDNS_RESPONSE_WITHOUT_EDNS", map[string]any{
							"ns":     server.String(),
							"domain": z.Name.String(),
						}); err != nil {
							return err
						}
						outcome.hasError = true
					} else if resp.Rcode() == "NOERROR" && resp.HasEdns() && resp.EdnsVersion() != 0 {
						if _, err := buf.Add("EDNS_VERSION_ERROR", map[string]any{
							"ns":     server.String(),
							"domain": z.Name.String(),
						}); err != nil {
							return err
						}
						outcome.hasError = true
					} else {
						if _, err := buf.Add("NS_ERROR", map[string]any{"ns": server.String()}); err != nil {
							return err
						}
						outcome.hasError = true
					}
				} else {
					resp2, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
					if err == nil && resp2.Msg != nil {
						if _, err := buf.Add("BREAKS_ON_EDNS", map[string]any{
							"ns":     server.String(),
							"domain": z.Name.String(),
						}); err != nil {
							return err
						}
						outcome.hasError = true
					} else {
						if _, err := buf.Add("NO_RESPONSE", map[string]any{
							"ns":     server.String(),
							"domain": z.Name.String(),
						}); err != nil {
							return err
						}
						outcome.hasError = true
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
	}

	nErrors := 0
	included := map[string]bool{}
	for _, outcome := range outcomes {
		if !outcome.included {
			continue
		}
		included[outcome.key] = true
		if outcome.hasError {
			nErrors++
		}
	}

	if len(included) > 0 && nErrors == 0 {
		keys := sortedKeys(included)
		if err := appendLog(ctx, &results, testcase, "EDNS0_SUPPORT", map[string]any{
			"ns_list": strings.Join(keys, ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver03 runs the NAMESERVER03 test case.
func Nameserver03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver03"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	ordered := uniqueServersByKey(nss)
	if len(ordered) > 0 {
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "AXFR"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				var firstRR dns.RR
				err := server.AXFR(ctx, z.Name.String(), func(rr dns.RR) bool {
					firstRR = rr
					return false
				}, "")
				if err != nil {
					if _, err := buf.Add("AXFR_FAILURE", map[string]any{"ns": server.String()}); err != nil {
						return err
					}
				} else if soa, ok := firstRR.(*dns.SOA); ok && soa != nil {
					if _, err := buf.Add("AXFR_AVAILABLE", map[string]any{"ns": server.String()}); err != nil {
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

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver04 runs the NAMESERVER04 test case.
func Nameserver04(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver04"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	type sourceOutcome struct {
		key      string
		included bool
		hasError bool
	}

	ordered := uniqueServersByKey(nss)
	var outcomes []sourceOutcome
	if len(ordered) > 0 {
		outcomes = make([]sourceOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := sourceOutcome{key: server.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}
				outcome.included = true

				resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if err == nil && resp.Msg != nil {
					if addr, ok := parseAnswerFrom(resp.AnswerFrom); ok && addr != server.Address {
						if _, err := buf.Add("DIFFERENT_SOURCE_IP", map[string]any{
							"ns":     server.String(),
							"source": resp.AnswerFrom,
						}); err != nil {
							return err
						}
						outcome.hasError = true
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
	}

	nErrors := 0
	included := map[string]bool{}
	for _, outcome := range outcomes {
		if !outcome.included {
			continue
		}
		included[outcome.key] = true
		if outcome.hasError {
			nErrors++
		}
	}

	if len(included) > 0 && nErrors == 0 {
		if err := appendLog(ctx, &results, testcase, "SAME_SOURCE_IP", map[string]any{
			"names": strings.Join(sortedKeys(included), ","),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver05 runs the NAMESERVER05 test case.
func Nameserver05(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver05"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	type aaaaOutcome struct {
		key       string
		included  bool
		aaaaIssue int
		aaaaOK    int
	}

	ordered := uniqueServersByKey(nss)
	var outcomes []aaaaOutcome
	if len(ordered) > 0 {
		outcomes = make([]aaaaOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := aaaaOutcome{key: server.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "A"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}
				outcome.included = true

				useVC := false
				resp, err := server.QueryWithOptions(ctx, z.Name.String(), "A", &ns.QueryOptions{UseVC: &useVC})
				if err != nil || resp.Msg == nil {
					if _, err := buf.Add("NO_RESPONSE", map[string]any{
						"ns":     server.String(),
						"domain": z.Name.String(),
					}); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}
				if resp.Rcode() != "NOERROR" {
					if _, err := buf.Add("A_UNEXPECTED_RCODE", map[string]any{
						"ns":    server.String(),
						"rcode": resp.Rcode(),
					}); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				resp, err = server.QueryWithOptions(ctx, z.Name.String(), "AAAA", &ns.QueryOptions{UseVC: &useVC})
				if err != nil || resp.Msg == nil {
					if _, err := buf.Add("AAAA_QUERY_DROPPED", map[string]any{"ns": server.String()}); err != nil {
						return err
					}
					outcome.aaaaIssue++
					outcomes[i] = outcome
					return nil
				}
				if resp.Rcode() != "NOERROR" {
					if _, err := buf.Add("AAAA_UNEXPECTED_RCODE", map[string]any{
						"ns":    server.String(),
						"rcode": resp.Rcode(),
					}); err != nil {
						return err
					}
					outcome.aaaaIssue++
					outcomes[i] = outcome
					return nil
				}

				for _, rr := range resp.GetRecords("AAAA", "answer") {
					if aaaa, ok := rr.(*dns.AAAA); ok {
						if !aaaa.Addr.IsValid() {
							if _, err := buf.Add("AAAA_BAD_RDATA", map[string]any{
								"ns":     server.String(),
								"length": 0,
							}); err != nil {
								return err
							}
							outcome.aaaaIssue++
						} else {
							outcome.aaaaOK++
						}
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
	}

	aaaaIssue := 0
	aaaaOK := 0
	included := map[string]bool{}
	for _, outcome := range outcomes {
		if !outcome.included {
			continue
		}
		included[outcome.key] = true
		aaaaIssue += outcome.aaaaIssue
		aaaaOK += outcome.aaaaOK
	}

	if aaaaOK > 0 && aaaaIssue == 0 {
		if err := appendLog(ctx, &results, testcase, "AAAA_WELL_PROCESSED", map[string]any{
			"ns_list": strings.Join(sortedKeys(included), ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver06 runs the NAMESERVER06 test case.
func Nameserver06(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver06"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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
		if err := appendLog(ctx, &results, testcase, "CAN_NOT_BE_RESOLVED", map[string]any{
			"nsname_list": strings.Join(withoutIP, ";"),
		}); err != nil {
			return results, err
		}
	} else if len(withIP) == 0 {
		if err := appendLog(ctx, &results, testcase, "NO_RESOLUTION", map[string]any{
			"names": strings.Join(withoutIP, ","),
		}); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "CAN_BE_RESOLVED", map[string]any{}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver07 runs the NAMESERVER07 test case.
func Nameserver07(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver07"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	if z != nil && z.Name.String() == "." {
		if err := appendLog(ctx, &results, testcase, "UPWARD_REFERRAL_IRRELEVANT", map[string]any{}); err != nil {
			return results, err
		}
		return appendTestCaseEnd(ctx, results, testcase)
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	type upwardOutcome struct {
		key      string
		name     string
		included bool
		hasError bool
	}

	ordered := uniqueServersByKey(nss)
	var outcomes []upwardOutcome
	if len(ordered) > 0 {
		outcomes = make([]upwardOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := upwardOutcome{key: server.String(), name: server.Name.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "NS"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}
				outcome.included = true

				resp, err := server.QueryWithOptions(ctx, ".", "NS", nil)
				if err == nil && resp.Msg != nil {
					if len(resp.GetRecords("NS", "authority")) > 0 {
						if _, err := buf.Add("UPWARD_REFERRAL", map[string]any{"ns": server.String()}); err != nil {
							return err
						}
						outcome.hasError = true
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
	}

	nErrors := 0
	nsnames := map[string]bool{}
	included := map[string]bool{}
	for _, outcome := range outcomes {
		if !outcome.included {
			continue
		}
		included[outcome.key] = true
		nsnames[outcome.name] = true
		if outcome.hasError {
			nErrors++
		}
	}

	if len(included) > 0 && nErrors == 0 {
		keys := make([]string, 0, len(nsnames))
		for name := range nsnames {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		if err := appendLog(ctx, &results, testcase, "NO_UPWARD_REFERRAL", map[string]any{
			"nsname_list": strings.Join(keys, ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver08 runs the NAMESERVER08 test case.
func Nameserver08(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver08"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	original := strings.TrimRight("www."+z.Name.String(), ".")
	randomized := scrambleCaseFunc(original)
	for randomized == original {
		randomized = scrambleCaseFunc(original)
	}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	ordered := uniqueServersByKey(nss)
	if len(ordered) > 0 {
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				resp, err := server.QueryWithOptions(ctx, randomized, "SOA", nil)
				if err == nil && resp.Msg != nil {
					questions := resp.Question()
					if len(questions) > 0 {
						qname := strings.TrimRight(questions[0].Header().Name, ".")
						if qname == randomized {
							if _, err := buf.Add("QNAME_CASE_SENSITIVE", map[string]any{
								"ns":     server.String(),
								"domain": randomized,
							}); err != nil {
								return err
							}
						} else {
							if _, err := buf.Add("QNAME_CASE_INSENSITIVE", map[string]any{
								"ns":     server.String(),
								"domain": randomized,
							}); err != nil {
								return err
							}
						}
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

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver09 runs the NAMESERVER09 test case.
func Nameserver09(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver09"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	original := strings.TrimRight("www."+z.Name.String(), ".")
	recordType := "SOA"
	random1 := scrambleCaseFunc(original)
	for random1 == original {
		random1 = scrambleCaseFunc(original)
	}
	random2 := scrambleCaseFunc(original)
	for random2 == original || random2 == random1 {
		random2 = scrambleCaseFunc(original)
	}

	allResultsMatch := true
	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	type caseOutcome struct {
		mismatch bool
	}

	ordered := uniqueServersByKey(nss)
	var outcomes []caseOutcome
	if len(ordered) > 0 {
		outcomes = make([]caseOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := caseOutcome{}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, recordType); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				p1, _ := server.QueryWithOptions(ctx, random1, recordType, nil)
				p2, _ := server.QueryWithOptions(ctx, random2, recordType, nil)

				answer1 := normalizedAnswer(p1)
				answer2 := ""
				if len(p1.Answer()) > 0 {
					answer2 = normalizedAnswer(p2)
					if answer1 == answer2 {
						if _, err := buf.Add("CASE_QUERY_SAME_ANSWER", map[string]any{
							"ns":     server.String(),
							"type":   recordType,
							"query1": random1,
							"query2": random2,
						}); err != nil {
							return err
						}
					} else {
						outcome.mismatch = true
						if _, err := buf.Add("CASE_QUERY_DIFFERENT_ANSWER", map[string]any{
							"ns":     server.String(),
							"type":   recordType,
							"query1": random1,
							"query2": random2,
						}); err != nil {
							return err
						}
					}
				} else if p1.Msg != nil && p2.Msg != nil {
					if p1.Rcode() == p2.Rcode() {
						if _, err := buf.Add("CASE_QUERY_SAME_RC", map[string]any{
							"ns":     server.String(),
							"type":   recordType,
							"query1": random1,
							"query2": random2,
							"rcode":  p1.Rcode(),
						}); err != nil {
							return err
						}
					} else {
						outcome.mismatch = true
						if _, err := buf.Add("CASE_QUERY_DIFFERENT_RC", map[string]any{
							"ns":     server.String(),
							"type":   recordType,
							"query1": random1,
							"query2": random2,
							"rcode1": p1.Rcode(),
							"rcode2": p2.Rcode(),
						}); err != nil {
							return err
						}
					}
				} else if p1.Msg != nil || p2.Msg != nil {
					outcome.mismatch = true
					if _, err := buf.Add("CASE_QUERY_NO_ANSWER", map[string]any{
						"ns":     server.String(),
						"type":   recordType,
						"domain": firstNonEmpty(random1, p1, random2, p2),
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
	}

	allResultsMatch = true
	for _, outcome := range outcomes {
		if outcome.mismatch {
			allResultsMatch = false
			break
		}
	}

	if allResultsMatch {
		if err := appendLog(ctx, &results, testcase, "CASE_QUERIES_RESULTS_OK", map[string]any{
			"type":   recordType,
			"domain": original,
		}); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "CASE_QUERIES_RESULTS_DIFFER", map[string]any{
			"type":   recordType,
			"domain": original,
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver10 runs the NAMESERVER10 test case.
func Nameserver10(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver10"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var noResponseEDNS1 []string
	unexpectedRcode := map[string][]string{}
	var ednsResponseError []string

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	type n10Outcome struct {
		ip                 string
		noResponseEDNS1    bool
		unexpectedRcode    string
		ednsResponseErrors bool
	}

	var outcomes []n10Outcome
	if len(nss) > 0 {
		outcomes = make([]n10Outcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, server := range nss {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := n10Outcome{ip: server.Address.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
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
							outcome.unexpectedRcode = resp2.Rcode()
						} else if resp2.EdnsVersion() == 0 && len(resp2.Answer()) == 0 {
							// expected: no logs
						} else {
							outcome.ednsResponseErrors = true
						}
					} else {
						outcome.noResponseEDNS1 = true
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
	}

	for _, outcome := range outcomes {
		if outcome.noResponseEDNS1 {
			noResponseEDNS1 = append(noResponseEDNS1, outcome.ip)
		}
		if outcome.unexpectedRcode != "" {
			unexpectedRcode[outcome.unexpectedRcode] = append(unexpectedRcode[outcome.unexpectedRcode], outcome.ip)
		}
		if outcome.ednsResponseErrors {
			ednsResponseError = append(ednsResponseError, outcome.ip)
		}
	}

	if len(noResponseEDNS1) > 0 {
		if err := appendLog(ctx, &results, testcase, "N10_NO_RESPONSE_EDNS1_QUERY", map[string]any{
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
			if err := appendLog(ctx, &results, testcase, "N10_UNEXPECTED_RCODE", map[string]any{
				"rcode":      rcode,
				"ns_ip_list": strings.Join(list, ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	if len(ednsResponseError) > 0 {
		if err := appendLog(ctx, &results, testcase, "N10_EDNS_RESPONSE_ERROR", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(ednsResponseError), ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver11 runs the NAMESERVER11 test case.
func Nameserver11(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver11"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var noResponse []string
	unexpectedRcode := map[string][]string{}
	var noEdns []string
	var unexpectedAnswer []string
	var unsetAA []string
	var unknownOpt []string

	optCode := uint16(137)
	unknownOptData := &dns.ERFC3597{EDNS0Code: optCode, Code: ""}

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	type n11Outcome struct {
		ip              string
		noResponse      bool
		unexpectedRcode string
		noEdns          bool
		unexpectedAns   bool
		unsetAA         bool
		unknownOpt      bool
	}

	var outcomes []n11Outcome
	if len(nss) > 0 {
		outcomes = make([]n11Outcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, server := range nss {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := n11Outcome{ip: server.Address.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				ver0 := uint8(0)
				resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver0}})
				if err != nil || resp.Msg == nil || !resp.HasEdns() || resp.Rcode() != "NOERROR" || !resp.AA() || len(resp.GetRecordsForName("SOA", z.Name, "answer")) == 0 {
					outcomes[i] = outcome
					return nil
				}

				resp, err = server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver0, Data: []dns.EDNS0{unknownOptData}}})
				if err != nil || resp.Msg == nil {
					outcome.noResponse = true
					outcomes[i] = outcome
					return nil
				}

				if resp.Rcode() != "NOERROR" {
					outcome.unexpectedRcode = resp.Rcode()
					outcomes[i] = outcome
					return nil
				}
				if !resp.HasEdns() {
					outcome.noEdns = true
					outcomes[i] = outcome
					return nil
				}
				if len(resp.GetRecordsForName("SOA", z.Name, "answer")) == 0 {
					outcome.unexpectedAns = true
					outcomes[i] = outcome
					return nil
				}
				if !resp.AA() {
					outcome.unsetAA = true
					outcomes[i] = outcome
					return nil
				}
				for _, opt := range resp.EdnsData() {
					if erfc, ok := opt.(*dns.ERFC3597); ok && erfc.EDNS0Code == optCode {
						outcome.unknownOpt = true
						break
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
	}

	for _, outcome := range outcomes {
		if outcome.noResponse {
			noResponse = append(noResponse, outcome.ip)
		}
		if outcome.unexpectedRcode != "" {
			unexpectedRcode[outcome.unexpectedRcode] = append(unexpectedRcode[outcome.unexpectedRcode], outcome.ip)
		}
		if outcome.noEdns {
			noEdns = append(noEdns, outcome.ip)
		}
		if outcome.unexpectedAns {
			unexpectedAnswer = append(unexpectedAnswer, outcome.ip)
		}
		if outcome.unsetAA {
			unsetAA = append(unsetAA, outcome.ip)
		}
		if outcome.unknownOpt {
			unknownOpt = append(unknownOpt, outcome.ip)
		}
	}

	if len(noResponse) > 0 {
		if err := appendLog(ctx, &results, testcase, "N11_NO_RESPONSE", map[string]any{
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
			if err := appendLog(ctx, &results, testcase, "N11_UNEXPECTED_RCODE", map[string]any{
				"rcode":      rcode,
				"ns_ip_list": strings.Join(list, ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	if len(noEdns) > 0 {
		if err := appendLog(ctx, &results, testcase, "N11_NO_EDNS", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(noEdns), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(unexpectedAnswer) > 0 {
		if err := appendLog(ctx, &results, testcase, "N11_UNEXPECTED_ANSWER_SECTION", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(unexpectedAnswer), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(unsetAA) > 0 {
		if err := appendLog(ctx, &results, testcase, "N11_UNSET_AA", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(unsetAA), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(unknownOpt) > 0 {
		if err := appendLog(ctx, &results, testcase, "N11_RETURNS_UNKNOWN_OPTION_CODE", map[string]any{
			"ns_ip_list": strings.Join(sortedStrings(unknownOpt), ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver12 runs the NAMESERVER12 test case.
func Nameserver12(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver12"
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
		for i, server := range nss {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				ver0 := uint8(0)
				zFlag := uint16(3)
				resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver0, Z: &zFlag}})
				if err == nil && resp.Msg != nil {
					if resp.Rcode() == "FORMERR" && resp.EdnsRcode() == 0 {
						if _, err := buf.Add("NO_EDNS_SUPPORT", map[string]any{"ns": server.String()}); err != nil {
							return err
						}
					} else if resp.EdnsZ() != 0 {
						if _, err := buf.Add("Z_FLAGS_NOTCLEAR", map[string]any{"ns": server.String()}); err != nil {
							return err
						}
					} else if resp.Rcode() == "NOERROR" && resp.EdnsRcode() == 0 && resp.EdnsVersion() == 0 && resp.EdnsZ() == 0 && len(resp.GetRecords("SOA", "answer")) > 0 {
						return nil
					} else {
						if _, err := buf.Add("NS_ERROR", map[string]any{"ns": server.String()}); err != nil {
							return err
						}
					}
				} else {
					if _, err := buf.Add("NO_RESPONSE", map[string]any{
						"ns":     server.String(),
						"domain": z.Name.String(),
					}); err != nil {
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

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver13 runs the NAMESERVER13 test case.
func Nameserver13(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver13"
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
		for i, server := range nss {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "DNSKEY"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				ver0 := uint8(0)
				doBit := true
				size := uint16(512)
				useVC := false
				fallback := false
				resp, err := server.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &ns.QueryOptions{
					UseVC:    &useVC,
					Fallback: &fallback,
					EDNSDetails: &transport.EDNSDetails{
						Version: &ver0,
						Do:      &doBit,
						Size:    &size,
					},
				})
				if err == nil && resp.Msg != nil {
					if resp.Rcode() == "FORMERR" && !resp.HasEdns() {
						if _, err := buf.Add("NO_EDNS_SUPPORT", map[string]any{"ns": server.String()}); err != nil {
							return err
						}
					} else if resp.TC() && !resp.HasEdns() {
						if _, err := buf.Add("MISSING_OPT_IN_TRUNCATED", map[string]any{"ns": server.String()}); err != nil {
							return err
						}
					} else if resp.Rcode() == "NOERROR" && resp.EdnsVersion() == 0 {
						return nil
					} else {
						if _, err := buf.Add("NS_ERROR", map[string]any{"ns": server.String()}); err != nil {
							return err
						}
					}
				} else {
					if _, err := buf.Add("NO_RESPONSE", map[string]any{
						"ns":     server.String(),
						"domain": z.Name.String(),
					}); err != nil {
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

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver15 runs the NAMESERVER15 test case.
func Nameserver15(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver15"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
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

	type versionOutcome struct {
		server       string
		txtData      map[string]map[string]bool
		errorQueries map[string]bool
		wrongClass   bool
		noVersion    bool
	}

	var outcomes []versionOutcome
	if len(nss) > 0 {
		outcomes = make([]versionOutcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, server := range nss {
			i, server := i, server
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := versionOutcome{server: server.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA TXT"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if err != nil || resp.Msg == nil {
					outcomes[i] = outcome
					return nil
				}

				revealed := false
				outcome.errorQueries = map[string]bool{}
				outcome.txtData = map[string]map[string]bool{}

				for _, queryName := range []string{"version.bind", "version.server"} {
					class := "CH"
					respTXT, err := server.QueryWithOptions(ctx, queryName, "TXT", &ns.QueryOptions{Class: class})
					if err != nil || respTXT.Msg == nil || respTXT.Rcode() == "SERVFAIL" {
						outcome.errorQueries[queryName] = true
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
							outcome.wrongClass = true
						}
						stringValue := strings.TrimSpace(strings.Join(txt.Txt, ""))
						if stringValue != "" {
							if outcome.txtData[stringValue] == nil {
								outcome.txtData[stringValue] = map[string]bool{}
							}
							outcome.txtData[stringValue][queryName] = true
							revealed = true
						}
					}
				}

				if !revealed {
					outcome.noVersion = true
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
		if outcome.server == "" {
			continue
		}
		if outcome.noVersion {
			sendingVersionQuery[outcome.server] = true
		}
		if outcome.wrongClass {
			wrongRecordClass[outcome.server] = true
		}
		for queryName := range outcome.errorQueries {
			errorOnVersionQuery[queryName] = append(errorOnVersionQuery[queryName], outcome.server)
		}
		for value, queries := range outcome.txtData {
			if txtData[value] == nil {
				txtData[value] = map[string][]string{}
			}
			for queryName := range queries {
				txtData[value][queryName] = append(txtData[value][queryName], outcome.server)
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
				if err := appendLog(ctx, &results, testcase, "N15_SOFTWARE_VERSION", map[string]any{
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
			if err := appendLog(ctx, &results, testcase, "N15_ERROR_ON_VERSION_QUERY", map[string]any{
				"query_name": queryName,
				"ns_list":    strings.Join(list, ";"),
			}); err != nil {
				return results, err
			}
		}
	}

	if len(sendingVersionQuery) > 0 {
		if err := appendLog(ctx, &results, testcase, "N15_NO_VERSION_REVEALED", map[string]any{
			"ns_list": strings.Join(sortedKeys(sendingVersionQuery), ";"),
		}); err != nil {
			return results, err
		}
	}

	if len(wrongRecordClass) > 0 {
		if err := appendLog(ctx, &results, testcase, "N15_WRONG_CLASS", map[string]any{
			"ns_list": strings.Join(sortedKeys(wrongRecordClass), ";"),
		}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
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

func uniqueServersByKey(nss []ns.Nameserver) []ns.Nameserver {
	seen := map[string]bool{}
	unique := make([]ns.Nameserver, 0, len(nss))
	for _, server := range nss {
		key := server.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, server)
	}
	return unique
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

func ipDisabledMessageWithLogger(ctx context.Context, buf *testlogger.Buffer, server ns.Nameserver, rrtypes ...string) (bool, error) {
	if server.Address.Is6() && !profile.FromContext(ctx).Net.IPv6 {
		for _, rrtype := range rrtypes {
			if _, err := buf.Add("IPV6_DISABLED", map[string]any{
				"ns":     server.String(),
				"rrtype": rrtype,
			}); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if server.Address.Is4() && !profile.FromContext(ctx).Net.IPv4 {
		for _, rrtype := range rrtypes {
			if _, err := buf.Add("IPV4_DISABLED", map[string]any{
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
