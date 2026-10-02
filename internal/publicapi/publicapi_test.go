package publicapi

import "testing"

func TestBase(t *testing.T) {
	cases := map[string]string{
		"http://localhost:8080/api/v1":  "http://localhost:8080/pub/api/v1",
		"http://localhost:8080/api/v1/": "http://localhost:8080/pub/api/v1",
		"https://example.com":           "https://example.com/pub/api/v1",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := Base(in); got != want {
				t.Errorf("Base(%q) = %q, want %q", in, got, want)
			}
		})
	}
}
