package validation

import "testing"

// ipCases exercise both validators against the same inputs, so each family
// check also covers rejecting the other family.
var ipCases = []struct {
	name   string
	input  string
	wantV4 bool
	wantV6 bool
}{
	{name: "IPv4 address", input: "192.0.2.1", wantV4: true},
	{name: "IPv4 octet out of range", input: "999.0.0.1"},
	{name: "IPv6 address", input: "2001:db8::1", wantV6: true},
	{name: "IPv6 with non-hex digits", input: "2001:db8::zz"},
}

func TestValidateIPv4(t *testing.T) {
	for _, tc := range ipCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateIPv4(tc.input); got != tc.wantV4 {
				t.Fatalf("ValidateIPv4(%q) = %v, want %v", tc.input, got, tc.wantV4)
			}
		})
	}
}

func TestValidateIPv6(t *testing.T) {
	for _, tc := range ipCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateIPv6(tc.input); got != tc.wantV6 {
				t.Fatalf("ValidateIPv6(%q) = %v, want %v", tc.input, got, tc.wantV6)
			}
		})
	}
}
