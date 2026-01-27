package util

import "codeberg.org/pawal/gonemaster/engine/constants"

// SerialGT implements RFC1982-style serial comparison.
func SerialGT(sa, sb uint32) bool {
	if sa < sb {
		return uint64(sb-sa) > (uint64(1) << (constants.SerialBits - 1))
	}
	if sa > sb {
		return uint64(sa-sb) < (uint64(1) << (constants.SerialBits - 1))
	}
	return false
}
