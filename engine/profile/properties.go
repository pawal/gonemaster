package profile

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/validation"
)

type propertyType int

const (
	propBool propertyType = iota
	propNum
	propStr
	propArray
	propMap
)

type valueSource int

const (
	sourceDirect valueSource = iota
	sourceJSON
)

const (
	duration5Minutes = 5 * 60
	duration1Hour    = 60 * 60
	duration4Hours   = 4 * 60 * 60
	duration12Hours  = 12 * 60 * 60
	duration1Day     = 24 * 60 * 60
	duration1Week    = 7 * 24 * 60 * 60
	duration180Days  = 180 * 24 * 60 * 60
)

type propertyDef struct {
	typ          propertyType
	min          *int
	max          *int
	defaultValue any
	hasDefault   bool
	validate     func(any) (any, error)
	setter       func(*Profile, any)
	getter       func(*Profile) any
}

var propertyDefs = map[string]propertyDef{
	"cache": {
		typ:          propMap,
		defaultValue: map[string]map[string]any{},
		hasDefault:   true,
		validate:     validateCache,
		setter: func(p *Profile, value any) {
			p.Cache = value.(map[string]map[string]any)
		},
		getter: func(p *Profile) any {
			return p.Cache
		},
	},
	"resolver.defaults.debug": {
		typ: propBool,
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.Debug = value.(bool)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.Debug
		},
	},
	"resolver.defaults.igntc": {
		typ: propBool,
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.IgnTC = value.(bool)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.IgnTC
		},
	},
	"resolver.defaults.fallback": {
		typ: propBool,
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.Fallback = value.(bool)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.Fallback
		},
	},
	"resolver.defaults.recurse": {
		typ: propBool,
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.Recurse = value.(bool)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.Recurse
		},
	},
	"resolver.defaults.retrans": {
		typ: propNum,
		min: intPtr(1),
		max: intPtr(255),
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.Retrans = value.(int)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.Retrans
		},
	},
	"resolver.defaults.retry": {
		typ: propNum,
		min: intPtr(1),
		max: intPtr(255),
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.Retry = value.(int)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.Retry
		},
	},
	"resolver.defaults.parallel": {
		typ: propNum,
		min: intPtr(1),
		max: intPtr(255),
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.Parallel = value.(int)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.Parallel
		},
	},
	"resolver.defaults.unordered": {
		typ: propBool,
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.Unordered = value.(bool)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.Unordered
		},
	},
	"resolver.defaults.usevc": {
		typ: propBool,
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.UseVC = value.(bool)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.UseVC
		},
	},
	"resolver.defaults.timeout": {
		typ: propNum,
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.Timeout = value.(int)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.Timeout
		},
	},
	"resolver.defaults.error_cache_ttl": {
		typ: propNum,
		min: intPtr(0),
		max: intPtr(86400),
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.ErrorCacheTTL = value.(int)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.ErrorCacheTTL
		},
	},
	"resolver.defaults.positive_cache_ttl": {
		typ: propNum,
		min: intPtr(0),
		max: intPtr(86400),
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.PositiveCacheTTL = value.(int)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.PositiveCacheTTL
		},
	},
	"resolver.defaults.negative_cache_ttl": {
		typ: propNum,
		min: intPtr(0),
		max: intPtr(86400),
		setter: func(p *Profile, value any) {
			p.Resolver.Defaults.NegativeCacheTTL = value.(int)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Defaults.NegativeCacheTTL
		},
	},
	"resolver.source4": {
		typ:          propStr,
		defaultValue: "",
		hasDefault:   true,
		validate:     validateSource4,
		setter: func(p *Profile, value any) {
			p.Resolver.Source4 = value.(string)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Source4
		},
	},
	"resolver.source6": {
		typ:          propStr,
		defaultValue: "",
		hasDefault:   true,
		validate:     validateSource6,
		setter: func(p *Profile, value any) {
			p.Resolver.Source6 = value.(string)
		},
		getter: func(p *Profile) any {
			return p.Resolver.Source6
		},
	},
	"net.ipv4": {
		typ: propBool,
		setter: func(p *Profile, value any) {
			p.Net.IPv4 = value.(bool)
		},
		getter: func(p *Profile) any {
			return p.Net.IPv4
		},
	},
	"net.ipv6": {
		typ: propBool,
		setter: func(p *Profile, value any) {
			p.Net.IPv6 = value.(bool)
		},
		getter: func(p *Profile) any {
			return p.Net.IPv6
		},
	},
	"no_network": {
		typ: propBool,
		setter: func(p *Profile, value any) {
			p.NoNetwork = value.(bool)
		},
		getter: func(p *Profile) any {
			return p.NoNetwork
		},
	},
	"asn_db.style": {
		typ:          propStr,
		defaultValue: "cymru",
		hasDefault:   true,
		validate:     validateASNStyle,
		setter: func(p *Profile, value any) {
			p.ASNDB.Style = value.(string)
		},
		getter: func(p *Profile) any {
			return p.ASNDB.Style
		},
	},
	"asn_db.sources": {
		typ:          propMap,
		defaultValue: map[string][]string{"cymru": {"asnlookup.zonemaster.net"}},
		hasDefault:   true,
		validate:     validateASNSources,
		setter: func(p *Profile, value any) {
			p.ASNDB.Sources = value.(map[string][]string)
		},
		getter: func(p *Profile) any {
			return p.ASNDB.Sources
		},
	},
	"badkeys.path": {
		typ:          propStr,
		defaultValue: "",
		hasDefault:   true,
		setter: func(p *Profile, value any) {
			p.Badkeys.Path = value.(string)
		},
		getter: func(p *Profile) any {
			return p.Badkeys.Path
		},
	},
	"logfilter": {
		typ:          propMap,
		defaultValue: map[string]map[string][]LogFilterRule{},
		hasDefault:   true,
		validate:     validateLogFilter,
		setter: func(p *Profile, value any) {
			p.LogFilter = value.(map[string]map[string][]LogFilterRule)
		},
		getter: func(p *Profile) any {
			return p.LogFilter
		},
	},
	"test_levels": {
		typ:      propMap,
		validate: validateTestLevels,
		setter: func(p *Profile, value any) {
			p.TestLevels = value.(map[string]map[string]string)
		},
		getter: func(p *Profile) any {
			return p.TestLevels
		},
	},
	"test_cases": {
		typ: propArray,
		setter: func(p *Profile, value any) {
			p.TestCases = value.([]any)
		},
		getter: func(p *Profile) any {
			return p.TestCases
		},
	},
	"test_cases_vars.dnssec04.REMAINING_SHORT": {
		typ:          propNum,
		min:          intPtr(1),
		defaultValue: duration12Hours,
		hasDefault:   true,
		setter: func(p *Profile, value any) {
			p.TestCasesVars.DNSSEC04.RemainingShort = value.(int)
		},
		getter: func(p *Profile) any {
			return p.TestCasesVars.DNSSEC04.RemainingShort
		},
	},
	"test_cases_vars.dnssec04.REMAINING_LONG": {
		typ:          propNum,
		min:          intPtr(1),
		defaultValue: duration180Days,
		hasDefault:   true,
		setter: func(p *Profile, value any) {
			p.TestCasesVars.DNSSEC04.RemainingLong = value.(int)
		},
		getter: func(p *Profile) any {
			return p.TestCasesVars.DNSSEC04.RemainingLong
		},
	},
	"test_cases_vars.dnssec04.DURATION_LONG": {
		typ:          propNum,
		min:          intPtr(1),
		defaultValue: duration180Days,
		hasDefault:   true,
		setter: func(p *Profile, value any) {
			p.TestCasesVars.DNSSEC04.DurationLong = value.(int)
		},
		getter: func(p *Profile) any {
			return p.TestCasesVars.DNSSEC04.DurationLong
		},
	},
	"test_cases_vars.zone02.SOA_REFRESH_MINIMUM_VALUE": {
		typ:          propNum,
		min:          intPtr(1),
		defaultValue: duration4Hours,
		hasDefault:   true,
		setter: func(p *Profile, value any) {
			p.TestCasesVars.Zone02.SOARefreshMinimumValue = value.(int)
		},
		getter: func(p *Profile) any {
			return p.TestCasesVars.Zone02.SOARefreshMinimumValue
		},
	},
	"test_cases_vars.zone04.SOA_RETRY_MINIMUM_VALUE": {
		typ:          propNum,
		min:          intPtr(1),
		defaultValue: duration1Hour,
		hasDefault:   true,
		setter: func(p *Profile, value any) {
			p.TestCasesVars.Zone04.SOARetryMinimumValue = value.(int)
		},
		getter: func(p *Profile) any {
			return p.TestCasesVars.Zone04.SOARetryMinimumValue
		},
	},
	"test_cases_vars.zone05.SOA_EXPIRE_MINIMUM_VALUE": {
		typ:          propNum,
		min:          intPtr(1),
		defaultValue: duration1Week,
		hasDefault:   true,
		setter: func(p *Profile, value any) {
			p.TestCasesVars.Zone05.SOAExpireMinimumValue = value.(int)
		},
		getter: func(p *Profile) any {
			return p.TestCasesVars.Zone05.SOAExpireMinimumValue
		},
	},
	"test_cases_vars.zone06.SOA_DEFAULT_TTL_MAXIMUM_VALUE": {
		typ:          propNum,
		min:          intPtr(1),
		defaultValue: duration1Day,
		hasDefault:   true,
		setter: func(p *Profile, value any) {
			p.TestCasesVars.Zone06.SOADefaultTTLMaximumValue = value.(int)
		},
		getter: func(p *Profile) any {
			return p.TestCasesVars.Zone06.SOADefaultTTLMaximumValue
		},
	},
	"test_cases_vars.zone06.SOA_DEFAULT_TTL_MINIMUM_VALUE": {
		typ:          propNum,
		min:          intPtr(1),
		defaultValue: duration5Minutes,
		hasDefault:   true,
		setter: func(p *Profile, value any) {
			p.TestCasesVars.Zone06.SOADefaultTTLMinimumValue = value.(int)
		},
		getter: func(p *Profile) any {
			return p.TestCasesVars.Zone06.SOADefaultTTLMinimumValue
		},
	},
}

