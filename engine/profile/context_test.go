package profile

import (
	"context"
	"net/netip"
	"testing"
)

func TestProfileContextRoundTrip(t *testing.T) {
	p := New()
	ctx := WithContext(context.Background(), p)
	if got := FromContext(ctx); got != p {
		t.Fatalf("expected profile %p, got %p", p, got)
	}
}

func TestProfileContextNil(t *testing.T) {
	var nilCtx context.Context
	if got := FromContext(nilCtx); got == nil {
		t.Fatalf("expected non-nil profile")
	}
}

func TestProfileMustFromContextPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	_ = MustFromContext(context.Background())
}

func TestSetEffective(t *testing.T) {
	defer ResetEffective()
	p := New()
	SetEffective(p)
	if Effective() != p {
		t.Fatalf("expected SetEffective to update effective profile")
	}
}

func TestAllowedTargets(t *testing.T) {
	// An address recorded in the allow-set is exempt; others are not. Keys are
	// matched after Unmap, so an IPv4-mapped IPv6 form of a stored IPv4 address
	// is recognised, and lookups against an empty/absent context are false.
	allowed := netip.MustParseAddr("10.0.0.1")
	set := map[netip.Addr]struct{}{allowed: {}}
	ctx := WithAllowedTargets(context.Background(), set)

	if !IsAllowedTarget(ctx, allowed) {
		t.Errorf("expected 10.0.0.1 to be allowed")
	}
	if !IsAllowedTarget(ctx, netip.MustParseAddr("::ffff:10.0.0.1")) {
		t.Errorf("expected v4-mapped form of an allowed address to be allowed")
	}
	if IsAllowedTarget(ctx, netip.MustParseAddr("192.168.0.1")) {
		t.Errorf("did not expect 192.168.0.1 to be allowed")
	}
	if IsAllowedTarget(context.Background(), allowed) {
		t.Errorf("did not expect an allowed target without the set in context")
	}
	if IsAllowedTarget(nil, allowed) {
		t.Errorf("did not expect an allowed target for nil context")
	}
}
