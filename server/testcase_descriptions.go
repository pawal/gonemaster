package server

import "strings"

// testcaseDescriptions maps testcase IDs to short English descriptions.
// Sourced from Zonemaster Engine TAG_DESCRIPTIONS plus gonemaster-specific tests.
var testcaseDescriptions = map[string]string{
	// ADDRESS
	"ADDRESS01": "Name server address must be globally routable",
	"ADDRESS02": "Reverse DNS entry exists for name server IP address",
	"ADDRESS03": "Reverse DNS entry matches name server name",

	// BASIC
	"BASIC01": "The domain must have a parent domain",
	"BASIC02": "The domain must have at least one working name server",
	"BASIC03": "The Broken but functional test",

	// CONNECTIVITY
	"CONNECTIVITY01": "UDP connectivity",
	"CONNECTIVITY02": "TCP connectivity",
	"CONNECTIVITY03": "AS diversity",
	"CONNECTIVITY04": "IP prefix diversity",

	// CONSISTENCY
	"CONSISTENCY01": "SOA serial number consistency",
	"CONSISTENCY02": "SOA RNAME consistency",
	"CONSISTENCY03": "SOA timers consistency",
	"CONSISTENCY04": "Name server NS consistency",
	"CONSISTENCY05": "Consistency between glue and authoritative data",
	"CONSISTENCY06": "SOA MNAME consistency",

	// DELEGATION
	"DELEGATION01": "Minimum number of name servers",
	"DELEGATION02": "Name servers must have distinct IP addresses",
	"DELEGATION03": "No truncation of referrals",
	"DELEGATION04": "Name server is authoritative",
	"DELEGATION05": "Name server must not point at CNAME alias",
	"DELEGATION06": "Existence of SOA",
	"DELEGATION07": "Parent glue name records present in child",

	// DNSSEC
	"DNSSEC01": "Legal values for the DS hash digest algorithm",
	"DNSSEC02": "DS must match a valid DNSKEY in the child zone",
	"DNSSEC03": "Verify NSEC3 parameters",
	"DNSSEC04": "Check for too short or too long RRSIG lifetimes",
	"DNSSEC05": "Check for invalid DNSKEY algorithms",
	"DNSSEC06": "Verify DNSSEC additional processing",
	"DNSSEC07": "DNSSEC signed zone and DS in parent for signed zone",
	"DNSSEC08": "Valid RRSIG for DNSKEY",
	"DNSSEC09": "RRSIG(SOA) must be valid and created by a valid DNSKEY",
	"DNSSEC10": "Zone contains NSEC or NSEC3 records",
	"DNSSEC11": "DS in delegation requires signed zone",
	"DNSSEC12": "Test for DNSSEC algorithm completeness",
	"DNSSEC13": "All DNSKEY algorithms used to sign the zone",
	"DNSSEC14": "Check for valid RSA DNSKEY key size",
	"DNSSEC15": "Existence of CDS and CDNSKEY",
	"DNSSEC16": "Validate CDS",
	"DNSSEC17": "Validate CDNSKEY",
	"DNSSEC18": "Validate trust from DS to CDS and CDNSKEY",
	"DNSSEC19": "Check DNSKEY records for known cryptographic weaknesses",
	"DNSSEC20": "NSEC/NSEC3 type bitmap at zone apex matches actual RR types",
	"DNSSEC21": "Parent zone signs the delegating DS RRset",

	// NAMESERVER
	"NAMESERVER01": "A name server should not be a recursor",
	"NAMESERVER02": "Test of EDNS0 support",
	"NAMESERVER03": "Test availability of zone transfer (AXFR)",
	"NAMESERVER04": "Same source address",
	"NAMESERVER05": "Behaviour against AAAA query",
	"NAMESERVER06": "NS can be resolved",
	"NAMESERVER07": "To check whether authoritative name servers return an upward referral",
	"NAMESERVER08": "Testing QNAME case insensitivity",
	"NAMESERVER09": "Testing QNAME case sensitivity",
	"NAMESERVER10": "Behaviour against unsupported EDNS version",
	"NAMESERVER11": "Behaviour against unknown EDNS option codes",
	"NAMESERVER12": "Behaviour against unknown EDNS Z flags",
	"NAMESERVER13": "Truncated EDNS responses include OPT record",
	"NAMESERVER15": "Name server does not reveal software version",
	"NAMESERVER16": "EDNS NSID option support",

	// SYNTAX
	"SYNTAX01": "No illegal characters in the domain name",
	"SYNTAX02": "No hyphen at the start or end of the domain name",
	"SYNTAX03": "No double hyphen in position 3 and 4 of the domain name",
	"SYNTAX04": "The NS name must have a valid domain/hostname",
	"SYNTAX05": "Misuse of '@' character in the SOA RNAME field",
	"SYNTAX06": "No illegal characters in the SOA RNAME field",
	"SYNTAX07": "No illegal characters in the SOA MNAME field",
	"SYNTAX08": "MX name must have a valid hostname",

	// ZONE
	"ZONE01": "Fully qualified master nameserver in SOA",
	"ZONE02": "SOA 'refresh' minimum value",
	"ZONE03": "SOA 'retry' lower than 'refresh'",
	"ZONE04": "SOA 'retry' at least 1 hour",
	"ZONE05": "SOA 'expire' minimum value",
	"ZONE06": "SOA 'minimum' maximum value",
	"ZONE07": "SOA master is not an alias",
	"ZONE08": "MX is not an alias",
	"ZONE09": "MX record present",
	"ZONE10": "No multiple SOA records",
	"ZONE11": "SPF policy at zone apex",
	"ZONE12": "CSYNC RR at zone apex",
	"ZONE13": "SPF DNS lookup limit compliance",
	"ZONE14": "ZONEMD RR at zone apex (RFC 8976 compliance)",
}

// TestcaseDescriptions returns the short testcase descriptions keyed by
// uppercase testcase ID. The returned map is shared and must not be modified.
func TestcaseDescriptions() map[string]string {
	return testcaseDescriptions
}

// testcaseDescriptionsForEntries returns descriptions for testcases present in entries.
// Keys in the returned map match the casing from the entries (typically lowercase).
func testcaseDescriptionsForEntries(entries []JobResultEntry) map[string]string {
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.Testcase != "" {
			seen[e.Testcase] = struct{}{}
		}
	}
	descs := make(map[string]string, len(seen))
	for tc := range seen {
		key := strings.ToUpper(tc)
		if d, ok := testcaseDescriptions[key]; ok {
			descs[tc] = d
		}
	}
	return descs
}
