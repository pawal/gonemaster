// Package publicapi maps a gonemaster-server admin API base onto the public
// API base, which serves the analysis reads and needs no token.
package publicapi

import "strings"

// Base returns the public API base that sits beside the admin base at base.
func Base(base string) string {
	trimmed := strings.TrimRight(base, "/")
	if strings.HasSuffix(trimmed, "/api/v1") {
		return strings.TrimSuffix(trimmed, "/api/v1") + "/pub/api/v1"
	}
	return trimmed + "/pub/api/v1"
}
