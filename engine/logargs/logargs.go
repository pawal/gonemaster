package logargs

import (
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
)

// SchemaID is the canonical args schema identifier for coherent log arguments.
const SchemaID = "gonemaster.logargs/1.1"

// Server represents a nameserver endpoint using canonical key names.
type Server struct {
	NS      string
	Address string
}

// NameserverLike is a minimal adapter for endpoint types used by ServersFromNameservers.
//
// It is intentionally defined here to avoid importing engine/nameserver and creating
// package cycles when nameserver code starts consuming these helpers.
type NameserverLike interface {
	NameString() string
	AddressString() string
}

// EnsureSchema sets the args schema marker on the map and returns it.
func EnsureSchema(args map[string]any) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	args["arg_schema"] = SchemaID
	return args
}

// NS returns canonical singular endpoint fields.
//
// Output keys:
// - "ns": normalized nameserver name (if non-empty)
// - "address": endpoint IP address (if non-empty)
func NS(name string, address string) map[string]any {
	args := map[string]any{}
	SetNS(args, name, address)
	return args
}

// SetNS writes canonical singular endpoint fields into args.
func SetNS(args map[string]any, name string, address string) {
	if args == nil {
		return
	}
	name = normalizeName(name)
	address = strings.TrimSpace(address)
	if name != "" {
		args["ns"] = name
	}
	if address != "" {
		args["address"] = address
	}
}

// Servers builds canonical structured endpoint list data.
//
// Output key:
// - "servers": []map[string]any{{"ns": "...", "address": "..."}, ...}
func Servers(items []Server) map[string]any {
	servers := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{}
		SetNS(entry, item.NS, item.Address)
		if len(entry) == 0 {
			continue
		}
		servers = append(servers, entry)
	}
	sort.Slice(servers, func(i, j int) bool {
		leftNS, _ := servers[i]["ns"].(string)
		rightNS, _ := servers[j]["ns"].(string)
		if leftNS != rightNS {
			return leftNS < rightNS
		}
		leftAddr, _ := servers[i]["address"].(string)
		rightAddr, _ := servers[j]["address"].(string)
		return leftAddr < rightAddr
	})
	return map[string]any{"servers": servers}
}

// ServersFromNameservers converts nameserver-like values to canonical servers data.
func ServersFromNameservers[T NameserverLike](items []T) map[string]any {
	servers := make([]Server, 0, len(items))
	for _, item := range items {
		servers = append(servers, Server{
			NS:      item.NameString(),
			Address: item.AddressString(),
		})
	}
	return Servers(servers)
}

// QueryIdentity returns canonical query identity fields.
//
// Output keys:
// - "query_name"
// - "query_type"
// - "query_class"
func QueryIdentity(name string, qtype string, qclass string) map[string]any {
	args := map[string]any{}
	SetQueryIdentity(args, name, qtype, qclass)
	return args
}

// SetQueryIdentity writes canonical query identity fields into args.
func SetQueryIdentity(args map[string]any, name string, qtype string, qclass string) {
	if args == nil {
		return
	}
	name = normalizeName(name)
	qtype = strings.ToUpper(strings.TrimSpace(qtype))
	qclass = strings.ToUpper(strings.TrimSpace(qclass))
	if name != "" {
		args["query_name"] = name
	}
	if qtype != "" {
		args["query_type"] = qtype
	}
	if qclass != "" {
		args["query_class"] = qclass
	}
}

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.ToLower(dnsname.New(name).String())
}
