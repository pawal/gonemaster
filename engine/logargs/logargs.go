package logargs

import (
	"net/netip"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
)

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
		if leftNS == "" && rightNS != "" {
			return false
		}
		if leftNS != "" && rightNS == "" {
			return true
		}
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

// ServersFromValues converts mixed endpoint identity values to canonical servers data.
//
// Each input value may be:
// - "<name>/<ip>"
// - "<name>"
// - "<ip>"
func ServersFromValues(values []string) map[string]any {
	if len(values) == 0 {
		return map[string]any{"servers": []map[string]any{}}
	}

	servers := make([]Server, 0, len(values))
	for _, value := range values {
		server, ok := serverFromValue(value)
		if !ok {
			continue
		}
		servers = append(servers, server)
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

// EnsureQueryIdentity adds canonical query identity keys when legacy query keys
// are present. Legacy keys are left intact to keep template compatibility
// during migration.
func EnsureQueryIdentity(args map[string]any) {
	if args == nil {
		return
	}

	// Prefer canonical key, then legacy aliases.
	if queryType := firstNonEmptyStringArg(args, "query_type", "rrtype", "type"); queryType != "" {
		args["query_type"] = strings.ToUpper(queryType)
	}

	// Prefer canonical key, then legacy alias.
	if queryClass := firstNonEmptyStringArg(args, "query_class", "class"); queryClass != "" {
		args["query_class"] = strings.ToUpper(queryClass)
	} else if _, ok := args["query_type"]; ok {
		// DNS query class is IN in current engine query callsites.
		args["query_class"] = "IN"
	}
}

// NormalizeQueryIdentity ensures canonical query keys and drops legacy
// query aliases used during migration.
func NormalizeQueryIdentity(args map[string]any) {
	if args == nil {
		return
	}
	EnsureQueryIdentity(args)
	delete(args, "rrtype")
	delete(args, "type")
	delete(args, "class")
}

// EndpointName returns a canonical nameserver name when value is a mixed
// endpoint identity in "<name>/<ip>" form. Non-endpoint values are returned as-is.
func EndpointName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	sep := strings.LastIndex(trimmed, "/")
	if sep <= 0 || sep >= len(trimmed)-1 {
		return trimmed
	}

	namePart := strings.TrimSpace(trimmed[:sep])
	addressPart := strings.TrimSpace(trimmed[sep+1:])
	if namePart == "" || addressPart == "" {
		return trimmed
	}

	if _, err := netip.ParseAddr(addressPart); err != nil {
		return trimmed
	}
	return normalizeName(namePart)
}

// UniqueSortedEndpointNames normalizes mixed endpoint identity strings to
// nameserver names when possible, then deduplicates and sorts.
func UniqueSortedEndpointNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		name := EndpointName(value)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func serverFromValue(value string) (Server, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Server{}, false
	}

	if ip, err := netip.ParseAddr(value); err == nil {
		return Server{Address: ip.String()}, true
	}

	sep := strings.LastIndex(value, "/")
	if sep > 0 && sep < len(value)-1 {
		namePart := strings.TrimSpace(value[:sep])
		addressPart := strings.TrimSpace(value[sep+1:])
		if namePart != "" && addressPart != "" {
			if ip, err := netip.ParseAddr(addressPart); err == nil {
				return Server{NS: namePart, Address: ip.String()}, true
			}
		}
	}

	return Server{NS: value}, true
}

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return strings.ToLower(dnsname.New(name).String())
}

func firstNonEmptyStringArg(args map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := args[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
	}
	return ""
}
