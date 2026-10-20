package integration

import (
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
