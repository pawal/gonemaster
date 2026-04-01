//go:build badkeys_embed

package share

import _ "embed"

// BadkeysBlocklist contains the gzip-compressed badkeys blocklist.
// Only available when built with the badkeys_embed build tag.
//
//go:embed badkeys/blocklist.dat.gz
var BadkeysBlocklist []byte

// BadkeysMetadata contains the badkeys metadata JSON.
// Only available when built with the badkeys_embed build tag.
//
//go:embed badkeys/badkeysdata.json
var BadkeysMetadata []byte
