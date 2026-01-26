package share

import "embed"

//go:embed named.root
var NamedRoot string

//go:embed profile.json
var ProfileJSON []byte

//go:embed iana-ipv4-special-registry.csv
var IanaIPv4CSV string

//go:embed iana-ipv6-special-registry.csv
var IanaIPv6CSV string

//go:embed lang/*.po
var POFiles embed.FS
