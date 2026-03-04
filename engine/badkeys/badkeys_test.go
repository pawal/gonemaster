package badkeys

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestParseDNSKEYRSA(t *testing.T) {
	algo, keyData := loadDNSKEYVector(t, "dnssec-root-rsa.dnskey")
	if algo != 8 {
		t.Fatalf("unexpected algorithm in test vector: got %d want 8", algo)
	}

	parsed, err := ParseDNSKEY(algo, keyData)
	if err != nil {
		t.Fatalf("parse RSA DNSKEY: %v", err)
	}

	if parsed.Type != KeyTypeRSA {
		t.Fatalf("unexpected key type: got %v want %v", parsed.Type, KeyTypeRSA)
	}
	if parsed.E == nil || parsed.N == nil {
		t.Fatalf("expected RSA exponent and modulus to be set")
	}
	if parsed.E.Cmp(big.NewInt(65537)) != 0 {
		t.Fatalf("unexpected RSA exponent: got %s want 65537", parsed.E.String())
	}
	if parsed.N.BitLen() != 2048 {
		t.Fatalf("unexpected RSA modulus size: got %d bits want 2048", parsed.N.BitLen())
	}
	if parsed.Val.Cmp(parsed.N) != 0 {
		t.Fatalf("RSA Val should equal modulus N")
	}
}

func TestParseDNSKEYRSALargeExponent(t *testing.T) {
	algo, keyData := loadDNSKEYVector(t, "dnssec-rsa-large-e.dnskey")
	if algo != 8 {
		t.Fatalf("unexpected algorithm in test vector: got %d want 8", algo)
	}
	if len(keyData) < 3 {
		t.Fatalf("large exponent test vector unexpectedly short: %d bytes", len(keyData))
	}
	if keyData[0] != 0 || keyData[1] != 1 || keyData[2] != 128 {
		t.Fatalf("expected 3-byte RSA exponent length prefix 0x000180, got %02x %02x %02x", keyData[0], keyData[1], keyData[2])
	}

	parsed, err := ParseDNSKEY(algo, keyData)
	if err != nil {
		t.Fatalf("parse RSA large exponent DNSKEY: %v", err)
	}

	if parsed.Type != KeyTypeRSA {
		t.Fatalf("unexpected key type: got %v want %v", parsed.Type, KeyTypeRSA)
	}
	if len(parsed.E.Bytes()) != 384 {
		t.Fatalf("unexpected exponent byte length: got %d want 384", len(parsed.E.Bytes()))
	}
	if parsed.E.BitLen() <= 32 {
		t.Fatalf("expected exponent > 32 bits, got %d", parsed.E.BitLen())
	}
	if parsed.N.BitLen() != 4096 {
		t.Fatalf("unexpected RSA modulus size: got %d bits want 4096", parsed.N.BitLen())
	}
}

