package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

const tokenHashPrefix = "sha256:"

// adminToken is a parsed, validated admin credential.
type adminToken struct {
	label string
	hash  []byte
}

// tokenSet is an immutable set of admin tokens. enabled is false in open mode.
type tokenSet struct {
	enabled bool
	tokens  []adminToken
}

// hashToken returns the "sha256:<hex>" form of a plaintext token.
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return tokenHashPrefix + hex.EncodeToString(sum[:])
}

// parseTokenHash decodes a "sha256:<hex>" string into a raw digest.
func parseTokenHash(s string) ([]byte, error) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(s), tokenHashPrefix)
	if !ok {
		return nil, fmt.Errorf("token hash must start with %q", tokenHashPrefix)
	}
	raw, err := hex.DecodeString(rest)
	if err != nil {
		return nil, fmt.Errorf("token hash is not valid hex: %w", err)
	}
	if len(raw) != sha256.Size {
		return nil, fmt.Errorf("token hash must be %d-byte sha256", sha256.Size)
	}
	return raw, nil
}

// newTokenSet builds a tokenSet from config, validating each hash.
func newTokenSet(cfg AuthConfig) (*tokenSet, error) {
	ts := &tokenSet{}
	for i, t := range cfg.AdminTokens {
		raw, err := parseTokenHash(t.Hash)
		if err != nil {
			return nil, fmt.Errorf("admin_tokens[%d] (%s): %w", i, t.Label, err)
		}
		ts.tokens = append(ts.tokens, adminToken{label: t.Label, hash: raw})
	}
	ts.enabled = len(ts.tokens) > 0
	return ts, nil
}

// ValidateAuthConfig reports whether the auth config parses. Used at startup.
func ValidateAuthConfig(cfg AuthConfig) error {
	_, err := newTokenSet(cfg)
	return err
}

// match compares plaintext against every token in constant time so timing does
// not reveal which token (or whether any) matched. Returns the label on success.
func (ts *tokenSet) match(plaintext string) (string, bool) {
	sum := sha256.Sum256([]byte(plaintext))
	found := 0
	label := ""
	for _, t := range ts.tokens {
		if subtle.ConstantTimeCompare(sum[:], t.hash) == 1 {
			found = 1
			label = t.label
		}
	}
	return label, found == 1
}
