package integration

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestCheckProfile(t *testing.T) {
	cases := []struct {
		name string
		json string
		want []ProfileError
	}{
		{"empty input", "", nil},
		{"defaults", `{}`, nil},
		{"valid", `{"net":{"ipv4":true},"test_cases":["basic01"]}`, nil},
		{"bad json", `{`, []ProfileError{{Code: ProfileInvalidJSON, Message: "load json profile: unexpected EOF"}}},
		{"unknown property", `{"net":1}`, []ProfileError{{Code: ProfileUnknownProperty, Path: "net", Message: `unknown property "net"`}}},
		{"invalid value", `{"net":{"ipv4":"yes"}}`, []ProfileError{{Code: ProfileInvalidValue, Path: "net.ipv4", Message: "property net.ipv4 expects boolean value"}}},
		{"unknown testcases", `{"test_cases":["basic01","nope",7]}`, []ProfileError{
			{Code: ProfileUnknownTestcase, Path: "test_cases", Message: "unknown testcase nope"},
			{Code: ProfileUnknownTestcase, Path: "test_cases", Message: "unknown testcase 7"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckProfile([]byte(tc.json))
			if len(got) != len(tc.want) {
				t.Fatalf("CheckProfile = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("error %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestReportProfile(t *testing.T) {
	cases := []struct {
		name          string
		json          string
		wantErrors    int
		wantEffective bool
	}{
		{"defaults", `{}`, 0, true},
		{"override", `{"net":{"ipv4":false}}`, 0, true},
		{"unknown testcase still applies", `{"test_cases":["nope"]}`, 1, true},
		{"unknown property", `{"net":1}`, 1, false},
		{"bad json", `{`, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ReportProfile([]byte(tc.json))
			if len(got.Errors) != tc.wantErrors || (len(got.Effective) > 0) != tc.wantEffective {
				t.Fatalf("ReportProfile = %+v", got)
			}
		})
	}
}

func TestEffectiveProfileAppliesOverridesToTheDefaults(t *testing.T) {
	var defaults, overridden struct {
		Net struct {
			IPv4 bool `json:"ipv4"`
			IPv6 bool `json:"ipv6"`
		} `json:"net"`
		TestCases []string `json:"test_cases"`
	}
	data, err := EffectiveProfile(nil)
	if err != nil {
		t.Fatalf("EffectiveProfile(nil): %v", err)
	}
	if err := json.Unmarshal(data, &defaults); err != nil {
		t.Fatalf("decode defaults: %v", err)
	}
	data, err = EffectiveProfile([]byte(`{"net":{"ipv4":false},"test_cases":["basic01"]}`))
	if err != nil {
		t.Fatalf("EffectiveProfile: %v", err)
	}
	if err := json.Unmarshal(data, &overridden); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !defaults.Net.IPv4 || !defaults.Net.IPv6 || len(defaults.TestCases) < 2 {
		t.Fatalf("defaults = %+v", defaults)
	}
	if overridden.Net.IPv4 || !overridden.Net.IPv6 || !slices.Equal(overridden.TestCases, []string{"basic01"}) {
		t.Fatalf("overridden = %+v", overridden)
	}
}

func TestEffectiveProfileRejectsABadProfile(t *testing.T) {
	if _, err := EffectiveProfile([]byte(`{"net":1}`)); err == nil {
		t.Fatal("EffectiveProfile accepted an unknown property")
	}
}