func TestParseDNSKEYOtherAlgorithms(t *testing.T) {
	t.Run("ECDSA_P256_Vector", func(t *testing.T) {
		algo, keyData := loadDNSKEYVector(t, "dnssec-p256-rfc6605.dnskey")
		if algo != 13 {
			t.Fatalf("unexpected algorithm in test vector: got %d want 13", algo)
		}

		parsed, err := ParseDNSKEY(algo, keyData)
		if err != nil {
			t.Fatalf("parse ECDSA P-256 DNSKEY: %v", err)
		}

		if parsed.Type != KeyTypeECDSA {
			t.Fatalf("unexpected key type: got %v want %v", parsed.Type, KeyTypeECDSA)
		}
		if parsed.Bits != 256 {
			t.Fatalf("unexpected key size: got %d bits want 256", parsed.Bits)
		}
		const wantX = "1a88c88615d437fbb8bf9e1942a1929f28562706ae6c2bd399e7b1bfb6d1e9e7"
		if got := parsed.Val.Text(16); got != wantX {
			t.Fatalf("unexpected P-256 X coordinate: got %s want %s", got, wantX)
		}
	})

	t.Run("ECDSA_P384_Synthetic", func(t *testing.T) {
		x := bytes.Repeat([]byte{0x11}, 48)
		y := bytes.Repeat([]byte{0x22}, 48)
		keyData := append(append([]byte{}, x...), y...)

		parsed, err := ParseDNSKEY(14, keyData)
		if err != nil {
			t.Fatalf("parse ECDSA P-384 DNSKEY: %v", err)
		}

		if parsed.Type != KeyTypeECDSA {
			t.Fatalf("unexpected key type: got %v want %v", parsed.Type, KeyTypeECDSA)
		}
		if parsed.Bits != 384 {
			t.Fatalf("unexpected key size: got %d bits want 384", parsed.Bits)
		}
		wantX := new(big.Int).SetBytes(x)
		if parsed.Val.Cmp(wantX) != 0 {
			t.Fatalf("unexpected P-384 X coordinate")
		}
	})

	t.Run("DSA_Synthetic", func(t *testing.T) {
		keyData := make([]byte, 213)
		keyData[0] = 0 // T
		wantY := new(big.Int).SetUint64(0x123456789abcdef)
		yBytes := wantY.Bytes()
		copy(keyData[len(keyData)-len(yBytes):], yBytes)

		parsed, err := ParseDNSKEY(3, keyData)
		if err != nil {
			t.Fatalf("parse DSA DNSKEY: %v", err)
		}

		if parsed.Type != KeyTypeDSA {
			t.Fatalf("unexpected key type: got %v want %v", parsed.Type, KeyTypeDSA)
		}
		if parsed.Bits != 512 {
			t.Fatalf("unexpected key size: got %d bits want 512", parsed.Bits)
		}
		if parsed.Val.Cmp(wantY) != 0 {
			t.Fatalf("unexpected DSA Y value: got %s want %s", parsed.Val.String(), wantY.String())
		}
	})

	t.Run("Ed25519_Synthetic", func(t *testing.T) {
		keyData := make([]byte, 32)
		for i := range keyData {
			keyData[i] = byte(i + 1)
		}

		parsed, err := ParseDNSKEY(15, keyData)
		if err != nil {
			t.Fatalf("parse Ed25519 DNSKEY: %v", err)
		}

		if parsed.Type != KeyTypeEdDSA {
			t.Fatalf("unexpected key type: got %v want %v", parsed.Type, KeyTypeEdDSA)
		}
		if parsed.Bits != 256 {
			t.Fatalf("unexpected key size: got %d bits want 256", parsed.Bits)
		}
		want := new(big.Int).SetBytes(keyData)
		if parsed.Val.Cmp(want) != 0 {
			t.Fatalf("unexpected Ed25519 value")
		}
	})

	t.Run("Ed448_Synthetic", func(t *testing.T) {
		keyData := make([]byte, 57)
		for i := range keyData {
			keyData[i] = byte(57 - i)
		}

		parsed, err := ParseDNSKEY(16, keyData)
		if err != nil {
			t.Fatalf("parse Ed448 DNSKEY: %v", err)
		}

		if parsed.Type != KeyTypeEdDSA {
			t.Fatalf("unexpected key type: got %v want %v", parsed.Type, KeyTypeEdDSA)
		}
		if parsed.Bits != 456 {
			t.Fatalf("unexpected key size: got %d bits want 456", parsed.Bits)
		}
		want := new(big.Int).SetBytes(keyData)
		if parsed.Val.Cmp(want) != 0 {
			t.Fatalf("unexpected Ed448 value")
		}
	})
}

func TestParseDNSKEYErrors(t *testing.T) {
	if _, err := ParseDNSKEY(99, []byte{1, 2, 3}); err == nil {
		t.Fatal("expected unsupported algorithm error")
	}
	if _, err := ParseDNSKEY(14, make([]byte, 95)); err == nil {
		t.Fatal("expected ECDSA P-384 length error")
	}
	if _, err := ParseDNSKEY(16, make([]byte, 56)); err == nil {
		t.Fatal("expected Ed448 length error")
	}
	if _, err := ParseDNSKEY(3, make([]byte, 212)); err == nil {
		t.Fatal("expected DSA length error")
	}
}

