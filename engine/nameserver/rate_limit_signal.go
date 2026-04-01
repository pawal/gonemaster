package nameserver

import (
	"errors"
	"strings"
	"syscall"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

type rateLimitSignal int

const (
	rateLimitSignalNone rateLimitSignal = iota
	rateLimitSignalTimeoutPattern
	rateLimitSignalServfailOrRefused
	rateLimitSignalConnectionError
	rateLimitSignalHardUnreachable
)

const (
	rateLimitTimeoutBurstThreshold       = 2
	rateLimitServfailRefusedMinSamples   = 6
	rateLimitServfailRefusedSpikePercent = 0.5
)

func rateLimitSignalString(signal rateLimitSignal) string {
	switch signal {
	case rateLimitSignalTimeoutPattern:
		return "timeout_pattern"
	case rateLimitSignalServfailOrRefused:
		return "servfail_or_refused"
	case rateLimitSignalConnectionError:
		return "connection_error"
	case rateLimitSignalHardUnreachable:
		return "hard_unreachable"
	default:
		return "none"
	}
}

func classifyRateLimitSignal(resp packet.Packet, err error) rateLimitSignal {
	if err != nil {
		switch {
		case isHardNetworkError(err):
			return rateLimitSignalHardUnreachable
		case isTimeoutPatternError(err):
			return rateLimitSignalTimeoutPattern
		case isConnectionRateLimitPatternError(err):
			return rateLimitSignalConnectionError
		default:
			return rateLimitSignalNone
		}
	}

	if resp.Msg == nil {
		return rateLimitSignalNone
	}
	switch resp.Msg.Rcode {
	case dns.RcodeServerFailure, dns.RcodeRefused:
		return rateLimitSignalServfailOrRefused
	default:
		return rateLimitSignalNone
	}
}

func isConnectionRateLimitPatternError(err error) bool {
	if err == nil {
		return false
	}
	if isHardNetworkError(err) || isTimeoutPatternError(err) {
		return false
	}

	candidates := []error{
		syscall.ECONNRESET,
		syscall.ECONNREFUSED,
		syscall.ECONNABORTED,
		syscall.EPIPE,
	}
	for _, target := range candidates {
		if errors.Is(err, target) {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection aborted") ||
		strings.Contains(msg, "broken pipe")
}

func isTimeoutBurstSignal(consecutiveTimeouts int) bool {
	return consecutiveTimeouts >= rateLimitTimeoutBurstThreshold
}

func isServfailRefusedRatioSpike(servfailOrRefusedCount int, totalCount int) bool {
	if totalCount < rateLimitServfailRefusedMinSamples || servfailOrRefusedCount <= 0 {
		return false
	}
	ratio := float64(servfailOrRefusedCount) / float64(totalCount)
	return ratio >= rateLimitServfailRefusedSpikePercent
}
