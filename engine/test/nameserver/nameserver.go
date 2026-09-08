package nameserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logargs"
	"codeberg.org/pawal/gonemaster/engine/logger"
	ns "codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/queryopts"
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
	glueNames = func(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
		return z.GlueNames(ctx)
	}
	apexNSNames = func(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
		return z.ApexNSNames(ctx)
	}
	allNameservers  = nsdiscovery.AllNameservers
	authoritativeNS = func(ctx context.Context, z *zone.Zone) ([]ns.Nameserver, error) {
		items, err := nsdiscovery.ZoneNameservers(ctx, z)
		if err != nil {
			return nil, err
		}
		return nameserversFromNSItems(ctx, z, items), nil
	}
	scrambleCaseFunc = util.ScrambleCase
)

// All runs the Nameserver test cases in order.
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
	if util.ShouldRunTest(ctx, "nameserver16") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver16(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver17") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver17(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "nameserver18") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Nameserver18(ctx, z)
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
		"nameserver16": {
			"N16_HAS_NSID",
			"N16_NO_NSID_REVEALED",
			"N16_NO_RESPONSE",
			"N16_UNEXPECTED_RCODE",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver17": {
			"N17_COOKIE_CLIENT_ONLY",
			"N17_COOKIE_ENFORCED",
			"N17_COOKIE_MALFORMED",
			"N17_COOKIE_ROUNDTRIP_OK",
			"N17_COOKIE_SELF_REJECT",
			"N17_COOKIE_SUPPORTED",
			"N17_NO_COOKIE",
			"N17_NO_RESPONSE",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"nameserver18": {
			"N18_EXTENDED_ERROR_REPORTED",
			"N18_FILTERED_RESPONSE",
			"N18_NO_EXTENDED_ERROR",
			"N18_NO_RESPONSE",
			"N18_RESOLVER_BEHAVIOR_REPORTED",
			"N18_SERVER_ERROR_REPORTED",
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

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type recursorOutcome struct {
		server     ns.Nameserver
		included   bool
		isRecursor bool
		noRecursor bool
	}

	if len(nss) > 0 {
		outcomes := make([]recursorOutcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, server := range nss {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "A"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				outcomes[i].server = server
				outcomes[i].included = true
				responseCount := 0
				nxdomainCount := 0
				isNoRecursor := true
				hasRA := false
				hasRAWithAnswer := false
				allNxdomainAA := true

				for _, name := range nonExistentNames {
					resp, err := server.QueryWithOptions(ctx, name, "A", nil)
					if err != nil || resp.Msg == nil {
						if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(server, map[string]any{
							"domain": name,
						})); err != nil {
							return err
						}
						isNoRecursor = false
						continue
					}

					responseCount++
					// RA alone is advisory (leaked on referrals); a real
					// recursive answer also needs ANSWER records.
					if resp.RA() {
						hasRA = true
						if len(resp.Answer()) > 0 {
							hasRAWithAnswer = true
						}
					}
					if resp.Rcode() == "NXDOMAIN" {
						nxdomainCount++
						if !resp.AA() {
							allNxdomainAA = false
						}
					}
				}

				if hasRAWithAnswer {
					outcomes[i].isRecursor = true
					isNoRecursor = false
				} else if hasRA && responseCount > 0 && nxdomainCount == responseCount && !allNxdomainAA {
					// NXDOMAIN without AA is recursion evidence only when RA=1.
					outcomes[i].isRecursor = true
					isNoRecursor = false
				}
				if isNoRecursor {
					outcomes[i].noRecursor = true
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

		var recursors, nonRecursors []ns.Nameserver
		for _, o := range outcomes {
			if !o.included {
				continue
			}
			if o.isRecursor {
				recursors = append(recursors, o.server)
			}
			if o.noRecursor {
				nonRecursors = append(nonRecursors, o.server)
			}
		}
		if len(recursors) > 0 {
			args := logargs.ServersFromNameservers(recursors)
			if err := appendLog(ctx, &results, testcase, "IS_A_RECURSOR", args); err != nil {
				return results, err
			}
		}
		if len(nonRecursors) > 0 {
			args := logargs.ServersFromNameservers(nonRecursors)
			if err := appendLog(ctx, &results, testcase, "NO_RECURSOR", args); err != nil {
				return results, err
			}
		}
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

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}
	ordered := uniqueServersByKey(nss)

	var outcomes []ednsOutcome
	if len(ordered) > 0 {
		outcomes = make([]ednsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
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
						if _, err := buf.Add("NO_EDNS_SUPPORT", withNameserverArgs(server, nil)); err != nil {
							return err
						}
						outcome.hasError = true
					} else if resp.Rcode() == "NOERROR" && resp.EdnsRcode() == 0 && len(resp.GetRecords("SOA", "answer")) > 0 && resp.EdnsVersion() == 0 {
						outcomes[i] = outcome
						return nil
					} else if resp.Rcode() == "NOERROR" && !resp.HasEdns() {
						if _, err := buf.Add("EDNS_RESPONSE_WITHOUT_EDNS", withNameserverArgs(server, map[string]any{
							"domain": z.Name.String(),
						})); err != nil {
							return err
						}
						outcome.hasError = true
					} else if resp.Rcode() == "NOERROR" && resp.HasEdns() && resp.EdnsVersion() != 0 {
						if _, err := buf.Add("EDNS_VERSION_ERROR", withNameserverArgs(server, map[string]any{
							"domain": z.Name.String(),
						})); err != nil {
							return err
						}
						outcome.hasError = true
					} else {
						if _, err := buf.Add("NS_ERROR", withNameserverArgs(server, nil)); err != nil {
							return err
						}
						outcome.hasError = true
					}
				} else {
					resp2, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
					if err == nil && resp2.Msg != nil {
						if _, err := buf.Add("BREAKS_ON_EDNS", withNameserverArgs(server, map[string]any{
							"domain": z.Name.String(),
						})); err != nil {
							return err
						}
						outcome.hasError = true
					} else {
						if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(server, map[string]any{
							"domain": z.Name.String(),
						})); err != nil {
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
		args := map[string]any{}
		setTypedServersFromNames(args, keys)
		if err := appendLog(ctx, &results, testcase, "EDNS0_SUPPORT", args); err != nil {
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

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type axfrOutcome struct {
		server    ns.Nameserver
		included  bool
		failure   bool
		available bool
	}

	ordered := uniqueServersByKey(nss)
	if len(ordered) > 0 {
		outcomes := make([]axfrOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "AXFR"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				outcomes[i].server = server
				outcomes[i].included = true

				var firstRR dns.RR
				err := server.AXFR(ctx, z.Name.String(), func(rr dns.RR) bool {
					firstRR = rr
					return false
				}, "")
				if err != nil {
					outcomes[i].failure = true
				} else if soa, ok := firstRR.(*dns.SOA); ok && soa != nil {
					outcomes[i].available = true
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

		var failures, available []ns.Nameserver
		for _, o := range outcomes {
			if !o.included {
				continue
			}
			if o.failure {
				failures = append(failures, o.server)
			}
			if o.available {
				available = append(available, o.server)
			}
		}
		if len(failures) > 0 {
			args := logargs.ServersFromNameservers(failures)
			if err := appendLog(ctx, &results, testcase, "AXFR_FAILURE", args); err != nil {
				return results, err
			}
		}
		if len(available) > 0 {
			args := logargs.ServersFromNameservers(available)
			if err := appendLog(ctx, &results, testcase, "AXFR_AVAILABLE", args); err != nil {
				return results, err
			}
		}
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

	nss, err := authoritativeNS(ctx, z)
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
						if _, err := buf.Add("DIFFERENT_SOURCE_IP", withNameserverArgs(server, map[string]any{
							"source": resp.AnswerFrom,
						})); err != nil {
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
		args := map[string]any{}
		setTypedServersFromNames(args, sortedKeys(included))
		if err := appendLog(ctx, &results, testcase, "SAME_SOURCE_IP", args); err != nil {
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

	nss, err := authoritativeNS(ctx, z)
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
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(server, map[string]any{
						"domain": z.Name.String(),
					})); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}
				if resp.Rcode() != "NOERROR" {
					if _, err := buf.Add("A_UNEXPECTED_RCODE", withNameserverArgs(server, map[string]any{
						"rcode": resp.Rcode(),
					})); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				resp, err = server.QueryWithOptions(ctx, z.Name.String(), "AAAA", &ns.QueryOptions{UseVC: &useVC})
				if err != nil || resp.Msg == nil {
					if _, err := buf.Add("AAAA_QUERY_DROPPED", withNameserverArgs(server, nil)); err != nil {
						return err
					}
					outcome.aaaaIssue++
					outcomes[i] = outcome
					return nil
				}
				if resp.Rcode() != "NOERROR" {
					if _, err := buf.Add("AAAA_UNEXPECTED_RCODE", withNameserverArgs(server, map[string]any{
						"rcode": resp.Rcode(),
					})); err != nil {
						return err
					}
					outcome.aaaaIssue++
					outcomes[i] = outcome
					return nil
				}

				for _, rr := range resp.GetRecords("AAAA", "answer") {
					if aaaa, ok := rr.(*dns.AAAA); ok {
						if !aaaa.Addr.IsValid() {
							if _, err := buf.Add("AAAA_BAD_RDATA", withNameserverArgs(server, map[string]any{
								"length": 0,
							})); err != nil {
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
		args := map[string]any{}
		setTypedServersFromNames(args, sortedKeys(included))
		if err := appendLog(ctx, &results, testcase, "AAAA_WELL_PROCESSED", args); err != nil {
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

	glueNames, err := glueNames(ctx, z)
	if err != nil {
		return results, err
	}
	childNames, err := apexNSNames(ctx, z)
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

	nss, err := allNameservers(ctx, z)
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
		args := map[string]any{}
		setTypedServersFromNames(args, withoutIP)
		if err := appendLog(ctx, &results, testcase, "CAN_NOT_BE_RESOLVED", args); err != nil {
			return results, err
		}
	} else if len(withIP) == 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, withoutIP)
		if err := appendLog(ctx, &results, testcase, "NO_RESOLUTION", args); err != nil {
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

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type upwardOutcome struct {
		server   ns.Nameserver
		included bool
		hasError bool
	}

	ordered := uniqueServersByKey(nss)
	var outcomes []upwardOutcome
	if len(ordered) > 0 {
		outcomes = make([]upwardOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "NS"); err != nil {
					return err
				} else if disabled {
					return nil
				}
				outcomes[i].server = server
				outcomes[i].included = true

				resp, err := server.QueryWithOptions(ctx, ".", "NS", nil)
				if err == nil && resp.Msg != nil {
					if len(resp.GetRecords("NS", "authority")) > 0 {
						outcomes[i].hasError = true
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

	var upwardServers, noUpwardServers []ns.Nameserver
	for _, o := range outcomes {
		if !o.included {
			continue
		}
		if o.hasError {
			upwardServers = append(upwardServers, o.server)
		} else {
			noUpwardServers = append(noUpwardServers, o.server)
		}
	}

	if len(upwardServers) > 0 {
		args := logargs.ServersFromNameservers(upwardServers)
		if err := appendLog(ctx, &results, testcase, "UPWARD_REFERRAL", args); err != nil {
			return results, err
		}
	}
	if len(noUpwardServers) > 0 {
		args := logargs.ServersFromNameservers(noUpwardServers)
		if err := appendLog(ctx, &results, testcase, "NO_UPWARD_REFERRAL", args); err != nil {
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

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type qnameOutcome struct {
		server      ns.Nameserver
		included    bool
		sensitive   bool
		insensitive bool
	}

	ordered := uniqueServersByKey(nss)
	if len(ordered) > 0 {
		outcomes := make([]qnameOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, server := range ordered {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				outcomes[i].server = server
				outcomes[i].included = true

				resp, err := server.QueryWithOptions(ctx, randomized, "SOA", nil)
				if err == nil && resp.Msg != nil {
					questions := resp.Question()
					if len(questions) > 0 {
						qname := strings.TrimRight(questions[0].Header().Name, ".")
						if qname == randomized {
							outcomes[i].sensitive = true
						} else {
							outcomes[i].insensitive = true
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

		var sensitiveServers, insensitiveServers []ns.Nameserver
		for _, o := range outcomes {
			if !o.included {
				continue
			}
			if o.sensitive {
				sensitiveServers = append(sensitiveServers, o.server)
			}
			if o.insensitive {
				insensitiveServers = append(insensitiveServers, o.server)
			}
		}
		if len(sensitiveServers) > 0 {
			args := logargs.ServersFromNameservers(sensitiveServers)
			args["domain"] = randomized
			if err := appendLog(ctx, &results, testcase, "QNAME_CASE_SENSITIVE", args); err != nil {
				return results, err
			}
		}
		if len(insensitiveServers) > 0 {
			args := logargs.ServersFromNameservers(insensitiveServers)
			args["domain"] = randomized
			if err := appendLog(ctx, &results, testcase, "QNAME_CASE_INSENSITIVE", args); err != nil {
				return results, err
			}
		}
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
	nss, err := authoritativeNS(ctx, z)
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
						if _, err := buf.Add("CASE_QUERY_SAME_ANSWER", withNameserverArgs(server, map[string]any{
							"query_type": recordType,
							"query1":     random1,
							"query2":     random2,
						})); err != nil {
							return err
						}
					} else {
						outcome.mismatch = true
						if _, err := buf.Add("CASE_QUERY_DIFFERENT_ANSWER", withNameserverArgs(server, map[string]any{
							"query_type": recordType,
							"query1":     random1,
							"query2":     random2,
						})); err != nil {
							return err
						}
					}
				} else if p1.Msg != nil && p2.Msg != nil {
					if p1.Rcode() == p2.Rcode() {
						if _, err := buf.Add("CASE_QUERY_SAME_RC", withNameserverArgs(server, map[string]any{
							"query_type": recordType,
							"query1":     random1,
							"query2":     random2,
							"rcode":      p1.Rcode(),
						})); err != nil {
							return err
						}
					} else {
						outcome.mismatch = true
						if _, err := buf.Add("CASE_QUERY_DIFFERENT_RC", withNameserverArgs(server, map[string]any{
							"query_type": recordType,
							"query1":     random1,
							"query2":     random2,
							"rcode1":     p1.Rcode(),
							"rcode2":     p2.Rcode(),
						})); err != nil {
							return err
						}
					}
				} else if p1.Msg != nil || p2.Msg != nil {
					outcome.mismatch = true
					if _, err := buf.Add("CASE_QUERY_NO_ANSWER", withNameserverArgs(server, map[string]any{
						"query_type": recordType,
						"domain":     firstNonEmpty(random1, p1, random2),
					})); err != nil {
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
			"query_type": recordType,
			"domain":     original,
		}); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "CASE_QUERIES_RESULTS_DIFFER", map[string]any{
			"query_type": recordType,
			"domain":     original,
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

	nss, err := authoritativeNS(ctx, z)
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
		args := map[string]any{}
		setTypedAddressesFromValues(args, noResponseEDNS1)
		if err := appendLog(ctx, &results, testcase, "N10_NO_RESPONSE_EDNS1_QUERY", args); err != nil {
			return results, err
		}
	}

	if len(unexpectedRcode) > 0 {
		keys := slices.Sorted(maps.Keys(unexpectedRcode))
		for _, rcode := range keys {
			args := map[string]any{
				"rcode": rcode,
			}
			setTypedAddressesFromValues(args, unexpectedRcode[rcode])
			if err := appendLog(ctx, &results, testcase, "N10_UNEXPECTED_RCODE", args); err != nil {
				return results, err
			}
		}
	}

	if len(ednsResponseError) > 0 {
		args := map[string]any{}
		setTypedAddressesFromValues(args, ednsResponseError)
		if err := appendLog(ctx, &results, testcase, "N10_EDNS_RESPONSE_ERROR", args); err != nil {
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

	nss, err := authoritativeNS(ctx, z)
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
		args := map[string]any{}
		setTypedAddressesFromValues(args, noResponse)
		if err := appendLog(ctx, &results, testcase, "N11_NO_RESPONSE", args); err != nil {
			return results, err
		}
	}

	if len(unexpectedRcode) > 0 {
		keys := slices.Sorted(maps.Keys(unexpectedRcode))
		for _, rcode := range keys {
			args := map[string]any{
				"rcode": rcode,
			}
			setTypedAddressesFromValues(args, unexpectedRcode[rcode])
			if err := appendLog(ctx, &results, testcase, "N11_UNEXPECTED_RCODE", args); err != nil {
				return results, err
			}
		}
	}

	if len(noEdns) > 0 {
		args := map[string]any{}
		setTypedAddressesFromValues(args, noEdns)
		if err := appendLog(ctx, &results, testcase, "N11_NO_EDNS", args); err != nil {
			return results, err
		}
	}

	if len(unexpectedAnswer) > 0 {
		args := map[string]any{}
		setTypedAddressesFromValues(args, unexpectedAnswer)
		if err := appendLog(ctx, &results, testcase, "N11_UNEXPECTED_ANSWER_SECTION", args); err != nil {
			return results, err
		}
	}

	if len(unsetAA) > 0 {
		args := map[string]any{}
		setTypedAddressesFromValues(args, unsetAA)
		if err := appendLog(ctx, &results, testcase, "N11_UNSET_AA", args); err != nil {
			return results, err
		}
	}

	if len(unknownOpt) > 0 {
		args := map[string]any{}
		setTypedAddressesFromValues(args, unknownOpt)
		if err := appendLog(ctx, &results, testcase, "N11_RETURNS_UNKNOWN_OPTION_CODE", args); err != nil {
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

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	if len(nss) > 0 {
		tasks := make([]runner.Task, len(nss))
		for i, server := range nss {
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
						if _, err := buf.Add("NO_EDNS_SUPPORT", withNameserverArgs(server, nil)); err != nil {
							return err
						}
					} else if resp.EdnsZ() != 0 {
						if _, err := buf.Add("Z_FLAGS_NOTCLEAR", withNameserverArgs(server, nil)); err != nil {
							return err
						}
					} else if resp.Rcode() == "NOERROR" && resp.EdnsRcode() == 0 && resp.EdnsVersion() == 0 && resp.EdnsZ() == 0 && len(resp.GetRecords("SOA", "answer")) > 0 {
						return nil
					} else {
						if _, err := buf.Add("NS_ERROR", withNameserverArgs(server, nil)); err != nil {
							return err
						}
					}
				} else {
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(server, map[string]any{
						"domain": z.Name.String(),
					})); err != nil {
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

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	if len(nss) > 0 {
		tasks := make([]runner.Task, len(nss))
		for i, server := range nss {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "DNSKEY"); err != nil {
					return err
				} else if disabled {
					return nil
				}

				resp, err := server.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", queryopts.SmallAnswerDNSKEY())
				if err == nil && resp.Msg != nil {
					if resp.Rcode() == "FORMERR" && !resp.HasEdns() {
						if _, err := buf.Add("NO_EDNS_SUPPORT", withNameserverArgs(server, nil)); err != nil {
							return err
						}
					} else if resp.TC() && !resp.HasEdns() {
						if _, err := buf.Add("MISSING_OPT_IN_TRUNCATED", withNameserverArgs(server, nil)); err != nil {
							return err
						}
					} else if resp.Rcode() == "NOERROR" && resp.EdnsVersion() == 0 {
						return nil
					} else {
						if _, err := buf.Add("NS_ERROR", withNameserverArgs(server, nil)); err != nil {
							return err
						}
					}
				} else {
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(server, map[string]any{
						"domain": z.Name.String(),
					})); err != nil {
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

	nss, err := authoritativeNS(ctx, z)
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
						stringValue := strings.TrimSpace(escapeUnprintable(strings.Join(txt.Txt, "")))
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
		stringsList := slices.Sorted(maps.Keys(txtData))
		for _, value := range stringsList {
			queries := txtData[value]
			queryNames := slices.Sorted(maps.Keys(queries))
			for _, queryName := range queryNames {
				list := sortedStrings(queries[queryName])
				args := map[string]any{
					"string":     value,
					"query_name": queryName,
				}
				setTypedServersFromNames(args, list)
				if err := appendLog(ctx, &results, testcase, "N15_SOFTWARE_VERSION", args); err != nil {
					return results, err
				}
			}
		}
	}

	if len(errorOnVersionQuery) > 0 {
		queryNames := slices.Sorted(maps.Keys(errorOnVersionQuery))
		for _, queryName := range queryNames {
			list := sortedStrings(errorOnVersionQuery[queryName])
			args := map[string]any{
				"query_name": queryName,
			}
			setTypedServersFromNames(args, list)
			if err := appendLog(ctx, &results, testcase, "N15_ERROR_ON_VERSION_QUERY", args); err != nil {
				return results, err
			}
		}
	}

	if len(sendingVersionQuery) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedKeys(sendingVersionQuery))
		if err := appendLog(ctx, &results, testcase, "N15_NO_VERSION_REVEALED", args); err != nil {
			return results, err
		}
	}

	if len(wrongRecordClass) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedKeys(wrongRecordClass))
		if err := appendLog(ctx, &results, testcase, "N15_WRONG_CLASS", args); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Nameserver16 runs the NAMESERVER16 test case.
func Nameserver16(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver16"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nsidData := map[string][]string{}
	var noNSID []string
	var noResponse []string
	unexpectedRcode := map[string][]string{}

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type n16Outcome struct {
		server          string
		nsidValue       string
		hasNSID         bool
		noNSID          bool
		noResponse      bool
		unexpectedRcode string
	}

	ver0 := uint8(0)
	nsidOpt := &dns.NSID{}

	var outcomes []n16Outcome
	if len(nss) > 0 {
		outcomes = make([]n16Outcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, server := range nss {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := n16Outcome{server: server.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				resp, err := server.QueryWithOptions(ctx, z.Name.String(), "SOA", &ns.QueryOptions{
					EDNSDetails: &transport.EDNSDetails{
						Version: &ver0,
						Data:    []dns.EDNS0{nsidOpt},
					},
				})
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

				for _, opt := range resp.EdnsData() {
					if nsid, ok := opt.(*dns.NSID); ok {
						if decoded, err := hex.DecodeString(nsid.Nsid); err == nil {
							if value, ok := nsidValue(decoded); ok {
								outcome.hasNSID = true
								outcome.nsidValue = value
							}
						}
						break
					}
				}
				if !outcome.hasNSID {
					outcome.noNSID = true
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
		if outcome.hasNSID {
			nsidData[outcome.nsidValue] = append(nsidData[outcome.nsidValue], outcome.server)
		}
		if outcome.noNSID {
			noNSID = append(noNSID, outcome.server)
		}
		if outcome.noResponse {
			noResponse = append(noResponse, outcome.server)
		}
		if outcome.unexpectedRcode != "" {
			unexpectedRcode[outcome.unexpectedRcode] = append(unexpectedRcode[outcome.unexpectedRcode], outcome.server)
		}
	}

	if len(nsidData) > 0 {
		valueList := slices.Sorted(maps.Keys(nsidData))
		for _, value := range valueList {
			list := sortedStrings(nsidData[value])
			args := map[string]any{
				"nsid": value,
			}
			setTypedServersFromNames(args, list)
			if err := appendLog(ctx, &results, testcase, "N16_HAS_NSID", args); err != nil {
				return results, err
			}
		}
	}

	if len(noNSID) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(noNSID))
		if err := appendLog(ctx, &results, testcase, "N16_NO_NSID_REVEALED", args); err != nil {
			return results, err
		}
	}

	if len(noResponse) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(noResponse))
		if err := appendLog(ctx, &results, testcase, "N16_NO_RESPONSE", args); err != nil {
			return results, err
		}
	}

	if len(unexpectedRcode) > 0 {
		keys := slices.Sorted(maps.Keys(unexpectedRcode))
		for _, rcode := range keys {
			list := sortedStrings(unexpectedRcode[rcode])
			args := map[string]any{
				"rcode": rcode,
			}
			setTypedServersFromNames(args, list)
			if err := appendLog(ctx, &results, testcase, "N16_UNEXPECTED_RCODE", args); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// nsidValue renders opaque NSID bytes for display, false when empty.
func nsidValue(raw []byte) (string, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return "", false
	}
	if isPrintableUTF8(s) {
		return s, true
	}
	return nsidHexASCII(raw), true
}

// isPrintableUTF8 reports valid UTF-8 with only printable runes.
func isPrintableUTF8(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// nsidHexASCII renders bytes in dig's form: hex bytes then a quoted ASCII view.
func nsidHexASCII(raw []byte) string {
	h := hex.EncodeToString(raw)
	var b strings.Builder
	b.Grow(len(h) + len(raw) + 4)
	for i := range raw {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(h[i*2 : i*2+2])
	}
	b.WriteString(` ("`)
	for _, c := range raw {
		if c >= 0x20 && c <= 0x7e {
			b.WriteByte(c)
		} else {
			b.WriteByte('.')
		}
	}
	b.WriteString(`")`)
	return b.String()
}

// Nameserver17 runs the NAMESERVER17 test case (DNS Cookie, RFC 7873 / RFC 9018).
func Nameserver17(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver17"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	clientCookieHex, err := newClientCookie()
	if err != nil {
		return results, err
	}

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type n17Outcome struct {
		server         string
		supported      bool
		enforced       bool
		noCookie       bool
		clientOnly     bool
		malformed      bool
		malformedBytes int
		roundtripOK    bool
		selfReject     bool
		noResponse     bool
	}

	var outcomes []n17Outcome
	if len(nss) > 0 {
		outcomes = make([]n17Outcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, server := range nss {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := n17Outcome{server: server.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				resp, err := cookieQuery(ctx, server, z.Name.String(), clientCookieHex)
				if err != nil || resp.Msg == nil || resp.TC() {
					// Truncated/no response: inconclusive, no TCP fallback.
					outcome.noResponse = true
					outcomes[i] = outcome
					return nil
				}
				if resp.Rcode() != "NOERROR" {
					// An enforcing server answers a client-only cookie with
					// BADCOOKIE plus a fresh, well-formed Server Cookie. Only
					// that case counts; other anomalies are graded by basic/N16.
					if resp.Msg.Rcode == dns.RcodeBadCookie {
						if tag, fullCookieHex, _ := classifyCookie(resp, clientCookieHex); tag == "N17_COOKIE_SUPPORTED" {
							outcome.enforced = true
							if cookieRoundTripOK(ctx, server, z.Name.String(), fullCookieHex) {
								outcome.roundtripOK = true
							} else {
								outcome.selfReject = true
							}
						}
					}
					outcomes[i] = outcome
					return nil
				}

				tag, fullCookieHex, cookieBytes := classifyCookie(resp, clientCookieHex)
				switch tag {
				case "N17_NO_COOKIE":
					outcome.noCookie = true
				case "N17_COOKIE_CLIENT_ONLY":
					outcome.clientOnly = true
				case "N17_COOKIE_MALFORMED":
					outcome.malformed = true
					outcome.malformedBytes = cookieBytes
				case "N17_COOKIE_SUPPORTED":
					outcome.supported = true
					if cookieRoundTripOK(ctx, server, z.Name.String(), fullCookieHex) {
						outcome.roundtripOK = true
					} else {
						outcome.selfReject = true
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

	var supported, enforced, noCookie, clientOnly, roundtripOK, selfReject, noResponse []string
	malformed := map[int][]string{}
	for _, outcome := range outcomes {
		if outcome.server == "" {
			continue
		}
		switch {
		case outcome.supported:
			supported = append(supported, outcome.server)
		case outcome.enforced:
			enforced = append(enforced, outcome.server)
		case outcome.noCookie:
			noCookie = append(noCookie, outcome.server)
		case outcome.clientOnly:
			clientOnly = append(clientOnly, outcome.server)
		case outcome.malformed:
			malformed[outcome.malformedBytes] = append(malformed[outcome.malformedBytes], outcome.server)
		case outcome.noResponse:
			noResponse = append(noResponse, outcome.server)
		}
		if outcome.roundtripOK {
			roundtripOK = append(roundtripOK, outcome.server)
		}
		if outcome.selfReject {
			selfReject = append(selfReject, outcome.server)
		}
	}

	if len(supported) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(supported))
		if err := appendLog(ctx, &results, testcase, "N17_COOKIE_SUPPORTED", args); err != nil {
			return results, err
		}
	}
	if len(enforced) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(enforced))
		if err := appendLog(ctx, &results, testcase, "N17_COOKIE_ENFORCED", args); err != nil {
			return results, err
		}
	}
	if len(noCookie) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(noCookie))
		if err := appendLog(ctx, &results, testcase, "N17_NO_COOKIE", args); err != nil {
			return results, err
		}
	}
	if len(roundtripOK) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(roundtripOK))
		if err := appendLog(ctx, &results, testcase, "N17_COOKIE_ROUNDTRIP_OK", args); err != nil {
			return results, err
		}
	}
	if len(clientOnly) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(clientOnly))
		if err := appendLog(ctx, &results, testcase, "N17_COOKIE_CLIENT_ONLY", args); err != nil {
			return results, err
		}
	}
	if len(malformed) > 0 {
		sizes := slices.Sorted(maps.Keys(malformed))
		for _, size := range sizes {
			args := map[string]any{"cookie_bytes": size}
			setTypedServersFromNames(args, sortedStrings(malformed[size]))
			if err := appendLog(ctx, &results, testcase, "N17_COOKIE_MALFORMED", args); err != nil {
				return results, err
			}
		}
	}
	if len(selfReject) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(selfReject))
		if err := appendLog(ctx, &results, testcase, "N17_COOKIE_SELF_REJECT", args); err != nil {
			return results, err
		}
	}
	if len(noResponse) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(noResponse))
		if err := appendLog(ctx, &results, testcase, "N17_NO_RESPONSE", args); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// newClientCookie returns a random 8-byte Client Cookie (RFC 7873) as hex.
func newClientCookie() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// cookieQuery sends a UDP-pinned SOA query carrying one COOKIE option.
func cookieQuery(ctx context.Context, server ns.Nameserver, qname string, cookieHex string) (packet.Packet, error) {
	useVC := false
	return server.QueryWithOptions(ctx, qname, "SOA", &ns.QueryOptions{
		UseVC: &useVC,
		EDNSDetails: &transport.EDNSDetails{
			Data: []dns.EDNS0{&dns.COOKIE{Cookie: cookieHex}},
		},
	})
}

// validCookieLen reports a well-formed COOKIE length: 8, or 16-40 (RFC 7873 5.2.2).
func validCookieLen(n int) bool {
	return n == 8 || (n >= 16 && n <= 40)
}

// classifyCookie returns the query-1 tag, the full cookie hex (only when
// supported), and the observed COOKIE option length in bytes.
func classifyCookie(resp packet.Packet, clientCookieHex string) (tag string, fullCookieHex string, cookieBytes int) {
	cookie := resp.Cookie()
	if cookie == nil {
		return "N17_NO_COOKIE", "", 0
	}
	cookieHex := strings.ToLower(cookie.Cookie)
	cookieBytes = len(cookieHex) / 2
	if !validCookieLen(cookieBytes) {
		return "N17_COOKIE_MALFORMED", "", cookieBytes
	}
	// Guard the slice and require the client portion to echo ours.
	if len(cookieHex) < 16 || cookieHex[:16] != strings.ToLower(clientCookieHex) {
		return "N17_COOKIE_MALFORMED", "", cookieBytes
	}
	if cookieBytes == 8 {
		return "N17_COOKIE_CLIENT_ONLY", "", cookieBytes
	}
	return "N17_COOKIE_SUPPORTED", cookieHex, cookieBytes
}

// cookieRoundTripOK re-queries with the issued cookie. A BADCOOKIE reply is
// retried once with the fresh Server Cookie it carries (RFC 7873 5.3); the server
// self-rejects only when both attempts return BADCOOKIE.
func cookieRoundTripOK(ctx context.Context, server ns.Nameserver, qname string, fullCookieHex string) bool {
	resp, err := cookieQuery(ctx, server, qname, fullCookieHex)
	if err != nil || resp.Msg == nil {
		return true
	}
	if resp.Msg.Rcode != dns.RcodeBadCookie {
		return true
	}
	retryCookieHex := fullCookieHex
	if c := resp.Cookie(); c != nil && len(c.Cookie) >= 16 {
		retryCookieHex = strings.ToLower(c.Cookie)
	}
	retry, err := cookieQuery(ctx, server, qname, retryCookieHex)
	if err != nil || retry.Msg == nil {
		return true
	}
	return retry.Msg.Rcode != dns.RcodeBadCookie
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

func firstNonEmpty(query1 string, resp1 packet.Packet, query2 string) string {
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

// Nameserver18 runs the NAMESERVER18 test case (Extended DNS Errors, RFC 8914).
func Nameserver18(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Nameserver18"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	nss, err := authoritativeNS(ctx, z)
	if err != nil {
		return results, err
	}

	type edeObserved struct {
		code uint16
		text string
	}
	type n18Outcome struct {
		server     string
		noResponse bool
		clean      bool // NOERROR carrying no EDE
		edes       []edeObserved
	}

	var outcomes []n18Outcome
	if len(nss) > 0 {
		outcomes = make([]n18Outcome, len(nss))
		tasks := make([]runner.Task, len(nss))
		for i, server := range nss {
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := n18Outcome{server: server.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, server, "SOA"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				// DO=0 apex SOA: deduplicates against existing SOA queries (no extra traffic).
				resp, err := server.Query(ctx, z.Name.String(), "SOA")
				if err != nil || resp.Msg == nil {
					outcome.noResponse = true
					outcomes[i] = outcome
					return nil
				}

				edes := resp.ExtendedErrors()
				if len(edes) == 0 {
					// Only NOERROR-with-no-EDE is "clean"; a non-NOERROR without EDE is
					// left to basic/N16 (3.4).
					outcome.clean = resp.Rcode() == "NOERROR"
					outcomes[i] = outcome
					return nil
				}
				for _, ede := range edes {
					outcome.edes = append(outcome.edes, edeObserved{code: ede.InfoCode, text: sanitizeExtraText(ede.ExtraText)})
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

	type edeKey struct {
		code uint16
		text string
	}
	var noResponse, clean []string
	edeServers := map[edeKey][]string{}
	for _, outcome := range outcomes {
		if outcome.server == "" {
			continue
		}
		if outcome.noResponse {
			noResponse = append(noResponse, outcome.server)
			continue
		}
		if outcome.clean {
			clean = append(clean, outcome.server)
		}
		for _, e := range outcome.edes {
			key := edeKey{code: e.code, text: e.text}
			edeServers[key] = append(edeServers[key], outcome.server)
		}
	}

	keys := make([]edeKey, 0, len(edeServers))
	for key := range edeServers {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].code != keys[b].code {
			return keys[a].code < keys[b].code
		}
		return keys[a].text < keys[b].text
	})
	for _, key := range keys {
		args := map[string]any{
			"info_code":  int(key.code),
			"info_name":  edeName(key.code),
			"extra_text": key.text,
		}
		setTypedServersFromNames(args, sortedStrings(edeServers[key]))
		if err := appendLog(ctx, &results, testcase, edeTagForCode(key.code), args); err != nil {
			return results, err
		}
	}

	if len(clean) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(clean))
		if err := appendLog(ctx, &results, testcase, "N18_NO_EXTENDED_ERROR", args); err != nil {
			return results, err
		}
	}

	if len(noResponse) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, sortedStrings(noResponse))
		if err := appendLog(ctx, &results, testcase, "N18_NO_RESPONSE", args); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// edeTagForCode classifies an EDE info-code (RFC 8914) by what its presence implies
// about a directly-queried authoritative server.
func edeTagForCode(code uint16) string {
	switch code {
	case dns.ExtendedErrorProhibited, // 18
		dns.ExtendedErrorNotAuthoritative, // 20
		dns.ExtendedErrorNotSupported:     // 21
		return "N18_SERVER_ERROR_REPORTED"
	case dns.ExtendedErrorForgedAnswer, // 4
		dns.ExtendedErrorBlocked,  // 15
		dns.ExtendedErrorCensored, // 16
		dns.ExtendedErrorFiltered: // 17
		return "N18_FILTERED_RESPONSE"
	case dns.ExtendedErrorUnsupportedDNSKEYAlgorithm, // 1
		dns.ExtendedErrorUnsupportedDSDigestType,     // 2
		dns.ExtendedErrorStaleAnswer,                 // 3
		dns.ExtendedErrorDNSSECIndeterminate,         // 5
		dns.ExtendedErrorDNSBogus,                    // 6
		dns.ExtendedErrorSignatureExpired,            // 7
		dns.ExtendedErrorSignatureNotYetValid,        // 8
		dns.ExtendedErrorDNSKEYMissing,               // 9
		dns.ExtendedErrorRRSIGsMissing,               // 10
		dns.ExtendedErrorNoZoneKeyBitSet,             // 11
		dns.ExtendedErrorNSECMissing,                 // 12
		dns.ExtendedErrorCachedError,                 // 13
		dns.ExtendedErrorStaleNXDOMAINAnswer,         // 19
		dns.ExtendedErrorNoReachableAuthority,        // 22
		dns.ExtendedErrorNetworkError,                // 23
		dns.ExtendedErrorSignatureExpiredBeforeValid, // 25
		dns.ExtendedErrorUnsupportedNSEC3IterValue,   // 27
		dns.ExtendedErrorSynthesized,                 // 29
		dns.ExtendedErrorNegativeTrustAnchor:         // 33 (RFC 7646)
		return "N18_RESOLVER_BEHAVIOR_REPORTED"
	default:
		// 0, 14, 24, 26, 28, 30, 31, 32, unassigned (>=34), private-use: benign annotation.
		return "N18_EXTENDED_ERROR_REPORTED"
	}
}

// edeName renders the EDE info-code name, falling back to "code N" for codes the
// library registry does not name (draft-assigned and private-use codes).
func edeName(code uint16) string {
	if s, ok := dns.ExtendedErrorToString[code]; ok {
		return s
	}
	return fmt.Sprintf("code %d", code)
}

// sanitizeExtraText makes free-form EDE EXTRA-TEXT safe for storage/UI: valid UTF-8,
// no trailing NUL, trimmed, capped at 256 bytes with a truncation marker.
func sanitizeExtraText(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.TrimSuffix(s, "\x00") // RFC 8914: EXTRA-TEXT MAY be NUL-terminated
	s = strings.TrimSpace(s)
	const maxLen = 256
	if len(s) > maxLen {
		s = strings.ToValidUTF8(s[:maxLen], "") + "..."
	}
	return s
}

// escapeUnprintable keeps printable ASCII, escapes backslash and every other byte as "\NNN".
func escapeUnprintable(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c >= 0x20 && c <= 0x7e:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, `\%03d`, c)
		}
	}
	return b.String()
}

func sortedStrings(values []string) []string {
	return uniqueSortedValues(values)
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

func setTypedAddressesFromValues(args map[string]any, values []string) {
	if args == nil || len(values) == 0 {
		return
	}
	addresses := uniqueSortedValues(values)
	if len(addresses) == 0 {
		return
	}
	args["addresses"] = append([]string(nil), addresses...)
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

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	return uniqueSortedValues(keys)
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

func withNameserverArgs(server ns.Nameserver, args map[string]any) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	logargs.NormalizeQueryIdentity(args)
	logargs.SetNS(args, server.NameString(), server.AddressString())
	return args
}

func ipDisabledMessageWithLogger(ctx context.Context, buf *testlogger.Buffer, server ns.Nameserver, rrtypes ...string) (bool, error) {
	if server.Address.Is6() && !profile.FromContext(ctx).Net.IPv6 {
		for _, rrtype := range rrtypes {
			if _, err := buf.Add("IPV6_DISABLED", withNameserverArgs(server, map[string]any{
				"query_type": rrtype,
			})); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if server.Address.Is4() && !profile.FromContext(ctx).Net.IPv4 {
		for _, rrtype := range rrtypes {
			if _, err := buf.Add("IPV4_DISABLED", withNameserverArgs(server, map[string]any{
				"query_type": rrtype,
			})); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}

func nameserversFromNSItems(ctx context.Context, z *zone.Zone, items []nsdiscovery.NSItem) []ns.Nameserver {
	if z == nil || z.Recursor() == nil {
		return nil
	}

	seen := map[string]ns.Nameserver{}
	for _, item := range items {
		if !item.HasAddress {
			continue
		}
		server, err := ns.NewWithContext(ctx, item.Name.String(), item.Address.String(), z.Recursor().Client())
		if err != nil {
			continue
		}
		seen[strings.ToLower(server.String())] = server
	}

	keys := slices.Sorted(maps.Keys(seen))

	out := make([]ns.Nameserver, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}
