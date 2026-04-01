package nameserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

func TestClassifyRateLimitSignal(t *testing.T) {
	t.Parallel()

	timeoutErr := &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	tests := []struct {
		name string
		resp packet.Packet
		err  error
		want rateLimitSignal
	}{
		{
			name: "none for empty response and nil error",
			want: rateLimitSignalNone,
		},
		{
			name: "timeout pattern error",
			err:  timeoutErr,
			want: rateLimitSignalTimeoutPattern,
		},
		{
			name: "servfail response",
			resp: packetWithRcode(dns.RcodeServerFailure),
			want: rateLimitSignalServfailOrRefused,
		},
		{
			name: "refused response",
			resp: packetWithRcode(dns.RcodeRefused),
			want: rateLimitSignalServfailOrRefused,
		},
		{
			name: "connection reset error",
			err:  fmt.Errorf("query failed: %w", syscall.ECONNRESET),
			want: rateLimitSignalConnectionError,
		},
		{
			name: "hard unreachable stays separate",
			err:  fmt.Errorf("query failed: %w", syscall.EHOSTUNREACH),
			want: rateLimitSignalHardUnreachable,
		},
		{
			name: "context cancellation is not rate-limit",
			err:  context.Canceled,
			want: rateLimitSignalNone,
		},
		{
			name: "non-throttling error",
			err:  errors.New("malformed response"),
			want: rateLimitSignalNone,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyRateLimitSignal(tc.resp, tc.err); got != tc.want {
				t.Fatalf("classifyRateLimitSignal() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsConnectionRateLimitPatternError(t *testing.T) {
	t.Parallel()

	if isConnectionRateLimitPatternError(nil) {
		t.Fatalf("nil error should not be considered connection rate-limit pattern")
	}
	if isConnectionRateLimitPatternError(context.Canceled) {
		t.Fatalf("context cancellation should not be considered connection rate-limit pattern")
	}
	if isConnectionRateLimitPatternError(&net.DNSError{Err: "i/o timeout", IsTimeout: true}) {
		t.Fatalf("timeout error should not be considered connection rate-limit pattern")
	}
	if isConnectionRateLimitPatternError(syscall.EHOSTUNREACH) {
		t.Fatalf("hard unreachable should not be considered connection rate-limit pattern")
	}

	if !isConnectionRateLimitPatternError(syscall.ECONNREFUSED) {
		t.Fatalf("expected ECONNREFUSED to be considered connection rate-limit pattern")
	}
	if !isConnectionRateLimitPatternError(errors.New("read tcp 1.1.1.1:53: connection reset by peer")) {
		t.Fatalf("expected connection reset message to be considered connection rate-limit pattern")
	}
}

func TestIsTimeoutBurstSignal(t *testing.T) {
	t.Parallel()

	if isTimeoutBurstSignal(rateLimitTimeoutBurstThreshold - 1) {
		t.Fatalf("expected value below burst threshold to be false")
	}
	if !isTimeoutBurstSignal(rateLimitTimeoutBurstThreshold) {
		t.Fatalf("expected threshold value to trigger timeout burst")
	}
}

func TestIsServfailRefusedRatioSpike(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		count     int
		total     int
		wantSpike bool
	}{
		{name: "below min samples", count: 4, total: rateLimitServfailRefusedMinSamples - 1, wantSpike: false},
		{name: "below ratio threshold", count: 2, total: rateLimitServfailRefusedMinSamples, wantSpike: false},
		{name: "at ratio threshold", count: 3, total: 6, wantSpike: true},
		{name: "above ratio threshold", count: 5, total: 8, wantSpike: true},
		{name: "zero count", count: 0, total: 10, wantSpike: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isServfailRefusedRatioSpike(tc.count, tc.total); got != tc.wantSpike {
				t.Fatalf("isServfailRefusedRatioSpike(%d, %d) = %v, want %v", tc.count, tc.total, got, tc.wantSpike)
			}
		})
	}
}

func packetWithRcode(rcode int) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = uint16(rcode)
	return packet.Packet{Msg: msg}
}