func intPtr(value int) *int {
	return &value
}

func (p *Profile) setWithSource(path string, value any, source valueSource) error {
	if p == nil {
		return fmt.Errorf("profile is nil")
	}
	def, ok := propertyDefs[path]
	if !ok {
		return fmt.Errorf("unknown property %q", path)
	}
	if value == nil {
		return fmt.Errorf("property %s can not be nil", path)
	}
	normalized, err := normalizeValue(path, def, value, source)
	if err != nil {
		return err
	}
	if def.validate != nil {
		normalized, err = def.validate(normalized)
		if err != nil {
			return err
		}
	}
	def.setter(p, normalized)
	p.markSet(path)
	return nil
}

func normalizeValue(path string, def propertyDef, value any, source valueSource) (any, error) {
	switch def.typ {
	case propBool:
		return normalizeBool(path, value, source)
	case propNum:
		return normalizeNum(path, value, source, def.min, def.max)
	case propStr:
		return normalizeString(path, value, source)
	case propArray:
		return normalizeArray(path, value)
	case propMap:
		return normalizeMap(path, value)
	default:
		return nil, fmt.Errorf("unknown property type for %s", path)
	}
}

func normalizeBool(path string, value any, source valueSource) (bool, error) {
	switch source {
	case sourceJSON:
		if v, ok := value.(bool); ok {
			return v, nil
		}
		return false, fmt.Errorf("property %s expects boolean value", path)
	default:
		switch v := value.(type) {
		case bool:
			return v, nil
		case json.Number:
			num, err := v.Int64()
			if err != nil {
				return false, fmt.Errorf("property %s expects boolean value", path)
			}
			return num != 0, nil
		case int:
			return v != 0, nil
		case int64:
			return v != 0, nil
		case int32:
			return v != 0, nil
		case uint:
			return v != 0, nil
		case uint64:
			return v != 0, nil
		case float64:
			return v != 0, nil
		case string:
			if v == "" || v == "0" {
				return false, nil
			}
			return true, nil
		default:
			return false, fmt.Errorf("property %s expects boolean value", path)
		}
	}
}

