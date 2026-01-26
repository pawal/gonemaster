package validation

import "testing"

func TestValidateIPv4(t *testing.T) {
	if !ValidateIPv4("192.0.2.1") {
		t.Fatalf("expected valid IPv4")
	}
	if ValidateIPv4("999.0.0.1") {
		t.Fatalf("expected invalid IPv4")
	}
	if ValidateIPv4("2001:db8::1") {
		t.Fatalf("expected IPv6 to be invalid for IPv4 check")
	}
}

func TestValidateIPv6(t *testing.T) {
	if !ValidateIPv6("2001:db8::1") {
		t.Fatalf("expected valid IPv6")
	}
	if ValidateIPv6("2001:db8::zz") {
		t.Fatalf("expected invalid IPv6")
	}
	if ValidateIPv6("192.0.2.1") {
		t.Fatalf("expected IPv4 to be invalid for IPv6 check")
	}
}
