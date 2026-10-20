package integration

import (
	"maps"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  NormalizedName
	}{
		{"lowercases", "Example.COM.", NormalizedName{Name: "example.com", Errors: []NameError{}}},
		{"root", ".", NormalizedName{Name: ".", Errors: []NameError{}}},
		{"idn", "räksmörgås.se", NormalizedName{Name: "xn--rksmrgs-5wao1o.se", Errors: []NameError{}}},
		{"empty", "", NormalizedName{Errors: []NameError{{Tag: "EMPTY_DOMAIN_NAME", Message: "Domain name is empty."}}}},
		{"repeated dots", "a..b", NormalizedName{Errors: []NameError{{Tag: "REPEATED_DOTS", Message: "Domain name has repeated dots."}}}},
		{"bad ascii", "!%~&", NormalizedName{Errors: []NameError{{Tag: "INVALID_ASCII", Params: map[string]string{"label": "!%~&"}, Message: `Domain name has an ASCII label ("!%~&") with a character not permitted.`}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeName(tc.input)
			if got.Name != tc.want.Name || len(got.Errors) != len(tc.want.Errors) {
				t.Fatalf("NormalizeName = %+v, want %+v", got, tc.want)
			}
			for i, e := range got.Errors {
				w := tc.want.Errors[i]
				if e.Tag != w.Tag || e.Message != w.Message || !maps.Equal(e.Params, w.Params) {
					t.Errorf("error %d = %+v, want %+v", i, e, w)
				}
			}
		})
	}
}
