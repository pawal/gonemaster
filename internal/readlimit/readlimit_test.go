package readlimit

import (
	"strings"
	"testing"
)

func TestReadAll(t *testing.T) {
	for _, tc := range []struct {
		in      string
		wantErr bool
	}{
		{"", false},
		{"abcd", false},
		{"abcde", true},
	} {
		got, err := ReadAll(strings.NewReader(tc.in), 4)
		if tc.wantErr {
			if err == nil || err.Error() != "stream exceeds 4 bytes" {
				t.Errorf("ReadAll(%q) error = %v, want stream exceeds 4 bytes", tc.in, err)
			}
			continue
		}
		if err != nil || string(got) != tc.in {
			t.Errorf("ReadAll(%q) = %q, %v; want %q", tc.in, got, err, tc.in)
		}
	}
}