func TestBKHASH120KnownVectors(t *testing.T) {
	tests := []struct {
		name string
		val  int64
		want string
	}{
		{name: "one", val: 1, want: "4bf5122f344554c53bde2ebb8cd2b7"},
		{name: "three", val: 3, want: "084fed08b978af4d7d196a7446a86b"},
		{name: "rsa_e_65537", val: 65537, want: "85f90dfea1d8027e1463e5ca971a25"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			hash := BKHASH120(big.NewInt(tc.val))
			got := hex.EncodeToString(hash[:])
			if got != tc.want {
				t.Fatalf("unexpected BKHASH120 for %d: got %s want %s", tc.val, got, tc.want)
			}
		})
	}
}

func TestBlocklistBinarySearch(t *testing.T) {
	type row struct {
		value    int64
		hashHex  string
		sourceID byte
	}

	rows := []row{
		{value: 12345, hashHex: "3514acf61732f662da19625f7fe781", sourceID: 2},
		{value: 67890, hashHex: "b87911af3bb7ba89d986b04cf23732", sourceID: 4},
		{value: 99999, hashHex: "e000eae56817e71461d7e09afee505", sourceID: 9},
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].hashHex < rows[j].hashHex
	})

	data := make([]byte, 0, len(rows)*blockSize)
	for _, row := range rows {
		hashBytes, err := hex.DecodeString(row.hashHex)
		if err != nil {
			t.Fatalf("decode test hash %q: %v", row.hashHex, err)
		}
		if len(hashBytes) != 15 {
			t.Fatalf("unexpected test hash length: got %d want 15", len(hashBytes))
		}
		data = append(data, hashBytes...)
		data = append(data, row.sourceID)
	}

	bl := &Blocklist{
		Data: data,
		Sources: map[int]string{
			2: "alpha",
			4: "beta",
		},
		Entries: len(rows),
	}

	found := bl.Check(big.NewInt(67890))
	if found == nil {
		t.Fatal("expected blocklist hit for value 67890")
	}
	if found.SourceID != 4 || found.SourceName != "beta" {
		t.Fatalf("unexpected blocklist result: got id=%d name=%q", found.SourceID, found.SourceName)
	}

	unknownSource := bl.Check(big.NewInt(99999))
	if unknownSource == nil {
		t.Fatal("expected blocklist hit for value 99999")
	}
	if unknownSource.SourceName != "id9" {
		t.Fatalf("unexpected fallback source name: got %q want id9", unknownSource.SourceName)
	}

	if miss := bl.Check(big.NewInt(11111)); miss != nil {
		t.Fatalf("expected blocklist miss for value 11111, got %+v", miss)
	}
}

