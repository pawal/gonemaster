package share

import "embed"

// NamedRoot contains the bundled root hints file.
//
//go:embed named.root
var NamedRoot string

// ProfileJSON contains the bundled default profile.
//
//go:embed profile.json
var ProfileJSON []byte

// IanaIPv4CSV contains bundled IANA IPv4 special-purpose registry data.
//
//go:embed iana-ipv4-special-registry.csv
var IanaIPv4CSV string

// IanaIPv6CSV contains bundled IANA IPv6 special-purpose registry data.
//
//go:embed iana-ipv6-special-registry.csv
var IanaIPv6CSV string

// POFiles contains bundled translation catalogs.
//
//go:embed lang/*.po
var POFiles embed.FS
