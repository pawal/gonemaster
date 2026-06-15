package badkeys

import (
	"fmt"
	"math/big"
)

// KeyType identifies the cryptographic key family.
type KeyType int

const (
	// KeyTypeRSA identifies RSA DNSKEY material.
	KeyTypeRSA KeyType = iota // Algorithms 1, 5, 7, 8, 10
	// KeyTypeDSA identifies DSA DNSKEY material.
	KeyTypeDSA // Algorithms 3, 6
	// KeyTypeECDSA identifies ECDSA DNSKEY material.
	KeyTypeECDSA // Algorithms 13, 14
	// KeyTypeEdDSA identifies EdDSA DNSKEY material.
	KeyTypeEdDSA // Algorithms 15, 16
)

// ParsedKey holds the extracted numeric values from a DNSKEY wire-format key.
type ParsedKey struct {
	// Type is the parsed key family.
	Type KeyType
	// N is the RSA modulus (RSA only).
	N *big.Int
	// E is the RSA public exponent (RSA only).
	E *big.Int
	// Val is the primary numeric value used for BKHASH and blocklist checks:
	//   RSA: N, DSA: Y, ECDSA: X coordinate, EdDSA: raw key as integer.
	Val *big.Int
	// Bits is the key size in bits (RSA: N.BitLen(), DSA: 512+t*64).
	Bits int
}

// ParseDNSKEY parses the DNSKEY RDATA key material (the public key bytes
// after the 4-byte DNSKEY header: flags, protocol, algorithm).
//
// algo is the DNSKEY algorithm number. keyData is the raw public key bytes.
//
// Wire format references:
//   - RSA:     RFC 3110 §2
//   - DSA:     RFC 2536 §2
//   - ECDSA:   RFC 6605
//   - Ed25519: RFC 8080
//   - Ed448:   RFC 8080
func ParseDNSKEY(algo uint8, keyData []byte) (*ParsedKey, error) {
	switch algo {
	case 1, 5, 7, 8, 10: // RSA
		return parseRSA(keyData)
	case 3, 6: // DSA
		return parseDSA(keyData)
	case 13: // ECDSA P-256
		return parseECDSA(keyData, 32, "P-256")
	case 14: // ECDSA P-384
		return parseECDSA(keyData, 48, "P-384")
	case 15: // Ed25519
		return parseEdDSA(keyData, 32, "Ed25519")
	case 16: // Ed448
		return parseEdDSA(keyData, 57, "Ed448")
	default:
		return nil, fmt.Errorf("unsupported DNSKEY algorithm %d", algo)
	}
}

// parseRSA parses RSA DNSKEY wire format per RFC 3110 §2.
//
// Format:
//
//	If key[0] != 0: elen = key[0], exponent starts at offset 1.
//	If key[0] == 0: elen = big-endian uint16 at key[1:3], exponent starts at offset 3.
//	Exponent e = key[eoffset : eoffset+elen]
//	Modulus  N = key[eoffset+elen :]
func parseRSA(key []byte) (*ParsedKey, error) {
	if len(key) < 3 {
		return nil, fmt.Errorf("RSA key too short (%d bytes)", len(key))
	}

	var elen int
	var eoffset int
	if key[0] == 0 {
		elen = int(key[1])<<8 | int(key[2])
		eoffset = 3
	} else {
		elen = int(key[0])
		eoffset = 1
	}

	if len(key) < eoffset+elen {
		return nil, fmt.Errorf("RSA key truncated: need %d bytes for exponent, have %d", elen, len(key)-eoffset)
	}

	e := new(big.Int).SetBytes(key[eoffset : eoffset+elen])
	n := new(big.Int).SetBytes(key[eoffset+elen:])

	if n.Sign() == 0 {
		return nil, fmt.Errorf("RSA modulus is zero")
	}

	return &ParsedKey{
		Type: KeyTypeRSA,
		N:    n,
		E:    e,
		Val:  n,
		Bits: n.BitLen(),
	}, nil
}

// parseDSA parses DSA DNSKEY wire format per RFC 2536 §2.
//
// Format:
//
//	key[0]   = T parameter (0..8)
//	Total length must be 213 + T*24 bytes.
//	Y starts at offset 149 + T*16.
func parseDSA(key []byte) (*ParsedKey, error) {
	if len(key) < 213 {
		return nil, fmt.Errorf("DSA key too short (%d bytes, minimum 213)", len(key))
	}

	t := int(key[0])
	if t > 8 {
		return nil, fmt.Errorf("DSA T parameter %d out of range (0..8)", t)
	}

	expectedLen := 213 + t*24
	if len(key) != expectedLen {
		return nil, fmt.Errorf("DSA key length %d does not match expected %d for T=%d", len(key), expectedLen, t)
	}

	yOffset := 149 + t*16
	y := new(big.Int).SetBytes(key[yOffset:])

	return &ParsedKey{
		Type: KeyTypeDSA,
		Val:  y,
		Bits: 512 + t*64,
	}, nil
}

// parseECDSA parses ECDSA DNSKEY wire format per RFC 6605.
//
// The key is the uncompressed point (X || Y) without the 0x04 prefix.
// P-256: 64 bytes total, X = first 32 bytes.
// P-384: 96 bytes total, X = first 48 bytes.
func parseECDSA(key []byte, coordLen int, curve string) (*ParsedKey, error) {
	expectedLen := coordLen * 2
	if len(key) != expectedLen {
		return nil, fmt.Errorf("ECDSA %s key must be %d bytes, got %d", curve, expectedLen, len(key))
	}

	x := new(big.Int).SetBytes(key[:coordLen])

	return &ParsedKey{
		Type: KeyTypeECDSA,
		Val:  x,
		Bits: coordLen * 8,
	}, nil
}

// parseEdDSA parses Ed25519/Ed448 DNSKEY wire format per RFC 8080.
//
// The entire key is converted to an integer (big-endian).
func parseEdDSA(key []byte, expectedLen int, curve string) (*ParsedKey, error) {
	if len(key) != expectedLen {
		return nil, fmt.Errorf("EdDSA %s key must be %d bytes, got %d", curve, expectedLen, len(key))
	}

	val := new(big.Int).SetBytes(key)

	return &ParsedKey{
		Type: KeyTypeEdDSA,
		Val:  val,
		Bits: expectedLen * 8,
	}, nil
}
