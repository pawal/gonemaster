package badkeys

// Finding represents a single detected vulnerability or blocklist match.
type Finding struct {
	// Check is the check name: "fermat", "pattern", "roca", "rsainvalid",
	// "smallfactors", "smalld", or "blocklist".
	Check string
	// BlocklistName is the blocklist source name (only for "blocklist" check).
	BlocklistName string
}

// CheckDNSKEY parses a DNSKEY record and runs all applicable badkeys checks.
//
// algo is the DNSKEY algorithm number. keyData is the raw public key bytes
// (after the 4-byte DNSKEY RDATA header: flags, protocol, algorithm).
// bl is the loaded blocklist (may be nil to skip blocklist checks).
//
// Returns a list of findings (empty if the key passes all checks).
func CheckDNSKEY(algo uint8, keyData []byte, bl *Blocklist) ([]Finding, error) {
	key, err := ParseDNSKEY(algo, keyData)
	if err != nil {
		return nil, err
	}

	var findings []Finding

	// Blocklist check applies to all key types.
	if bl != nil {
		if result := bl.Check(key.Val); result != nil {
			findings = append(findings, Finding{
				Check:         "blocklist",
				BlocklistName: result.SourceName,
			})
		}
	}

	// RSA-specific checks.
	if key.Type == KeyTypeRSA {
		if checkRSAInvalid(key.N, key.E) {
			findings = append(findings, Finding{Check: "rsainvalid"})
		}
		if checkFermat(key.N) {
			findings = append(findings, Finding{Check: "fermat"})
		}
		if checkPattern(key.N) {
			findings = append(findings, Finding{Check: "pattern"})
		}
		if checkROCA(key.N) {
			findings = append(findings, Finding{Check: "roca"})
		}
		if checkSmallFactors(key.N) {
			findings = append(findings, Finding{Check: "smallfactors"})
		}
		if checkSmallD(key.N, key.E) {
			findings = append(findings, Finding{Check: "smalld"})
		}
	}

	return findings, nil
}
