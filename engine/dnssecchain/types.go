// Package dnssecchain extracts a per-run DNSSEC chain summary from responses
// already cached during the run; it never issues a query.
package dnssecchain

// Version is the schema version of the emitted Summary. Changes are additive.
const Version = 1

// Roll-up status values.
const (
	StatusSecure        = "secure"
	StatusBroken        = "broken"
	StatusIsland        = "island"
	StatusUnsigned      = "unsigned"
	StatusIndeterminate = "indeterminate"
)

// Delegation kinds.
const (
	DelegationNormal      = "normal"
	DelegationUndelegated = "undelegated"
)

// DS source kinds.
const (
	DSSourceParent = "parent"
	DSSourceInput  = "input"
	DSSourceNone   = "none"
)

// RRSIG signature states.
const (
	SigValid       = "valid"
	SigExpired     = "expired"
	SigNotYetValid = "not_yet_valid"
	SigBogus       = "bogus"
	SigUnsupported = "unsupported_algorithm"
	SigNoKey       = "no_key"
	SigUnverified  = "unverified"
)

// DS-to-DNSKEY link statuses.
const (
	LinkMatch             = "match"
	LinkDigestMismatch    = "digest_mismatch"
	LinkNoDNSKEY          = "no_dnskey"
	LinkUnsupportedDigest = "unsupported_digest"
)

// Summary is the versioned chain document persisted per public run.
type Summary struct {
	Version    int    `json:"version"`
	Zone       string `json:"zone"`
	ParentZone string `json:"parent_zone"`
	Delegation string `json:"delegation"`
	Status     string `json:"status"`
	Truncated  bool   `json:"truncated"`

	Parent Parent `json:"parent"`
	Child  Child  `json:"child"`
	Links  []Link `json:"links"`
}

// Parent holds the DS evidence gathered from the parent zone's servers.
type Parent struct {
	DSSource           string   `json:"ds_source"`
	DS                 []DS     `json:"ds"`
	DSRRSIG            []RRSIG  `json:"ds_rrsig"`
	DNSKEYs            []DNSKEY `json:"dnskeys,omitempty"` // parent keys that sign the DS RRset
	ServersQueried     []string `json:"servers_queried"`
	ServersWithoutDS   []string `json:"servers_without_ds"`
	ServersDisagreeing []string `json:"servers_disagreeing"`
}

// Child holds the DNSKEY and signature evidence from the tested zone's servers.
type Child struct {
	DNSKEYs              []DNSKEY      `json:"dnskeys"`
	DNSKEYRRSIG          []RRSIG       `json:"dnskey_rrsig"`
	Signed               []SignedRRset `json:"signed"`
	ServersQueried       []string      `json:"servers_queried"`
	ServersWithoutDNSKEY []string      `json:"servers_without_dnskey"`
	ServersDisagreeing   []string      `json:"servers_disagreeing"`
}

// SignedRRset is one apex RRset the zone publishes together with the signatures
// covering it (for example SOA, NSEC3PARAM, CDS, CDNSKEY). Refs lists the
// DNSKEY key tags the RRset points at (CDS/CDNSKEY name a key by tag).
// For CDS/CDNSKEY, DSMatch reports whether those key tags match the parent DS
// and NewKeys lists the signaled key tags with no DS at the parent yet.
type SignedRRset struct {
	Type    string   `json:"type"`
	RRSIG   []RRSIG  `json:"rrsig"`
	Refs    []uint16 `json:"refs,omitempty"`
	DSMatch string   `json:"ds_match,omitempty"`
	NewKeys []uint16 `json:"new_keys,omitempty"`
	TTL     uint32   `json:"ttl"`
}

// CDS/CDNSKEY-to-parent-DS verdicts.
const (
	CDSMatchExact    = "match"
	CDSMatchRollover = "rollover"
)

// DS is one delegation-signer record in the union across parent servers.
type DS struct {
	KeyTag     uint16   `json:"key_tag"`
	Algorithm  uint8    `json:"algorithm"`
	DigestType uint8    `json:"digest_type"`
	Digest     string   `json:"digest"`
	TTL        uint32   `json:"ttl"`
	Servers    []string `json:"servers"`
}

// DNSKEY is one public key in the union across child servers. Key material is
// deliberately excluded; only derived properties are recorded. Anchored is set
// when a matching DS at the parent names this key.
type DNSKEY struct {
	KeyTag    uint16   `json:"key_tag"`
	Algorithm uint8    `json:"algorithm"`
	Flags     uint16   `json:"flags"`
	SEP       bool     `json:"sep"`
	ZoneKey   bool     `json:"zone_key"`
	Revoked   bool     `json:"revoked"`
	KeySize   int      `json:"key_size"`
	Anchored  bool     `json:"anchored,omitempty"`
	TTL       uint32   `json:"ttl"`
	Servers   []string `json:"servers"`
}

// RRSIG is one signature covering a DS, DNSKEY, or SOA RRset.
type RRSIG struct {
	KeyTag     uint16   `json:"key_tag"`
	Algorithm  uint8    `json:"algorithm"`
	Inception  int64    `json:"inception"`
	Expiration int64    `json:"expiration"`
	Signer     string   `json:"signer"`
	State      string   `json:"state"`
	Servers    []string `json:"servers"`
}

// Link is one DS-to-DNSKEY edge. DNSKEYKeyTag is omitted when no key matches.
type Link struct {
	DSKeyTag     uint16   `json:"ds_key_tag"`
	DSDigestType uint8    `json:"ds_digest_type"`
	DNSKEYKeyTag uint16   `json:"dnskey_key_tag,omitempty"`
	Status       string   `json:"status"`
	Servers      []string `json:"servers"`
}
