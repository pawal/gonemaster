package dnsname

import "testing"

func TestNameBasics(t *testing.T) {
	name := New("www.example.com")
	if name.String() != "www.example.com" {
		t.Fatalf("unexpected string: %q", name.String())
	}
	if name.FQDN() != "www.example.com." {
		t.Fatalf("unexpected FQDN: %q", name.FQDN())
	}

	parent, ok := name.NextHigher()
	if !ok || parent.String() != "example.com" {
		t.Fatalf("unexpected parent: %v %q", ok, parent.String())
	}

	root, ok := parent.NextHigher()
	if !ok || root.String() != "com" {
		t.Fatalf("unexpected higher name: %v %q", ok, root.String())
	}
	if _, ok := New("").NextHigher(); ok {
		t.Fatalf("expected no higher name for root")
	}
}

func TestNameComparisons(t *testing.T) {
	left := New("Example.COM")
	right := New("example.com")
	if left.Compare(right) != 0 {
		t.Fatalf("expected Compare to ignore case")
	}
	if left.CompareString("example.com.") != 0 {
		t.Fatalf("expected CompareString to ignore trailing dot")
	}
}

func TestNameCommonAndBailiwick(t *testing.T) {
	zone := New("example.com")
	target := New("www.example.com")
	if !zone.IsInBailiwick(target) {
		t.Fatalf("expected name in bailiwick")
	}
	if zone.Common(New("other.net")) != 0 {
		t.Fatalf("expected no common suffix")
	}
	if New("a.b.c").Common(New("x.b.c")) != 2 {
		t.Fatalf("expected common suffix length 2")
	}
}

func TestNamePrepend(t *testing.T) {
	name := New("example.com").Prepend("www")
	if name.String() != "www.example.com" {
		t.Fatalf("unexpected prepend result: %q", name.String())
	}
}

func TestFromStringTrailingDot(t *testing.T) {
	name, err := FromString("example.com.")
	if err != nil {
		t.Fatalf("from string: %v", err)
	}
	if name.String() != "example.com" {
		t.Fatalf("unexpected string: %q", name.String())
	}
}