func TestRSAChecks(t *testing.T) {
	t.Run("Fermat", func(t *testing.T) {
		// n = 1000003 * 1000033 (close primes).
		n := new(big.Int).SetInt64(1000036000099)
		if !checkFermat(n) {
			t.Fatal("expected Fermat check to detect close-prime modulus")
		}
		if checkFermat(big.NewInt(1000000007)) {
			t.Fatal("did not expect Fermat hit for control value")
		}
	})

	t.Run("Pattern", func(t *testing.T) {
		patternN := new(big.Int).SetBytes(bytes.Repeat([]byte{0xaa}, 17))
		if !checkPattern(patternN) {
			t.Fatal("expected pattern check to detect repeated bytes")
		}

		control := new(big.Int).SetBytes([]byte{
			0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
			0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe,
			0x11, 0x22,
		})
		if checkPattern(control) {
			t.Fatal("did not expect pattern hit for control value")
		}
	})

	t.Run("ROCA", func(t *testing.T) {
		if !checkROCA(big.NewInt(65537)) {
			t.Fatal("expected ROCA check hit for known fingerprint value")
		}
		if checkROCA(big.NewInt(17)) {
			t.Fatal("did not expect ROCA hit for control value")
		}
	})

	t.Run("RSAInvalid", func(t *testing.T) {
		if !checkRSAInvalid(big.NewInt(101), big.NewInt(2)) {
			t.Fatal("expected invalid RSA when e < 3")
		}
		if !checkRSAInvalid(big.NewInt(101), big.NewInt(101)) {
			t.Fatal("expected invalid RSA when e >= n")
		}
		if checkRSAInvalid(big.NewInt(101), big.NewInt(17)) {
			t.Fatal("did not expect invalid RSA for valid parameters")
		}
	})

	t.Run("SmallFactors", func(t *testing.T) {
		if !checkSmallFactors(big.NewInt(21)) {
			t.Fatal("expected small factor detection for n=21")
		}

		control := new(big.Int).Mul(big.NewInt(65539), big.NewInt(65543))
		if checkSmallFactors(control) {
			t.Fatal("did not expect small factors for control value")
		}
	})

	t.Run("SmallD", func(t *testing.T) {
		// Synthetic Wiener-vulnerable RSA parameters.
		n := newBigFromDecimal("1000040000111")
		e := newBigFromDecimal("400015200029")
		if !checkSmallD(n, e) {
			t.Fatal("expected small d detection for synthetic vulnerable key")
		}

		if checkSmallD(n, big.NewInt(65537)) {
			t.Fatal("did not expect small d hit when e <= 32 bits")
		}
	})
}

func TestCheckDNSKEYCleanKeysAndBlocklist(t *testing.T) {
	rootAlgo, rootKey := loadDNSKEYVector(t, "dnssec-root-rsa.dnskey")
	findings, err := CheckDNSKEY(rootAlgo, rootKey, nil)
	if err != nil {
		t.Fatalf("check root RSA DNSKEY: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings for clean root RSA key, got %+v", findings)
	}

	p256Algo, p256Key := loadDNSKEYVector(t, "dnssec-p256-rfc6605.dnskey")
	findings, err = CheckDNSKEY(p256Algo, p256Key, nil)
	if err != nil {
		t.Fatalf("check P-256 DNSKEY without blocklist: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings for clean P-256 key without blocklist, got %+v", findings)
	}

	parsed, err := ParseDNSKEY(p256Algo, p256Key)
	if err != nil {
		t.Fatalf("parse P-256 DNSKEY for blocklist setup: %v", err)
	}
	hash := BKHASH120(parsed.Val)
	data := append(append([]byte{}, hash[:]...), byte(7))
	bl := &Blocklist{
		Data:    data,
		Sources: map[int]string{7: "rfc-example"},
		Entries: 1,
	}

	findings, err = CheckDNSKEY(p256Algo, p256Key, bl)
	if err != nil {
		t.Fatalf("check P-256 DNSKEY with blocklist: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly one blocklist finding, got %+v", findings)
	}
	if findings[0].Check != "blocklist" {
		t.Fatalf("unexpected finding check: got %q want blocklist", findings[0].Check)
	}
	if findings[0].BlocklistName != "rfc-example" {
		t.Fatalf("unexpected blocklist name: got %q want rfc-example", findings[0].BlocklistName)
	}
}

func loadDNSKEYVector(t *testing.T, name string) (uint8, []byte) {
	t.Helper()

	path := filepath.Join("testdata", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	fields := strings.Fields(string(raw))
	if len(fields) < 4 {
		t.Fatalf("unexpected DNSKEY vector format in %s", path)
	}

	algoNum, err := strconv.Atoi(fields[2])
	if err != nil {
		t.Fatalf("parse algorithm from %s: %v", path, err)
	}

	keyB64 := strings.Join(fields[3:], "")
	keyData, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		t.Fatalf("decode key material from %s: %v", path, err)
	}

	return uint8(algoNum), keyData
}
