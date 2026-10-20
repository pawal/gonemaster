package profile

import (
	"errors"
	"testing"
)

func TestFromJSONReportsPropertyErrors(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		path    string
		unknown bool
		message string
	}{
		{"unknown property", `{"net":1}`, "net", true, `unknown property "net"`},
		{"invalid value", `{"net":{"ipv4":"yes"}}`, "net.ipv4", false, "property net.ipv4 expects boolean value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FromJSON(tc.json)
			var pe *PropertyError
			if !errors.As(err, &pe) {
				t.Fatalf("err = %v, want *PropertyError", err)
			}
			if pe.Path != tc.path || pe.Unknown != tc.unknown {
				t.Fatalf("PropertyError = {%q %v}, want {%q %v}", pe.Path, pe.Unknown, tc.path, tc.unknown)
			}
			if err.Error() != tc.message {
				t.Fatalf("message = %q, want %q", err.Error(), tc.message)
			}
		})
	}
}

func TestSetReportsPropertyPath(t *testing.T) {
	var pe *PropertyError
	if err := New().Set("resolver.defaults.retry", nil); !errors.As(err, &pe) || pe.Path != "resolver.defaults.retry" || pe.Unknown {
		t.Fatalf("Set(nil) = %v, want PropertyError for resolver.defaults.retry", err)
	}
	if _, err := New().Get("nope"); !errors.As(err, &pe) || !pe.Unknown {
		t.Fatalf("Get(nope) = %v, want unknown PropertyError", err)
	}
}
