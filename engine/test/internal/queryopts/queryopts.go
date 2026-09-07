// Package queryopts holds query option sets used by more than one testcase.
// Testcases asking the same question must pass identical options: the per-run
// response cache keys on them, so a mismatch turns one exchange into two.
package queryopts

import (
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

// SmallEDNSPayload is the advertised payload of the small-answer probe.
const SmallEDNSPayload = 512

// SmallAnswerDNSKEY is a DO-enabled DNSKEY query advertising 512 bytes over
// UDP with fallback off. Shared by nameserver13 and connectivity05.
func SmallAnswerDNSKEY() *nameserver.QueryOptions {
	version := uint8(0)
	do := true
	size := uint16(SmallEDNSPayload)
	useVC := false
	fallback := false
	return &nameserver.QueryOptions{
		UseVC:    &useVC,
		Fallback: &fallback,
		EDNSDetails: &transport.EDNSDetails{
			Version: &version,
			Do:      &do,
			Size:    &size,
		},
	}
}
