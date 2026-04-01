package syntax

import (
	"context"
	"fmt"
	"net/mail"
	"net/netip"
	"sort"
	"strings"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/parallel"
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

const moduleName = "Syntax"

// All runs the Syntax test cases in order, mirroring the Perl implementation.
func All(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	onlyAllowedChars := true
	if util.ShouldRunTest(ctx, "syntax01") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Syntax01(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
		onlyAllowedChars = hasTag(results, "ONLY_ALLOWED_CHARS")
	}

	if util.ShouldRunTest(ctx, "syntax02") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Syntax02(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if util.ShouldRunTest(ctx, "syntax03") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Syntax03(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if !onlyAllowedChars {
		return results, nil
	}

	if util.ShouldRunTest(ctx, "syntax04") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Syntax04(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	allSOAResponses := true
	if util.ShouldRunTest(ctx, "syntax05") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Syntax05(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
		allSOAResponses = !hasTag(results, "NO_RESPONSE_SOA_QUERY")
	}

	if allSOAResponses {
		if util.ShouldRunTest(ctx, "syntax06") {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Syntax06(ctx, z)
			})
			results = append(results, entries...)
			if err != nil {
				return results, err
			}
		}

		if util.ShouldRunTest(ctx, "syntax07") {
			entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
				return Syntax07(ctx, z)
			})
			results = append(results, entries...)
			if err != nil {
				return results, err
			}
		}
	}

	if util.ShouldRunTest(ctx, "syntax08") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return Syntax08(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// Metadata returns tags emitted by Syntax test cases.
func Metadata() map[string][]string {
	return map[string][]string{
		"syntax01": {
			"ONLY_ALLOWED_CHARS",
			"NON_ALLOWED_CHARS",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"syntax02": {
			"INITIAL_HYPHEN",
			"TERMINAL_HYPHEN",
			"NO_ENDING_HYPHENS",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"syntax03": {
			"DISCOURAGED_DOUBLE_DASH",
			"NO_DOUBLE_DASH",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"syntax04": {
			"NAMESERVER_DISCOURAGED_DOUBLE_DASH",
			"NAMESERVER_NON_ALLOWED_CHARS",
			"NAMESERVER_NUMERIC_TLD",
			"NAMESERVER_SYNTAX_OK",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"syntax05": {
			"RNAME_MISUSED_AT_SIGN",
			"RNAME_NO_AT_SIGN",
			"NO_RESPONSE_SOA_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"syntax06": {
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"NO_RESPONSE",
			"NO_RESPONSE_SOA_QUERY",
			"RNAME_MAIL_DOMAIN_INVALID",
			"RNAME_MAIL_DOMAIN_LOCALHOST",
			"RNAME_MAIL_ILLEGAL_CNAME",
			"RNAME_RFC822_INVALID",
			"RNAME_RFC822_VALID",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"syntax07": {
			"MNAME_DISCOURAGED_DOUBLE_DASH",
			"MNAME_NON_ALLOWED_CHARS",
			"MNAME_NUMERIC_TLD",
			"MNAME_SYNTAX_OK",
			"NO_RESPONSE_SOA_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"syntax08": {
			"MX_DISCOURAGED_DOUBLE_DASH",
			"MX_NON_ALLOWED_CHARS",
			"MX_NUMERIC_TLD",
			"MX_SYNTAX_OK",
			"NO_RESPONSE_MX_QUERY",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// Syntax01 runs the SYNTAX01 test case.
func Syntax01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Syntax01"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	name := z.Name
	if nameHasOnlyLegalCharacters(name) {
		if err := appendLog(ctx, &results, testcase, "ONLY_ALLOWED_CHARS", map[string]any{"domain": name.String()}); err != nil {
			return results, err
		}
	} else {
		if err := appendLog(ctx, &results, testcase, "NON_ALLOWED_CHARS", map[string]any{"domain": name.String()}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Syntax02 runs the SYNTAX02 test case.
func Syntax02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Syntax02"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	name := z.Name
	hadIssue := false
	for _, label := range name.Labels() {
		if labelStartsWithHyphen(label) {
			if err := appendLog(ctx, &results, testcase, "INITIAL_HYPHEN", map[string]any{
				"label":  label,
				"domain": name.String(),
			}); err != nil {
				return results, err
			}
			hadIssue = true
		}
		if labelEndsWithHyphen(label) {
			if err := appendLog(ctx, &results, testcase, "TERMINAL_HYPHEN", map[string]any{
				"label":  label,
				"domain": name.String(),
			}); err != nil {
				return results, err
			}
			hadIssue = true
		}
	}

	if len(name.Labels()) > 0 && !hadIssue {
		if err := appendLog(ctx, &results, testcase, "NO_ENDING_HYPHENS", map[string]any{"domain": name.String()}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Syntax03 runs the SYNTAX03 test case.
func Syntax03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Syntax03"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	name := z.Name
	hadIssue := false
	for _, label := range name.Labels() {
		if labelNotACEHasDoubleHyphen(label) {
			if err := appendLog(ctx, &results, testcase, "DISCOURAGED_DOUBLE_DASH", map[string]any{
				"label":  label,
				"domain": name.String(),
			}); err != nil {
				return results, err
			}
			hadIssue = true
		}
	}

	if len(name.Labels()) > 0 && !hadIssue {
		if err := appendLog(ctx, &results, testcase, "NO_DOUBLE_DASH", map[string]any{"domain": name.String()}); err != nil {
			return results, err
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Syntax04 runs the SYNTAX04 test case.
func Syntax04(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Syntax04"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	glueNames, err := methods.Method2(ctx, z)
	if err != nil {
		return results, err
	}
	nsNames, err := methods.Method3(ctx, z)
	if err != nil {
		return results, err
	}

	seen := map[string]dnsname.Name{}
	for _, name := range glueNames {
		seen[strings.ToLower(name.String())] = name
	}
	for _, name := range nsNames {
		seen[strings.ToLower(name.String())] = name
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	if len(keys) > 0 {
		tasks := make([]runner.Task, len(keys))
		for i, key := range keys {
			name := seen[key]
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				return checkNameSyntaxWithLogger(buf, "NAMESERVER", name)
			}
		}
		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		entries, err := runner.Run(ctx, tasks, runner.Options{Parallel: parallelism, CancelOnError: false})
		if err != nil {
			return results, err
		}

		// Consolidate NAMESERVER_SYNTAX_OK entries into a single message with servers list.
		var okNames []string
		for _, entry := range entries {
			if entry != nil && entry.Tag == "NAMESERVER_SYNTAX_OK" {
				if domain, ok := entry.Args["domain"].(string); ok {
					okNames = append(okNames, domain)
				}
				continue
			}
			results = append(results, entry)
		}
		if len(okNames) > 0 {
			sort.Strings(okNames)
			args := logargs.ServersFromValues(okNames)
			if err := appendLog(ctx, &results, testcase, "NAMESERVER_SYNTAX_OK", args); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Syntax05 runs the SYNTAX05 test case.
func Syntax05(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Syntax05"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := z.QueryOne(ctx, z.Name.String(), "SOA", nil)
	if err != nil {
		return results, err
	}

	if resp.Msg != nil {
		for _, rr := range resp.GetRecords("SOA", "answer") {
			soa, ok := rr.(*dns.SOA)
			if !ok {
				continue
			}
			rname := soa.Mbox
			check := strings.ReplaceAll(rname, `\.`, ".")
			if strings.Contains(check, "@") {
				if err := appendLog(ctx, &results, testcase, "RNAME_MISUSED_AT_SIGN", map[string]any{"rname": rname}); err != nil {
					return results, err
				}
			} else {
				if err := appendLog(ctx, &results, testcase, "RNAME_NO_AT_SIGN", map[string]any{"rname": rname}); err != nil {
					return results, err
				}
			}
			return appendTestCaseEnd(ctx, results, testcase)
		}
	}

	if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
		return results, err
	}
	return appendTestCaseEnd(ctx, results, testcase)
}

// Syntax06 runs the SYNTAX06 test case.
func Syntax06(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Syntax06"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	rec := z.Recursor()
	if rec == nil {
		return results, fmt.Errorf("missing recursor")
	}

	glueNS, err := methods.Method4(ctx, z)
	if err != nil {
		return results, err
	}
	authNS, err := methods.Method5(ctx, z)
	if err != nil {
		return results, err
	}

	uniqueNS := map[string]nameserver.Nameserver{}
	for _, ns := range glueNS {
		uniqueNS[ns.String()] = ns
	}
	for _, ns := range authNS {
		uniqueNS[ns.String()] = ns
	}

	keys := make([]string, 0, len(uniqueNS))
	for key := range uniqueNS {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var nss []nameserver.Nameserver
	for _, key := range keys {
		nss = append(nss, uniqueNS[key])
	}

	rnameCandidates := map[string]bool{}
	seenMailServers := map[string]bool{}
	invalidExchanges := -1
	parentLog := util.LoggerFromContext(ctx)

	type mailOutcome struct {
		entries       []*logger.Entry
		exchangeValid bool
	}

	processMailServer := func(ctx context.Context, mailServer string) (mailOutcome, error) {
		buf := logger.New()
		buf.CopyConfigFrom(parentLog)
		buf.CopyStartTimeFrom(parentLog)
		tlog := testlogger.Wrap(buf, moduleName, testcase)
		exchangeValid := false

		pA, err := rec.Recurse(ctx, mailServer, "A", "IN")
		if err != nil {
			return mailOutcome{entries: buf.Entries()}, err
		}
		if pA.Msg != nil {
			if len(pA.GetRecords("CNAME", "answer")) > 0 {
				if _, err := tlog.Add("RNAME_MAIL_ILLEGAL_CNAME", map[string]any{"domain": mailServer}); err != nil {
					return mailOutcome{entries: buf.Entries()}, err
				}
			} else {
				records := matchingARecords(pA, mailServer)
				if hasIPv4Loopback(records) {
					if _, err := tlog.Add("RNAME_MAIL_DOMAIN_LOCALHOST", map[string]any{
						"domain":    mailServer,
						"localhost": "127.0.0.1",
					}); err != nil {
						return mailOutcome{entries: buf.Entries()}, err
					}
				} else if len(records) > 0 {
					exchangeValid = true
				}
			}
		}

		pAAAA, err := rec.Recurse(ctx, mailServer, "AAAA", "IN")
		if err != nil {
			return mailOutcome{entries: buf.Entries()}, err
		}
		if pAAAA.Msg != nil {
			if len(pAAAA.GetRecords("CNAME", "answer")) > 0 {
				if _, err := tlog.Add("RNAME_MAIL_ILLEGAL_CNAME", map[string]any{"domain": mailServer}); err != nil {
					return mailOutcome{entries: buf.Entries()}, err
				}
			} else {
				records := matchingAAAARecords(pAAAA, mailServer)
				if hasIPv6Loopback(records) {
					if _, err := tlog.Add("RNAME_MAIL_DOMAIN_LOCALHOST", map[string]any{
						"domain":    mailServer,
						"localhost": "::1",
					}); err != nil {
						return mailOutcome{entries: buf.Entries()}, err
					}
				} else if len(records) > 0 {
					exchangeValid = true
				}
			}
		}

		if !exchangeValid {
			if _, err := tlog.Add("RNAME_MAIL_DOMAIN_INVALID", map[string]any{"domain": mailServer}); err != nil {
				return mailOutcome{entries: buf.Entries()}, err
			}
		}

		return mailOutcome{entries: buf.Entries(), exchangeValid: exchangeValid}, nil
	}

	for _, ns := range nss {
		disabled, err := ipDisabledMessage(ctx, &results, testcase, ns, "SOA")
		if err != nil {
			return results, err
		}
		if disabled {
			continue
		}

		recurse := false
		usevc := false
		resp, err := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", &nameserver.QueryOptions{
			Recurse: &recurse,
			UseVC:   &usevc,
		})
		if err != nil || resp.Msg == nil {
			if err := appendLog(ctx, &results, testcase, "NO_RESPONSE", withNameserverArgs(ns, map[string]any{
				"domain": z.Name.String(),
			})); err != nil {
				return results, err
			}
			continue
		}

		var soa *dns.SOA
		for _, rr := range resp.GetRecords("SOA", "answer") {
			if item, ok := rr.(*dns.SOA); ok {
				soa = item
				break
			}
		}
		if soa == nil {
			if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
				return results, err
			}
			continue
		}

		rawRname := soa.Mbox
		rname := rnameToEmail(rawRname)
		if !validEmailAddress(rname) {
			if err := appendLog(ctx, &results, testcase, "RNAME_RFC822_INVALID", map[string]any{"rname": rname}); err != nil {
				return results, err
			}
			continue
		}

		parts := strings.SplitN(rname, "@", 2)
		if len(parts) != 2 || parts[1] == "" {
			if err := appendLog(ctx, &results, testcase, "RNAME_RFC822_INVALID", map[string]any{"rname": rname}); err != nil {
				return results, err
			}
			continue
		}

		domain := dnsname.New(parts[1])
		pMX, err := rec.Recurse(ctx, domain.String(), "MX", "IN")
		if err != nil {
			return results, err
		}
		if pMX.Msg == nil || pMX.Rcode() != "NOERROR" {
			if err := appendLog(ctx, &results, testcase, "RNAME_MAIL_DOMAIN_INVALID", map[string]any{"domain": domain.String()}); err != nil {
				return results, err
			}
			continue
		}

		if q := pMX.Question(); len(q) > 0 {
			qname := dnsname.New(q[0].Header().Name)
			if !strings.EqualFold(qname.String(), domain.String()) {
				domain = qname
			} else if len(pMX.GetRecords("CNAME", "answer")) > 0 {
				cnames := map[string]string{}
				for _, rr := range pMX.GetRecords("CNAME", "answer") {
					if cname, ok := rr.(*dns.CNAME); ok {
						owner := dnsname.New(cname.Hdr.Name)
						target := dnsname.New(cname.Target)
						cnames[strings.ToLower(owner.String())] = strings.ToLower(target.String())
					}
				}
				for {
					next, ok := cnames[strings.ToLower(domain.String())]
					if !ok {
						break
					}
					domain = dnsname.New(next)
				}
			}
		}

		var mailServers []string
		mxRecords := pMX.GetRecordsForName("MX", domain)
		if len(mxRecords) > 0 {
			seenMX := map[string]bool{}
			for _, rr := range mxRecords {
				if mx, ok := rr.(*dns.MX); ok {
					mxName := dnsname.New(mx.Mx)
					name := mxName.String()
					if !seenMX[name] {
						seenMX[name] = true
						mailServers = append(mailServers, name)
					}
				}
			}
		} else {
			mailServers = []string{domain.String()}
		}

		mailServersToCheck := make([]string, 0, len(mailServers))
		for _, mailServer := range mailServers {
			if seenMailServers[mailServer] {
				continue
			}
			seenMailServers[mailServer] = true
			mailServersToCheck = append(mailServersToCheck, mailServer)
		}
		if len(mailServersToCheck) == 0 {
			continue
		}
		if invalidExchanges < 0 {
			invalidExchanges = 0
		}

		tasks := make([]parallel.Task[mailOutcome], len(mailServersToCheck))
		for i, mailServer := range mailServersToCheck {
			mailServer := mailServer
			tasks[i] = func(ctx context.Context) (mailOutcome, error) {
				return processMailServer(ctx, mailServer)
			}
		}
		parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
		if parallelism < 1 {
			parallelism = 1
		}
		mailResults := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: parallelism, CancelOnError: false})
		for _, res := range mailResults {
			results = append(results, res.Value.entries...)
			if res.Err != nil {
				return results, res.Err
			}
			if res.Value.exchangeValid {
				rnameCandidates[rname] = true
			} else {
				delete(rnameCandidates, rname)
				invalidExchanges++
			}
		}
	}

	if invalidExchanges == 0 {
		keys := make([]string, 0, len(rnameCandidates))
		for key := range rnameCandidates {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, rname := range keys {
			if err := appendLog(ctx, &results, testcase, "RNAME_RFC822_VALID", map[string]any{"rname": rname}); err != nil {
				return results, err
			}
		}
	}

	return appendTestCaseEnd(ctx, results, testcase)
}

// Syntax07 runs the SYNTAX07 test case.
func Syntax07(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Syntax07"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := z.QueryOne(ctx, z.Name.String(), "SOA", nil)
	if err != nil {
		return results, err
	}
	if resp.Msg != nil {
		for _, rr := range resp.GetRecords("SOA", "answer") {
			if soa, ok := rr.(*dns.SOA); ok {
				entries, err := checkNameSyntax("MNAME", dnsname.New(soa.Ns), testcase)
				if err != nil {
					return results, err
				}
				results = append(results, entries...)
				return appendTestCaseEnd(ctx, results, testcase)
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_SOA_QUERY", map[string]any{}); err != nil {
		return results, err
	}
	return appendTestCaseEnd(ctx, results, testcase)
}

// Syntax08 runs the SYNTAX08 test case.
func Syntax08(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "Syntax08"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	resp, err := z.QueryOne(ctx, z.Name.String(), "MX", nil)
	if err != nil {
		return results, err
	}
	if resp.Msg != nil {
		seen := map[string]bool{}
		var targets []dnsname.Name
		for _, rr := range resp.GetRecords("MX", "answer") {
			if mx, ok := rr.(*dns.MX); ok {
				target := dnsname.New(mx.Mx)
				key := strings.ToLower(target.String())
				if seen[key] {
					continue
				}
				seen[key] = true
				targets = append(targets, target)
			}
		}
		if len(targets) > 0 {
			tasks := make([]runner.Task, len(targets))
			for i, target := range targets {
				target := target
				tasks[i] = func(ctx context.Context, log *logger.Logger) error {
					buf := testlogger.Wrap(log, moduleName, testcase)
					return checkNameSyntaxWithLogger(buf, "MX", target)
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

	if err := appendLog(ctx, &results, testcase, "NO_RESPONSE_MX_QUERY", map[string]any{}); err != nil {
		return results, err
	}
	return appendTestCaseEnd(ctx, results, testcase)
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

func ipDisabledMessage(ctx context.Context, results *[]*logger.Entry, testcase string, ns nameserver.Nameserver, rrtype string) (bool, error) {
	if ns.Address.Is4() && !profile.FromContext(ctx).Net.IPv4 {
		if err := appendLog(ctx, results, testcase, "IPV4_DISABLED", withNameserverArgs(ns, map[string]any{
			"query_type": rrtype,
		})); err != nil {
			return true, err
		}
		return true, nil
	}
	if ns.Address.Is6() && !profile.FromContext(ctx).Net.IPv6 {
		if err := appendLog(ctx, results, testcase, "IPV6_DISABLED", withNameserverArgs(ns, map[string]any{
			"query_type": rrtype,
		})); err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

func nameHasOnlyLegalCharacters(name dnsname.Name) bool {
	for _, label := range name.Labels() {
		if !labelHasOnlyLegalCharacters(label) {
			return false
		}
	}
	return true
}

func labelHasOnlyLegalCharacters(label string) bool {
	if label == "" {
		return false
	}
	for i := 0; i < len(label); i++ {
		ch := label[i]
		if ch >= 'A' && ch <= 'Z' {
			continue
		}
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '-' {
			continue
		}
		return false
	}
	return true
}

func labelStartsWithHyphen(label string) bool {
	return len(label) > 0 && label[0] == '-'
}

func labelEndsWithHyphen(label string) bool {
	return len(label) > 0 && label[len(label)-1] == '-'
}

func labelNotACEHasDoubleHyphen(label string) bool {
	if len(label) < 4 {
		return false
	}
	if strings.HasPrefix(strings.ToLower(label), "xn") {
		return false
	}
	return label[2:4] == "--"
}

func checkNameSyntax(prefix string, name dnsname.Name, testcase string) ([]*logger.Entry, error) {
	buf := testlogger.New(moduleName, testcase)
	if err := checkNameSyntaxWithLogger(buf, prefix, name); err != nil {
		return buf.Entries(), err
	}
	return buf.Entries(), nil
}

func checkNameSyntaxWithLogger(buf *testlogger.Buffer, prefix string, name dnsname.Name) error {
	domain := name.String()
	hadIssue := false

	if !nameHasOnlyLegalCharacters(name) {
		if _, err := buf.Add(prefix+"_NON_ALLOWED_CHARS", map[string]any{"domain": domain}); err != nil {
			return err
		}
		hadIssue = true
	}

	if domain != "." {
		for _, label := range name.Labels() {
			if labelNotACEHasDoubleHyphen(label) {
				if _, err := buf.Add(prefix+"_DISCOURAGED_DOUBLE_DASH", map[string]any{
					"label":  label,
					"domain": domain,
				}); err != nil {
					return err
				}
				hadIssue = true
			}
		}

		labels := name.Labels()
		if len(labels) > 0 {
			tld := labels[len(labels)-1]
			if isNumericLabel(tld) {
				if _, err := buf.Add(prefix+"_NUMERIC_TLD", map[string]any{
					"domain": domain,
					"tld":    tld,
				}); err != nil {
					return err
				}
				hadIssue = true
			}
		}
	}

	if !hadIssue {
		if _, err := buf.Add(prefix+"_SYNTAX_OK", map[string]any{"domain": domain}); err != nil {
			return err
		}
	}

	return nil
}

func isNumericLabel(label string) bool {
	if label == "" {
		return false
	}
	for i := 0; i < len(label); i++ {
		if label[i] < '0' || label[i] > '9' {
			return false
		}
	}
	return true
}

func rnameToEmail(rname string) string {
	var b strings.Builder
	replaced := false
	prevBackslash := false
	for i := 0; i < len(rname); i++ {
		ch := rname[i]
		if !replaced && ch == '.' && !prevBackslash {
			b.WriteByte('@')
			replaced = true
			prevBackslash = false
			continue
		}
		if ch == '\\' {
			prevBackslash = true
		} else {
			prevBackslash = false
		}
		b.WriteByte(ch)
	}
	out := b.String()
	out = strings.ReplaceAll(out, `\.`, ".")
	out = strings.TrimSuffix(out, ".")
	return out
}

func validEmailAddress(addr string) bool {
	parsed, err := mail.ParseAddress(addr)
	if err != nil {
		return false
	}
	if parsed.Address != addr {
		return false
	}

	parts := strings.SplitN(addr, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	return validEmailDomain(parts[1])
}

func validEmailDomain(domain string) bool {
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !labelHasOnlyLegalCharacters(label) {
			return false
		}
		if labelStartsWithHyphen(label) || labelEndsWithHyphen(label) {
			return false
		}
	}
	return true
}

func matchingARecords(resp packet.Packet, owner string) []*dns.A {
	if resp.Msg == nil {
		return nil
	}
	ownerName := dnsname.New(owner)
	var out []*dns.A
	for _, rr := range resp.GetRecords("A", "answer") {
		if a, ok := rr.(*dns.A); ok {
			name := dnsname.New(a.Header().Name)
			if strings.EqualFold(name.String(), ownerName.String()) {
				out = append(out, a)
			}
		}
	}
	return out
}

func matchingAAAARecords(resp packet.Packet, owner string) []*dns.AAAA {
	if resp.Msg == nil {
		return nil
	}
	ownerName := dnsname.New(owner)
	var out []*dns.AAAA
	for _, rr := range resp.GetRecords("AAAA", "answer") {
		if aaaa, ok := rr.(*dns.AAAA); ok {
			name := dnsname.New(aaaa.Header().Name)
			if strings.EqualFold(name.String(), ownerName.String()) {
				out = append(out, aaaa)
			}
		}
	}
	return out
}

func hasIPv4Loopback(records []*dns.A) bool {
	loopback := netip.MustParseAddr("127.0.0.1")
	for _, rr := range records {
		if rr == nil {
			continue
		}
		if rr.Addr == loopback {
			return true
		}
	}
	return false
}

func hasIPv6Loopback(records []*dns.AAAA) bool {
	loopback := netip.MustParseAddr("::1")
	for _, rr := range records {
		if rr == nil {
			continue
		}
		if rr.Addr == loopback {
			return true
		}
	}
	return false
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
