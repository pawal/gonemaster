package profile

import (
	"context"
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
