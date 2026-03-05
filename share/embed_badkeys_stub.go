//go:build !badkeys_embed

package share

// BadkeysBlocklist is nil when built without the badkeys_embed build tag.
var BadkeysBlocklist []byte

// BadkeysMetadata is nil when built without the badkeys_embed build tag.
var BadkeysMetadata []byte