func normalizeNum(path string, value any, source valueSource, min *int, max *int) (int, error) {
	num, err := parseIntValue(value, source == sourceDirect)
	if err != nil {
		return 0, fmt.Errorf("property %s expects non-negative integer", path)
	}
	if min != nil && num < *min {
		return 0, fmt.Errorf("property %s value is out of limit (smaller)", path)
	}
	if max != nil && num > *max {
		return 0, fmt.Errorf("property %s value is out of limit (bigger)", path)
	}
	return num, nil
}

func parseIntValue(value any, allowString bool) (int, error) {
	switch v := value.(type) {
	case json.Number:
		if strings.Contains(v.String(), ".") {
			return 0, fmt.Errorf("float")
		}
		num, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, err
		}
		if num < 0 {
			return 0, fmt.Errorf("negative")
		}
		return num, nil
	case float64:
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("float")
		}
		if v < 0 {
			return 0, fmt.Errorf("negative")
		}
		return int(v), nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("negative")
		}
		return v, nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("negative")
		}
		return int(v), nil
	case int32:
		if v < 0 {
			return 0, fmt.Errorf("negative")
		}
		return int(v), nil
	case uint:
		return int(v), nil
	case uint64:
		return int(v), nil
	case string:
		if !allowString {
			return 0, fmt.Errorf("string")
		}
		if v == "" || strings.ContainsAny(v, "+-.") {
			return 0, fmt.Errorf("string")
		}
		num, err := strconv.Atoi(v)
		if err != nil || num < 0 {
			return 0, fmt.Errorf("string")
		}
		return num, nil
	default:
		return 0, fmt.Errorf("invalid")
	}
}

