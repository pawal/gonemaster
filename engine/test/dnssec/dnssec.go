package dnssec

import (
	"context"
	"encoding/base64"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/parallel"
	"codeberg.org/pawal/gonemaster/engine/logargs"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/methods"
	methodsv2 "codeberg.org/pawal/gonemaster/engine/methodsv2"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/runner"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testcase"
	"codeberg.org/pawal/gonemaster/engine/test/internal/testlogger"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

const moduleName = "DNSSEC"

type algoProperty struct {
	description string
	mnemonic    string
}

type rsaKeySizeDetails struct {
	minSize int
	maxSize int
	recSize int
}

var (
	method1                = methods.Method1
	method4                = methods.Method4
	method5                = methods.Method5
	method4and5            = methods.Method4and5
	getParentNSNamesAndIPs = methodsv2.GetParentNSNamesAndIPs
	getDelNSNamesAndIPs    = methodsv2.GetDelNSNamesAndIPs
	getZoneNSNamesAndIPs   = methodsv2.GetZoneNSNamesAndIPs
	zoneQueryOne           = defaultZoneQueryOne
	zoneQueryAll           = defaultZoneQueryAll
	zoneParent             = defaultZoneParent
	parentNameservers      = defaultParentNameservers
	hasFakeAddresses       = defaultHasFakeAddresses
)

var algoProperties = map[uint8]algoProperty{
	0:   {description: "Delete DS", mnemonic: "DELETE"},
	1:   {description: "RSA/MD5", mnemonic: "RSAMD5"},
	2:   {description: "Diffie-Hellman", mnemonic: "DH"},
	3:   {description: "DSA/SHA1", mnemonic: "DSA"},
	4:   {description: "Reserved", mnemonic: "RESERVED"},
	5:   {description: "RSA/SHA1", mnemonic: "RSASHA1"},
	6:   {description: "DSA-NSEC3-SHA1", mnemonic: "DSA-NSEC3-SHA1"},
	7:   {description: "RSASHA1-NSEC3-SHA1", mnemonic: "RSASHA1-NSEC3-SHA1"},
	8:   {description: "RSA/SHA-256", mnemonic: "RSASHA256"},
	9:   {description: "Reserved", mnemonic: "RESERVED"},
	10:  {description: "RSA/SHA-512", mnemonic: "RSASHA512"},
	11:  {description: "Reserved", mnemonic: "RESERVED"},
	12:  {description: "GOST R 34.10-2001", mnemonic: "ECC-GOST"},
	13:  {description: "ECDSA Curve P-256 with SHA-256", mnemonic: "ECDSAP256SHA256"},
	14:  {description: "ECDSA Curve P-384 with SHA-384", mnemonic: "ECDSAP384SHA384"},
	15:  {description: "Ed25519", mnemonic: "ED25519"},
	16:  {description: "Ed448", mnemonic: "ED448"},
	17:  {description: "SM2 signing algo w SM3 hash algo", mnemonic: "SM2SM3"},
	23:  {description: "GOST R 34.10-2012", mnemonic: "ECC-GOST12"},
	252: {description: "Reserved for Indirect Keys", mnemonic: "INDIRECT"},
	253: {description: "private algorithm", mnemonic: "PRIVATEDNS"},
	254: {description: "private algorithm OID", mnemonic: "PRIVATEOID"},
	255: {description: "Reserved", mnemonic: "RESERVED"},
}

var rsaKeySizeByAlgo = map[uint8]rsaKeySizeDetails{
	5:  {minSize: 512, maxSize: 4096, recSize: 2048},
	7:  {minSize: 512, maxSize: 4096, recSize: 2048},
	8:  {minSize: 512, maxSize: 4096, recSize: 2048},
	10: {minSize: 1024, maxSize: 4096, recSize: 2048},
}

// All runs the DNSSEC test cases in order, mirroring the Perl implementation.
func All(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	var results []*logger.Entry

	if util.ShouldRunTest(ctx, "dnssec07") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC07(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if hasTag(results, "DS07_NOT_SIGNED") {
		return results, nil
	}

	if util.ShouldRunTest(ctx, "dnssec01") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC01(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec02") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC02(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec03") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC03(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec04") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC04(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec05") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC05(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec06") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC06(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if util.ShouldRunTest(ctx, "dnssec08") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC08(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if util.ShouldRunTest(ctx, "dnssec09") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC09(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	if util.ShouldRunTest(ctx, "dnssec10") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC10(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec11") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC11(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec13") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC13(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec14") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC14(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec15") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC15(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec16") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC16(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec17") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC17(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}
	if util.ShouldRunTest(ctx, "dnssec18") {
		entries, err := testcase.Run(ctx, func(ctx context.Context) ([]*logger.Entry, error) {
			return DNSSEC18(ctx, z)
		})
		results = append(results, entries...)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// Metadata returns the set of tags emitted by DNSSEC test cases.
func Metadata() map[string][]string {
	return map[string][]string{
		"dnssec01": {
			"DS01_DS_ALGO_2_MISSING",
			"DS01_DS_ALGO_DEPRECATED",
			"DS01_DS_ALGO_NOT_DS",
			"DS01_DS_ALGO_OK",
			"DS01_DS_ALGO_PRIVATE",
			"DS01_DS_ALGO_RESERVED",
			"DS01_DS_ALGO_UNASSIGNED",
			"DS01_NO_RESPONSE",
			"DS01_PARENT_SERVER_NO_DS",
			"DS01_PARENT_ZONE_NO_DS",
			"DS01_ROOT_N_NO_UNDEL_DS",
			"DS01_UNDEL_N_NO_UNDEL_DS",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec02": {
			"DS02_ALGO_NOT_SUPPORTED_BY_ZM",
			"DS02_DNSKEY_NOT_FOR_ZONE_SIGNING",
			"DS02_DNSKEY_NOT_SEP",
			"DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS",
			"DS02_NO_DNSKEY_FOR_DS",
			"DS02_NO_MATCHING_DNSKEY_RRSIG",
			"DS02_NO_MATCH_DS_DNSKEY",
			"DS02_NO_VALID_DNSKEY_FOR_ANY_DS",
			"DS02_RRSIG_NOT_VALID_BY_DNSKEY",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec03": {
			"DS03_ERR_MULT_NSEC3",
			"DS03_ILLEGAL_HASH_ALGO",
			"DS03_ILLEGAL_ITERATION_VALUE",
			"DS03_ILLEGAL_SALT_LENGTH",
			"DS03_INCONSISTENT_HASH_ALGO",
			"DS03_INCONSISTENT_ITERATION",
			"DS03_INCONSISTENT_NSEC3_FLAGS",
			"DS03_INCONSISTENT_SALT_LENGTH",
			"DS03_LEGAL_EMPTY_SALT",
			"DS03_LEGAL_HASH_ALGO",
			"DS03_LEGAL_ITERATION_VALUE",
			"DS03_NO_RESPONSE_NSEC_QUERY",
			"DS03_ERROR_RESPONSE_NSEC_QUERY",
			"DS03_NO_DNSSEC_SUPPORT",
			"DS03_NO_NSEC3",
			"DS03_NSEC3_OPT_OUT_DISABLED",
			"DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD",
			"DS03_NSEC3_OPT_OUT_ENABLED_TLD",
			"DS03_SERVER_NO_DNSSEC_SUPPORT",
			"DS03_SERVER_NO_NSEC3",
			"DS03_UNASSIGNED_FLAG_USED",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec04": {
			"RRSIG_EXPIRATION",
			"RRSIG_EXPIRED",
			"REMAINING_SHORT",
			"REMAINING_LONG",
			"DURATION_LONG",
			"DURATION_OK",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec05": {
			"DS05_ALGO_DEPRECATED",
			"DS05_ALGO_NOT_RECOMMENDED",
			"DS05_ALGO_NOT_ZONE_SIGN",
			"DS05_ALGO_OK",
			"DS05_ALGO_PRIVATE",
			"DS05_ALGO_RESERVED",
			"DS05_ALGO_UNASSIGNED",
			"DS05_NO_RESPONSE",
			"DS05_SERVER_NO_DNSSEC",
			"DS05_ZONE_NO_DNSSEC",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec06": {
			"EXTRA_PROCESSING_OK",
			"EXTRA_PROCESSING_BROKEN",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec07": {
			"DS07_DS_FOR_SIGNED_ZONE",
			"DS07_DS_ON_PARENT_SERVER",
			"DS07_INCONSISTENT_DS",
			"DS07_INCONSISTENT_SIGNED",
			"DS07_NON_AUTH_RESPONSE_DNSKEY",
			"DS07_NOT_SIGNED",
			"DS07_NOT_SIGNED_ON_SERVER",
			"DS07_NO_DS_ON_PARENT_SERVER",
			"DS07_NO_DS_FOR_SIGNED_ZONE",
			"DS07_NO_RESPONSE_DNSKEY",
			"DS07_SIGNED",
			"DS07_SIGNED_ON_SERVER",
			"DS07_UNEXP_RCODE_RESP_DNSKEY",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec08": {
			"DS08_ALGO_NOT_SUPPORTED_BY_ZM",
			"DS08_DNSKEY_RRSIG_EXPIRED",
			"DS08_DNSKEY_RRSIG_NOT_YET_VALID",
			"DS08_MISSING_RRSIG_IN_RESPONSE",
			"DS08_NO_MATCHING_DNSKEY",
			"DS08_RRSIG_NOT_VALID_BY_DNSKEY",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec09": {
			"DS09_ALGO_NOT_SUPPORTED_BY_ZM",
			"DS09_MISSING_RRSIG_IN_RESPONSE",
			"DS09_NO_MATCHING_DNSKEY",
			"DS09_RRSIG_NOT_VALID_BY_DNSKEY",
			"DS09_SOA_RRSIG_EXPIRED",
			"DS09_SOA_RRSIG_NOT_YET_VALID",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec10": {
			"DS10_ALGO_NOT_SUPPORTED_BY_ZM",
			"DS10_ERR_MULT_NSEC",
			"DS10_ERR_MULT_NSEC3",
			"DS10_ERR_MULT_NSEC3PARAM",
			"DS10_EXPECTED_NSEC_NSEC3_MISSING",
			"DS10_HAS_NSEC",
			"DS10_HAS_NSEC3",
			"DS10_INCONSISTENT_NSEC",
			"DS10_INCONSISTENT_NSEC3",
			"DS10_INCONSISTENT_NSEC_NSEC3",
			"DS10_MIXED_NSEC_NSEC3",
			"DS10_NSEC3PARAM_GIVES_ERR_ANSWER",
			"DS10_NSEC3PARAM_MISMATCHES_APEX",
			"DS10_NSEC3PARAM_QUERY_RESPONSE_ERR",
			"DS10_NSEC3_ERR_TYPE_LIST",
			"DS10_NSEC3_MISMATCHES_APEX",
			"DS10_NSEC3_MISSING_SIGNATURE",
			"DS10_NSEC3_NODATA_MISSING_SOA",
			"DS10_NSEC3_NODATA_WRONG_SOA",
			"DS10_NSEC3_NO_VERIFIED_SIGNATURE",
			"DS10_NSEC3_RRSIG_EXPIRED",
			"DS10_NSEC3_RRSIG_NOT_YET_VALID",
			"DS10_NSEC3_RRSIG_NO_DNSKEY",
			"DS10_NSEC3_RRSIG_VERIFY_ERROR",
			"DS10_NSEC_ERR_TYPE_LIST",
			"DS10_NSEC_GIVES_ERR_ANSWER",
			"DS10_NSEC_MISMATCHES_APEX",
			"DS10_NSEC_MISSING_SIGNATURE",
			"DS10_NSEC_NODATA_MISSING_SOA",
			"DS10_NSEC_NODATA_WRONG_SOA",
			"DS10_NSEC_NO_VERIFIED_SIGNATURE",
			"DS10_NSEC_QUERY_RESPONSE_ERR",
			"DS10_NSEC_RRSIG_EXPIRED",
			"DS10_NSEC_RRSIG_NOT_YET_VALID",
			"DS10_NSEC_RRSIG_NO_DNSKEY",
			"DS10_NSEC_RRSIG_VERIFY_ERROR",
			"DS10_SERVER_NO_DNSSEC",
			"DS10_ZONE_NO_DNSSEC",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec11": {
			"DS11_INCONSISTENT_DS",
			"DS11_INCONSISTENT_SIGNED_ZONE",
			"DS11_UNDETERMINED_DS",
			"DS11_UNDETERMINED_SIGNED_ZONE",
			"DS11_PARENT_WITHOUT_DS",
			"DS11_PARENT_WITH_DS",
			"DS11_NS_WITH_SIGNED_ZONE",
			"DS11_NS_WITH_UNSIGNED_ZONE",
			"DS11_DS_BUT_UNSIGNED_ZONE",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec13": {
			"DS13_ALGO_NOT_SIGNED_DNSKEY",
			"DS13_ALGO_NOT_SIGNED_NS",
			"DS13_ALGO_NOT_SIGNED_SOA",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec14": {
			"NO_RESPONSE",
			"NO_RESPONSE_DNSKEY",
			"DNSKEY_SMALLER_THAN_REC",
			"DNSKEY_TOO_SMALL_FOR_ALGO",
			"DNSKEY_TOO_LARGE_FOR_ALGO",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"KEY_SIZE_OK",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec15": {
			"DS15_HAS_CDNSKEY_NO_CDS",
			"DS15_HAS_CDS_AND_CDNSKEY",
			"DS15_HAS_CDS_NO_CDNSKEY",
			"DS15_INCONSISTENT_CDNSKEY",
			"DS15_INCONSISTENT_CDS",
			"DS15_MISMATCH_CDS_CDNSKEY",
			"DS15_NO_CDS_CDNSKEY",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec16": {
			"DS16_CDS_INVALID_RRSIG",
			"DS16_CDS_MATCHES_NON_SEP_DNSKEY",
			"DS16_CDS_MATCHES_NON_ZONE_DNSKEY",
			"DS16_CDS_MATCHES_NO_DNSKEY",
			"DS16_CDS_NOT_SIGNED_BY_CDS",
			"DS16_CDS_SIGNED_BY_UNKNOWN_DNSKEY",
			"DS16_CDS_UNSIGNED",
			"DS16_CDS_WITHOUT_DNSKEY",
			"DS16_DELETE_CDS",
			"DS16_DNSKEY_NOT_SIGNED_BY_CDS",
			"DS16_MIXED_DELETE_CDS",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec17": {
			"DS17_CDNSKEY_INVALID_RRSIG",
			"DS17_CDNSKEY_IS_NON_SEP",
			"DS17_CDNSKEY_IS_NON_ZONE",
			"DS17_CDNSKEY_MATCHES_NO_DNSKEY",
			"DS17_CDNSKEY_NOT_SIGNED_BY_CDNSKEY",
			"DS17_CDNSKEY_SIGNED_BY_UNKNOWN_DNSKEY",
			"DS17_CDNSKEY_UNSIGNED",
			"DS17_CDNSKEY_WITHOUT_DNSKEY",
			"DS17_DELETE_CDNSKEY",
			"DS17_DNSKEY_NOT_SIGNED_BY_CDNSKEY",
			"DS17_MIXED_DELETE_CDNSKEY",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
		"dnssec18": {
			"DS18_NO_MATCH_CDS_RRSIG_DS",
			"DS18_NO_MATCH_CDNSKEY_RRSIG_DS",
			"IPV4_DISABLED",
			"IPV6_DISABLED",
			"TEST_CASE_END",
			"TEST_CASE_START",
		},
	}
}

// DNSSEC01 runs the DNSSEC01 test case.
func DNSSEC01(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC01"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var ignoredParentNS []string
	var respondsWithoutValidDS []string
	var respondsWithDS []string

	algo2DS := map[uint16][]string{}
	nonAlgo2DS := map[uint16][]string{}

	sets := map[string]map[uint8]map[uint16][]string{
		"DS01_DS_ALGO_DEPRECATED": {},
		"DS01_DS_ALGO_RESERVED":   {},
		"DS01_DS_ALGO_UNASSIGNED": {},
		"DS01_DS_ALGO_PRIVATE":    {},
		"DS01_DS_ALGO_NOT_DS":     {},
		"DS01_DS_ALGO_OK":         {},
	}

	parentNS, err := getParentNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}

	var undelegatedDS []dns.RR
	if z != nil {
		parent, perr := zoneParent(ctx, z)
		if perr == nil && parent != nil {
			parentNSList, nerr := parent.NS(ctx)
			if nerr == nil {
				for _, ns := range parentNSList {
					records := ns.FakeDSRecords(z.Name.String())
					if len(records) == 0 {
						continue
					}
					undelegatedDS = records
					nsLabel := "-"
					for _, rr := range records {
						ds, ok := rr.(*dns.DS)
						if !ok {
							continue
						}
						digest := ds.DigestType
						keytag := ds.KeyTag
						tag := dnssec01TagForDigest(digest)
						if sets[tag][digest] == nil {
							sets[tag][digest] = map[uint16][]string{}
						}
						sets[tag][digest][keytag] = append(sets[tag][digest][keytag], nsLabel)
						if digest == 2 {
							algo2DS[keytag] = append(algo2DS[keytag], nsLabel)
						} else {
							nonAlgo2DS[keytag] = append(nonAlgo2DS[keytag], nsLabel)
						}
					}
					respondsWithDS = append(respondsWithDS, nsLabel)
					parentNS = nil
					break
				}
			}
		}
	}

	if len(parentNS) > 0 {
		type parentOutcome struct {
			ignoredParentNS      []string
			respondsWithoutValid []string
			respondsWith         []string
			sets                 map[string]map[uint8]map[uint16][]string
			algo2DS              map[uint16][]string
			nonAlgo2DS           map[uint16][]string
		}

		nsByIP := nameserversByIP(parentNS)
		outcomes := make([]parentOutcome, len(nsByIP))
		tasks := make([]runner.Task, len(nsByIP))
		for i, matchingNS := range nsByIP {
			i, matchingNS := i, matchingNS
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				if len(matchingNS) == 0 {
					return nil
				}
				buf := testlogger.Wrap(log, moduleName, testcase)
				ns := matchingNS[0]
				outcome := parentOutcome{
					sets:       map[string]map[uint8]map[uint16][]string{},
					algo2DS:    map[uint16][]string{},
					nonAlgo2DS: map[uint16][]string{},
				}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DS"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				matchingStrings := nsStrings(matchingNS)

				dnssecOn := true
				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DS", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.HasEdns() || !resp.DO() || !resp.AA() {
					outcome.ignoredParentNS = append(outcome.ignoredParentNS, matchingStrings...)
					outcomes[i] = outcome
					return nil
				}

				rrs := resp.GetRecords("DS", "answer")
				validDS := false
				for _, rr := range rrs {
					ds, ok := rr.(*dns.DS)
					if !ok {
						continue
					}
					owner := dnsname.New(ds.Hdr.Name)
					if owner.String() == z.Name.String() {
						validDS = true
						break
					}
				}

				if !validDS {
					outcome.respondsWithoutValid = append(outcome.respondsWithoutValid, matchingStrings...)
					outcomes[i] = outcome
					return nil
				}

				outcome.respondsWith = append(outcome.respondsWith, matchingStrings...)

				for _, rr := range rrs {
					ds, ok := rr.(*dns.DS)
					if !ok {
						continue
					}
					digest := ds.DigestType
					keytag := ds.KeyTag
					tag := dnssec01TagForDigest(digest)
					if outcome.sets[tag] == nil {
						outcome.sets[tag] = map[uint8]map[uint16][]string{}
					}
					if outcome.sets[tag][digest] == nil {
						outcome.sets[tag][digest] = map[uint16][]string{}
					}
					outcome.sets[tag][digest][keytag] = append(outcome.sets[tag][digest][keytag], matchingStrings...)
					if digest == 2 {
						outcome.algo2DS[keytag] = append(outcome.algo2DS[keytag], matchingStrings...)
					} else {
						outcome.nonAlgo2DS[keytag] = append(outcome.nonAlgo2DS[keytag], matchingStrings...)
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

		for _, outcome := range outcomes {
			ignoredParentNS = append(ignoredParentNS, outcome.ignoredParentNS...)
			respondsWithoutValidDS = append(respondsWithoutValidDS, outcome.respondsWithoutValid...)
			respondsWithDS = append(respondsWithDS, outcome.respondsWith...)

			for tag, digestMap := range outcome.sets {
				if sets[tag] == nil {
					sets[tag] = map[uint8]map[uint16][]string{}
				}
				for digest, keytagMap := range digestMap {
					if sets[tag][digest] == nil {
						sets[tag][digest] = map[uint16][]string{}
					}
					for keytag, nsList := range keytagMap {
						sets[tag][digest][keytag] = append(sets[tag][digest][keytag], nsList...)
					}
				}
			}
			for keytag, nsList := range outcome.algo2DS {
				algo2DS[keytag] = append(algo2DS[keytag], nsList...)
			}
			for keytag, nsList := range outcome.nonAlgo2DS {
				nonAlgo2DS[keytag] = append(nonAlgo2DS[keytag], nsList...)
			}
		}
	}

	tagKeys := make([]string, 0, len(sets))
	for tag := range sets {
		tagKeys = append(tagKeys, tag)
	}
	sort.Strings(tagKeys)
	for _, tag := range tagKeys {
		values := sets[tag]
		digestKeys := make([]int, 0, len(values))
		for digest := range values {
			digestKeys = append(digestKeys, int(digest))
		}
		sort.Ints(digestKeys)
		for _, digestKey := range digestKeys {
			digest := uint8(digestKey)
			keytags := values[digest]
			keytagKeys := make([]int, 0, len(keytags))
			for keytag := range keytags {
				keytagKeys = append(keytagKeys, int(keytag))
			}
			sort.Ints(keytagKeys)
			for _, keytagKey := range keytagKeys {
				keytag := uint16(keytagKey)
				args := map[string]any{
					"keytag":        keytag,
					"ds_algo_num":   digest,
					"ds_algo_descr": digestDescription(digest),
				}
				setTypedServersFromNames(args, keytags[keytag])
				if err := appendLog(ctx, &results, testcase, tag, args); err != nil {
					return results, err
				}
			}
		}
	}

	for keytag, nsList := range nonAlgo2DS {
		missing := differenceStrings(nsList, algo2DS[keytag])
		if len(missing) == 0 {
			continue
		}
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedServersFromNames(args, missing)
		if err := appendLog(ctx, &results, testcase, "DS01_DS_ALGO_2_MISSING", args); err != nil {
			return results, err
		}
	}

	if len(respondsWithoutValidDS) == 0 && len(respondsWithDS) == 0 && len(ignoredParentNS) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, ignoredParentNS)
		if err := appendLog(ctx, &results, testcase, "DS01_NO_RESPONSE", args); err != nil {
			return results, err
		}
	}

	if z != nil && z.Name.String() == "." && len(undelegatedDS) == 0 {
		if err := appendLog(ctx, &results, testcase, "DS01_ROOT_N_NO_UNDEL_DS", map[string]any{}); err != nil {
			return results, err
		}
	}

	if z != nil && z.Name.String() != "." && hasFakeAddresses(z) && len(undelegatedDS) == 0 {
		if err := appendLog(ctx, &results, testcase, "DS01_UNDEL_N_NO_UNDEL_DS", map[string]any{}); err != nil {
			return results, err
		}
	}

	if len(respondsWithoutValidDS) > 0 {
		tag := "DS01_PARENT_ZONE_NO_DS"
		if len(respondsWithDS) > 0 {
			tag = "DS01_PARENT_SERVER_NO_DS"
		}
		args := map[string]any{}
		setTypedServersFromNames(args, respondsWithoutValidDS)
		if err := appendLog(ctx, &results, testcase, tag, args); err != nil {
			return results, err
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC02 runs the DNSSEC02 test case.
func DNSSEC02(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC02"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var dsRecords []*dns.DS
	noDNSKEYForDS := map[uint16][]string{}
	noMatchDSDNSKEY := map[uint16][]string{}
	dnskeyNotForZoneSigning := map[uint16][]string{}
	dnskeyNotSEP := map[uint16][]string{}
	noMatchingDNSKEYRRSIG := map[uint16][]string{}
	algoNotSupportedByZM := map[uint16]map[uint8][]string{}
	rrsigNotValidByDNSKEY := map[uint16][]string{}
	respondingChildNS := map[string]bool{}
	hasDNSKEYMatchDS := map[string]bool{}
	hasRRSIGMatchDS := map[string]bool{}
	var nsDNSKEY []string
	var nsRRSIG []string

	parentNS, err := getParentNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}

	if len(parentNS) > 0 {
		type parentOutcome struct {
			dsRecords []*dns.DS
		}

		nsByIP := nameserversByIP(parentNS)
		outcomes := make([]parentOutcome, len(nsByIP))
		tasks := make([]runner.Task, len(nsByIP))
		for i, matchingNS := range nsByIP {
			i, matchingNS := i, matchingNS
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				if len(matchingNS) == 0 {
					return nil
				}
				buf := testlogger.Wrap(log, moduleName, testcase)
				ns := matchingNS[0]
				outcome := parentOutcome{}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DS"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DS", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.HasEdns() || !resp.DO() || !resp.AA() {
					outcomes[i] = outcome
					return nil
				}

				tmpDSRecords := resp.GetRecordsForName("DS", z.Name, "answer")
				if len(tmpDSRecords) == 0 {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range tmpDSRecords {
					ds, ok := rr.(*dns.DS)
					if !ok {
						continue
					}
					outcome.dsRecords = append(outcome.dsRecords, ds)
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
			for _, ds := range outcome.dsRecords {
				if !containsDS(dsRecords, ds) {
					dsRecords = append(dsRecords, ds)
				}
			}
		}
	}

	continueWithChildTests := len(dsRecords) > 0
	if continueWithChildTests {
		nssDel, err := method4(ctx, z)
		if err != nil {
			return results, err
		}
		nssChild, err := method5(ctx, z)
		if err != nil {
			return results, err
		}

		nss := map[string]nameserver.Nameserver{}
		for _, ns := range append(nssDel, nssChild...) {
			nss[ns.String()] = ns
		}

		keys := make([]string, 0, len(nss))
		for key := range nss {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		type childOutcome struct {
			nsIP                    string
			responding              bool
			hasDNSKEYMatchDS        bool
			hasRRSIGMatchDS         bool
			noDNSKEYForDS           map[uint16]bool
			noMatchDSDNSKEY         map[uint16]bool
			dnskeyNotForZoneSigning map[uint16]bool
			dnskeyNotSEP            map[uint16]bool
			noMatchingDNSKEYRRSIG   map[uint16]bool
			algoNotSupportedByZM    map[uint16]map[uint8]bool
			rrsigNotValidByDNSKEY   map[uint16]bool
		}

		var ordered []nameserver.Nameserver
		ipAlreadyProcessed := map[string]bool{}
		for _, key := range keys {
			ns := nss[key]
			nsIP := ns.Address.String()
			if ipAlreadyProcessed[nsIP] {
				continue
			}
			ipAlreadyProcessed[nsIP] = true
			ordered = append(ordered, ns)
		}

		outcomes := make([]childOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := childOutcome{
					nsIP:                    ns.Address.String(),
					noDNSKEYForDS:           map[uint16]bool{},
					noMatchDSDNSKEY:         map[uint16]bool{},
					dnskeyNotForZoneSigning: map[uint16]bool{},
					dnskeyNotSEP:            map[uint16]bool{},
					noMatchingDNSKEYRRSIG:   map[uint16]bool{},
					algoNotSupportedByZM:    map[uint16]map[uint8]bool{},
					rrsigNotValidByDNSKEY:   map[uint16]bool{},
				}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DNSKEY"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				useVC := false
				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.HasEdns() || !resp.DO() || !resp.AA() {
					outcomes[i] = outcome
					return nil
				}

				dnskeyRRs := resp.GetRecordsForName("DNSKEY", z.Name, "answer")
				if len(dnskeyRRs) == 0 {
					outcomes[i] = outcome
					return nil
				}

				var dnskeyRecords []*dns.DNSKEY
				for _, rr := range dnskeyRRs {
					if key, ok := rr.(*dns.DNSKEY); ok {
						dnskeyRecords = append(dnskeyRecords, key)
					}
				}
				if len(dnskeyRecords) == 0 {
					outcomes[i] = outcome
					return nil
				}

				outcome.responding = true

				rrsigRRs := resp.GetRecordsForName("RRSIG", z.Name, "answer")
				var dnskeyRRSIG []*dns.RRSIG
				for _, rr := range rrsigRRs {
					if sig, ok := rr.(*dns.RRSIG); ok {
						dnskeyRRSIG = append(dnskeyRRSIG, sig)
					}
				}

				dnskeyMatchingDS := map[*dns.DNSKEY]uint16{}

				for _, ds := range dsRecords {
					var matchingDNSKEY *dns.DNSKEY
					var matchingKeytagDNSKEYs []*dns.DNSKEY
					matchDSDNSKEY := false

					for _, key := range dnskeyRecords {
						if ds.KeyTag == key.KeyTag() {
							matchingKeytagDNSKEYs = append(matchingKeytagDNSKEYs, key)
						}
					}

					for _, key := range matchingKeytagDNSKEYs {
						if dsDigestSupported(ds.DigestType) {
							tmpDS := key.ToDS(ds.DigestType)
							if tmpDS == nil || strings.EqualFold(tmpDS.Digest, ds.Digest) {
								matchingDNSKEY = key
								matchDSDNSKEY = true
								break
							}
						} else {
							matchingDNSKEY = key
							matchDSDNSKEY = true
							break
						}
					}

					if matchingDNSKEY == nil && len(matchingKeytagDNSKEYs) > 0 {
						matchingDNSKEY = matchingKeytagDNSKEYs[0]
					}

					if matchingDNSKEY == nil {
						outcome.noDNSKEYForDS[ds.KeyTag] = true
						continue
					}

					if !matchDSDNSKEY {
						outcome.noMatchDSDNSKEY[ds.KeyTag] = true
					}

					if matchingDNSKEY.Flags&dns.FlagZONE == 0 {
						outcome.dnskeyNotForZoneSigning[ds.KeyTag] = true
						continue
					}
					if matchingDNSKEY.Flags&dns.FlagSEP == 0 {
						outcome.dnskeyNotSEP[ds.KeyTag] = true
					}

					dnskeyMatchingDS[matchingDNSKEY] = matchingDNSKEY.KeyTag()
					outcome.hasDNSKEYMatchDS = true

					rrset := dnskeyRRset(dnskeyRecords)
					testTime := packetTime(resp)

					for dnskey, keytag := range dnskeyMatchingDS {
						var matchingRRSIG []*dns.RRSIG
						for _, sig := range dnskeyRRSIG {
							if sig.KeyTag == keytag {
								matchingRRSIG = append(matchingRRSIG, sig)
							}
						}

						foundMatch := false
						for _, sig := range matchingRRSIG {
							if err := verifyRRSIG(sig, rrset, dnskey, testTime); err != nil {
								if errors.Is(err, dns.ErrAlg) {
									if outcome.algoNotSupportedByZM[keytag] == nil {
										outcome.algoNotSupportedByZM[keytag] = map[uint8]bool{}
									}
									outcome.algoNotSupportedByZM[keytag][sig.Algorithm] = true
								} else {
									outcome.rrsigNotValidByDNSKEY[keytag] = true
								}
							} else {
								foundMatch = true
							}
						}

						if len(matchingRRSIG) == 0 || !foundMatch {
							outcome.noMatchingDNSKEYRRSIG[keytag] = true
						} else {
							outcome.hasRRSIGMatchDS = true
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

		for _, outcome := range outcomes {
			if !outcome.responding {
				continue
			}

			respondingChildNS[outcome.nsIP] = true
			if outcome.hasDNSKEYMatchDS {
				hasDNSKEYMatchDS[outcome.nsIP] = true
			}
			if outcome.hasRRSIGMatchDS {
				hasRRSIGMatchDS[outcome.nsIP] = true
			}

			for keytag := range outcome.noDNSKEYForDS {
				noDNSKEYForDS[keytag] = append(noDNSKEYForDS[keytag], outcome.nsIP)
			}
			for keytag := range outcome.noMatchDSDNSKEY {
				noMatchDSDNSKEY[keytag] = append(noMatchDSDNSKEY[keytag], outcome.nsIP)
			}
			for keytag := range outcome.dnskeyNotForZoneSigning {
				dnskeyNotForZoneSigning[keytag] = append(dnskeyNotForZoneSigning[keytag], outcome.nsIP)
			}
			for keytag := range outcome.dnskeyNotSEP {
				dnskeyNotSEP[keytag] = append(dnskeyNotSEP[keytag], outcome.nsIP)
			}
			for keytag := range outcome.noMatchingDNSKEYRRSIG {
				noMatchingDNSKEYRRSIG[keytag] = append(noMatchingDNSKEYRRSIG[keytag], outcome.nsIP)
			}
			for keytag, algoMap := range outcome.algoNotSupportedByZM {
				if algoNotSupportedByZM[keytag] == nil {
					algoNotSupportedByZM[keytag] = map[uint8][]string{}
				}
				for algo := range algoMap {
					algoNotSupportedByZM[keytag][algo] = append(algoNotSupportedByZM[keytag][algo], outcome.nsIP)
				}
			}
			for keytag := range outcome.rrsigNotValidByDNSKEY {
				rrsigNotValidByDNSKEY[keytag] = append(rrsigNotValidByDNSKEY[keytag], outcome.nsIP)
			}
		}
	}

	for keytag, nsList := range noDNSKEYForDS {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS02_NO_DNSKEY_FOR_DS", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range noMatchDSDNSKEY {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS02_NO_MATCH_DS_DNSKEY", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range dnskeyNotForZoneSigning {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS02_DNSKEY_NOT_FOR_ZONE_SIGNING", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range dnskeyNotSEP {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS02_DNSKEY_NOT_SEP", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range noMatchingDNSKEYRRSIG {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS02_NO_MATCHING_DNSKEY_RRSIG", args); err != nil {
			return results, err
		}
	}
	for keytag, algoMap := range algoNotSupportedByZM {
		for algo, nsList := range algoMap {
			prop := algoPropertyFor(algo)
			args := map[string]any{
				"keytag":     keytag,
				"algo_num":   algo,
				"algo_mnemo": prop.mnemonic,
			}
			setTypedAddressesFromValues(args, nsList)
			if err := appendLog(ctx, &results, testcase, "DS02_ALGO_NOT_SUPPORTED_BY_ZM", args); err != nil {
				return results, err
			}
		}
	}
	for keytag, nsList := range rrsigNotValidByDNSKEY {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS02_RRSIG_NOT_VALID_BY_DNSKEY", args); err != nil {
			return results, err
		}
	}

	for nsIP := range respondingChildNS {
		if !hasDNSKEYMatchDS[nsIP] {
			nsDNSKEY = append(nsDNSKEY, nsIP)
		}
		if !hasRRSIGMatchDS[nsIP] {
			nsRRSIG = append(nsRRSIG, nsIP)
		}
	}

	if len(nsDNSKEY) > 0 {
		args := map[string]any{}
		setTypedAddressesFromValues(args, nsDNSKEY)
		if err := appendLog(ctx, &results, testcase, "DS02_NO_VALID_DNSKEY_FOR_ANY_DS", args); err != nil {
			return results, err
		}
	} else if len(nsRRSIG) > 0 {
		args := map[string]any{}
		setTypedAddressesFromValues(args, nsRRSIG)
		if err := appendLog(ctx, &results, testcase, "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS", args); err != nil {
			return results, err
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC03 runs the DNSSEC03 test case.
func DNSSEC03(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC03"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var respondsWithoutDNSKEY []string
	var respondsWithDNSKEY []string
	var respondsWithoutNSEC3 []string
	var respondsWithNSEC3 []string
	var multipleNSEC3 []string
	hashAlgorithm := map[uint8][]string{}
	nsec3Flags := map[uint8][]string{}
	nsec3Iterations := map[uint16][]string{}
	nsec3SaltLength := map[int][]string{}
	var noResponseNSECQuery []string
	var errorResponseNSECQuery []string

	nss, err := method4and5(ctx, z)
	if err != nil {
		return results, err
	}

	type nsOutcome struct {
		ns                     string
		respondsWithoutDNSKEY  bool
		respondsWithDNSKEY     bool
		respondsWithoutNSEC3   bool
		respondsWithNSEC3      bool
		multipleNSEC3          bool
		hasNSEC3Details        bool
		hashAlgorithm          uint8
		nsec3Flags             uint8
		nsec3Iterations        uint16
		nsec3SaltLength        int
		noResponseNSECQuery    bool
		errorResponseNSECQuery bool
	}

	var ordered []nameserver.Nameserver
	ipAlreadyProcessed := map[string]bool{}
	for _, ns := range nss {
		nsIP := ns.Address.String()
		if ipAlreadyProcessed[nsIP] {
			continue
		}
		ipAlreadyProcessed[nsIP] = true
		ordered = append(ordered, ns)
	}

	outcomes := make([]nsOutcome, len(ordered))
	tasks := make([]runner.Task, len(ordered))
	for i, ns := range ordered {
		i, ns := i, ns
		tasks[i] = func(ctx context.Context, log *logger.Logger) error {
			buf := testlogger.Wrap(log, moduleName, testcase)
			outcome := nsOutcome{ns: ns.String()}

			if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DNSKEY", "NSEC"); err != nil {
				return err
			} else if disabled {
				outcomes[i] = outcome
				return nil
			}

			dnssecOn := true
			dnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
			if dnskeyResp.Msg == nil || dnskeyResp.Rcode() != "NOERROR" || !dnskeyResp.AA() {
				outcomes[i] = outcome
				return nil
			}

			if len(dnskeyResp.GetRecordsForName("DNSKEY", z.Name, "answer")) == 0 {
				outcome.respondsWithoutDNSKEY = true
				outcomes[i] = outcome
				return nil
			}

			outcome.respondsWithDNSKEY = true

			nsecResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "NSEC", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
			if nsecResp.Msg == nil {
				outcome.noResponseNSECQuery = true
				outcomes[i] = outcome
				return nil
			}
			if nsecResp.Rcode() != "NOERROR" || !nsecResp.AA() {
				outcome.errorResponseNSECQuery = true
				outcomes[i] = outcome
				return nil
			}

			nsec3RRs := nsecResp.GetRecords("NSEC3", "authority")
			if len(nsec3RRs) == 0 {
				outcome.respondsWithoutNSEC3 = true
				outcomes[i] = outcome
				return nil
			}

			outcome.respondsWithNSEC3 = true
			if len(nsec3RRs) > 1 {
				outcome.multipleNSEC3 = true
			}

			rr, ok := nsec3RRs[0].(*dns.NSEC3)
			if !ok {
				outcomes[i] = outcome
				return nil
			}

			outcome.hashAlgorithm = rr.Hash
			outcome.nsec3Flags = rr.Flags
			outcome.nsec3Iterations = rr.Iterations

			saltLength := 0
			if rr.Salt != "" {
				saltLength = len(rr.Salt)
			}
			outcome.nsec3SaltLength = saltLength
			outcome.hasNSEC3Details = true

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
		if outcome.respondsWithoutDNSKEY {
			respondsWithoutDNSKEY = append(respondsWithoutDNSKEY, outcome.ns)
		}
		if outcome.respondsWithDNSKEY {
			respondsWithDNSKEY = append(respondsWithDNSKEY, outcome.ns)
		}
		if outcome.respondsWithoutNSEC3 {
			respondsWithoutNSEC3 = append(respondsWithoutNSEC3, outcome.ns)
		}
		if outcome.respondsWithNSEC3 {
			respondsWithNSEC3 = append(respondsWithNSEC3, outcome.ns)
		}
		if outcome.multipleNSEC3 {
			multipleNSEC3 = append(multipleNSEC3, outcome.ns)
		}
		if outcome.hasNSEC3Details {
			hashAlgorithm[outcome.hashAlgorithm] = append(hashAlgorithm[outcome.hashAlgorithm], outcome.ns)
			nsec3Flags[outcome.nsec3Flags] = append(nsec3Flags[outcome.nsec3Flags], outcome.ns)
			nsec3Iterations[outcome.nsec3Iterations] = append(nsec3Iterations[outcome.nsec3Iterations], outcome.ns)
			nsec3SaltLength[outcome.nsec3SaltLength] = append(nsec3SaltLength[outcome.nsec3SaltLength], outcome.ns)
		}
		if outcome.noResponseNSECQuery {
			noResponseNSECQuery = append(noResponseNSECQuery, outcome.ns)
		}
		if outcome.errorResponseNSECQuery {
			errorResponseNSECQuery = append(errorResponseNSECQuery, outcome.ns)
		}
	}

	if len(respondsWithDNSKEY) == 0 && len(respondsWithoutDNSKEY) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, respondsWithoutDNSKEY)
		if err := appendLog(ctx, &results, testcase, "DS03_NO_DNSSEC_SUPPORT", args); err != nil {
			return results, err
		}
	}
	if len(respondsWithDNSKEY) > 0 && len(respondsWithoutDNSKEY) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, respondsWithoutDNSKEY)
		if err := appendLog(ctx, &results, testcase, "DS03_SERVER_NO_DNSSEC_SUPPORT", args); err != nil {
			return results, err
		}
	}

	if len(respondsWithNSEC3) == 0 && len(respondsWithoutNSEC3) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, respondsWithoutNSEC3)
		if err := appendLog(ctx, &results, testcase, "DS03_NO_NSEC3", args); err != nil {
			return results, err
		}
	}
	if len(respondsWithNSEC3) > 0 && len(respondsWithoutNSEC3) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, respondsWithoutNSEC3)
		if err := appendLog(ctx, &results, testcase, "DS03_SERVER_NO_NSEC3", args); err != nil {
			return results, err
		}
	}

	if len(multipleNSEC3) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, multipleNSEC3)
		if err := appendLog(ctx, &results, testcase, "DS03_ERR_MULT_NSEC3", args); err != nil {
			return results, err
		}
	}

	if len(hashAlgorithm) > 0 {
		if len(hashAlgorithm) > 1 {
			if err := appendLog(ctx, &results, testcase, "DS03_INCONSISTENT_HASH_ALGO", map[string]any{}); err != nil {
				return results, err
			}
		}

		hashKeys := make([]int, 0, len(hashAlgorithm))
		for algo := range hashAlgorithm {
			hashKeys = append(hashKeys, int(algo))
		}
		sort.Ints(hashKeys)
		for _, algoKey := range hashKeys {
			algo := uint8(algoKey)
			if algo == 1 {
				args := map[string]any{}
				setTypedServersFromNames(args, hashAlgorithm[algo])
				if err := appendLog(ctx, &results, testcase, "DS03_LEGAL_HASH_ALGO", args); err != nil {
					return results, err
				}
			} else {
				args := map[string]any{
					"algo_num": algo,
				}
				setTypedServersFromNames(args, hashAlgorithm[algo])
				if err := appendLog(ctx, &results, testcase, "DS03_ILLEGAL_HASH_ALGO", args); err != nil {
					return results, err
				}
			}
		}
	}

	isTLD := false
	if z != nil {
		if z.Name.String() == "." {
			isTLD = true
		} else if higher, ok := z.Name.NextHigher(); ok && higher.String() == "." {
			isTLD = true
		}
	}

	if len(nsec3Flags) > 0 {
		if len(nsec3Flags) > 1 {
			if err := appendLog(ctx, &results, testcase, "DS03_INCONSISTENT_NSEC3_FLAGS", map[string]any{}); err != nil {
				return results, err
			}
		}

		flagKeys := make([]int, 0, len(nsec3Flags))
		for flag := range nsec3Flags {
			flagKeys = append(flagKeys, int(flag))
		}
		sort.Ints(flagKeys)
		for _, flagKey := range flagKeys {
			flag := uint8(flagKey)
			nsNames := nsec3Flags[flag]

			var bitPositions []int
			for bit := 0; bit < 8; bit++ {
				if flag&(1<<(7-uint(bit))) != 0 {
					bitPositions = append(bitPositions, bit)
				}
			}

			for _, bit := range bitPositions {
				if bit >= 0 && bit <= 6 {
					args := map[string]any{
						"int": bit,
					}
					setTypedServersFromNames(args, nsNames)
					if err := appendLog(ctx, &results, testcase, "DS03_UNASSIGNED_FLAG_USED", args); err != nil {
						return results, err
					}
				}
			}

			optOut := false
			for _, bit := range bitPositions {
				if bit == 7 {
					optOut = true
					break
				}
			}
			if optOut {
				tag := "DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD"
				if isTLD {
					tag = "DS03_NSEC3_OPT_OUT_ENABLED_TLD"
				}
				args := map[string]any{}
				setTypedServersFromNames(args, nsNames)
				if err := appendLog(ctx, &results, testcase, tag, args); err != nil {
					return results, err
				}
			} else {
				args := map[string]any{}
				setTypedServersFromNames(args, nsNames)
				if err := appendLog(ctx, &results, testcase, "DS03_NSEC3_OPT_OUT_DISABLED", args); err != nil {
					return results, err
				}
			}
		}
	}

	if len(nsec3Iterations) > 0 {
		if len(nsec3Iterations) > 1 {
			if err := appendLog(ctx, &results, testcase, "DS03_INCONSISTENT_ITERATION", map[string]any{}); err != nil {
				return results, err
			}
		}

		iterKeys := make([]int, 0, len(nsec3Iterations))
		for iter := range nsec3Iterations {
			iterKeys = append(iterKeys, int(iter))
		}
		sort.Ints(iterKeys)
		for _, iterKey := range iterKeys {
			iter := uint16(iterKey)
			if iter == 0 {
				args := map[string]any{}
				setTypedServersFromNames(args, nsec3Iterations[iter])
				if err := appendLog(ctx, &results, testcase, "DS03_LEGAL_ITERATION_VALUE", args); err != nil {
					return results, err
				}
			} else {
				args := map[string]any{
					"int": iter,
				}
				setTypedServersFromNames(args, nsec3Iterations[iter])
				if err := appendLog(ctx, &results, testcase, "DS03_ILLEGAL_ITERATION_VALUE", args); err != nil {
					return results, err
				}
			}
		}
	}

	if len(nsec3SaltLength) > 0 {
		if len(nsec3SaltLength) > 1 {
			if err := appendLog(ctx, &results, testcase, "DS03_INCONSISTENT_SALT_LENGTH", map[string]any{}); err != nil {
				return results, err
			}
		}

		saltKeys := make([]int, 0, len(nsec3SaltLength))
		for salt := range nsec3SaltLength {
			saltKeys = append(saltKeys, salt)
		}
		sort.Ints(saltKeys)
		for _, salt := range saltKeys {
			if salt == 0 {
				args := map[string]any{}
				setTypedServersFromNames(args, nsec3SaltLength[salt])
				if err := appendLog(ctx, &results, testcase, "DS03_LEGAL_EMPTY_SALT", args); err != nil {
					return results, err
				}
			} else {
				args := map[string]any{
					"int": salt,
				}
				setTypedServersFromNames(args, nsec3SaltLength[salt])
				if err := appendLog(ctx, &results, testcase, "DS03_ILLEGAL_SALT_LENGTH", args); err != nil {
					return results, err
				}
			}
		}
	}

	if len(noResponseNSECQuery) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, noResponseNSECQuery)
		if err := appendLog(ctx, &results, testcase, "DS03_NO_RESPONSE_NSEC_QUERY", args); err != nil {
			return results, err
		}
	}

	if len(errorResponseNSECQuery) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, errorResponseNSECQuery)
		if err := appendLog(ctx, &results, testcase, "DS03_ERROR_RESPONSE_NSEC_QUERY", args); err != nil {
			return results, err
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC04 runs the DNSSEC04 test case.
func DNSSEC04(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC04"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	dnssecOn := true
	parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel

	var dnskeyResp packet.Packet
	var soaResp packet.Packet

	if parallelism <= 1 {
		var err error
		dnskeyResp, err = zoneQueryOne(ctx, z, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
		if err != nil {
			return results, err
		}
		if dnskeyResp.Msg == nil {
			if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
				return results, err
			}
			return results, nil
		}

		soaResp, err = zoneQueryOne(ctx, z, z.Name.String(), "SOA", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
		if err != nil {
			return results, err
		}
		if soaResp.Msg == nil {
			if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
				return results, err
			}
			return results, nil
		}
	} else {
		tasks := []parallel.Task[packet.Packet]{
			func(ctx context.Context) (packet.Packet, error) {
				return zoneQueryOne(ctx, z, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
			},
			func(ctx context.Context) (packet.Packet, error) {
				return zoneQueryOne(ctx, z, z.Name.String(), "SOA", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
			},
		}

		queryResults := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: parallelism, CancelOnError: false})
		dnskeyResp, soaResp = queryResults[0].Value, queryResults[1].Value
		dnskeyErr, soaErr := queryResults[0].Err, queryResults[1].Err

		if dnskeyErr != nil {
			return results, dnskeyErr
		}
		if dnskeyResp.Msg == nil {
			if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
				return results, err
			}
			return results, nil
		}
		if soaErr != nil {
			return results, soaErr
		}
		if soaResp.Msg == nil {
			if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
				return results, err
			}
			return results, nil
		}
	}

	keySigs := dnskeyResp.GetRecords("RRSIG", "answer")
	soaSigs := soaResp.GetRecords("RRSIG", "answer")

	now := packetTime(dnskeyResp).Unix()
	remainingShortLimit := int64(profile.FromContext(ctx).TestCasesVars.DNSSEC04.RemainingShort)
	remainingLongLimit := int64(profile.FromContext(ctx).TestCasesVars.DNSSEC04.RemainingLong)
	durationLongLimit := int64(profile.FromContext(ctx).TestCasesVars.DNSSEC04.DurationLong)

	for _, rr := range append(keySigs, soaSigs...) {
		sig, ok := rr.(*dns.RRSIG)
		if !ok {
			continue
		}

		expiration := int64(sig.Expiration)
		types := rrsigTypeString(sig.TypeCovered)
		date := time.Unix(expiration, 0).UTC().Format(time.ANSIC)

		if err := appendLog(ctx, &results, testcase, "RRSIG_EXPIRATION", map[string]any{
			"date":   date,
			"keytag": sig.KeyTag,
			"types":  types,
		}); err != nil {
			return results, err
		}

		remaining := expiration - now
		remainingLogged := false
		if remaining < 0 {
			if err := appendLog(ctx, &results, testcase, "RRSIG_EXPIRED", map[string]any{
				"expiration": expiration,
				"keytag":     sig.KeyTag,
				"types":      types,
			}); err != nil {
				return results, err
			}
			remainingLogged = true
		} else if remaining < remainingShortLimit {
			if err := appendLog(ctx, &results, testcase, "REMAINING_SHORT", map[string]any{
				"duration": remaining,
				"keytag":   sig.KeyTag,
				"types":    types,
			}); err != nil {
				return results, err
			}
			remainingLogged = true
		} else if remaining > remainingLongLimit {
			if err := appendLog(ctx, &results, testcase, "REMAINING_LONG", map[string]any{
				"duration": remaining,
				"keytag":   sig.KeyTag,
				"types":    types,
			}); err != nil {
				return results, err
			}
			remainingLogged = true
		}

		duration := expiration - int64(sig.Inception)
		durationLogged := false
		if duration > durationLongLimit {
			if err := appendLog(ctx, &results, testcase, "DURATION_LONG", map[string]any{
				"duration": duration,
				"keytag":   sig.KeyTag,
				"types":    types,
			}); err != nil {
				return results, err
			}
			durationLogged = true
		}

		if !remainingLogged && !durationLogged {
			if err := appendLog(ctx, &results, testcase, "DURATION_OK", map[string]any{
				"duration": duration,
				"keytag":   sig.KeyTag,
				"types":    types,
			}); err != nil {
				return results, err
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC05 runs the DNSSEC05 test case.
func DNSSEC05(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC05"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var ignoredNS []string
	var respondsWithoutDNSKEY []string
	var respondsWithDNSKEY []string

	sets := map[string]map[uint8]map[uint16][]string{
		"DS05_ALGO_DEPRECATED":      {},
		"DS05_ALGO_RESERVED":        {},
		"DS05_ALGO_UNASSIGNED":      {},
		"DS05_ALGO_NOT_RECOMMENDED": {},
		"DS05_ALGO_PRIVATE":         {},
		"DS05_ALGO_NOT_ZONE_SIGN":   {},
		"DS05_ALGO_OK":              {},
	}

	delItems, err := getDelNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}
	zoneItems, err := getZoneNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}

	nss := nameserversFromNSItems(ctx, z, append(delItems, zoneItems...))
	groups := nameserversByIP(nss)
	if len(groups) > 0 {
		type nsOutcome struct {
			ignoredNS             []string
			respondsWithoutDNSKEY []string
			respondsWithDNSKEY    []string
			sets                  map[string]map[uint8]map[uint16][]string
		}

		outcomes := make([]nsOutcome, len(groups))
		tasks := make([]runner.Task, len(groups))
		for i, group := range groups {
			i, group := i, group
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				if len(group) == 0 {
					return nil
				}
				buf := testlogger.Wrap(log, moduleName, testcase)
				ns := group[0]
				outcome := nsOutcome{
					sets: map[string]map[uint8]map[uint16][]string{},
				}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DNSKEY"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				matchingStrings := nsStrings(group)

				dnssecOn := true
				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() {
					outcome.ignoredNS = append(outcome.ignoredNS, matchingStrings...)
					outcomes[i] = outcome
					return nil
				}

				dnskeyRRs := resp.GetRecordsForName("DNSKEY", z.Name, "answer")
				if len(dnskeyRRs) == 0 {
					outcome.respondsWithoutDNSKEY = append(outcome.respondsWithoutDNSKEY, matchingStrings...)
					outcomes[i] = outcome
					return nil
				}

				outcome.respondsWithDNSKEY = append(outcome.respondsWithDNSKEY, matchingStrings...)
				for _, rr := range dnskeyRRs {
					key, ok := rr.(*dns.DNSKEY)
					if !ok {
						continue
					}
					algo := key.Algorithm
					keytag := key.KeyTag()
					tag := dnssec05TagForAlgorithm(algo)
					if sets[tag] == nil {
						continue
					}
					if outcome.sets[tag] == nil {
						outcome.sets[tag] = map[uint8]map[uint16][]string{}
					}
					if outcome.sets[tag][algo] == nil {
						outcome.sets[tag][algo] = map[uint16][]string{}
					}
					outcome.sets[tag][algo][keytag] = append(outcome.sets[tag][algo][keytag], matchingStrings...)
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
			ignoredNS = append(ignoredNS, outcome.ignoredNS...)
			respondsWithoutDNSKEY = append(respondsWithoutDNSKEY, outcome.respondsWithoutDNSKEY...)
			respondsWithDNSKEY = append(respondsWithDNSKEY, outcome.respondsWithDNSKEY...)
			for tag, algoMap := range outcome.sets {
				if sets[tag] == nil {
					sets[tag] = map[uint8]map[uint16][]string{}
				}
				for algo, keytagMap := range algoMap {
					if sets[tag][algo] == nil {
						sets[tag][algo] = map[uint16][]string{}
					}
					for keytag, nsList := range keytagMap {
						sets[tag][algo][keytag] = append(sets[tag][algo][keytag], nsList...)
					}
				}
			}
		}
	}

	tagKeys := make([]string, 0, len(sets))
	for tag := range sets {
		tagKeys = append(tagKeys, tag)
	}
	sort.Strings(tagKeys)
	for _, tag := range tagKeys {
		algoMap := sets[tag]
		algoKeys := make([]int, 0, len(algoMap))
		for algo := range algoMap {
			algoKeys = append(algoKeys, int(algo))
		}
		sort.Ints(algoKeys)
		for _, algoKey := range algoKeys {
			algo := uint8(algoKey)
			keytagMap := algoMap[algo]
			keytagKeys := make([]int, 0, len(keytagMap))
			for keytag := range keytagMap {
				keytagKeys = append(keytagKeys, int(keytag))
			}
			sort.Ints(keytagKeys)
			for _, keytagKey := range keytagKeys {
				keytag := uint16(keytagKey)
				prop := algoPropertyFor(algo)
				args := map[string]any{
					"keytag":     keytag,
					"algo_num":   algo,
					"algo_descr": prop.description,
					"algo_mnemo": prop.mnemonic,
				}
				setTypedServersFromNames(args, keytagMap[keytag])
				if err := appendLog(ctx, &results, testcase, tag, args); err != nil {
					return results, err
				}
			}
		}
	}

	if len(respondsWithoutDNSKEY) == 0 && len(respondsWithDNSKEY) == 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, ignoredNS)
		if err := appendLog(ctx, &results, testcase, "DS05_NO_RESPONSE", args); err != nil {
			return results, err
		}
	}

	if len(respondsWithoutDNSKEY) > 0 {
		tag := "DS05_SERVER_NO_DNSSEC"
		if len(respondsWithDNSKEY) == 0 {
			tag = "DS05_ZONE_NO_DNSSEC"
		}
		args := map[string]any{}
		setTypedServersFromNames(args, respondsWithoutDNSKEY)
		if err := appendLog(ctx, &results, testcase, tag, args); err != nil {
			return results, err
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC06 runs the DNSSEC06 test case.
func DNSSEC06(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC06"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	dnssecOn := true
	responses, err := zoneQueryAll(ctx, z, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
	if err != nil {
		return results, err
	}

	for _, resp := range responses {
		if resp.Msg == nil {
			continue
		}
		keys := resp.GetRecords("DNSKEY", "answer")
		sigs := resp.GetRecords("RRSIG", "answer")
		if len(keys) > 0 && len(sigs) > 0 {
			if err := appendLog(ctx, &results, testcase, "EXTRA_PROCESSING_OK", map[string]any{
				"server": resp.AnswerFromString(),
				"keys":   len(keys),
				"sigs":   len(sigs),
			}); err != nil {
				return results, err
			}
		} else if resp.Rcode() == "NOERROR" {
			if err := appendLog(ctx, &results, testcase, "EXTRA_PROCESSING_BROKEN", map[string]any{
				"server": resp.AnswerFromString(),
				"keys":   len(keys),
				"sigs":   len(sigs),
			}); err != nil {
				return results, err
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC07 runs the DNSSEC07 test case.
func DNSSEC07(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC07"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var ignoredChildNS []string
	var ignoredParentNS []string
	var noResponseDNSKEY []string
	var signedResponse []string
	var noAuthDNSKEY []string
	errorRcodeDNSKEY := map[string][]string{}
	var noDNSKEY []string
	var noDS []string
	var dsInResponse []string

	delItems, err := getDelNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}
	zoneItems, err := getZoneNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}

	childNS := nameserversFromNSItems(ctx, z, append(delItems, zoneItems...))
	childNames := nsStrings(childNS)

	queryTypes := []string{"SOA", "DNSKEY", "DS"}
	type childOutcome struct {
		matchingStrings []string
		ignored         bool
		noResponse      bool
		noAuth          bool
		errorRcode      string
		signed          bool
		noDNSKEY        bool
	}

	childGroups := nameserversByIP(childNS)
	if len(childGroups) > 0 {
		outcomes := make([]childOutcome, len(childGroups))
		tasks := make([]runner.Task, len(childGroups))
		for i, group := range childGroups {
			i, group := i, group
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				if len(group) == 0 {
					return nil
				}
				buf := testlogger.Wrap(log, moduleName, testcase)
				ns := group[0]
				outcome := childOutcome{matchingStrings: nsStrings(group)}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryTypes...); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				soaResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", nil)
				if soaResp.Msg == nil || soaResp.Rcode() != "NOERROR" || !soaResp.AA() || len(soaResp.GetRecords("SOA", "answer")) == 0 {
					outcome.ignored = true
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				dnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if dnskeyResp.Msg == nil {
					outcome.noResponse = true
					outcomes[i] = outcome
					return nil
				}
				if !dnskeyResp.AA() {
					outcome.noAuth = true
					outcomes[i] = outcome
					return nil
				}
				if dnskeyResp.Rcode() != "NOERROR" {
					outcome.errorRcode = dnskeyResp.Rcode()
					outcomes[i] = outcome
					return nil
				}

				rrsigRRs := dnskeyResp.GetRecords("RRSIG", "answer")
				coveredDNSKEY := false
				for _, rr := range rrsigRRs {
					if sig, ok := rr.(*dns.RRSIG); ok && sig.TypeCovered == dns.TypeDNSKEY {
						coveredDNSKEY = true
						break
					}
				}

				if coveredDNSKEY {
					outcome.signed = true
				} else {
					outcome.noDNSKEY = true
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
			if len(outcome.matchingStrings) == 0 {
				continue
			}
			switch {
			case outcome.ignored:
				ignoredChildNS = append(ignoredChildNS, outcome.matchingStrings...)
			case outcome.noResponse:
				noResponseDNSKEY = append(noResponseDNSKEY, outcome.matchingStrings...)
			case outcome.noAuth:
				noAuthDNSKEY = append(noAuthDNSKEY, outcome.matchingStrings...)
			case outcome.errorRcode != "":
				errorRcodeDNSKEY[outcome.errorRcode] = append(errorRcodeDNSKEY[outcome.errorRcode], outcome.matchingStrings...)
			case outcome.signed:
				signedResponse = append(signedResponse, outcome.matchingStrings...)
			case outcome.noDNSKEY:
				noDNSKEY = append(noDNSKEY, outcome.matchingStrings...)
			}
		}
	}

	parentNS, err := getParentNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}

	if z != nil {
		parent, perr := zoneParent(ctx, z)
		if perr == nil && parent != nil {
			parentNSList, nerr := parent.NS(ctx)
			if nerr == nil {
				for _, ns := range parentNSList {
					records := ns.FakeDSRecords(z.Name.String())
					if len(records) == 0 {
						continue
					}
					dsInResponse = append(dsInResponse, "-")
					parentNS = nil
					break
				}
			}
		}
	}

	if len(signedResponse) == 0 {
		parentNS = nil
		dsInResponse = nil
	}

	if len(parentNS) > 0 {
		type parentOutcome struct {
			matchingStrings []string
			ignored         bool
			dsInResponse    bool
			noDS            bool
		}

		parentGroups := nameserversByIP(parentNS)
		outcomes := make([]parentOutcome, len(parentGroups))
		tasks := make([]runner.Task, len(parentGroups))
		for i, group := range parentGroups {
			i, group := i, group
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				if len(group) == 0 {
					return nil
				}
				buf := testlogger.Wrap(log, moduleName, testcase)
				ns := group[0]
				outcome := parentOutcome{matchingStrings: nsStrings(group)}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DS"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				dsResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DS", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if dsResp.Msg == nil || dsResp.Rcode() != "NOERROR" || !dsResp.HasEdns() || !dsResp.DO() || !dsResp.AA() {
					outcome.ignored = true
					outcomes[i] = outcome
					return nil
				}

				rrsigRRs := dsResp.GetRecordsForName("RRSIG", z.Name, "answer")
				coveredDS := false
				for _, rr := range rrsigRRs {
					if sig, ok := rr.(*dns.RRSIG); ok && sig.TypeCovered == dns.TypeDS {
						coveredDS = true
						break
					}
				}

				if coveredDS {
					outcome.dsInResponse = true
				} else {
					outcome.noDS = true
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
			if len(outcome.matchingStrings) == 0 {
				continue
			}
			if outcome.ignored {
				ignoredParentNS = append(ignoredParentNS, outcome.matchingStrings...)
				continue
			}
			if outcome.dsInResponse {
				dsInResponse = append(dsInResponse, outcome.matchingStrings...)
			}
			if outcome.noDS {
				noDS = append(noDS, outcome.matchingStrings...)
			}
		}
	}

	combined := append([]string{}, ignoredChildNS...)
	combined = append(combined, noResponseDNSKEY...)
	combined = append(combined, noAuthDNSKEY...)
	for _, list := range errorRcodeDNSKEY {
		combined = append(combined, list...)
	}
	if equalStringSets(combined, childNames) {
		if err := appendLog(ctx, &results, testcase, "DS07_NOT_SIGNED", map[string]any{}); err != nil {
			return results, err
		}
	}

	if len(noResponseDNSKEY) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, noResponseDNSKEY)
		if err := appendLog(ctx, &results, testcase, "DS07_NO_RESPONSE_DNSKEY", args); err != nil {
			return results, err
		}
	}

	if len(noAuthDNSKEY) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, noAuthDNSKEY)
		if err := appendLog(ctx, &results, testcase, "DS07_NON_AUTH_RESPONSE_DNSKEY", args); err != nil {
			return results, err
		}
	}

	if len(errorRcodeDNSKEY) > 0 {
		rcodeKeys := make([]string, 0, len(errorRcodeDNSKEY))
		for rcode := range errorRcodeDNSKEY {
			rcodeKeys = append(rcodeKeys, rcode)
		}
		sort.Strings(rcodeKeys)
		for _, rcode := range rcodeKeys {
			args := map[string]any{
				"rcode": rcode,
			}
			setTypedServersFromNames(args, errorRcodeDNSKEY[rcode])
			if err := appendLog(ctx, &results, testcase, "DS07_UNEXP_RCODE_RESP_DNSKEY", args); err != nil {
				return results, err
			}
		}
	}

	if len(signedResponse) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, signedResponse)
		if err := appendLog(ctx, &results, testcase, "DS07_SIGNED_ON_SERVER", args); err != nil {
			return results, err
		}
	}

	if len(noDNSKEY) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, noDNSKEY)
		if err := appendLog(ctx, &results, testcase, "DS07_NOT_SIGNED_ON_SERVER", args); err != nil {
			return results, err
		}
	}

	if len(signedResponse) > 0 && len(noDNSKEY) > 0 {
		if err := appendLog(ctx, &results, testcase, "DS07_INCONSISTENT_SIGNED", map[string]any{}); err != nil {
			return results, err
		}
	}

	if len(signedResponse) > 0 && len(noDNSKEY) == 0 {
		if err := appendLog(ctx, &results, testcase, "DS07_SIGNED", map[string]any{}); err != nil {
			return results, err
		}
	}

	if len(signedResponse) == 0 && len(noDNSKEY) > 0 {
		if err := appendLog(ctx, &results, testcase, "DS07_NOT_SIGNED", map[string]any{}); err != nil {
			return results, err
		}
	}

	if len(noDS) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, noDS)
		if err := appendLog(ctx, &results, testcase, "DS07_NO_DS_ON_PARENT_SERVER", args); err != nil {
			return results, err
		}
	}

	if len(dsInResponse) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, dsInResponse)
		if err := appendLog(ctx, &results, testcase, "DS07_DS_ON_PARENT_SERVER", args); err != nil {
			return results, err
		}
	}

	if len(noDS) > 0 && len(dsInResponse) > 0 {
		if err := appendLog(ctx, &results, testcase, "DS07_INCONSISTENT_DS", map[string]any{}); err != nil {
			return results, err
		}
	}

	if len(noDNSKEY) == 0 && len(signedResponse) > 0 {
		if len(noDS) > 0 && len(dsInResponse) == 0 {
			if err := appendLog(ctx, &results, testcase, "DS07_NO_DS_FOR_SIGNED_ZONE", map[string]any{}); err != nil {
				return results, err
			}
		}
		if len(noDS) == 0 && len(dsInResponse) > 0 {
			if err := appendLog(ctx, &results, testcase, "DS07_DS_FOR_SIGNED_ZONE", map[string]any{}); err != nil {
				return results, err
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC08 runs the DNSSEC08 test case.
func DNSSEC08(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC08"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	dnskeyWithoutRRSIG := []string{}
	dnskeyRRSIGNotYetValid := map[uint16][]string{}
	dnskeyRRSIGExpired := map[uint16][]string{}
	noMatchingDNSKEY := map[uint16][]string{}
	rrsigNotValidByDNSKEY := map[uint16][]string{}
	algoNotSupportedByZM := map[uint16]map[uint8][]string{}

	nssDel, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	nssChild, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	nss := map[string]nameserver.Nameserver{}
	for _, ns := range append(nssDel, nssChild...) {
		nss[ns.String()] = ns
	}

	keys := make([]string, 0, len(nss))
	for key := range nss {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var ordered []nameserver.Nameserver
	ipAlreadyProcessed := map[string]bool{}
	for _, key := range keys {
		ns := nss[key]
		nsIP := ns.Address.String()
		if ipAlreadyProcessed[nsIP] {
			continue
		}
		ipAlreadyProcessed[nsIP] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		type nsOutcome struct {
			nsIP                   string
			dnskeyWithoutRRSIG     bool
			dnskeyRRSIGNotYetValid map[uint16]bool
			dnskeyRRSIGExpired     map[uint16]bool
			noMatchingDNSKEY       map[uint16]bool
			rrsigNotValidByDNSKEY  map[uint16]bool
			algoNotSupportedByZM   map[uint16]map[uint8]bool
		}

		outcomes := make([]nsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{
					nsIP:                   ns.Address.String(),
					dnskeyRRSIGNotYetValid: map[uint16]bool{},
					dnskeyRRSIGExpired:     map[uint16]bool{},
					noMatchingDNSKEY:       map[uint16]bool{},
					rrsigNotValidByDNSKEY:  map[uint16]bool{},
					algoNotSupportedByZM:   map[uint16]map[uint8]bool{},
				}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DNSKEY"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() {
					outcomes[i] = outcome
					return nil
				}

				dnskeyRRs := resp.GetRecordsForName("DNSKEY", z.Name, "answer")
				if len(dnskeyRRs) == 0 {
					outcomes[i] = outcome
					return nil
				}

				var dnskeyRecords []*dns.DNSKEY
				for _, rr := range resp.GetRecords("DNSKEY", "answer") {
					if dnskey, ok := rr.(*dns.DNSKEY); ok {
						dnskeyRecords = append(dnskeyRecords, dnskey)
					}
				}
				if len(dnskeyRecords) == 0 {
					outcomes[i] = outcome
					return nil
				}

				rrsigRRs := resp.GetRecords("RRSIG", "answer")
				if len(rrsigRRs) == 0 {
					outcome.dnskeyWithoutRRSIG = true
					outcomes[i] = outcome
					return nil
				}

				testTime := packetTime(resp)

				for _, rr := range rrsigRRs {
					sig, ok := rr.(*dns.RRSIG)
					if !ok {
						continue
					}

					if int64(sig.Inception) > testTime.Unix() {
						outcome.dnskeyRRSIGNotYetValid[sig.KeyTag] = true
						continue
					}
					if int64(sig.Expiration) < testTime.Unix() {
						outcome.dnskeyRRSIGExpired[sig.KeyTag] = true
						continue
					}

					if !dnssecAlgorithmSupported(sig.Algorithm) {
						if outcome.algoNotSupportedByZM[sig.KeyTag] == nil {
							outcome.algoNotSupportedByZM[sig.KeyTag] = map[uint8]bool{}
						}
						outcome.algoNotSupportedByZM[sig.KeyTag][sig.Algorithm] = true
						continue
					}

					var matchingDNSKEYs []*dns.DNSKEY
					for _, dnskey := range dnskeyRecords {
						if dnskey.KeyTag() == sig.KeyTag {
							matchingDNSKEYs = append(matchingDNSKEYs, dnskey)
						}
					}

					if len(matchingDNSKEYs) == 0 {
						outcome.noMatchingDNSKEY[sig.KeyTag] = true
						continue
					}

					rrset := dnskeyRRset(dnskeyRecords)
					valid := false
					algoUnsupported := false
					for _, dnskey := range matchingDNSKEYs {
						if err := verifyRRSIG(sig, rrset, dnskey, testTime); err != nil {
							if errors.Is(err, dns.ErrAlg) {
								algoUnsupported = true
							}
							continue
						}
						valid = true
						break
					}

					if algoUnsupported {
						if outcome.algoNotSupportedByZM[sig.KeyTag] == nil {
							outcome.algoNotSupportedByZM[sig.KeyTag] = map[uint8]bool{}
						}
						outcome.algoNotSupportedByZM[sig.KeyTag][sig.Algorithm] = true
						continue
					}

					if !valid {
						outcome.rrsigNotValidByDNSKEY[sig.KeyTag] = true
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

		for _, outcome := range outcomes {
			if outcome.dnskeyWithoutRRSIG {
				dnskeyWithoutRRSIG = append(dnskeyWithoutRRSIG, outcome.nsIP)
			}
			for keytag := range outcome.dnskeyRRSIGNotYetValid {
				dnskeyRRSIGNotYetValid[keytag] = append(dnskeyRRSIGNotYetValid[keytag], outcome.nsIP)
			}
			for keytag := range outcome.dnskeyRRSIGExpired {
				dnskeyRRSIGExpired[keytag] = append(dnskeyRRSIGExpired[keytag], outcome.nsIP)
			}
			for keytag := range outcome.noMatchingDNSKEY {
				noMatchingDNSKEY[keytag] = append(noMatchingDNSKEY[keytag], outcome.nsIP)
			}
			for keytag := range outcome.rrsigNotValidByDNSKEY {
				rrsigNotValidByDNSKEY[keytag] = append(rrsigNotValidByDNSKEY[keytag], outcome.nsIP)
			}
			for keytag, algoMap := range outcome.algoNotSupportedByZM {
				if algoNotSupportedByZM[keytag] == nil {
					algoNotSupportedByZM[keytag] = map[uint8][]string{}
				}
				for algo := range algoMap {
					algoNotSupportedByZM[keytag][algo] = append(algoNotSupportedByZM[keytag][algo], outcome.nsIP)
				}
			}
		}
	}

	if len(dnskeyWithoutRRSIG) > 0 {
		args := map[string]any{}
		setTypedAddressesFromValues(args, dnskeyWithoutRRSIG)
		if err := appendLog(ctx, &results, testcase, "DS08_MISSING_RRSIG_IN_RESPONSE", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range dnskeyRRSIGNotYetValid {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS08_DNSKEY_RRSIG_NOT_YET_VALID", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range dnskeyRRSIGExpired {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS08_DNSKEY_RRSIG_EXPIRED", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range noMatchingDNSKEY {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS08_NO_MATCHING_DNSKEY", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range rrsigNotValidByDNSKEY {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS08_RRSIG_NOT_VALID_BY_DNSKEY", args); err != nil {
			return results, err
		}
	}
	for keytag, algoMap := range algoNotSupportedByZM {
		for algo, nsList := range algoMap {
			prop := algoPropertyFor(algo)
			args := map[string]any{
				"keytag":     keytag,
				"algo_num":   algo,
				"algo_mnemo": prop.mnemonic,
			}
			setTypedAddressesFromValues(args, nsList)
			if err := appendLog(ctx, &results, testcase, "DS08_ALGO_NOT_SUPPORTED_BY_ZM", args); err != nil {
				return results, err
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC09 runs the DNSSEC09 test case.
func DNSSEC09(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC09"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	soaWithoutRRSIG := []string{}
	soaRRSIGNotYetValid := map[uint16][]string{}
	soaRRSIGExpired := map[uint16][]string{}
	noMatchingDNSKEY := map[uint16][]string{}
	rrsigNotValidByDNSKEY := map[uint16][]string{}
	algoNotSupportedByZM := map[uint16]map[uint8][]string{}

	nssDel, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	nssChild, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	nss := map[string]nameserver.Nameserver{}
	for _, ns := range append(nssDel, nssChild...) {
		nss[ns.String()] = ns
	}

	keys := make([]string, 0, len(nss))
	for key := range nss {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var ordered []nameserver.Nameserver
	ipAlreadyProcessed := map[string]bool{}
	for _, key := range keys {
		ns := nss[key]
		nsIP := ns.Address.String()
		if ipAlreadyProcessed[nsIP] {
			continue
		}
		ipAlreadyProcessed[nsIP] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		type nsOutcome struct {
			nsIP                  string
			soaWithoutRRSIG       bool
			soaRRSIGNotYetValid   map[uint16]bool
			soaRRSIGExpired       map[uint16]bool
			noMatchingDNSKEY      map[uint16]bool
			rrsigNotValidByDNSKEY map[uint16]bool
			algoNotSupportedByZM  map[uint16]map[uint8]bool
		}

		outcomes := make([]nsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{
					nsIP:                  ns.Address.String(),
					soaRRSIGNotYetValid:   map[uint16]bool{},
					soaRRSIGExpired:       map[uint16]bool{},
					noMatchingDNSKEY:      map[uint16]bool{},
					rrsigNotValidByDNSKEY: map[uint16]bool{},
					algoNotSupportedByZM:  map[uint16]map[uint8]bool{},
				}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DNSKEY"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				dnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if dnskeyResp.Msg == nil || dnskeyResp.Rcode() != "NOERROR" || !dnskeyResp.AA() {
					outcomes[i] = outcome
					return nil
				}

				dnskeyRRs := dnskeyResp.GetRecordsForName("DNSKEY", z.Name, "answer")
				if len(dnskeyRRs) == 0 {
					outcomes[i] = outcome
					return nil
				}

				var dnskeyRecords []*dns.DNSKEY
				for _, rr := range dnskeyRRs {
					if dnskey, ok := rr.(*dns.DNSKEY); ok {
						dnskeyRecords = append(dnskeyRecords, dnskey)
					}
				}
				if len(dnskeyRecords) == 0 {
					outcomes[i] = outcome
					return nil
				}

				useVC := false
				soaResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if soaResp.Msg == nil || soaResp.Rcode() != "NOERROR" || !soaResp.AA() {
					outcomes[i] = outcome
					return nil
				}

				soaRRs := soaResp.GetRecordsForName("SOA", z.Name, "answer")
				if len(soaRRs) == 0 {
					outcomes[i] = outcome
					return nil
				}

				rrsigRRs := soaResp.GetRecords("RRSIG", "answer")
				if len(rrsigRRs) == 0 {
					outcome.soaWithoutRRSIG = true
					outcomes[i] = outcome
					return nil
				}

				testTime := packetTime(dnskeyResp)

				for _, rr := range rrsigRRs {
					sig, ok := rr.(*dns.RRSIG)
					if !ok {
						continue
					}

					if int64(sig.Inception) > testTime.Unix() {
						outcome.soaRRSIGNotYetValid[sig.KeyTag] = true
						continue
					}
					if int64(sig.Expiration) < testTime.Unix() {
						outcome.soaRRSIGExpired[sig.KeyTag] = true
						continue
					}

					if !dnssecAlgorithmSupported(sig.Algorithm) {
						if outcome.algoNotSupportedByZM[sig.KeyTag] == nil {
							outcome.algoNotSupportedByZM[sig.KeyTag] = map[uint8]bool{}
						}
						outcome.algoNotSupportedByZM[sig.KeyTag][sig.Algorithm] = true
						continue
					}

					var matchingDNSKEYs []*dns.DNSKEY
					for _, dnskey := range dnskeyRecords {
						if dnskey.KeyTag() == sig.KeyTag {
							matchingDNSKEYs = append(matchingDNSKEYs, dnskey)
						}
					}

					if len(matchingDNSKEYs) == 0 {
						outcome.noMatchingDNSKEY[sig.KeyTag] = true
						continue
					}

					rrset := append([]dns.RR{}, soaRRs...)
					valid := false
					algoUnsupported := false
					for _, dnskey := range matchingDNSKEYs {
						if err := verifyRRSIG(sig, rrset, dnskey, testTime); err != nil {
							if errors.Is(err, dns.ErrAlg) {
								algoUnsupported = true
							}
							continue
						}
						valid = true
						break
					}

					if algoUnsupported {
						if outcome.algoNotSupportedByZM[sig.KeyTag] == nil {
							outcome.algoNotSupportedByZM[sig.KeyTag] = map[uint8]bool{}
						}
						outcome.algoNotSupportedByZM[sig.KeyTag][sig.Algorithm] = true
						continue
					}

					if !valid {
						outcome.rrsigNotValidByDNSKEY[sig.KeyTag] = true
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

		for _, outcome := range outcomes {
			if outcome.soaWithoutRRSIG {
				soaWithoutRRSIG = append(soaWithoutRRSIG, outcome.nsIP)
			}
			for keytag := range outcome.soaRRSIGNotYetValid {
				soaRRSIGNotYetValid[keytag] = append(soaRRSIGNotYetValid[keytag], outcome.nsIP)
			}
			for keytag := range outcome.soaRRSIGExpired {
				soaRRSIGExpired[keytag] = append(soaRRSIGExpired[keytag], outcome.nsIP)
			}
			for keytag := range outcome.noMatchingDNSKEY {
				noMatchingDNSKEY[keytag] = append(noMatchingDNSKEY[keytag], outcome.nsIP)
			}
			for keytag := range outcome.rrsigNotValidByDNSKEY {
				rrsigNotValidByDNSKEY[keytag] = append(rrsigNotValidByDNSKEY[keytag], outcome.nsIP)
			}
			for keytag, algoMap := range outcome.algoNotSupportedByZM {
				if algoNotSupportedByZM[keytag] == nil {
					algoNotSupportedByZM[keytag] = map[uint8][]string{}
				}
				for algo := range algoMap {
					algoNotSupportedByZM[keytag][algo] = append(algoNotSupportedByZM[keytag][algo], outcome.nsIP)
				}
			}
		}
	}

	if len(soaWithoutRRSIG) > 0 {
		args := map[string]any{}
		setTypedAddressesFromValues(args, soaWithoutRRSIG)
		if err := appendLog(ctx, &results, testcase, "DS09_MISSING_RRSIG_IN_RESPONSE", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range soaRRSIGNotYetValid {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS09_SOA_RRSIG_NOT_YET_VALID", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range soaRRSIGExpired {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS09_SOA_RRSIG_EXPIRED", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range noMatchingDNSKEY {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS09_NO_MATCHING_DNSKEY", args); err != nil {
			return results, err
		}
	}
	for keytag, nsList := range rrsigNotValidByDNSKEY {
		args := map[string]any{
			"keytag": keytag,
		}
		setTypedAddressesFromValues(args, nsList)
		if err := appendLog(ctx, &results, testcase, "DS09_RRSIG_NOT_VALID_BY_DNSKEY", args); err != nil {
			return results, err
		}
	}
	for keytag, algoMap := range algoNotSupportedByZM {
		for algo, nsList := range algoMap {
			prop := algoPropertyFor(algo)
			args := map[string]any{
				"keytag":     keytag,
				"algo_num":   algo,
				"algo_mnemo": prop.mnemonic,
			}
			setTypedAddressesFromValues(args, nsList)
			if err := appendLog(ctx, &results, testcase, "DS09_ALGO_NOT_SUPPORTED_BY_ZM", args); err != nil {
				return results, err
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC10 runs the DNSSEC10 test case.
func DNSSEC10(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC10"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	typeSOA := "SOA"
	typeDNSKEY := "DNSKEY"
	typeNSEC := "NSEC"
	typeNSEC3 := "NSEC3"
	typeNSEC3PARAM := "NSEC3PARAM"
	queryTypes := []string{typeDNSKEY, typeNSEC, typeNSEC3PARAM}

	algoNotSupportedByZM := map[uint16]map[uint8][]string{}
	var erroneousMultipleNSEC []string
	var erroneousMultipleNSEC3 []string
	var erroneousMultipleNSEC3PARAM []string
	var nsecInAnswer []string
	var nsec3paramInAnswer []string
	var nsecIncorrectTypeList []string
	var nsec3IncorrectTypeList []string
	var nsecMismatchesApex []string
	var nsec3MismatchesApex []string
	var nsec3paramMismatchesApex []string
	var nsecMissingSignature []string
	var nsec3MissingSignature []string
	nsecNodataWrongSOA := map[string][]string{}
	nsec3NodataWrongSOA := map[string][]string{}
	var nsecNodataMissingSOA []string
	var nsec3NodataMissingSOA []string
	var nsecErroneousAnswer []string
	var nsec3paramErroneousAnswer []string
	var nsecNsec3Nodata []string
	var nsec3paramNsecNodata []string
	nsecRRSIGVerifyError := map[uint16][]string{}
	nsec3RRSIGVerifyError := map[uint16][]string{}
	nsecRRSIGExpired := map[uint16][]string{}
	nsec3RRSIGExpired := map[uint16][]string{}
	nsecRRSIGNotYetValid := map[uint16][]string{}
	nsec3RRSIGNotYetValid := map[uint16][]string{}
	nsecRRSIGNoDNSKEY := map[uint16][]string{}
	nsec3RRSIGNoDNSKEY := map[uint16][]string{}
	var nsecRRSIGVerified []string
	var nsec3RRSIGVerified []string
	var nsecResponseError []string
	var nsec3paramResponseError []string
	var withDNSKEY []string
	var withoutDNSKEY []string
	var ignoredNS []string

	delItems, err := getDelNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}
	zoneItems, err := getZoneNSNamesAndIPs(ctx, z)
	if err != nil {
		return results, err
	}

	nss := nameserversFromNSItems(ctx, z, append(delItems, zoneItems...))
	allNS := uniqueStrings(nsStrings(nss))
	testingTime := time.Now().UTC()
	testingTimeUnix := testingTime.Unix()

	groups := nameserversByIP(nss)
	if len(groups) > 0 {
		type nsOutcome struct {
			groupList                   []string
			ignoredNS                   bool
			withDNSKEY                  bool
			withoutDNSKEY               bool
			erroneousMultipleNSEC       bool
			erroneousMultipleNSEC3      bool
			erroneousMultipleNSEC3PARAM bool
			nsecInAnswer                bool
			nsec3paramInAnswer          bool
			nsecIncorrectTypeList       bool
			nsec3IncorrectTypeList      bool
			nsecMismatchesApex          bool
			nsec3MismatchesApex         bool
			nsec3paramMismatchesApex    bool
			nsecMissingSignature        bool
			nsec3MissingSignature       bool
			nsecNodataWrongSOA          bool
			nsec3NodataWrongSOA         bool
			nsecNodataMissingSOA        bool
			nsec3NodataMissingSOA       bool
			nsecErroneousAnswer         bool
			nsec3paramErroneousAnswer   bool
			nsecNsec3Nodata             bool
			nsec3paramNsecNodata        bool
			nsecRRSIGVerifyError        map[uint16]bool
			nsec3RRSIGVerifyError       map[uint16]bool
			nsecRRSIGExpired            map[uint16]bool
			nsec3RRSIGExpired           map[uint16]bool
			nsecRRSIGNotYetValid        map[uint16]bool
			nsec3RRSIGNotYetValid       map[uint16]bool
			nsecRRSIGNoDNSKEY           map[uint16]bool
			nsec3RRSIGNoDNSKEY          map[uint16]bool
			nsecRRSIGVerified           bool
			nsec3RRSIGVerified          bool
			nsecResponseError           bool
			nsec3paramResponseError     bool
			algoNotSupportedByZM        map[uint16]map[uint8]bool
		}

		outcomes := make([]nsOutcome, len(groups))
		tasks := make([]runner.Task, len(groups))
		for i, group := range groups {
			i, group := i, group
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				if len(group) == 0 {
					return nil
				}
				buf := testlogger.Wrap(log, moduleName, testcase)
				ns := group[0]
				outcome := nsOutcome{
					groupList:             nsStrings(group),
					nsecRRSIGVerifyError:  map[uint16]bool{},
					nsec3RRSIGVerifyError: map[uint16]bool{},
					nsecRRSIGExpired:      map[uint16]bool{},
					nsec3RRSIGExpired:     map[uint16]bool{},
					nsecRRSIGNotYetValid:  map[uint16]bool{},
					nsec3RRSIGNotYetValid: map[uint16]bool{},
					nsecRRSIGNoDNSKEY:     map[uint16]bool{},
					nsec3RRSIGNoDNSKEY:    map[uint16]bool{},
					algoNotSupportedByZM:  map[uint16]map[uint8]bool{},
				}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryTypes...); err != nil {
					return err
				} else if disabled {
					outcome.ignoredNS = true
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				dnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), typeDNSKEY, &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if dnskeyResp.Msg == nil || dnskeyResp.Rcode() != "NOERROR" || !dnskeyResp.AA() {
					outcome.ignoredNS = true
					outcomes[i] = outcome
					return nil
				}

				dnskeyRRs := dnskeyResp.GetRecordsForName(typeDNSKEY, z.Name, "answer")
				if len(dnskeyRRs) == 0 {
					outcome.withoutDNSKEY = true
					outcomes[i] = outcome
					return nil
				}

				var dnskeyRecords []*dns.DNSKEY
				for _, rr := range dnskeyRRs {
					if dnskey, ok := rr.(*dns.DNSKEY); ok {
						dnskeyRecords = append(dnskeyRecords, dnskey)
					}
				}
				if len(dnskeyRecords) == 0 {
					outcome.withoutDNSKEY = true
					outcomes[i] = outcome
					return nil
				}
				outcome.withDNSKEY = true

				nsecResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), typeNSEC, &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if nsecResp.Msg == nil || nsecResp.Rcode() != "NOERROR" || !nsecResp.AA() {
					outcome.nsecResponseError = true
				} else if len(nsecResp.Answer()) > 0 {
					nsecRRs := nsecResp.GetRecords(typeNSEC, "answer")
					if len(nsecRRs) > 0 {
						outcome.nsecInAnswer = true
						if len(nsecRRs) > 1 {
							outcome.erroneousMultipleNSEC = true
						} else if !rrOwnerMatchesZone(nsecRRs[0], z.Name) {
							outcome.nsecMismatchesApex = true
						}
					} else {
						outcome.nsecErroneousAnswer = true
					}
				} else if len(nsecResp.GetRecords(typeNSEC3, "authority")) > 0 {
					outcome.nsecNsec3Nodata = true

					soaRRs := nsecResp.GetRecords(typeSOA, "authority")
					if len(soaRRs) == 0 {
						outcome.nsec3NodataMissingSOA = true
					} else if !rrOwnerMatchesZone(soaRRs[0], z.Name) {
						outcome.nsec3NodataWrongSOA = true
					}

					nsec3RRsRaw := nsecResp.GetRecords(typeNSEC3, "authority")
					var nsec3RRs []*dns.NSEC3
					for _, rr := range nsec3RRsRaw {
						if nsec3, ok := rr.(*dns.NSEC3); ok {
							nsec3RRs = append(nsec3RRs, nsec3)
						}
					}

					if len(nsec3RRs) > 1 {
						outcome.erroneousMultipleNSEC3 = true
					} else if len(nsec3RRs) == 1 {
						nsec3RR := nsec3RRs[0]
						if !nsec3OwnerMatchesApex(nsec3RR, z.Name) {
							outcome.nsec3MismatchesApex = true
						} else {
							mandatory := []string{"SOA", "NS", "DNSKEY", "NSEC3PARAM", "RRSIG"}
							forbidden := []string{"NSEC", "NSEC3"}
							if typeListIncorrect(typeMapFromBitmap(nsec3RR.TypeBitMap), mandatory, forbidden) {
								outcome.nsec3IncorrectTypeList = true
							}
						}

						rrsigRRs := filterRRSIGByType(nsecResp.GetRecordsForName("RRSIG", dnsname.New(nsec3RR.Hdr.Name)), dns.TypeNSEC3)
						if len(rrsigRRs) == 0 {
							outcome.nsec3MissingSignature = true
						} else {
							rrset := rrsetForName(nsec3RRsRaw, nsec3RR.Hdr.Name)
							for _, sig := range rrsigRRs {
								keytag := sig.KeyTag
								var matchingDNSKEYs []*dns.DNSKEY
								for _, dnskey := range dnskeyRecords {
									if dnskey.KeyTag() == keytag {
										matchingDNSKEYs = append(matchingDNSKEYs, dnskey)
									}
								}
								if len(matchingDNSKEYs) == 0 {
									outcome.nsec3RRSIGNoDNSKEY[keytag] = true
									continue
								}
								if int64(sig.Expiration) < testingTimeUnix {
									outcome.nsec3RRSIGExpired[keytag] = true
									continue
								}
								if int64(sig.Inception) > testingTimeUnix {
									outcome.nsec3RRSIGNotYetValid[keytag] = true
									continue
								}

								for idx, dnskey := range matchingDNSKEYs {
									if err := verifyRRSIG(sig, rrset, dnskey, testingTime); err == nil {
										outcome.nsec3RRSIGVerified = true
										break
									} else if idx == len(matchingDNSKEYs)-1 {
										if errors.Is(err, dns.ErrAlg) {
											key := dnskey.KeyTag()
											if outcome.algoNotSupportedByZM[key] == nil {
												outcome.algoNotSupportedByZM[key] = map[uint8]bool{}
											}
											outcome.algoNotSupportedByZM[key][dnskey.Algorithm] = true
										} else {
											outcome.nsec3RRSIGVerifyError[keytag] = true
										}
									}
								}
							}
						}
					}
				}

				nsec3paramResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), typeNSEC3PARAM, &nameserver.QueryOptions{DNSSEC: &dnssecOn})
				if nsec3paramResp.Msg == nil || nsec3paramResp.Rcode() != "NOERROR" || !nsec3paramResp.AA() {
					outcome.nsec3paramResponseError = true
				} else if len(nsec3paramResp.Answer()) > 0 {
					nsec3paramRRs := nsec3paramResp.GetRecords(typeNSEC3PARAM, "answer")
					if len(nsec3paramRRs) > 0 {
						outcome.nsec3paramInAnswer = true
						if len(nsec3paramRRs) > 1 {
							outcome.erroneousMultipleNSEC3PARAM = true
						} else if !rrOwnerMatchesZone(nsec3paramRRs[0], z.Name) {
							outcome.nsec3paramMismatchesApex = true
						}
					} else {
						outcome.nsec3paramErroneousAnswer = true
					}
				} else if len(nsec3paramResp.GetRecords(typeNSEC, "authority")) > 0 {
					outcome.nsec3paramNsecNodata = true

					soaRRs := nsec3paramResp.GetRecords(typeSOA, "authority")
					if len(soaRRs) == 0 {
						outcome.nsecNodataMissingSOA = true
					} else if !rrOwnerMatchesZone(soaRRs[0], z.Name) {
						outcome.nsecNodataWrongSOA = true
					}

					nsecRRsRaw := nsec3paramResp.GetRecords(typeNSEC, "authority")
					var nsecRRs []*dns.NSEC
					for _, rr := range nsecRRsRaw {
						if nsec, ok := rr.(*dns.NSEC); ok {
							nsecRRs = append(nsecRRs, nsec)
						}
					}

					if len(nsecRRs) > 1 {
						outcome.erroneousMultipleNSEC = true
					} else if len(nsecRRs) == 1 {
						nsecRR := nsecRRs[0]
						if !rrOwnerMatchesZone(nsecRR, z.Name) {
							outcome.nsecMismatchesApex = true
						} else {
							mandatory := []string{"SOA", "NS", "DNSKEY", "NSEC", "RRSIG"}
							forbidden := []string{"NSEC3PARAM", "NSEC3"}
							if typeListIncorrect(typeMapFromBitmap(nsecRR.TypeBitMap), mandatory, forbidden) {
								outcome.nsecIncorrectTypeList = true
							}
						}

						rrsigRRs := filterRRSIGByType(nsec3paramResp.GetRecordsForName("RRSIG", dnsname.New(nsecRR.Hdr.Name)), dns.TypeNSEC)
						if len(rrsigRRs) == 0 {
							outcome.nsecMissingSignature = true
						} else {
							rrset := rrsetForName(nsecRRsRaw, nsecRR.Hdr.Name)
							for _, sig := range rrsigRRs {
								keytag := sig.KeyTag
								var matchingDNSKEYs []*dns.DNSKEY
								for _, dnskey := range dnskeyRecords {
									if dnskey.KeyTag() == keytag {
										matchingDNSKEYs = append(matchingDNSKEYs, dnskey)
									}
								}
								if len(matchingDNSKEYs) == 0 {
									outcome.nsecRRSIGNoDNSKEY[keytag] = true
									continue
								}
								if int64(sig.Expiration) < testingTimeUnix {
									outcome.nsecRRSIGExpired[keytag] = true
									continue
								}
								if int64(sig.Inception) > testingTimeUnix {
									outcome.nsecRRSIGNotYetValid[keytag] = true
									continue
								}

								for idx, dnskey := range matchingDNSKEYs {
									if err := verifyRRSIG(sig, rrset, dnskey, testingTime); err == nil {
										outcome.nsecRRSIGVerified = true
										break
									} else if idx == len(matchingDNSKEYs)-1 {
										if errors.Is(err, dns.ErrAlg) {
											key := dnskey.KeyTag()
											if outcome.algoNotSupportedByZM[key] == nil {
												outcome.algoNotSupportedByZM[key] = map[uint8]bool{}
											}
											outcome.algoNotSupportedByZM[key][dnskey.Algorithm] = true
										} else {
											outcome.nsecRRSIGVerifyError[keytag] = true
										}
									}
								}
							}
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

		for _, outcome := range outcomes {
			if len(outcome.groupList) == 0 {
				continue
			}
			if outcome.ignoredNS {
				ignoredNS = append(ignoredNS, outcome.groupList...)
			}
			if outcome.withoutDNSKEY {
				withoutDNSKEY = append(withoutDNSKEY, outcome.groupList...)
			}
			if outcome.withDNSKEY {
				withDNSKEY = append(withDNSKEY, outcome.groupList...)
			}
			if outcome.erroneousMultipleNSEC {
				erroneousMultipleNSEC = append(erroneousMultipleNSEC, outcome.groupList...)
			}
			if outcome.erroneousMultipleNSEC3 {
				erroneousMultipleNSEC3 = append(erroneousMultipleNSEC3, outcome.groupList...)
			}
			if outcome.erroneousMultipleNSEC3PARAM {
				erroneousMultipleNSEC3PARAM = append(erroneousMultipleNSEC3PARAM, outcome.groupList...)
			}
			if outcome.nsecInAnswer {
				nsecInAnswer = append(nsecInAnswer, outcome.groupList...)
			}
			if outcome.nsec3paramInAnswer {
				nsec3paramInAnswer = append(nsec3paramInAnswer, outcome.groupList...)
			}
			if outcome.nsecIncorrectTypeList {
				nsecIncorrectTypeList = append(nsecIncorrectTypeList, outcome.groupList...)
			}
			if outcome.nsec3IncorrectTypeList {
				nsec3IncorrectTypeList = append(nsec3IncorrectTypeList, outcome.groupList...)
			}
			if outcome.nsecMismatchesApex {
				nsecMismatchesApex = append(nsecMismatchesApex, outcome.groupList...)
			}
			if outcome.nsec3MismatchesApex {
				nsec3MismatchesApex = append(nsec3MismatchesApex, outcome.groupList...)
			}
			if outcome.nsec3paramMismatchesApex {
				nsec3paramMismatchesApex = append(nsec3paramMismatchesApex, outcome.groupList...)
			}
			if outcome.nsecMissingSignature {
				nsecMissingSignature = append(nsecMissingSignature, outcome.groupList...)
			}
			if outcome.nsec3MissingSignature {
				nsec3MissingSignature = append(nsec3MissingSignature, outcome.groupList...)
			}
			if outcome.nsecNodataWrongSOA {
				key := z.Name.String()
				nsecNodataWrongSOA[key] = append(nsecNodataWrongSOA[key], outcome.groupList...)
			}
			if outcome.nsec3NodataWrongSOA {
				key := z.Name.String()
				nsec3NodataWrongSOA[key] = append(nsec3NodataWrongSOA[key], outcome.groupList...)
			}
			if outcome.nsecNodataMissingSOA {
				nsecNodataMissingSOA = append(nsecNodataMissingSOA, outcome.groupList...)
			}
			if outcome.nsec3NodataMissingSOA {
				nsec3NodataMissingSOA = append(nsec3NodataMissingSOA, outcome.groupList...)
			}
			if outcome.nsecErroneousAnswer {
				nsecErroneousAnswer = append(nsecErroneousAnswer, outcome.groupList...)
			}
			if outcome.nsec3paramErroneousAnswer {
				nsec3paramErroneousAnswer = append(nsec3paramErroneousAnswer, outcome.groupList...)
			}
			if outcome.nsecNsec3Nodata {
				nsecNsec3Nodata = append(nsecNsec3Nodata, outcome.groupList...)
			}
			if outcome.nsec3paramNsecNodata {
				nsec3paramNsecNodata = append(nsec3paramNsecNodata, outcome.groupList...)
			}
			if outcome.nsecResponseError {
				nsecResponseError = append(nsecResponseError, outcome.groupList...)
			}
			if outcome.nsec3paramResponseError {
				nsec3paramResponseError = append(nsec3paramResponseError, outcome.groupList...)
			}
			if outcome.nsecRRSIGVerified {
				nsecRRSIGVerified = append(nsecRRSIGVerified, outcome.groupList...)
			}
			if outcome.nsec3RRSIGVerified {
				nsec3RRSIGVerified = append(nsec3RRSIGVerified, outcome.groupList...)
			}
			for keytag := range outcome.nsecRRSIGVerifyError {
				nsecRRSIGVerifyError[keytag] = append(nsecRRSIGVerifyError[keytag], outcome.groupList...)
			}
			for keytag := range outcome.nsec3RRSIGVerifyError {
				nsec3RRSIGVerifyError[keytag] = append(nsec3RRSIGVerifyError[keytag], outcome.groupList...)
			}
			for keytag := range outcome.nsecRRSIGExpired {
				nsecRRSIGExpired[keytag] = append(nsecRRSIGExpired[keytag], outcome.groupList...)
			}
			for keytag := range outcome.nsec3RRSIGExpired {
				nsec3RRSIGExpired[keytag] = append(nsec3RRSIGExpired[keytag], outcome.groupList...)
			}
			for keytag := range outcome.nsecRRSIGNotYetValid {
				nsecRRSIGNotYetValid[keytag] = append(nsecRRSIGNotYetValid[keytag], outcome.groupList...)
			}
			for keytag := range outcome.nsec3RRSIGNotYetValid {
				nsec3RRSIGNotYetValid[keytag] = append(nsec3RRSIGNotYetValid[keytag], outcome.groupList...)
			}
			for keytag := range outcome.nsecRRSIGNoDNSKEY {
				nsecRRSIGNoDNSKEY[keytag] = append(nsecRRSIGNoDNSKEY[keytag], outcome.groupList...)
			}
			for keytag := range outcome.nsec3RRSIGNoDNSKEY {
				nsec3RRSIGNoDNSKEY[keytag] = append(nsec3RRSIGNoDNSKEY[keytag], outcome.groupList...)
			}
			for keytag, algoMap := range outcome.algoNotSupportedByZM {
				if algoNotSupportedByZM[keytag] == nil {
					algoNotSupportedByZM[keytag] = map[uint8][]string{}
				}
				for algo := range algoMap {
					algoNotSupportedByZM[keytag][algo] = append(algoNotSupportedByZM[keytag][algo], outcome.groupList...)
				}
			}
		}
	}

	if len(erroneousMultipleNSEC) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, erroneousMultipleNSEC)
		if err := appendLog(ctx, &results, testcase, "DS10_ERR_MULT_NSEC", args); err != nil {
			return results, err
		}
	}
	if len(erroneousMultipleNSEC3) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, erroneousMultipleNSEC3)
		if err := appendLog(ctx, &results, testcase, "DS10_ERR_MULT_NSEC3", args); err != nil {
			return results, err
		}
	}
	if len(erroneousMultipleNSEC3PARAM) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, erroneousMultipleNSEC3PARAM)
		if err := appendLog(ctx, &results, testcase, "DS10_ERR_MULT_NSEC3PARAM", args); err != nil {
			return results, err
		}
	}

	diff := symmetricDifferenceStrings(nsecInAnswer, nsec3paramNsecNodata)
	union := uniqueStrings(append(nsec3paramInAnswer, nsecNsec3Nodata...))
	finalDiff := symmetricDifferenceStrings(diff, union)
	if len(diff) > 0 && len(finalDiff) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, finalDiff)
		if err := appendLog(ctx, &results, testcase, "DS10_INCONSISTENT_NSEC", args); err != nil {
			return results, err
		}
	}

	diff = symmetricDifferenceStrings(nsec3paramInAnswer, nsecNsec3Nodata)
	union = uniqueStrings(append(nsecInAnswer, nsec3paramNsecNodata...))
	finalDiff = symmetricDifferenceStrings(diff, union)
	if len(diff) > 0 && len(finalDiff) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, finalDiff)
		if err := appendLog(ctx, &results, testcase, "DS10_INCONSISTENT_NSEC3", args); err != nil {
			return results, err
		}
	}

	intersection := intersectionStrings(append(nsec3paramInAnswer, nsecNsec3Nodata...), append(nsecInAnswer, nsec3paramNsecNodata...))
	if len(intersection) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, intersection)
		if err := appendLog(ctx, &results, testcase, "DS10_MIXED_NSEC_NSEC3", args); err != nil {
			return results, err
		}
	}

	if (len(nsecInAnswer) > 0 || len(nsec3paramNsecNodata) > 0) && len(nsec3paramInAnswer) == 0 && len(nsecNsec3Nodata) == 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, append(nsecInAnswer, nsec3paramNsecNodata...))
		if err := appendLog(ctx, &results, testcase, "DS10_HAS_NSEC", args); err != nil {
			return results, err
		}
	}

	if (len(nsec3paramInAnswer) > 0 || len(nsecNsec3Nodata) > 0) && len(nsecInAnswer) == 0 && len(nsec3paramNsecNodata) == 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, append(nsec3paramInAnswer, nsecNsec3Nodata...))
		if err := appendLog(ctx, &results, testcase, "DS10_HAS_NSEC3", args); err != nil {
			return results, err
		}
	}

	union = uniqueStrings(append(nsec3paramInAnswer, nsecNsec3Nodata...))
	secondUnion := uniqueStrings(append(nsecInAnswer, nsec3paramNsecNodata...))
	first := differenceStrings(union, secondUnion)
	second := differenceStrings(secondUnion, union)
	if len(first) > 0 && len(second) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, append(union, secondUnion...))
		if err := appendLog(ctx, &results, testcase, "DS10_INCONSISTENT_NSEC_NSEC3", args); err != nil {
			return results, err
		}
	}

	if len(nsecIncorrectTypeList) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsecIncorrectTypeList)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC_ERR_TYPE_LIST", args); err != nil {
			return results, err
		}
	}
	if len(nsecMismatchesApex) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsecMismatchesApex)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC_MISMATCHES_APEX", args); err != nil {
			return results, err
		}
	}
	if len(nsecNodataWrongSOA) > 0 {
		keys := make([]string, 0, len(nsecNodataWrongSOA))
		for key := range nsecNodataWrongSOA {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args := map[string]any{
				"domain": key,
			}
			setTypedServersFromNames(args, nsecNodataWrongSOA[key])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC_NODATA_WRONG_SOA", args); err != nil {
				return results, err
			}
		}
	}
	if len(nsecNodataMissingSOA) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsecNodataMissingSOA)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC_NODATA_MISSING_SOA", args); err != nil {
			return results, err
		}
	}
	if len(nsecErroneousAnswer) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsecErroneousAnswer)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC_GIVES_ERR_ANSWER", args); err != nil {
			return results, err
		}
	}
	if len(nsecResponseError) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsecResponseError)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC_QUERY_RESPONSE_ERR", args); err != nil {
			return results, err
		}
	}
	if len(nsec3IncorrectTypeList) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsec3IncorrectTypeList)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_ERR_TYPE_LIST", args); err != nil {
			return results, err
		}
	}
	if len(nsec3MismatchesApex) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsec3MismatchesApex)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_MISMATCHES_APEX", args); err != nil {
			return results, err
		}
	}
	if len(nsec3NodataWrongSOA) > 0 {
		keys := make([]string, 0, len(nsec3NodataWrongSOA))
		for key := range nsec3NodataWrongSOA {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args := map[string]any{
				"domain": key,
			}
			setTypedServersFromNames(args, nsec3NodataWrongSOA[key])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_NODATA_WRONG_SOA", args); err != nil {
				return results, err
			}
		}
	}
	if len(nsec3NodataMissingSOA) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsec3NodataMissingSOA)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_NODATA_MISSING_SOA", args); err != nil {
			return results, err
		}
	}
	if len(nsec3paramErroneousAnswer) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsec3paramErroneousAnswer)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC3PARAM_GIVES_ERR_ANSWER", args); err != nil {
			return results, err
		}
	}
	if len(nsec3paramMismatchesApex) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsec3paramMismatchesApex)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC3PARAM_MISMATCHES_APEX", args); err != nil {
			return results, err
		}
	}
	if len(nsec3paramResponseError) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsec3paramResponseError)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC3PARAM_QUERY_RESPONSE_ERR", args); err != nil {
			return results, err
		}
	}
	if len(nsecMissingSignature) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsecMissingSignature)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC_MISSING_SIGNATURE", args); err != nil {
			return results, err
		}
	}
	if len(nsec3MissingSignature) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, nsec3MissingSignature)
		if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_MISSING_SIGNATURE", args); err != nil {
			return results, err
		}
	}

	if len(nsecRRSIGNoDNSKEY) > 0 {
		keytags := make([]int, 0, len(nsecRRSIGNoDNSKEY))
		for keytag := range nsecRRSIGNoDNSKEY {
			keytags = append(keytags, int(keytag))
		}
		sort.Ints(keytags)
		for _, keytag := range keytags {
			kt := uint16(keytag)
			args := map[string]any{
				"keytag": kt,
			}
			setTypedServersFromNames(args, nsecRRSIGNoDNSKEY[kt])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC_RRSIG_NO_DNSKEY", args); err != nil {
				return results, err
			}
		}
	}
	if len(nsecRRSIGExpired) > 0 {
		keytags := make([]int, 0, len(nsecRRSIGExpired))
		for keytag := range nsecRRSIGExpired {
			keytags = append(keytags, int(keytag))
		}
		sort.Ints(keytags)
		for _, keytag := range keytags {
			kt := uint16(keytag)
			args := map[string]any{
				"keytag": kt,
			}
			setTypedServersFromNames(args, nsecRRSIGExpired[kt])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC_RRSIG_EXPIRED", args); err != nil {
				return results, err
			}
		}
	}
	if len(nsecRRSIGNotYetValid) > 0 {
		keytags := make([]int, 0, len(nsecRRSIGNotYetValid))
		for keytag := range nsecRRSIGNotYetValid {
			keytags = append(keytags, int(keytag))
		}
		sort.Ints(keytags)
		for _, keytag := range keytags {
			kt := uint16(keytag)
			args := map[string]any{
				"keytag": kt,
			}
			setTypedServersFromNames(args, nsecRRSIGNotYetValid[kt])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC_RRSIG_NOT_YET_VALID", args); err != nil {
				return results, err
			}
		}
	}
	if len(nsecRRSIGVerifyError) > 0 {
		keytags := make([]int, 0, len(nsecRRSIGVerifyError))
		for keytag := range nsecRRSIGVerifyError {
			keytags = append(keytags, int(keytag))
		}
		sort.Ints(keytags)
		for _, keytag := range keytags {
			kt := uint16(keytag)
			args := map[string]any{
				"keytag": kt,
			}
			setTypedServersFromNames(args, nsecRRSIGVerifyError[kt])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC_RRSIG_VERIFY_ERROR", args); err != nil {
				return results, err
			}
		}
	}

	if len(nsecRRSIGNoDNSKEY) > 0 || len(nsecRRSIGExpired) > 0 || len(nsecRRSIGNotYetValid) > 0 || len(nsecRRSIGVerifyError) > 0 {
		verifiedSet := map[string]bool{}
		for _, ns := range nsecRRSIGVerified {
			verifiedSet[ns] = true
		}
		var nsList []string
		for _, nsSlice := range nsecRRSIGNoDNSKEY {
			for _, ns := range nsSlice {
				if !verifiedSet[ns] {
					nsList = append(nsList, ns)
				}
			}
		}
		for _, nsSlice := range nsecRRSIGExpired {
			for _, ns := range nsSlice {
				if !verifiedSet[ns] {
					nsList = append(nsList, ns)
				}
			}
		}
		for _, nsSlice := range nsecRRSIGNotYetValid {
			for _, ns := range nsSlice {
				if !verifiedSet[ns] {
					nsList = append(nsList, ns)
				}
			}
		}
		for _, nsSlice := range nsecRRSIGVerifyError {
			for _, ns := range nsSlice {
				if !verifiedSet[ns] {
					nsList = append(nsList, ns)
				}
			}
		}
		if len(nsList) > 0 {
			args := map[string]any{}
			setTypedServersFromNames(args, nsList)
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC_NO_VERIFIED_SIGNATURE", args); err != nil {
				return results, err
			}
		}
	}

	if len(nsec3RRSIGNoDNSKEY) > 0 {
		keytags := make([]int, 0, len(nsec3RRSIGNoDNSKEY))
		for keytag := range nsec3RRSIGNoDNSKEY {
			keytags = append(keytags, int(keytag))
		}
		sort.Ints(keytags)
		for _, keytag := range keytags {
			kt := uint16(keytag)
			args := map[string]any{
				"keytag": kt,
			}
			setTypedServersFromNames(args, nsec3RRSIGNoDNSKEY[kt])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_RRSIG_NO_DNSKEY", args); err != nil {
				return results, err
			}
		}
	}
	if len(nsec3RRSIGExpired) > 0 {
		keytags := make([]int, 0, len(nsec3RRSIGExpired))
		for keytag := range nsec3RRSIGExpired {
			keytags = append(keytags, int(keytag))
		}
		sort.Ints(keytags)
		for _, keytag := range keytags {
			kt := uint16(keytag)
			args := map[string]any{
				"keytag": kt,
			}
			setTypedServersFromNames(args, nsec3RRSIGExpired[kt])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_RRSIG_EXPIRED", args); err != nil {
				return results, err
			}
		}
	}
	if len(nsec3RRSIGNotYetValid) > 0 {
		keytags := make([]int, 0, len(nsec3RRSIGNotYetValid))
		for keytag := range nsec3RRSIGNotYetValid {
			keytags = append(keytags, int(keytag))
		}
		sort.Ints(keytags)
		for _, keytag := range keytags {
			kt := uint16(keytag)
			args := map[string]any{
				"keytag": kt,
			}
			setTypedServersFromNames(args, nsec3RRSIGNotYetValid[kt])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_RRSIG_NOT_YET_VALID", args); err != nil {
				return results, err
			}
		}
	}
	if len(nsec3RRSIGVerifyError) > 0 {
		keytags := make([]int, 0, len(nsec3RRSIGVerifyError))
		for keytag := range nsec3RRSIGVerifyError {
			keytags = append(keytags, int(keytag))
		}
		sort.Ints(keytags)
		for _, keytag := range keytags {
			kt := uint16(keytag)
			args := map[string]any{
				"keytag": kt,
			}
			setTypedServersFromNames(args, nsec3RRSIGVerifyError[kt])
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_RRSIG_VERIFY_ERROR", args); err != nil {
				return results, err
			}
		}
	}

	if len(nsec3RRSIGNoDNSKEY) > 0 || len(nsec3RRSIGExpired) > 0 || len(nsec3RRSIGNotYetValid) > 0 || len(nsec3RRSIGVerifyError) > 0 {
		verifiedSet := map[string]bool{}
		for _, ns := range nsec3RRSIGVerified {
			verifiedSet[ns] = true
		}
		var nsList []string
		for _, nsSlice := range nsec3RRSIGNoDNSKEY {
			for _, ns := range nsSlice {
				if !verifiedSet[ns] {
					nsList = append(nsList, ns)
				}
			}
		}
		for _, nsSlice := range nsec3RRSIGExpired {
			for _, ns := range nsSlice {
				if !verifiedSet[ns] {
					nsList = append(nsList, ns)
				}
			}
		}
		for _, nsSlice := range nsec3RRSIGNotYetValid {
			for _, ns := range nsSlice {
				if !verifiedSet[ns] {
					nsList = append(nsList, ns)
				}
			}
		}
		for _, nsSlice := range nsec3RRSIGVerifyError {
			for _, ns := range nsSlice {
				if !verifiedSet[ns] {
					nsList = append(nsList, ns)
				}
			}
		}
		if len(nsList) > 0 {
			args := map[string]any{}
			setTypedServersFromNames(args, nsList)
			if err := appendLog(ctx, &results, testcase, "DS10_NSEC3_NO_VERIFIED_SIGNATURE", args); err != nil {
				return results, err
			}
		}
	}

	if len(algoNotSupportedByZM) > 0 {
		keytags := make([]int, 0, len(algoNotSupportedByZM))
		for keytag := range algoNotSupportedByZM {
			keytags = append(keytags, int(keytag))
		}
		sort.Ints(keytags)
		for _, keytag := range keytags {
			kt := uint16(keytag)
			algoMap := algoNotSupportedByZM[kt]
			algos := make([]int, 0, len(algoMap))
			for algo := range algoMap {
				algos = append(algos, int(algo))
			}
			sort.Ints(algos)
			for _, algoKey := range algos {
				algo := uint8(algoKey)
				prop := algoPropertyFor(algo)
				args := map[string]any{
					"keytag":     kt,
					"algo_num":   algo,
					"algo_mnemo": prop.mnemonic,
				}
				setTypedAddressesFromValues(args, algoMap[algo])
				if err := appendLog(ctx, &results, testcase, "DS10_ALGO_NOT_SUPPORTED_BY_ZM", args); err != nil {
					return results, err
				}
			}
		}
	}

	if len(withDNSKEY) == 0 && len(withoutDNSKEY) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, withoutDNSKEY)
		if err := appendLog(ctx, &results, testcase, "DS10_ZONE_NO_DNSSEC", args); err != nil {
			return results, err
		}
	}
	if len(withDNSKEY) > 0 && len(withoutDNSKEY) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, withoutDNSKEY)
		if err := appendLog(ctx, &results, testcase, "DS10_SERVER_NO_DNSSEC", args); err != nil {
			return results, err
		}
	}

	combined := uniqueStrings(append(append(append(append(append(ignoredNS, withoutDNSKEY...), nsecInAnswer...), nsec3paramNsecNodata...), nsec3paramInAnswer...), nsecNsec3Nodata...))
	missing := differenceStrings(allNS, combined)
	if len(missing) > 0 {
		args := map[string]any{}
		setTypedServersFromNames(args, missing)
		if err := appendLog(ctx, &results, testcase, "DS10_EXPECTED_NSEC_NSEC3_MISSING", args); err != nil {
			return results, err
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC11 runs the DNSSEC11 test case.
func DNSSEC11(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC11"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var undeterminedDS []string
	var noDSRecord []string
	var hasDSRecord []string
	continueWithChildTests := true

	parentNS, err := parentNameservers(ctx, z)
	if err != nil {
		return results, err
	}

	nss := map[string]nameserver.Nameserver{}
	for _, ns := range parentNS {
		nss[ns.String()] = ns
	}

	keys := make([]string, 0, len(nss))
	for key := range nss {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	isUndelegated := hasFakeAddresses(z)
	if isUndelegated {
		for _, key := range keys {
			ns := nss[key]
			if len(ns.FakeDSRecords(z.Name.String())) == 0 {
				if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
					return results, err
				}
				return results, nil
			}
			break
		}
	}

	var ordered []nameserver.Nameserver
	ipAlreadyProcessed := map[string]bool{}
	for _, key := range keys {
		ns := nss[key]
		nsIP := ns.Address.String()
		if ipAlreadyProcessed[nsIP] {
			continue
		}
		ipAlreadyProcessed[nsIP] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		type parentOutcome struct {
			nsIP         string
			undetermined bool
			noDS         bool
			hasDS        bool
		}

		outcomes := make([]parentOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := parentOutcome{nsIP: ns.Address.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DS"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				useVC := false
				dsResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DS", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if dsResp.TC() {
					useVC = true
					dsResp, _ = ns.QueryWithOptions(ctx, z.Name.String(), "DS", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				}

				if dsResp.Msg == nil || dsResp.Rcode() != "NOERROR" || !dsResp.AA() {
					outcome.undetermined = true
					outcomes[i] = outcome
					return nil
				}

				dsRRs := dsResp.GetRecordsForName("DS", z.Name, "answer")
				if len(dsRRs) == 0 {
					outcome.noDS = true
				} else {
					outcome.hasDS = true
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
			if outcome.undetermined {
				undeterminedDS = append(undeterminedDS, outcome.nsIP)
			}
			if outcome.noDS {
				noDSRecord = append(noDSRecord, outcome.nsIP)
			}
			if outcome.hasDS {
				hasDSRecord = append(hasDSRecord, outcome.nsIP)
			}
		}
	}

	if len(undeterminedDS) > 0 && len(noDSRecord) == 0 && len(hasDSRecord) == 0 {
		if err := appendLog(ctx, &results, testcase, "DS11_UNDETERMINED_DS", map[string]any{}); err != nil {
			return results, err
		}
		continueWithChildTests = false
	} else if len(noDSRecord) > 0 && len(hasDSRecord) == 0 {
		continueWithChildTests = false
	} else if len(noDSRecord) > 0 && len(hasDSRecord) > 0 {
		if err := appendLog(ctx, &results, testcase, "DS11_INCONSISTENT_DS", map[string]any{}); err != nil {
			return results, err
		}
		args := map[string]any{}
		setTypedAddressesFromValues(args, noDSRecord)
		if err := appendLog(ctx, &results, testcase, "DS11_PARENT_WITHOUT_DS", args); err != nil {
			return results, err
		}
		args = map[string]any{}
		setTypedAddressesFromValues(args, hasDSRecord)
		if err := appendLog(ctx, &results, testcase, "DS11_PARENT_WITH_DS", args); err != nil {
			return results, err
		}
	}

	if continueWithChildTests {
		queryTypes := []string{"SOA", "DNSKEY"}
		var undeterminedDNSKEY []string
		var noDNSKEYRecord []string
		var hasDNSKEYRecord []string

		nssDel, err := method4(ctx, z)
		if err != nil {
			return results, err
		}
		nssChild, err := method5(ctx, z)
		if err != nil {
			return results, err
		}

		childNSS := map[string]nameserver.Nameserver{}
		for _, ns := range append(nssDel, nssChild...) {
			childNSS[ns.String()] = ns
		}

		childKeys := make([]string, 0, len(childNSS))
		for key := range childNSS {
			childKeys = append(childKeys, key)
		}
		sort.Strings(childKeys)

		var ordered []nameserver.Nameserver
		ipAlreadyProcessed = map[string]bool{}
		for _, key := range childKeys {
			ns := childNSS[key]
			nsIP := ns.Address.String()
			if ipAlreadyProcessed[nsIP] {
				continue
			}
			ipAlreadyProcessed[nsIP] = true
			ordered = append(ordered, ns)
		}

		if len(ordered) > 0 {
			type childOutcome struct {
				nsIP         string
				undetermined bool
				noDNSKEY     bool
				hasDNSKEY    bool
			}

			outcomes := make([]childOutcome, len(ordered))
			tasks := make([]runner.Task, len(ordered))
			for i, ns := range ordered {
				i, ns := i, ns
				tasks[i] = func(ctx context.Context, log *logger.Logger) error {
					buf := testlogger.Wrap(log, moduleName, testcase)
					outcome := childOutcome{nsIP: ns.Address.String()}

					if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryTypes...); err != nil {
						return err
					} else if disabled {
						outcomes[i] = outcome
						return nil
					}

					useVC := false
					soaResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "SOA", &nameserver.QueryOptions{UseVC: &useVC})
					if soaResp.Msg == nil || soaResp.Rcode() != "NOERROR" || !soaResp.AA() {
						outcomes[i] = outcome
						return nil
					}
					if len(soaResp.GetRecordsForName("SOA", z.Name, "answer")) == 0 {
						outcomes[i] = outcome
						return nil
					}

					useVC = false
					dnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{UseVC: &useVC})
					if dnskeyResp.TC() {
						useVC = true
						dnskeyResp, _ = ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{UseVC: &useVC})
					}

					if dnskeyResp.Msg == nil || dnskeyResp.Rcode() != "NOERROR" || !dnskeyResp.AA() {
						outcome.undetermined = true
						outcomes[i] = outcome
						return nil
					}

					dnskeyRRs := dnskeyResp.GetRecordsForName("DNSKEY", z.Name, "answer")
					if len(dnskeyRRs) == 0 {
						outcome.noDNSKEY = true
					} else {
						outcome.hasDNSKEY = true
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
				if outcome.undetermined {
					undeterminedDNSKEY = append(undeterminedDNSKEY, outcome.nsIP)
				}
				if outcome.noDNSKEY {
					noDNSKEYRecord = append(noDNSKEYRecord, outcome.nsIP)
				}
				if outcome.hasDNSKEY {
					hasDNSKEYRecord = append(hasDNSKEYRecord, outcome.nsIP)
				}
			}
		}

		if len(undeterminedDNSKEY) > 0 && len(noDNSKEYRecord) == 0 && len(hasDNSKEYRecord) == 0 {
			if err := appendLog(ctx, &results, testcase, "DS11_UNDETERMINED_SIGNED_ZONE", map[string]any{}); err != nil {
				return results, err
			}
		} else if len(noDNSKEYRecord) > 0 && len(hasDNSKEYRecord) == 0 {
			if err := appendLog(ctx, &results, testcase, "DS11_DS_BUT_UNSIGNED_ZONE", map[string]any{}); err != nil {
				return results, err
			}
		} else if len(noDNSKEYRecord) > 0 && len(hasDNSKEYRecord) > 0 {
			if err := appendLog(ctx, &results, testcase, "DS11_INCONSISTENT_SIGNED_ZONE", map[string]any{}); err != nil {
				return results, err
			}
			args := map[string]any{}
			setTypedAddressesFromValues(args, noDNSKEYRecord)
			if err := appendLog(ctx, &results, testcase, "DS11_NS_WITH_UNSIGNED_ZONE", args); err != nil {
				return results, err
			}
			args = map[string]any{}
			setTypedAddressesFromValues(args, hasDNSKEYRecord)
			if err := appendLog(ctx, &results, testcase, "DS11_NS_WITH_SIGNED_ZONE", args); err != nil {
				return results, err
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC13 runs the DNSSEC13 test case.
func DNSSEC13(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC13"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	queryTypes := []string{"DNSKEY", "SOA", "NS"}
	algoNotSigned := map[string]map[uint8][]string{}

	nssDel, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	nssChild, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	nss := map[string]nameserver.Nameserver{}
	for _, ns := range append(nssDel, nssChild...) {
		nss[ns.String()] = ns
	}

	keys := make([]string, 0, len(nss))
	for key := range nss {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var ordered []nameserver.Nameserver
	ipAlreadyProcessed := map[string]bool{}
	for _, key := range keys {
		ns := nss[key]
		nsIP := ns.Address.String()
		if ipAlreadyProcessed[nsIP] {
			continue
		}
		ipAlreadyProcessed[nsIP] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		type nsOutcome struct {
			nsIP          string
			algoNotSigned map[string]map[uint8]bool
		}

		outcomes := make([]nsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{
					nsIP:          ns.Address.String(),
					algoNotSigned: map[string]map[uint8]bool{},
				}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryTypes...); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnskeyAlgorithms := map[uint8]bool{}
				for _, queryType := range queryTypes {
					dnssecOn := true
					useVC := false
					resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), queryType, &nameserver.QueryOptions{
						DNSSEC: &dnssecOn,
						UseVC:  &useVC,
					})
					if resp.Msg == nil || resp.Rcode() != "NOERROR" || !resp.AA() {
						continue
					}

					typeRecords := resp.GetRecords(queryType, "answer")
					if len(typeRecords) == 0 {
						continue
					}

					rrsigRecords := resp.GetRecords("RRSIG", "answer")
					if len(rrsigRecords) == 0 {
						continue
					}

					if queryType == "DNSKEY" {
						for _, rr := range typeRecords {
							if dnskey, ok := rr.(*dns.DNSKEY); ok {
								dnskeyAlgorithms[dnskey.Algorithm] = true
							}
						}
					}

					if len(dnskeyAlgorithms) == 0 {
						continue
					}

					for algorithm := range dnskeyAlgorithms {
						found := false
						for _, rr := range rrsigRecords {
							sig, ok := rr.(*dns.RRSIG)
							if !ok {
								continue
							}
							if sig.Algorithm == algorithm {
								found = true
								break
							}
						}
						if found {
							continue
						}
						key := strings.ToLower(queryType)
						if outcome.algoNotSigned[key] == nil {
							outcome.algoNotSigned[key] = map[uint8]bool{}
						}
						outcome.algoNotSigned[key][algorithm] = true
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

		for _, outcome := range outcomes {
			for key, algoMap := range outcome.algoNotSigned {
				if algoNotSigned[key] == nil {
					algoNotSigned[key] = map[uint8][]string{}
				}
				for algo := range algoMap {
					algoNotSigned[key][algo] = append(algoNotSigned[key][algo], outcome.nsIP)
				}
			}
		}
	}

	for _, queryType := range queryTypes {
		key := strings.ToLower(queryType)
		if len(algoNotSigned[key]) == 0 {
			continue
		}
		algos := make([]int, 0, len(algoNotSigned[key]))
		for algo := range algoNotSigned[key] {
			algos = append(algos, int(algo))
		}
		sort.Ints(algos)
		for _, algoKey := range algos {
			algo := uint8(algoKey)
			prop := algoPropertyFor(algo)
			args := map[string]any{
				"algo_num":   algo,
				"algo_mnemo": prop.mnemonic,
			}
			setTypedAddressesFromValues(args, algoNotSigned[key][algo])
			if err := appendLog(ctx, &results, testcase, "DS13_ALGO_NOT_SIGNED_"+queryType, args); err != nil {
				return results, err
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC14 runs the DNSSEC14 test case.
func DNSSEC14(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC14"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var dnskeyRRs []*dns.DNSKEY

	nssDel, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	nssChild, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	nss := map[string]nameserver.Nameserver{}
	for _, ns := range append(nssDel, nssChild...) {
		nss[ns.String()] = ns
	}

	keys := make([]string, 0, len(nss))
	for key := range nss {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var ordered []nameserver.Nameserver
	for _, key := range keys {
		ordered = append(ordered, nss[key])
	}

	if len(ordered) > 0 {
		type nsOutcome struct {
			dnskeyRRs []*dns.DNSKEY
		}

		outcomes := make([]nsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DNSKEY"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				useVC := false
				resp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if resp.Msg == nil {
					if _, err := buf.Add("NO_RESPONSE", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				keyRecords := resp.GetRecords("DNSKEY", "answer")
				if len(keyRecords) == 0 {
					if _, err := buf.Add("NO_RESPONSE_DNSKEY", withNameserverArgs(ns, nil)); err != nil {
						return err
					}
					outcomes[i] = outcome
					return nil
				}

				for _, rr := range keyRecords {
					if dnskey, ok := rr.(*dns.DNSKEY); ok {
						outcome.dnskeyRRs = append(outcome.dnskeyRRs, dnskey)
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

		for _, outcome := range outcomes {
			dnskeyRRs = append(dnskeyRRs, outcome.dnskeyRRs...)
		}
	}

	investigatedKeys := map[string]bool{}
	for _, key := range dnskeyRRs {
		algo := key.Algorithm
		details, ok := rsaKeySizeByAlgo[algo]
		if !ok {
			continue
		}

		keysize := dnskeyKeySize(key)
		keytag := key.KeyTag()
		keyRef := strconv.Itoa(int(keytag)) + ":" + strconv.Itoa(keysize) + ":" + strconv.Itoa(int(algo))
		if investigatedKeys[keyRef] {
			continue
		}

		prop := algoPropertyFor(algo)
		args := map[string]any{
			"algo_num":   algo,
			"algo_descr": prop.description,
			"keytag":     keytag,
			"keysize":    keysize,
			"keysizemin": details.minSize,
			"keysizemax": details.maxSize,
			"keysizerec": details.recSize,
		}

		if keysize < details.minSize {
			if err := appendLog(ctx, &results, testcase, "DNSKEY_TOO_SMALL_FOR_ALGO", args); err != nil {
				return results, err
			}
		}

		if keysize < details.recSize {
			if err := appendLog(ctx, &results, testcase, "DNSKEY_SMALLER_THAN_REC", args); err != nil {
				return results, err
			}
		}

		if keysize > details.maxSize {
			if err := appendLog(ctx, &results, testcase, "DNSKEY_TOO_LARGE_FOR_ALGO", args); err != nil {
				return results, err
			}
		}

		investigatedKeys[keyRef] = true
	}

	if len(dnskeyRRs) > 0 {
		noResponseCount := 0
		for _, entry := range results {
			if entry == nil {
				continue
			}
			if entry.Tag == "NO_RESPONSE" {
				noResponseCount++
			}
		}
		if len(results) == noResponseCount {
			if err := appendLog(ctx, &results, testcase, "KEY_SIZE_OK", map[string]any{}); err != nil {
				return results, err
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC15 runs the DNSSEC15 test case.
func DNSSEC15(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC15"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	queryTypes := []string{"CDS", "CDNSKEY"}
	cdsRRsets := map[string][]dns.RR{}
	cdnskeyRRsets := map[string][]dns.RR{}
	mismatch := map[string]bool{}
	hasCDSNoCDNSKEY := map[string]bool{}
	hasCDNSKEYNoCDS := map[string]bool{}
	hasCDSAndCDNSKEY := map[string]bool{}

	nssDel, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	nssChild, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	nss := map[string]nameserver.Nameserver{}
	for _, ns := range append(nssDel, nssChild...) {
		nss[ns.String()] = ns
	}

	keys := make([]string, 0, len(nss))
	for key := range nss {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var ordered []nameserver.Nameserver
	ipAlreadyProcessed := map[string]bool{}
	for _, key := range keys {
		ns := nss[key]
		nsIP := ns.Address.String()
		if ipAlreadyProcessed[nsIP] {
			continue
		}
		ipAlreadyProcessed[nsIP] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		type nsOutcome struct {
			nsIP       string
			cdsRRs     []dns.RR
			cdnskeyRRs []dns.RR
			cdsOK      bool
			cdnskeyOK  bool
		}

		outcomes := make([]nsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{nsIP: ns.Address.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryTypes...); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				useVC := false
				cdsResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "CDS", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if cdsResp.Msg != nil && cdsResp.AA() && cdsResp.Rcode() == "NOERROR" {
					outcome.cdsOK = true
					outcome.cdsRRs = cdsResp.GetRecords("CDS", "answer")
				}

				useVC = false
				cdnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "CDNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if cdnskeyResp.Msg != nil && cdnskeyResp.AA() && cdnskeyResp.Rcode() == "NOERROR" {
					outcome.cdnskeyOK = true
					outcome.cdnskeyRRs = cdnskeyResp.GetRecords("CDNSKEY", "answer")
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
			if outcome.cdsOK {
				cdsRRsets[outcome.nsIP] = outcome.cdsRRs
			}
			if outcome.cdnskeyOK {
				cdnskeyRRsets[outcome.nsIP] = outcome.cdnskeyRRs
			}
		}
	}

	noCDSCDNSKEY := true
	for _, rrset := range cdsRRsets {
		if len(rrset) > 0 {
			noCDSCDNSKEY = false
			break
		}
	}
	if noCDSCDNSKEY {
		for _, rrset := range cdnskeyRRsets {
			if len(rrset) > 0 {
				noCDSCDNSKEY = false
				break
			}
		}
	}

	if noCDSCDNSKEY {
		if err := appendLog(ctx, &results, testcase, "DS15_NO_CDS_CDNSKEY", map[string]any{}); err != nil {
			return results, err
		}
	} else {
		for nsIP, cdsRRs := range cdsRRsets {
			cdnsRRs, ok := cdnskeyRRsets[nsIP]
			if !ok {
				continue
			}

			if len(cdsRRs) > 0 && len(cdnsRRs) == 0 {
				hasCDSNoCDNSKEY[nsIP] = true
			} else if len(cdnsRRs) > 0 && len(cdsRRs) == 0 {
				hasCDNSKEYNoCDS[nsIP] = true
			} else if len(cdnsRRs) > 0 && len(cdsRRs) > 0 {
				hasCDSAndCDNSKEY[nsIP] = true
			}
		}

		for nsIP, cdsRRs := range cdsRRsets {
			cdnsRRs, ok := cdnskeyRRsets[nsIP]
			if !ok || len(cdsRRs) == 0 || len(cdnsRRs) == 0 {
				continue
			}

			var cdsRecords []*dns.CDS
			for _, rr := range cdsRRs {
				if cds, ok := rr.(*dns.CDS); ok {
					cdsRecords = append(cdsRecords, cds)
				}
			}

			var cdnskeyRecords []*dns.CDNSKEY
			for _, rr := range cdnsRRs {
				if cdnskey, ok := rr.(*dns.CDNSKEY); ok {
					cdnskeyRecords = append(cdnskeyRecords, cdnskey)
				}
			}

			for _, cds := range cdsRecords {
				matched := false
				for _, cdnskey := range cdnskeyRecords {
					if cds.KeyTag == cdnskey.KeyTag() || (cds.Algorithm == 0 && cdnskey.Algorithm == 0) {
						matched = true
						break
					}
				}
				if !matched {
					mismatch[nsIP] = true
				}
			}

			for _, cdnskey := range cdnskeyRecords {
				matched := false
				for _, cds := range cdsRecords {
					if cdnskey.KeyTag() == cds.KeyTag || (cdnskey.Algorithm == 0 && cds.Algorithm == 0) {
						matched = true
						break
					}
				}
				if !matched {
					mismatch[nsIP] = true
				}
			}
		}

		if len(hasCDSNoCDNSKEY) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(hasCDSNoCDNSKEY))
			if err := appendLog(ctx, &results, testcase, "DS15_HAS_CDS_NO_CDNSKEY", args); err != nil {
				return results, err
			}
		}
		if len(hasCDNSKEYNoCDS) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(hasCDNSKEYNoCDS))
			if err := appendLog(ctx, &results, testcase, "DS15_HAS_CDNSKEY_NO_CDS", args); err != nil {
				return results, err
			}
		}
		if len(hasCDSAndCDNSKEY) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(hasCDSAndCDNSKEY))
			if err := appendLog(ctx, &results, testcase, "DS15_HAS_CDS_AND_CDNSKEY", args); err != nil {
				return results, err
			}
		}

		if rrsetInconsistent(cdsRRsets) {
			if err := appendLog(ctx, &results, testcase, "DS15_INCONSISTENT_CDS", map[string]any{}); err != nil {
				return results, err
			}
		}

		if rrsetInconsistent(cdnskeyRRsets) {
			if err := appendLog(ctx, &results, testcase, "DS15_INCONSISTENT_CDNSKEY", map[string]any{}); err != nil {
				return results, err
			}
		}

		if len(mismatch) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(mismatch))
			if err := appendLog(ctx, &results, testcase, "DS15_MISMATCH_CDS_CDNSKEY", args); err != nil {
				return results, err
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC16 runs the DNSSEC16 test case.
func DNSSEC16(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC16"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	queryTypes := []string{"CDS", "DNSKEY"}
	cdsRRsets := map[string][]*dns.CDS{}
	cdsRRSIG := map[string][]*dns.RRSIG{}
	dnskeyRRsets := map[string][]*dns.DNSKEY{}
	dnskeyRRSIG := map[string][]*dns.RRSIG{}
	noDNSKEYRRset := map[string]bool{}
	mixedDeleteCDS := map[string]bool{}
	deleteCDS := map[string]bool{}
	noMatchCDSWithDNSKEY := map[uint16][]string{}
	cdsPointsToNonZoneDNSKEY := map[uint16][]string{}
	cdsPointsToNonSEPDNSKEY := map[uint16][]string{}
	dnskeyNotSignedByCDS := map[uint16][]string{}
	cdsNotSignedByCDS := map[uint16][]string{}
	cdsNotSigned := map[string]bool{}
	cdsSignedByUnknownDNSKEY := map[uint16][]string{}
	cdsInvalidRRSIG := map[uint16][]string{}

	nssDel, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	nssChild, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	nss := map[string]nameserver.Nameserver{}
	for _, ns := range append(nssDel, nssChild...) {
		nss[ns.String()] = ns
	}

	keys := make([]string, 0, len(nss))
	for key := range nss {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	testingTime := time.Now().UTC()
	var ordered []nameserver.Nameserver
	ipAlreadyProcessed := map[string]bool{}
	for _, key := range keys {
		ns := nss[key]
		nsIP := ns.Address.String()
		if ipAlreadyProcessed[nsIP] {
			continue
		}
		ipAlreadyProcessed[nsIP] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		type nsOutcome struct {
			nsIP           string
			cdsRecords     []*dns.CDS
			cdsRRSIG       []*dns.RRSIG
			dnskeyRecords  []*dns.DNSKEY
			dnskeyRRSIG    []*dns.RRSIG
			testingTime    time.Time
			hasTestingTime bool
		}

		outcomes := make([]nsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{nsIP: ns.Address.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryTypes...); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				useVC := false
				cdsResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "CDS", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if cdsResp.Msg == nil || !cdsResp.AA() || cdsResp.Rcode() != "NOERROR" {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range cdsResp.GetRecords("CDS", "answer") {
					if cds, ok := rr.(*dns.CDS); ok {
						outcome.cdsRecords = append(outcome.cdsRecords, cds)
					}
				}
				if len(outcome.cdsRecords) == 0 {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range cdsResp.GetRecords("RRSIG", "answer") {
					if sig, ok := rr.(*dns.RRSIG); ok {
						outcome.cdsRRSIG = append(outcome.cdsRRSIG, sig)
					}
				}

				useVC = false
				dnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if dnskeyResp.Msg == nil || !dnskeyResp.AA() || dnskeyResp.Rcode() != "NOERROR" {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range dnskeyResp.GetRecords("DNSKEY", "answer") {
					if dnskey, ok := rr.(*dns.DNSKEY); ok {
						outcome.dnskeyRecords = append(outcome.dnskeyRecords, dnskey)
					}
				}
				if len(outcome.dnskeyRecords) == 0 {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range dnskeyResp.GetRecords("RRSIG", "answer") {
					if sig, ok := rr.(*dns.RRSIG); ok {
						outcome.dnskeyRRSIG = append(outcome.dnskeyRRSIG, sig)
					}
				}
				outcome.testingTime = packetTime(dnskeyResp)
				outcome.hasTestingTime = true
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
			if len(outcome.cdsRecords) > 0 {
				cdsRRsets[outcome.nsIP] = outcome.cdsRecords
				cdsRRSIG[outcome.nsIP] = outcome.cdsRRSIG
			}
			if len(outcome.dnskeyRecords) > 0 {
				dnskeyRRsets[outcome.nsIP] = outcome.dnskeyRecords
				dnskeyRRSIG[outcome.nsIP] = outcome.dnskeyRRSIG
				if outcome.hasTestingTime {
					testingTime = outcome.testingTime
				}
			}
		}
	}

	if len(cdsRRsets) > 0 {
		orderedIPs := make([]string, 0, len(ordered))
		seenIPs := map[string]bool{}
		for _, ns := range ordered {
			ip := ns.Address.String()
			orderedIPs = append(orderedIPs, ip)
			seenIPs[ip] = true
		}
		var extraIPs []string
		for nsIP := range cdsRRsets {
			if !seenIPs[nsIP] {
				extraIPs = append(extraIPs, nsIP)
			}
		}
		sort.Strings(extraIPs)
		orderedIPs = append(orderedIPs, extraIPs...)

		type validationOutcome struct {
			nsIP                     string
			noDNSKEYRRset            bool
			mixedDeleteCDS           bool
			deleteCDS                bool
			cdsNotSigned             bool
			noMatchCDSWithDNSKEY     map[uint16]bool
			cdsPointsToNonZoneDNSKEY map[uint16]bool
			cdsPointsToNonSEPDNSKEY  map[uint16]bool
			dnskeyNotSignedByCDS     map[uint16]bool
			cdsNotSignedByCDS        map[uint16]bool
			cdsSignedByUnknownDNSKEY map[uint16]bool
			cdsInvalidRRSIG          map[uint16]bool
		}

		var tasks []parallel.Task[validationOutcome]
		for _, nsIP := range orderedIPs {
			cdsRecords := cdsRRsets[nsIP]
			if len(cdsRecords) == 0 {
				continue
			}
			nsIP := nsIP
			tasks = append(tasks, func(_ context.Context) (validationOutcome, error) {
				outcome := validationOutcome{
					nsIP:                     nsIP,
					noMatchCDSWithDNSKEY:     map[uint16]bool{},
					cdsPointsToNonZoneDNSKEY: map[uint16]bool{},
					cdsPointsToNonSEPDNSKEY:  map[uint16]bool{},
					dnskeyNotSignedByCDS:     map[uint16]bool{},
					cdsNotSignedByCDS:        map[uint16]bool{},
					cdsSignedByUnknownDNSKEY: map[uint16]bool{},
					cdsInvalidRRSIG:          map[uint16]bool{},
				}

				hasDelete := false
				hasNonDelete := false
				for _, cds := range cdsRecords {
					if cds.Algorithm == 0 {
						hasDelete = true
					} else {
						hasNonDelete = true
					}
				}
				if hasDelete {
					if hasNonDelete {
						outcome.mixedDeleteCDS = true
					} else {
						outcome.deleteCDS = true
					}
					return outcome, nil
				}

				dnskeys := dnskeyRRsets[nsIP]
				if len(dnskeys) == 0 {
					outcome.noDNSKEYRRset = true
					return outcome, nil
				}

				for _, cds := range cdsRecords {
					if cds.Algorithm == 0 {
						continue
					}
					keytag := cds.KeyTag
					var matchingDNSKEYs []*dns.DNSKEY
					for _, dnskey := range dnskeys {
						if dnskey.KeyTag() == keytag {
							matchingDNSKEYs = append(matchingDNSKEYs, dnskey)
						}
					}
					if len(matchingDNSKEYs) == 0 {
						outcome.noMatchCDSWithDNSKEY[keytag] = true
						continue
					}
					hasNonZone := false
					for _, dnskey := range matchingDNSKEYs {
						if dnskey.Flags&dns.FlagZONE == 0 {
							hasNonZone = true
							break
						}
					}
					if hasNonZone {
						outcome.cdsPointsToNonZoneDNSKEY[keytag] = true
						continue
					}

					if !rrsigHasKeytag(dnskeyRRSIG[nsIP], keytag) {
						outcome.dnskeyNotSignedByCDS[keytag] = true
					}
					if !rrsigHasKeytag(cdsRRSIG[nsIP], keytag) {
						outcome.cdsNotSignedByCDS[keytag] = true
					}
					hasNonSEP := false
					for _, dnskey := range matchingDNSKEYs {
						if dnskey.Flags&dns.FlagSEP == 0 {
							hasNonSEP = true
							break
						}
					}
					if hasNonSEP {
						outcome.cdsPointsToNonSEPDNSKEY[keytag] = true
					}
				}

				if len(cdsRRSIG[nsIP]) == 0 {
					outcome.cdsNotSigned = true
				} else {
					rrset := make([]dns.RR, 0, len(cdsRecords))
					for _, cds := range cdsRecords {
						rrset = append(rrset, cds)
					}
					for _, sig := range cdsRRSIG[nsIP] {
						keytag := sig.KeyTag
						var matchingDNSKEYs []*dns.DNSKEY
						for _, dnskey := range dnskeys {
							if dnskey.KeyTag() == keytag {
								matchingDNSKEYs = append(matchingDNSKEYs, dnskey)
							}
						}
						if len(matchingDNSKEYs) == 0 {
							outcome.cdsSignedByUnknownDNSKEY[keytag] = true
							continue
						}
						valid := false
						for _, dnskey := range matchingDNSKEYs {
							if verifyRRSIG(sig, rrset, dnskey, testingTime) == nil {
								valid = true
								break
							}
						}
						if !valid {
							outcome.cdsInvalidRRSIG[keytag] = true
						}
					}
				}

				return outcome, nil
			})
		}

		if len(tasks) > 0 {
			parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
			validationResults := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: parallelism, CancelOnError: false})
			for _, res := range validationResults {
				if res.Err != nil {
					return results, res.Err
				}
				outcome := res.Value
				if outcome.noDNSKEYRRset {
					noDNSKEYRRset[outcome.nsIP] = true
				}
				if outcome.mixedDeleteCDS {
					mixedDeleteCDS[outcome.nsIP] = true
				}
				if outcome.deleteCDS {
					deleteCDS[outcome.nsIP] = true
				}
				if outcome.cdsNotSigned {
					cdsNotSigned[outcome.nsIP] = true
				}
				for keytag := range outcome.noMatchCDSWithDNSKEY {
					noMatchCDSWithDNSKEY[keytag] = append(noMatchCDSWithDNSKEY[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdsPointsToNonZoneDNSKEY {
					cdsPointsToNonZoneDNSKEY[keytag] = append(cdsPointsToNonZoneDNSKEY[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdsPointsToNonSEPDNSKEY {
					cdsPointsToNonSEPDNSKEY[keytag] = append(cdsPointsToNonSEPDNSKEY[keytag], outcome.nsIP)
				}
				for keytag := range outcome.dnskeyNotSignedByCDS {
					dnskeyNotSignedByCDS[keytag] = append(dnskeyNotSignedByCDS[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdsNotSignedByCDS {
					cdsNotSignedByCDS[keytag] = append(cdsNotSignedByCDS[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdsSignedByUnknownDNSKEY {
					cdsSignedByUnknownDNSKEY[keytag] = append(cdsSignedByUnknownDNSKEY[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdsInvalidRRSIG {
					cdsInvalidRRSIG[keytag] = append(cdsInvalidRRSIG[keytag], outcome.nsIP)
				}
			}
		}

		if len(noDNSKEYRRset) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(noDNSKEYRRset))
			if err := appendLog(ctx, &results, testcase, "DS16_CDS_WITHOUT_DNSKEY", args); err != nil {
				return results, err
			}
		}

		if len(mixedDeleteCDS) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(mixedDeleteCDS))
			if err := appendLog(ctx, &results, testcase, "DS16_MIXED_DELETE_CDS", args); err != nil {
				return results, err
			}
		}

		if len(deleteCDS) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(deleteCDS))
			if err := appendLog(ctx, &results, testcase, "DS16_DELETE_CDS", args); err != nil {
				return results, err
			}
		}

		if len(noMatchCDSWithDNSKEY) > 0 {
			keytags := make([]int, 0, len(noMatchCDSWithDNSKEY))
			for keytag := range noMatchCDSWithDNSKEY {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, noMatchCDSWithDNSKEY[kt])
				if err := appendLog(ctx, &results, testcase, "DS16_CDS_MATCHES_NO_DNSKEY", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdsPointsToNonZoneDNSKEY) > 0 {
			keytags := make([]int, 0, len(cdsPointsToNonZoneDNSKEY))
			for keytag := range cdsPointsToNonZoneDNSKEY {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdsPointsToNonZoneDNSKEY[kt])
				if err := appendLog(ctx, &results, testcase, "DS16_CDS_MATCHES_NON_ZONE_DNSKEY", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdsPointsToNonSEPDNSKEY) > 0 {
			keytags := make([]int, 0, len(cdsPointsToNonSEPDNSKEY))
			for keytag := range cdsPointsToNonSEPDNSKEY {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdsPointsToNonSEPDNSKEY[kt])
				if err := appendLog(ctx, &results, testcase, "DS16_CDS_MATCHES_NON_SEP_DNSKEY", args); err != nil {
					return results, err
				}
			}
		}

		if len(dnskeyNotSignedByCDS) > 0 {
			keytags := make([]int, 0, len(dnskeyNotSignedByCDS))
			for keytag := range dnskeyNotSignedByCDS {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, dnskeyNotSignedByCDS[kt])
				if err := appendLog(ctx, &results, testcase, "DS16_DNSKEY_NOT_SIGNED_BY_CDS", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdsNotSignedByCDS) > 0 {
			keytags := make([]int, 0, len(cdsNotSignedByCDS))
			for keytag := range cdsNotSignedByCDS {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdsNotSignedByCDS[kt])
				if err := appendLog(ctx, &results, testcase, "DS16_CDS_NOT_SIGNED_BY_CDS", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdsInvalidRRSIG) > 0 {
			keytags := make([]int, 0, len(cdsInvalidRRSIG))
			for keytag := range cdsInvalidRRSIG {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdsInvalidRRSIG[kt])
				if err := appendLog(ctx, &results, testcase, "DS16_CDS_INVALID_RRSIG", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdsNotSigned) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(cdsNotSigned))
			if err := appendLog(ctx, &results, testcase, "DS16_CDS_UNSIGNED", args); err != nil {
				return results, err
			}
		}

		if len(cdsSignedByUnknownDNSKEY) > 0 {
			keytags := make([]int, 0, len(cdsSignedByUnknownDNSKEY))
			for keytag := range cdsSignedByUnknownDNSKEY {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdsSignedByUnknownDNSKEY[kt])
				if err := appendLog(ctx, &results, testcase, "DS16_CDS_SIGNED_BY_UNKNOWN_DNSKEY", args); err != nil {
					return results, err
				}
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC17 runs the DNSSEC17 test case.
func DNSSEC17(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC17"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	queryTypes := []string{"CDNSKEY", "DNSKEY"}
	cdnskeyRRsets := map[string][]*dns.CDNSKEY{}
	cdnskeyRRSIG := map[string][]*dns.RRSIG{}
	dnskeyRRsets := map[string][]*dns.DNSKEY{}
	dnskeyRRSIG := map[string][]*dns.RRSIG{}
	noDNSKEYRRset := map[string]bool{}
	mixedDeleteCDNSKEY := map[string]bool{}
	deleteCDNSKEY := map[string]bool{}
	noMatchCDNSKEYWithDNSKEY := map[uint16][]string{}
	cdnskeyIsNonZone := map[uint16][]string{}
	cdnskeyIsNonSEP := map[uint16][]string{}
	dnskeyNotSignedByCDNSKEY := map[uint16][]string{}
	cdnskeyNotSignedByCDNSKEY := map[uint16][]string{}
	cdnskeyNotSigned := map[string]bool{}
	cdnskeySignedByUnknownDNSKEY := map[uint16][]string{}
	cdnskeyInvalidRRSIG := map[uint16][]string{}

	nssDel, err := method4(ctx, z)
	if err != nil {
		return results, err
	}
	nssChild, err := method5(ctx, z)
	if err != nil {
		return results, err
	}

	nss := map[string]nameserver.Nameserver{}
	for _, ns := range append(nssDel, nssChild...) {
		nss[ns.String()] = ns
	}

	keys := make([]string, 0, len(nss))
	for key := range nss {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	testingTime := time.Now().UTC()
	var ordered []nameserver.Nameserver
	ipAlreadyProcessed := map[string]bool{}
	for _, key := range keys {
		ns := nss[key]
		nsIP := ns.Address.String()
		if ipAlreadyProcessed[nsIP] {
			continue
		}
		ipAlreadyProcessed[nsIP] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		type nsOutcome struct {
			nsIP           string
			cdnskeyRecords []*dns.CDNSKEY
			cdnskeyRRSIG   []*dns.RRSIG
			dnskeyRecords  []*dns.DNSKEY
			dnskeyRRSIG    []*dns.RRSIG
			testingTime    time.Time
			hasTestingTime bool
		}

		outcomes := make([]nsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{nsIP: ns.Address.String()}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryTypes...); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				useVC := false
				cdnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "CDNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if cdnskeyResp.Msg == nil || !cdnskeyResp.AA() || cdnskeyResp.Rcode() != "NOERROR" {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range cdnskeyResp.GetRecords("CDNSKEY", "answer") {
					if cdnskey, ok := rr.(*dns.CDNSKEY); ok {
						outcome.cdnskeyRecords = append(outcome.cdnskeyRecords, cdnskey)
					}
				}
				if len(outcome.cdnskeyRecords) == 0 {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range cdnskeyResp.GetRecords("RRSIG", "answer") {
					if sig, ok := rr.(*dns.RRSIG); ok {
						outcome.cdnskeyRRSIG = append(outcome.cdnskeyRRSIG, sig)
					}
				}

				useVC = false
				dnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if dnskeyResp.Msg == nil || !dnskeyResp.AA() || dnskeyResp.Rcode() != "NOERROR" {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range dnskeyResp.GetRecords("DNSKEY", "answer") {
					if dnskey, ok := rr.(*dns.DNSKEY); ok {
						outcome.dnskeyRecords = append(outcome.dnskeyRecords, dnskey)
					}
				}
				if len(outcome.dnskeyRecords) == 0 {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range dnskeyResp.GetRecords("RRSIG", "answer") {
					if sig, ok := rr.(*dns.RRSIG); ok {
						outcome.dnskeyRRSIG = append(outcome.dnskeyRRSIG, sig)
					}
				}
				outcome.testingTime = packetTime(dnskeyResp)
				outcome.hasTestingTime = true
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
			if len(outcome.cdnskeyRecords) > 0 {
				cdnskeyRRsets[outcome.nsIP] = outcome.cdnskeyRecords
				cdnskeyRRSIG[outcome.nsIP] = outcome.cdnskeyRRSIG
			}
			if len(outcome.dnskeyRecords) > 0 {
				dnskeyRRsets[outcome.nsIP] = outcome.dnskeyRecords
				dnskeyRRSIG[outcome.nsIP] = outcome.dnskeyRRSIG
				if outcome.hasTestingTime {
					testingTime = outcome.testingTime
				}
			}
		}
	}

	if len(cdnskeyRRsets) > 0 {
		orderedIPs := make([]string, 0, len(ordered))
		seenIPs := map[string]bool{}
		for _, ns := range ordered {
			ip := ns.Address.String()
			orderedIPs = append(orderedIPs, ip)
			seenIPs[ip] = true
		}
		var extraIPs []string
		for nsIP := range cdnskeyRRsets {
			if !seenIPs[nsIP] {
				extraIPs = append(extraIPs, nsIP)
			}
		}
		sort.Strings(extraIPs)
		orderedIPs = append(orderedIPs, extraIPs...)

		type validationOutcome struct {
			nsIP                         string
			noDNSKEYRRset                bool
			mixedDeleteCDNSKEY           bool
			deleteCDNSKEY                bool
			cdnskeyNotSigned             bool
			noMatchCDNSKEYWithDNSKEY     map[uint16]bool
			cdnskeyIsNonZone             map[uint16]bool
			cdnskeyIsNonSEP              map[uint16]bool
			dnskeyNotSignedByCDNSKEY     map[uint16]bool
			cdnskeyNotSignedByCDNSKEY    map[uint16]bool
			cdnskeySignedByUnknownDNSKEY map[uint16]bool
			cdnskeyInvalidRRSIG          map[uint16]bool
		}

		var tasks []parallel.Task[validationOutcome]
		for _, nsIP := range orderedIPs {
			cdnskeyRecords := cdnskeyRRsets[nsIP]
			if len(cdnskeyRecords) == 0 {
				continue
			}
			nsIP := nsIP
			tasks = append(tasks, func(_ context.Context) (validationOutcome, error) {
				outcome := validationOutcome{
					nsIP:                         nsIP,
					noMatchCDNSKEYWithDNSKEY:     map[uint16]bool{},
					cdnskeyIsNonZone:             map[uint16]bool{},
					cdnskeyIsNonSEP:              map[uint16]bool{},
					dnskeyNotSignedByCDNSKEY:     map[uint16]bool{},
					cdnskeyNotSignedByCDNSKEY:    map[uint16]bool{},
					cdnskeySignedByUnknownDNSKEY: map[uint16]bool{},
					cdnskeyInvalidRRSIG:          map[uint16]bool{},
				}

				hasDelete := false
				hasNonDelete := false
				for _, cdnskey := range cdnskeyRecords {
					if cdnskey.Algorithm == 0 {
						hasDelete = true
					} else {
						hasNonDelete = true
					}
				}
				if hasDelete {
					if hasNonDelete {
						outcome.mixedDeleteCDNSKEY = true
					} else {
						outcome.deleteCDNSKEY = true
					}
					return outcome, nil
				}

				dnskeys := dnskeyRRsets[nsIP]
				if len(dnskeys) == 0 {
					outcome.noDNSKEYRRset = true
					return outcome, nil
				}

				for _, cdnskey := range cdnskeyRecords {
					if cdnskey.Algorithm == 0 {
						continue
					}
					keytag := cdnskey.KeyTag()
					if cdnskey.Flags&dns.FlagZONE == 0 {
						outcome.cdnskeyIsNonZone[keytag] = true
						continue
					}
					if cdnskey.Flags&dns.FlagSEP == 0 {
						outcome.cdnskeyIsNonSEP[keytag] = true
					}

					var matchingDNSKEYs []*dns.DNSKEY
					for _, dnskey := range dnskeys {
						if dnskey.KeyTag() == keytag {
							matchingDNSKEYs = append(matchingDNSKEYs, dnskey)
						}
					}
					if len(matchingDNSKEYs) == 0 {
						outcome.noMatchCDNSKEYWithDNSKEY[keytag] = true
						continue
					}

					if !rrsigHasKeytag(dnskeyRRSIG[nsIP], keytag) {
						outcome.dnskeyNotSignedByCDNSKEY[keytag] = true
					}
					if !rrsigHasKeytag(cdnskeyRRSIG[nsIP], keytag) {
						outcome.cdnskeyNotSignedByCDNSKEY[keytag] = true
					}
				}

				if len(cdnskeyRRSIG[nsIP]) == 0 {
					outcome.cdnskeyNotSigned = true
				} else {
					rrset := make([]dns.RR, 0, len(cdnskeyRecords))
					for _, cdnskey := range cdnskeyRecords {
						rrset = append(rrset, cdnskey)
					}
					for _, sig := range cdnskeyRRSIG[nsIP] {
						keytag := sig.KeyTag
						var matchingDNSKEYs []*dns.DNSKEY
						for _, dnskey := range dnskeys {
							if dnskey.KeyTag() == keytag {
								matchingDNSKEYs = append(matchingDNSKEYs, dnskey)
							}
						}
						if len(matchingDNSKEYs) == 0 {
							outcome.cdnskeySignedByUnknownDNSKEY[keytag] = true
							continue
						}
						valid := false
						for _, dnskey := range matchingDNSKEYs {
							if verifyRRSIG(sig, rrset, dnskey, testingTime) == nil {
								valid = true
								break
							}
						}
						if !valid {
							outcome.cdnskeyInvalidRRSIG[keytag] = true
						}
					}
				}

				return outcome, nil
			})
		}

		if len(tasks) > 0 {
			parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
			validationResults := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: parallelism, CancelOnError: false})
			for _, res := range validationResults {
				if res.Err != nil {
					return results, res.Err
				}
				outcome := res.Value
				if outcome.noDNSKEYRRset {
					noDNSKEYRRset[outcome.nsIP] = true
				}
				if outcome.mixedDeleteCDNSKEY {
					mixedDeleteCDNSKEY[outcome.nsIP] = true
				}
				if outcome.deleteCDNSKEY {
					deleteCDNSKEY[outcome.nsIP] = true
				}
				if outcome.cdnskeyNotSigned {
					cdnskeyNotSigned[outcome.nsIP] = true
				}
				for keytag := range outcome.noMatchCDNSKEYWithDNSKEY {
					noMatchCDNSKEYWithDNSKEY[keytag] = append(noMatchCDNSKEYWithDNSKEY[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdnskeyIsNonZone {
					cdnskeyIsNonZone[keytag] = append(cdnskeyIsNonZone[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdnskeyIsNonSEP {
					cdnskeyIsNonSEP[keytag] = append(cdnskeyIsNonSEP[keytag], outcome.nsIP)
				}
				for keytag := range outcome.dnskeyNotSignedByCDNSKEY {
					dnskeyNotSignedByCDNSKEY[keytag] = append(dnskeyNotSignedByCDNSKEY[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdnskeyNotSignedByCDNSKEY {
					cdnskeyNotSignedByCDNSKEY[keytag] = append(cdnskeyNotSignedByCDNSKEY[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdnskeySignedByUnknownDNSKEY {
					cdnskeySignedByUnknownDNSKEY[keytag] = append(cdnskeySignedByUnknownDNSKEY[keytag], outcome.nsIP)
				}
				for keytag := range outcome.cdnskeyInvalidRRSIG {
					cdnskeyInvalidRRSIG[keytag] = append(cdnskeyInvalidRRSIG[keytag], outcome.nsIP)
				}
			}
		}

		if len(noDNSKEYRRset) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(noDNSKEYRRset))
			if err := appendLog(ctx, &results, testcase, "DS17_CDNSKEY_WITHOUT_DNSKEY", args); err != nil {
				return results, err
			}
		}

		if len(mixedDeleteCDNSKEY) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(mixedDeleteCDNSKEY))
			if err := appendLog(ctx, &results, testcase, "DS17_MIXED_DELETE_CDNSKEY", args); err != nil {
				return results, err
			}
		}

		if len(deleteCDNSKEY) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(deleteCDNSKEY))
			if err := appendLog(ctx, &results, testcase, "DS17_DELETE_CDNSKEY", args); err != nil {
				return results, err
			}
		}

		if len(noMatchCDNSKEYWithDNSKEY) > 0 {
			keytags := make([]int, 0, len(noMatchCDNSKEYWithDNSKEY))
			for keytag := range noMatchCDNSKEYWithDNSKEY {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, noMatchCDNSKEYWithDNSKEY[kt])
				if err := appendLog(ctx, &results, testcase, "DS17_CDNSKEY_MATCHES_NO_DNSKEY", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdnskeyIsNonZone) > 0 {
			keytags := make([]int, 0, len(cdnskeyIsNonZone))
			for keytag := range cdnskeyIsNonZone {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdnskeyIsNonZone[kt])
				if err := appendLog(ctx, &results, testcase, "DS17_CDNSKEY_IS_NON_ZONE", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdnskeyIsNonSEP) > 0 {
			keytags := make([]int, 0, len(cdnskeyIsNonSEP))
			for keytag := range cdnskeyIsNonSEP {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdnskeyIsNonSEP[kt])
				if err := appendLog(ctx, &results, testcase, "DS17_CDNSKEY_IS_NON_SEP", args); err != nil {
					return results, err
				}
			}
		}

		if len(dnskeyNotSignedByCDNSKEY) > 0 {
			keytags := make([]int, 0, len(dnskeyNotSignedByCDNSKEY))
			for keytag := range dnskeyNotSignedByCDNSKEY {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, dnskeyNotSignedByCDNSKEY[kt])
				if err := appendLog(ctx, &results, testcase, "DS17_DNSKEY_NOT_SIGNED_BY_CDNSKEY", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdnskeyNotSignedByCDNSKEY) > 0 {
			keytags := make([]int, 0, len(cdnskeyNotSignedByCDNSKEY))
			for keytag := range cdnskeyNotSignedByCDNSKEY {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdnskeyNotSignedByCDNSKEY[kt])
				if err := appendLog(ctx, &results, testcase, "DS17_CDNSKEY_NOT_SIGNED_BY_CDNSKEY", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdnskeyInvalidRRSIG) > 0 {
			keytags := make([]int, 0, len(cdnskeyInvalidRRSIG))
			for keytag := range cdnskeyInvalidRRSIG {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdnskeyInvalidRRSIG[kt])
				if err := appendLog(ctx, &results, testcase, "DS17_CDNSKEY_INVALID_RRSIG", args); err != nil {
					return results, err
				}
			}
		}

		if len(cdnskeyNotSigned) > 0 {
			args := map[string]any{}
			setTypedAddressesFromValues(args, mapKeysSorted(cdnskeyNotSigned))
			if err := appendLog(ctx, &results, testcase, "DS17_CDNSKEY_UNSIGNED", args); err != nil {
				return results, err
			}
		}

		if len(cdnskeySignedByUnknownDNSKEY) > 0 {
			keytags := make([]int, 0, len(cdnskeySignedByUnknownDNSKEY))
			for keytag := range cdnskeySignedByUnknownDNSKEY {
				keytags = append(keytags, int(keytag))
			}
			sort.Ints(keytags)
			for _, keytag := range keytags {
				kt := uint16(keytag)
				args := map[string]any{
					"keytag": kt,
				}
				setTypedAddressesFromValues(args, cdnskeySignedByUnknownDNSKEY[kt])
				if err := appendLog(ctx, &results, testcase, "DS17_CDNSKEY_SIGNED_BY_UNKNOWN_DNSKEY", args); err != nil {
					return results, err
				}
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

// DNSSEC18 runs the DNSSEC18 test case.
func DNSSEC18(ctx context.Context, z *zone.Zone) ([]*logger.Entry, error) {
	const testcase = "DNSSEC18"
	var results []*logger.Entry

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_START", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}

	var dsRecords []*dns.DS
	dsNoMatchCDSRRSIG := map[string]bool{}
	dsNoMatchCDNSKEYRRSIG := map[string]bool{}

	parentNS, err := parentNameservers(ctx, z)
	if err != nil {
		return results, err
	}

	nss := map[string]nameserver.Nameserver{}
	for _, ns := range parentNS {
		nss[ns.String()] = ns
	}

	keys := make([]string, 0, len(nss))
	for key := range nss {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var ordered []nameserver.Nameserver
	ipAlreadyProcessed := map[string]bool{}
	for _, key := range keys {
		ns := nss[key]
		nsIP := ns.Address.String()
		if ipAlreadyProcessed[nsIP] {
			continue
		}
		ipAlreadyProcessed[nsIP] = true
		ordered = append(ordered, ns)
	}

	if len(ordered) > 0 {
		type nsOutcome struct {
			dsRecords []*dns.DS
		}

		outcomes := make([]nsOutcome, len(ordered))
		tasks := make([]runner.Task, len(ordered))
		for i, ns := range ordered {
			i, ns := i, ns
			tasks[i] = func(ctx context.Context, log *logger.Logger) error {
				buf := testlogger.Wrap(log, moduleName, testcase)
				outcome := nsOutcome{}

				if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, "DS"); err != nil {
					return err
				} else if disabled {
					outcomes[i] = outcome
					return nil
				}

				dnssecOn := true
				useVC := false
				dsResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DS", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
				if dsResp.Msg == nil || dsResp.Rcode() != "NOERROR" || !dsResp.AA() {
					outcomes[i] = outcome
					return nil
				}

				dsRRs := dsResp.GetRecordsForName("DS", z.Name, "answer")
				if len(dsRRs) == 0 {
					outcomes[i] = outcome
					return nil
				}
				for _, rr := range dsRRs {
					if ds, ok := rr.(*dns.DS); ok {
						if !containsDS(outcome.dsRecords, ds) {
							outcome.dsRecords = append(outcome.dsRecords, ds)
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

		for _, outcome := range outcomes {
			for _, ds := range outcome.dsRecords {
				if !containsDS(dsRecords, ds) {
					dsRecords = append(dsRecords, ds)
				}
			}
		}
	}

	if len(dsRecords) > 0 {
		queryTypes := []string{"CDNSKEY", "CDS", "DNSKEY"}
		cdsRRSIG := map[string][]*dns.RRSIG{}
		cdnskeyRRSIG := map[string][]*dns.RRSIG{}
		cdsRRsets := map[string]bool{}
		cdnskeyRRsets := map[string]bool{}
		dnskeyRRsets := map[string][]*dns.DNSKEY{}

		nssDel, err := method4(ctx, z)
		if err != nil {
			return results, err
		}
		nssChild, err := method5(ctx, z)
		if err != nil {
			return results, err
		}

		childNSS := map[string]nameserver.Nameserver{}
		for _, ns := range append(nssDel, nssChild...) {
			childNSS[ns.String()] = ns
		}

		childKeys := make([]string, 0, len(childNSS))
		for key := range childNSS {
			childKeys = append(childKeys, key)
		}
		sort.Strings(childKeys)

		var ordered []nameserver.Nameserver
		ipAlreadyProcessed = map[string]bool{}
		for _, key := range childKeys {
			ns := childNSS[key]
			nsIP := ns.Address.String()
			if ipAlreadyProcessed[nsIP] {
				continue
			}
			ipAlreadyProcessed[nsIP] = true
			ordered = append(ordered, ns)
		}

		if len(ordered) > 0 {
			type nsOutcome struct {
				nsIP            string
				cdsHasRRset     bool
				cdnskeyHasRRset bool
				cdsRRSIG        []*dns.RRSIG
				cdnskeyRRSIG    []*dns.RRSIG
				dnskeyRecords   []*dns.DNSKEY
			}

			outcomes := make([]nsOutcome, len(ordered))
			tasks := make([]runner.Task, len(ordered))
			for i, ns := range ordered {
				i, ns := i, ns
				tasks[i] = func(ctx context.Context, log *logger.Logger) error {
					buf := testlogger.Wrap(log, moduleName, testcase)
					outcome := nsOutcome{nsIP: ns.Address.String()}

					if disabled, err := ipDisabledMessageWithLogger(ctx, buf, ns, queryTypes...); err != nil {
						return err
					} else if disabled {
						outcomes[i] = outcome
						return nil
					}

					dnssecOn := true
					useVC := false
					cdsResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "CDS", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
					if cdsResp.Msg == nil || !cdsResp.AA() || cdsResp.Rcode() != "NOERROR" {
						outcomes[i] = outcome
						return nil
					}
					if len(cdsResp.GetRecords("CDS", "answer")) > 0 {
						outcome.cdsHasRRset = true
						for _, rr := range cdsResp.GetRecords("RRSIG", "answer") {
							if sig, ok := rr.(*dns.RRSIG); ok {
								outcome.cdsRRSIG = append(outcome.cdsRRSIG, sig)
							}
						}
					}

					useVC = false
					cdnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "CDNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
					if cdnskeyResp.Msg == nil || !cdnskeyResp.AA() || cdnskeyResp.Rcode() != "NOERROR" {
						outcomes[i] = outcome
						return nil
					}
					if len(cdnskeyResp.GetRecords("CDNSKEY", "answer")) > 0 {
						outcome.cdnskeyHasRRset = true
						for _, rr := range cdnskeyResp.GetRecords("RRSIG", "answer") {
							if sig, ok := rr.(*dns.RRSIG); ok {
								outcome.cdnskeyRRSIG = append(outcome.cdnskeyRRSIG, sig)
							}
						}
					}

					useVC = false
					dnskeyResp, _ := ns.QueryWithOptions(ctx, z.Name.String(), "DNSKEY", &nameserver.QueryOptions{DNSSEC: &dnssecOn, UseVC: &useVC})
					if dnskeyResp.Msg == nil || dnskeyResp.Rcode() != "NOERROR" || !dnskeyResp.AA() {
						outcomes[i] = outcome
						return nil
					}
					for _, rr := range dnskeyResp.GetRecords("DNSKEY", "answer") {
						if dnskey, ok := rr.(*dns.DNSKEY); ok {
							outcome.dnskeyRecords = append(outcome.dnskeyRecords, dnskey)
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

			for _, outcome := range outcomes {
				if outcome.cdsHasRRset {
					cdsRRsets[outcome.nsIP] = true
					cdsRRSIG[outcome.nsIP] = outcome.cdsRRSIG
				}
				if outcome.cdnskeyHasRRset {
					cdnskeyRRsets[outcome.nsIP] = true
					cdnskeyRRSIG[outcome.nsIP] = outcome.cdnskeyRRSIG
				}
				if len(outcome.dnskeyRecords) > 0 {
					dnskeyRRsets[outcome.nsIP] = outcome.dnskeyRecords
				}
			}
		}

		if (len(cdsRRsets) > 0 || len(cdnskeyRRsets) > 0) && len(dnskeyRRsets) > 0 {
			orderedIPs := make([]string, 0, len(ordered))
			seenIPs := map[string]bool{}
			for _, ns := range ordered {
				ip := ns.Address.String()
				orderedIPs = append(orderedIPs, ip)
				seenIPs[ip] = true
			}
			var extraIPs []string
			for nsIP := range cdsRRsets {
				if !seenIPs[nsIP] {
					extraIPs = append(extraIPs, nsIP)
				}
			}
			for nsIP := range cdnskeyRRsets {
				if !seenIPs[nsIP] {
					extraIPs = append(extraIPs, nsIP)
				}
			}
			if len(extraIPs) > 0 {
				sort.Strings(extraIPs)
				orderedIPs = append(orderedIPs, extraIPs...)
			}

			if len(cdsRRsets) > 0 {
				type sigOutcome struct {
					nsIP    string
					noMatch bool
				}

				var tasks []parallel.Task[sigOutcome]
				for _, nsIP := range orderedIPs {
					if !cdsRRsets[nsIP] {
						continue
					}
					nsIP := nsIP
					tasks = append(tasks, func(_ context.Context) (sigOutcome, error) {
						rrsig := cdsRRSIG[nsIP]
						dnskeys := dnskeyRRsets[nsIP]
						match := false
						for _, ds := range dsRecords {
							if !dnskeyHasKeytag(dnskeys, ds.KeyTag) {
								continue
							}
							if rrsigHasKeytag(rrsig, ds.KeyTag) {
								match = true
								break
							}
						}
						return sigOutcome{nsIP: nsIP, noMatch: !match}, nil
					})
				}

				if len(tasks) > 0 {
					parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
					validationResults := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: parallelism, CancelOnError: false})
					for _, res := range validationResults {
						if res.Err != nil {
							return results, res.Err
						}
						if res.Value.noMatch {
							dsNoMatchCDSRRSIG[res.Value.nsIP] = true
						}
					}
				}
			}

			if len(cdnskeyRRsets) > 0 {
				type sigOutcome struct {
					nsIP    string
					noMatch bool
				}

				var tasks []parallel.Task[sigOutcome]
				for _, nsIP := range orderedIPs {
					if !cdnskeyRRsets[nsIP] {
						continue
					}
					nsIP := nsIP
					tasks = append(tasks, func(_ context.Context) (sigOutcome, error) {
						rrsig := cdnskeyRRSIG[nsIP]
						dnskeys := dnskeyRRsets[nsIP]
						match := false
						for _, ds := range dsRecords {
							if !dnskeyHasKeytag(dnskeys, ds.KeyTag) {
								continue
							}
							if rrsigHasKeytag(rrsig, ds.KeyTag) {
								match = true
								break
							}
						}
						return sigOutcome{nsIP: nsIP, noMatch: !match}, nil
					})
				}

				if len(tasks) > 0 {
					parallelism := profile.FromContext(ctx).Resolver.Defaults.Parallel
					validationResults := parallel.RunOrdered(ctx, tasks, parallel.Options{Limit: parallelism, CancelOnError: false})
					for _, res := range validationResults {
						if res.Err != nil {
							return results, res.Err
						}
						if res.Value.noMatch {
							dsNoMatchCDNSKEYRRSIG[res.Value.nsIP] = true
						}
					}
				}
			}

			if len(dsNoMatchCDSRRSIG) > 0 {
				args := map[string]any{}
				setTypedAddressesFromValues(args, mapKeysSorted(dsNoMatchCDSRRSIG))
				if err := appendLog(ctx, &results, testcase, "DS18_NO_MATCH_CDS_RRSIG_DS", args); err != nil {
					return results, err
				}
			}
			if len(dsNoMatchCDNSKEYRRSIG) > 0 {
				args := map[string]any{}
				setTypedAddressesFromValues(args, mapKeysSorted(dsNoMatchCDNSKEYRRSIG))
				if err := appendLog(ctx, &results, testcase, "DS18_NO_MATCH_CDNSKEY_RRSIG_DS", args); err != nil {
					return results, err
				}
			}
		}
	}

	if err := appendLog(ctx, &results, testcase, "TEST_CASE_END", map[string]any{"testcase": testcase}); err != nil {
		return results, err
	}
	return results, nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func symmetricDifferenceStrings(left []string, right []string) []string {
	leftSet := map[string]bool{}
	rightSet := map[string]bool{}
	for _, value := range left {
		leftSet[value] = true
	}
	for _, value := range right {
		rightSet[value] = true
	}
	out := []string{}
	for value := range leftSet {
		if !rightSet[value] {
			out = append(out, value)
		}
	}
	for value := range rightSet {
		if !leftSet[value] {
			out = append(out, value)
		}
	}
	return out
}

func intersectionStrings(left []string, right []string) []string {
	rightSet := map[string]bool{}
	for _, value := range right {
		rightSet[value] = true
	}
	out := []string{}
	for _, value := range uniqueStrings(left) {
		if rightSet[value] {
			out = append(out, value)
		}
	}
	return out
}

func typeMapFromBitmap(types []uint16) map[string]bool {
	if len(types) == 0 {
		return nil
	}
	out := map[string]bool{}
	for _, t := range types {
		if name, ok := dns.TypeToString[t]; ok {
			out[name] = true
		} else {
			out[strconv.Itoa(int(t))] = true
		}
	}
	return out
}

func typeListIncorrect(typeMap map[string]bool, mandatory []string, forbidden []string) bool {
	for _, t := range mandatory {
		if !typeMap[t] {
			return true
		}
	}
	for _, t := range forbidden {
		if typeMap[t] {
			return true
		}
	}
	return false
}

func rrOwnerMatchesZone(rr dns.RR, zoneName dnsname.Name) bool {
	if rr == nil {
		return false
	}
	owner := dnsname.New(rr.Header().Name)
	return strings.EqualFold(owner.String(), zoneName.String())
}

func nsec3OwnerMatchesApex(rr *dns.NSEC3, apex dnsname.Name) bool {
	if rr == nil {
		return false
	}
	hash := dnsutil.NSEC3Name(apex.FQDN(), rr.Salt, rr.Iterations)
	if hash == "" {
		return false
	}
	labels := dnsname.New(rr.Hdr.Name).Labels()
	if len(labels) == 0 {
		return false
	}
	return strings.EqualFold(hash, labels[0])
}

func filterRRSIGByType(rrs []dns.RR, typeCovered uint16) []*dns.RRSIG {
	var out []*dns.RRSIG
	for _, rr := range rrs {
		sig, ok := rr.(*dns.RRSIG)
		if !ok {
			continue
		}
		if sig.TypeCovered != typeCovered {
			continue
		}
		out = append(out, sig)
	}
	return out
}

func rrsetForName(rrs []dns.RR, owner string) []dns.RR {
	if len(rrs) == 0 {
		return nil
	}
	ownerName := dnsname.New(owner)
	var out []dns.RR
	for _, rr := range rrs {
		if rr == nil {
			continue
		}
		rrName := dnsname.New(rr.Header().Name)
		if strings.EqualFold(rrName.String(), ownerName.String()) {
			out = append(out, rr)
		}
	}
	return out
}

func dnskeyKeySize(key *dns.DNSKEY) int {
	if key == nil || key.PublicKey == "" {
		return 0
	}
	keybuf, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil || len(keybuf) == 0 {
		return 0
	}

	explen := int(keybuf[0])
	keyoff := 1
	if explen == 0 {
		if len(keybuf) < 3 {
			return 0
		}
		explen = int(keybuf[1])<<8 | int(keybuf[2])
		keyoff = 3
	}
	if explen <= 0 || keyoff+explen >= len(keybuf) {
		return 0
	}

	modulus := keybuf[keyoff+explen:]
	if len(modulus) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(modulus).BitLen()
}

func rrsigHasKeytag(rrs []*dns.RRSIG, keytag uint16) bool {
	for _, sig := range rrs {
		if sig == nil {
			continue
		}
		if sig.KeyTag == keytag {
			return true
		}
	}
	return false
}

func dnskeyHasKeytag(rrs []*dns.DNSKEY, keytag uint16) bool {
	for _, dnskey := range rrs {
		if dnskey == nil {
			continue
		}
		if dnskey.KeyTag() == keytag {
			return true
		}
	}
	return false
}

func mapKeysSorted(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func rrsetSignature(rrs []dns.RR) string {
	if len(rrs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(rrs))
	for _, rr := range rrs {
		if rr == nil {
			continue
		}
		parts = append(parts, rr.String())
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func rrsetInconsistent(rrsets map[string][]dns.RR) bool {
	if len(rrsets) == 0 {
		return false
	}
	keys := make([]string, 0, len(rrsets))
	for key := range rrsets {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	first := true
	var signature string
	for _, key := range keys {
		current := rrsetSignature(rrsets[key])
		if first {
			signature = current
			first = false
			continue
		}
		if current != signature {
			return true
		}
	}
	return false
}

func appendLog(ctx context.Context, results *[]*logger.Entry, testcase string, tag string, args map[string]any) error {
	entry, err := util.LoggerFromContext(ctx).Add(tag, args, moduleName, testcase)
	if err != nil {
		return err
	}
	*results = append(*results, entry)
	return nil
}

func withNameserverArgs(server nameserver.Nameserver, args map[string]any) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	logargs.NormalizeQueryIdentity(args)
	logargs.SetNS(args, server.NameString(), server.AddressString())
	return args
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

func ipDisabledMessageWithLogger(ctx context.Context, buf *testlogger.Buffer, server nameserver.Nameserver, rrtypes ...string) (bool, error) {
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

func ipDisabledMessage(ctx context.Context, results *[]*logger.Entry, testcase string, server nameserver.Nameserver, rrtypes ...string) (bool, error) {
	if !profile.FromContext(ctx).Net.IPv6 && server.Address.Is6() {
		for _, rrtype := range rrtypes {
			if err := appendLog(ctx, results, testcase, "IPV6_DISABLED", withNameserverArgs(server, map[string]any{
				"query_type": rrtype,
			})); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if !profile.FromContext(ctx).Net.IPv4 && server.Address.Is4() {
		for _, rrtype := range rrtypes {
			if err := appendLog(ctx, results, testcase, "IPV4_DISABLED", withNameserverArgs(server, map[string]any{
				"query_type": rrtype,
			})); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	return false, nil
}

func nameserversByIP(servers []nameserver.Nameserver) [][]nameserver.Nameserver {
	if len(servers) == 0 {
		return nil
	}
	seen := map[string]int{}
	var grouped [][]nameserver.Nameserver
	for _, ns := range servers {
		ip := ns.Address.String()
		if idx, ok := seen[ip]; ok {
			grouped[idx] = append(grouped[idx], ns)
			continue
		}
		seen[ip] = len(grouped)
		grouped = append(grouped, []nameserver.Nameserver{ns})
	}
	return grouped
}

func nsStrings(servers []nameserver.Nameserver) []string {
	values := make([]string, 0, len(servers))
	for _, ns := range servers {
		values = append(values, ns.NameString())
	}
	return values
}

func joinUniqueSorted(values []string) string {
	return strings.Join(logargs.UniqueSortedEndpointNames(values), ";")
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
	names := logargs.UniqueSortedEndpointNames(values)
	if len(names) == 0 {
		return
	}
	servers := make([]logargs.Server, 0, len(names))
	for _, name := range names {
		servers = append(servers, logargs.Server{NS: name})
	}
	if typed, ok := logargs.Servers(servers)["servers"]; ok {
		args["servers"] = typed
	}
}

func differenceStrings(left []string, right []string) []string {
	if len(left) == 0 {
		return nil
	}
	rightSet := map[string]bool{}
	for _, value := range right {
		rightSet[value] = true
	}
	out := []string{}
	for _, value := range left {
		if !rightSet[value] {
			out = append(out, value)
		}
	}
	return out
}

func dnssec01TagForDigest(digest uint8) string {
	switch {
	case digest == 0:
		return "DS01_DS_ALGO_NOT_DS"
	case digest == 1:
		return "DS01_DS_ALGO_DEPRECATED"
	case digest == 2:
		return "DS01_DS_ALGO_OK"
	case digest == 3:
		return "DS01_DS_ALGO_DEPRECATED"
	case digest == 4:
		return "DS01_DS_ALGO_OK"
	case digest == 5:
		return "DS01_DS_ALGO_OK"
	case digest == 6:
		return "DS01_DS_ALGO_OK"
	case digest >= 7 && digest <= 127:
		return "DS01_DS_ALGO_UNASSIGNED"
	case digest >= 128 && digest <= 252:
		return "DS01_DS_ALGO_RESERVED"
	case digest == 253 || digest == 254:
		return "DS01_DS_ALGO_PRIVATE"
	case digest == 255:
		return "DS01_DS_ALGO_UNASSIGNED"
	default:
		return "DS01_DS_ALGO_UNASSIGNED"
	}
}

func dnssec05TagForAlgorithm(algo uint8) string {
	switch {
	case algo == 0:
		return "DS05_ALGO_NOT_ZONE_SIGN"
	case algo == 1:
		return "DS05_ALGO_DEPRECATED"
	case algo == 2:
		return "DS05_ALGO_NOT_ZONE_SIGN"
	case algo == 3:
		return "DS05_ALGO_DEPRECATED"
	case algo == 4:
		return "DS05_ALGO_RESERVED"
	case algo == 5:
		return "DS05_ALGO_DEPRECATED"
	case algo == 6:
		return "DS05_ALGO_DEPRECATED"
	case algo == 7:
		return "DS05_ALGO_DEPRECATED"
	case algo == 8:
		return "DS05_ALGO_OK"
	case algo == 9:
		return "DS05_ALGO_RESERVED"
	case algo == 10:
		return "DS05_ALGO_NOT_RECOMMENDED"
	case algo == 11:
		return "DS05_ALGO_RESERVED"
	case algo == 12:
		return "DS05_ALGO_DEPRECATED"
	case algo == 13:
		return "DS05_ALGO_OK"
	case algo == 14:
		return "DS05_ALGO_OK"
	case algo == 15:
		return "DS05_ALGO_OK"
	case algo == 16:
		return "DS05_ALGO_OK"
	case algo == 17:
		return "DS05_ALGO_OK"
	case algo >= 18 && algo <= 22:
		return "DS05_ALGO_UNASSIGNED"
	case algo == 23:
		return "DS05_ALGO_OK"
	case algo >= 24 && algo <= 122:
		return "DS05_ALGO_UNASSIGNED"
	case algo >= 123 && algo <= 251:
		return "DS05_ALGO_RESERVED"
	case algo == 252:
		return "DS05_ALGO_NOT_ZONE_SIGN"
	case algo == 253 || algo == 254:
		return "DS05_ALGO_PRIVATE"
	case algo == 255:
		return "DS05_ALGO_RESERVED"
	default:
		return "DS05_ALGO_UNASSIGNED"
	}
}

func digestDescription(digest uint8) string {
	switch {
	case digest == 0:
		return "Reserved"
	case digest == 1:
		return "SHA-1"
	case digest == 2:
		return "SHA-256"
	case digest == 3:
		return "GOST R 34.11-94"
	case digest == 4:
		return "SHA-384"
	case digest == 5:
		return "GOST R 34.11-2012"
	case digest == 6:
		return "SM3"
	case digest >= 7 && digest <= 127:
		return "Unassigned"
	case digest >= 128 && digest <= 252:
		return "Reserved"
	case digest == 253 || digest == 254:
		return "Reserved for Private Use"
	case digest == 255:
		return "Unassigned"
	default:
		return "Unassigned"
	}
}

func defaultZoneQueryOne(ctx context.Context, z *zone.Zone, name string, rrtype string, opts *nameserver.QueryOptions) (packet.Packet, error) {
	if z == nil {
		return packet.Packet{}, errors.New("zone is nil")
	}
	return z.QueryOne(ctx, name, rrtype, opts)
}

func defaultZoneQueryAll(ctx context.Context, z *zone.Zone, name string, rrtype string, opts *nameserver.QueryOptions) ([]packet.Packet, error) {
	if z == nil {
		return nil, errors.New("zone is nil")
	}
	return z.QueryAll(ctx, name, rrtype, opts)
}

func defaultZoneParent(ctx context.Context, z *zone.Zone) (*zone.Zone, error) {
	if z == nil {
		return nil, errors.New("zone is nil")
	}
	return z.Parent(ctx)
}

func defaultParentNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if z == nil {
		return nil, errors.New("zone is nil")
	}
	parent, err := z.Parent(ctx)
	if err != nil {
		return nil, err
	}
	return parent.NS(ctx)
}

func defaultHasFakeAddresses(z *zone.Zone) bool {
	if z == nil {
		return false
	}
	rec := z.Recursor()
	if rec == nil {
		return false
	}
	return rec.HasFakeAddresses(z.Name.String())
}

func containsDS(records []*dns.DS, candidate *dns.DS) bool {
	if candidate == nil {
		return false
	}
	for _, ds := range records {
		if ds == nil {
			continue
		}
		if ds.KeyTag == candidate.KeyTag &&
			ds.DigestType == candidate.DigestType &&
			ds.Algorithm == candidate.Algorithm &&
			strings.EqualFold(ds.Digest, candidate.Digest) {
			return true
		}
	}
	return false
}

func dsDigestSupported(digest uint8) bool {
	switch digest {
	case 1, 2, 3, 4:
		return true
	default:
		return false
	}
}

func dnskeyRRset(keys []*dns.DNSKEY) []dns.RR {
	rrs := make([]dns.RR, 0, len(keys))
	for _, key := range keys {
		if key == nil {
			continue
		}
		rrs = append(rrs, key)
	}
	return rrs
}

func nameserversFromNSItems(ctx context.Context, z *zone.Zone, items []methodsv2.NSItem) []nameserver.Nameserver {
	if z == nil {
		return nil
	}
	rec := z.Recursor()
	if rec == nil {
		return nil
	}

	seen := map[string]nameserver.Nameserver{}
	for _, item := range items {
		if !item.HasAddress {
			continue
		}
		ns, err := nameserver.NewWithContext(ctx, item.Name.String(), item.Address.String(), rec.Client())
		if err != nil {
			continue
		}
		seen[strings.ToLower(ns.String())] = ns
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]nameserver.Nameserver, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func equalStringSets(left []string, right []string) bool {
	leftSet := map[string]bool{}
	for _, value := range left {
		leftSet[value] = true
	}
	rightSet := map[string]bool{}
	for _, value := range right {
		rightSet[value] = true
	}
	if len(leftSet) != len(rightSet) {
		return false
	}
	for value := range leftSet {
		if !rightSet[value] {
			return false
		}
	}
	return true
}

func packetTime(pkt packet.Packet) time.Time {
	if pkt.Timestamp.IsZero() {
		return time.Now().UTC()
	}
	return pkt.Timestamp.UTC()
}

func rrsigTypeString(typeCovered uint16) string {
	if name, ok := dns.TypeToString[typeCovered]; ok && name != "" {
		return name
	}
	return strconv.Itoa(int(typeCovered))
}

func dnssecAlgorithmSupported(algo uint8) bool {
	switch algo {
	case dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512, dns.ECDSAP256SHA256, dns.ECDSAP384SHA384, dns.ED25519:
		return true
	default:
		return false
	}
}

func verifyRRSIG(sig *dns.RRSIG, rrset []dns.RR, key *dns.DNSKEY, at time.Time) error {
	if sig == nil || key == nil {
		return errors.New("missing rrsig or key")
	}
	if !sig.ValidPeriod(at) {
		return errors.New("rrsig not valid at time")
	}
	return sig.Verify(key, rrset, &dns.SignOption{})
}

func algoPropertyFor(algo uint8) algoProperty {
	if prop, ok := algoProperties[algo]; ok {
		return prop
	}
	switch {
	case algo >= 18 && algo <= 22:
		return algoProperty{description: "Unassigned", mnemonic: "UNASSIGNED"}
	case algo >= 24 && algo <= 122:
		return algoProperty{description: "Unassigned", mnemonic: "UNASSIGNED"}
	case algo >= 123 && algo <= 251:
		return algoProperty{description: "Reserved", mnemonic: "RESERVED"}
	default:
		return algoProperty{description: "Reserved", mnemonic: "RESERVED"}
	}
}
