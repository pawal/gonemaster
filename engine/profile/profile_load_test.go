package profile

import (
	"encoding/json"
	"testing"
)

func TestNewProfileStartsUnset(t *testing.T) {
	p := New()
	if value, err := p.Get("net.ipv4"); err != nil || value != nil {
		t.Fatalf("expected net.ipv4 unset, got %#v (err=%v)", value, err)
	}
	if value, err := p.Get("resolver.defaults.retry"); err != nil || value != nil {
		t.Fatalf("expected resolver.defaults.retry unset, got %#v (err=%v)", value, err)
	}
	if value, err := p.Get("test_levels"); err != nil || value != nil {
		t.Fatalf("expected test_levels unset, got %#v (err=%v)", value, err)
	}
}

func TestFromJSONParsesValues(t *testing.T) {
	const profileJSON = `{
		"resolver": {
			"defaults": {
				"usevc": true,
				"recurse": false,
				"retry": 123,
				"retrans": 234,
				"positive_cache_ttl": 30,
				"negative_cache_ttl": 45
			},
			"source4": "192.0.2.53",
			"source6": "2001:db8::42"
		},
		"net": {
			"ipv4": true,
			"ipv6": false
		},
		"no_network": true,
		"asn_db": {
			"style": "RIPE",
			"sources": {
				"ripe": ["asn1.example.com"]
			}
		},
		"logfilter": {
			"Zone": {
				"TAG": [
					{"when": {"bananas": 0}, "set": "WARNING"}
				]
			}
		},
		"test_levels": {
			"Zone": {"TAG": "INFO"}
		},
		"test_cases": ["Zone01"],
		"cache": {"redis": {"server": "127.0.0.1:6379", "expire": 3600}}
	}`

	p, err := FromJSON(profileJSON)
	if err != nil {
		t.Fatalf("from json: %v", err)
	}

	value, err := p.Get("resolver.defaults.usevc")
	if err != nil || value != true {
		t.Fatalf("expected usevc true, got %#v (err=%v)", value, err)
	}
	value, err = p.Get("resolver.defaults.retry")
	if err != nil || value != 123 {
		t.Fatalf("expected retry 123, got %#v (err=%v)", value, err)
	}
	value, err = p.Get("resolver.source4")
	if err != nil || value != "192.0.2.53" {
		t.Fatalf("expected source4, got %#v (err=%v)", value, err)
	}
	value, err = p.Get("resolver.defaults.positive_cache_ttl")
	if err != nil || value != 30 {
		t.Fatalf("expected positive_cache_ttl 30, got %#v (err=%v)", value, err)
	}
	value, err = p.Get("resolver.defaults.negative_cache_ttl")
	if err != nil || value != 45 {
		t.Fatalf("expected negative_cache_ttl 45, got %#v (err=%v)", value, err)
	}
	value, err = p.Get("asn_db.style")
	if err != nil || value != "ripe" {
		t.Fatalf("expected asn_db.style normalized to ripe, got %#v (err=%v)", value, err)
	}

	if p.LogFilter["Zone"]["TAG"][0].Set != "WARNING" {
		t.Fatalf("expected logfilter rule")
	}
	if p.TestLevels["Zone"]["TAG"] != "INFO" {
		t.Fatalf("expected test_levels rule")
	}
	if len(p.TestCases) != 1 {
		t.Fatalf("expected test_cases entry")
	}

	copyValue, err := p.Get("logfilter")
	if err != nil {
		t.Fatalf("get logfilter: %v", err)
	}
	logFilter := copyValue.(map[string]map[string][]LogFilterRule)
	logFilter["Zone"]["TAG"][0].Set = "ERROR"
	if p.LogFilter["Zone"]["TAG"][0].Set != "WARNING" {
		t.Fatalf("expected logfilter deep copy")
	}
}

func TestFromJSONRejectsUnknown(t *testing.T) {
	if _, err := FromJSON(`{"net":1}`); err == nil {
		t.Fatalf("expected error for unknown property")
	}
}

func TestFromJSONRejectsInvalidTypes(t *testing.T) {
	if _, err := FromJSON(`{"net":{"ipv4":1}}`); err == nil {
		t.Fatalf("expected error for invalid ipv4 type")
	}
}

func TestSetTruthiness(t *testing.T) {
	p := New()
	if err := p.Set("no_network", "0"); err != nil {
		t.Fatalf("set no_network: %v", err)
	}
	value, err := p.Get("no_network")
	if err != nil || value != false {
		t.Fatalf("expected false for \"0\", got %#v (err=%v)", value, err)
	}

	if err := p.Set("no_network", "0\n"); err != nil {
		t.Fatalf("set no_network: %v", err)
	}
	value, err = p.Get("no_network")
	if err != nil || value != true {
		t.Fatalf("expected true for \"0\\n\", got %#v (err=%v)", value, err)
	}
}

func TestMergeProfile(t *testing.T) {
	p1, err := FromJSON(`{"net":{"ipv4":true},"resolver":{"defaults":{"retry":5}}}`)
	if err != nil {
		t.Fatalf("from json: %v", err)
	}
	p2 := New()
	if err := p2.Set("net.ipv4", false); err != nil {
		t.Fatalf("set net.ipv4: %v", err)
	}
	if err := p1.Merge(p2); err != nil {
		t.Fatalf("merge: %v", err)
	}
	value, err := p1.Get("net.ipv4")
	if err != nil || value != false {
		t.Fatalf("expected net.ipv4 false after merge, got %#v (err=%v)", value, err)
	}
	value, err = p1.Get("resolver.defaults.retry")
	if err != nil || value != 5 {
		t.Fatalf("expected retry to remain 5, got %#v (err=%v)", value, err)
	}
	if value, err := p2.Get("resolver.defaults.retry"); err != nil || value != nil {
		t.Fatalf("expected merge not to alter source profile, got %#v (err=%v)", value, err)
	}
}

func TestToJSONIncludesSetProperties(t *testing.T) {
	p := New()
	if err := p.Set("net.ipv4", true); err != nil {
		t.Fatalf("set net.ipv4: %v", err)
	}
	raw, err := p.ToJSON()
	if err != nil {
		t.Fatalf("to json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	netValue, ok := payload["net"].(map[string]any)
	if !ok || netValue["ipv4"] != true {
		t.Fatalf("expected net.ipv4 serialized, got %#v", payload)
	}
	if _, ok := payload["resolver"]; ok {
		t.Fatalf("did not expect unrelated resolver properties")
	}
}