func normalizeString(path string, value any, source valueSource) (string, error) {
	if source == sourceJSON {
		if v, ok := value.(string); ok {
			return v, nil
		}
		return "", fmt.Errorf("property %s expects string value", path)
	}
	if v, ok := value.(string); ok {
		return v, nil
	}
	return "", fmt.Errorf("property %s expects string value", path)
}

func normalizeArray(path string, value any) ([]any, error) {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("property %s is not an array", path)
	}
	out := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out, nil
}

func normalizeMap(path string, value any) (any, error) {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map {
		return nil, fmt.Errorf("property %s is not a map", path)
	}
	if rv.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("property %s expects string keys", path)
	}
	return value, nil
}

func validateSource4(value any) (any, error) {
	source := value.(string)
	if source == "" {
		return source, nil
	}
	if !validation.ValidateIPv4(source) {
		return nil, fmt.Errorf("property resolver.source4 must be an IPv4 address or the empty string")
	}
	return source, nil
}

func validateSource6(value any) (any, error) {
	source := value.(string)
	if source == "" {
		return source, nil
	}
	if !validation.ValidateIPv6(source) {
		return nil, fmt.Errorf("property resolver.source6 must be a valid IPv6 address or the empty string")
	}
	return source, nil
}

func validateASNStyle(value any) (any, error) {
	style := strings.ToLower(value.(string))
	if style != "cymru" && style != "ripe" {
		return nil, fmt.Errorf("property asn_db.style has 2 possible values : Cymru or RIPE (case-insensitive)")
	}
	return style, nil
}

func validateASNSources(value any) (any, error) {
	raw, err := mapStringAny(value)
	if err != nil {
		return nil, fmt.Errorf("property asn_db.sources keys have 2 possible values : Cymru or RIPE (case-insensitive)")
	}
	out := map[string][]string{}
	for key, rawList := range raw {
		lower := strings.ToLower(key)
		if lower != "cymru" && lower != "ripe" {
			return nil, fmt.Errorf("property asn_db.sources keys have 2 possible values : Cymru or RIPE (case-insensitive)")
		}
		list, err := toStringSlice(rawList)
		if err != nil {
			return nil, fmt.Errorf("property asn_db.sources.%s has a non scalar item", key)
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("property asn_db.sources.%s has no items", key)
		}
		for _, item := range list {
			if item == "" {
				return nil, fmt.Errorf("property asn_db.sources.%s has a NULL item", key)
			}
			if len(item) > 255 {
				return nil, fmt.Errorf("property asn_db.sources.%s has an item too long", key)
			}
			if !isValidDomain(item) {
				return nil, fmt.Errorf("property asn_db.sources.%s has a non domain name item", key)
			}
		}
		out[lower] = list
	}
	return out, nil
}

func validateLogFilter(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("property logfilter is not a map")
	}
	var out map[string]map[string][]LogFilterRule
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("property logfilter is not a map")
	}
	return out, nil
}

func validateTestLevels(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("property test_levels is not a map")
	}
	var out map[string]map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("property test_levels is not a map")
	}
	return out, nil
}

func validateCache(value any) (any, error) {
	raw, err := mapStringAny(value)
	if err != nil {
		return nil, fmt.Errorf("property cache is not a map")
	}
	allowedKeys := map[string]bool{"redis": true}
	out := map[string]map[string]any{}
	for key, rawSub := range raw {
		if !allowedKeys[key] {
			return nil, fmt.Errorf("property cache keys have 1 possible values: redis")
		}
		sub, err := mapStringAny(rawSub)
		if err != nil {
			return nil, fmt.Errorf("property cache.%s is not a map", key)
		}
		if len(sub) == 0 {
			return nil, fmt.Errorf("property cache.%s has no items", key)
		}
		outSub := map[string]any{}
		allowedSub := map[string]bool{"server": true, "expire": true}
		for subKey, subValue := range sub {
			if !allowedSub[subKey] {
				return nil, fmt.Errorf("property cache.%s subkeys have 2 possible values: server, expire", key)
			}
			if isEmptyValue(subValue) {
				return nil, fmt.Errorf("property cache.%s.%s has a NULL or empty item", key, subKey)
			}
			if isNegativeNumber(subValue) {
				return nil, fmt.Errorf("property cache.%s.%s has a negative value", key, subKey)
			}
			outSub[subKey] = subValue
		}
		out[key] = outSub
	}
	return out, nil
}

func mapStringAny(value any) (map[string]any, error) {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("not a map")
	}
	out := map[string]any{}
	for _, key := range rv.MapKeys() {
		out[key.String()] = rv.MapIndex(key).Interface()
	}
	return out, nil
}

func toStringSlice(value any) ([]string, error) {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("not a list")
	}
	out := make([]string, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()
		switch v := item.(type) {
		case string:
			out[i] = v
		case json.Number:
			out[i] = v.String()
		case fmt.Stringer:
			out[i] = v.String()
		default:
			if rv.Index(i).Kind() == reflect.String {
				out[i] = rv.Index(i).String()
			} else {
				return nil, fmt.Errorf("not a string")
			}
		}
	}
	return out, nil
}

var domainLabelRe = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$`)

func isValidDomain(name string) bool {
	labels := strings.Split(name, ".")
	for _, label := range labels {
		if label == "" || !domainLabelRe.MatchString(label) {
			return false
		}
	}
	return true
}

func isEmptyValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == "" || v == "0"
	case json.Number:
		return v.String() == "0"
	case int:
		return v == 0
	case int64:
		return v == 0
	case int32:
		return v == 0
	case uint:
		return v == 0
	case uint64:
		return v == 0
	case float64:
		return v == 0
	default:
		return false
	}
}

func isNegativeNumber(value any) bool {
	switch v := value.(type) {
	case json.Number:
		num, err := v.Int64()
		return err == nil && num < 0
	case int:
		return v < 0
	case int64:
		return v < 0
	case int32:
		return v < 0
	case float64:
		return v < 0
	default:
		return false
	}
}

func collectPaths(out map[string]bool, data map[string]any, prefix []string) {
	for key, value := range data {
		path := append(prefix, key)
		joined := strings.Join(path, ".")
		if nested, ok := value.(map[string]any); ok && !propertyExists(joined) {
			collectPaths(out, nested, path)
			continue
		}
		out[joined] = true
	}
}

func propertyExists(path string) bool {
	_, ok := propertyDefs[path]
	return ok
}

func getNestedValue(data map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = data
	for _, part := range parts {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := asMap[part]
		if !ok || value == nil {
			return nil, false
		}
		current = value
	}
	return current, true
}

func setNestedValue(target map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	last := len(parts) - 1
	current := target
	for i, part := range parts {
		if i == last {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}

func deepCopy(value any) any {
	if value == nil {
		return nil
	}
	return deepCopyValue(reflect.ValueOf(value)).Interface()
}

func deepCopyValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		return reflect.ValueOf(deepCopy(value.Elem().Interface()))
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.New(value.Type().Elem())
		cloned.Elem().Set(deepCopyValue(value.Elem()))
		return cloned
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		for _, key := range value.MapKeys() {
			cloned.SetMapIndex(key, deepCopyValue(value.MapIndex(key)))
		}
		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(deepCopyValue(value.Index(i)))
		}
		return cloned
	case reflect.Array:
		cloned := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(deepCopyValue(value.Index(i)))
		}
		return cloned
	case reflect.Struct:
		cloned := reflect.New(value.Type()).Elem()
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if cloned.Field(i).CanSet() {
				cloned.Field(i).Set(deepCopyValue(field))
			}
		}
		return cloned
	default:
		return value
	}
}
