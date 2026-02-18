package engine

import (
	"strings"
	"testing"
)

func TestParseUndelegatedNameserver(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    UndelegatedNameserver
		wantErr bool
	}{
		{
			name: "name and ip",
			spec: "NS1.Example.COM/192.0.2.1",
			want: UndelegatedNameserver{Name: "ns1.example.com", IP: "192.0.2.1"},
		},
		{
			name: "name only",
			spec: "NS2.Example.COM",
			want: UndelegatedNameserver{Name: "ns2.example.com"},
		},
		{
			name:    "invalid name",
			spec:    "bad!name.example/192.0.2.1",
			wantErr: true,
		},
		{
			name:    "invalid ip",
			spec:    "ns1.example.com/not-an-ip",
			wantErr: true,
		},
		{
			name:    "invalid format",
			spec:    "ns1.example.com/192.0.2.1/extra",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseUndelegatedNameserver(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse undelegated nameserver: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected parse result: got=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestParseUndelegatedDS(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    UndelegatedDSInfo
		wantErr bool
	}{
		{
			name: "valid",
			spec: "12345,13,2," + strings.Repeat("a", 64),
			want: UndelegatedDSInfo{
				KeyTag:     12345,
				Algorithm:  13,
				DigestType: 2,
				Digest:     strings.Repeat("A", 64),
			},
		},
		{
			name:    "bad field count",
			spec:    "12345,13,2",
			wantErr: true,
		},
		{
			name:    "bad keytag",
			spec:    "-1,13,2," + strings.Repeat("a", 64),
			wantErr: true,
		},
		{
			name:    "bad digest",
			spec:    "12345,13,2,not-hex",
			wantErr: true,
		},
		{
			name:    "bad digest length",
			spec:    "12345,13,2,ABCD",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseUndelegatedDS(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse undelegated DS: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected parse result: got=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestNormalizeUndelegatedInputs(t *testing.T) {
	nameservers := []UndelegatedNameserver{
		{Name: "NS1.Example.COM", IP: "192.0.2.1"},
		{Name: "ns1.example.com", IP: "192.0.2.1"},
		{Name: "ns2.example.com"},
	}
	ds := []UndelegatedDSInfo{
		{
			KeyTag:     12345,
			Algorithm:  13,
			DigestType: 2,
			Digest:     strings.Repeat("a", 64),
		},
		{
			KeyTag:     12345,
			Algorithm:  13,
			DigestType: 2,
			Digest:     strings.Repeat("A", 64),
		},
	}

	normalizedNS, normalizedDS, err := NormalizeUndelegatedInputs(nameservers, ds)
	if err != nil {
		t.Fatalf("normalize undelegated inputs: %v", err)
	}
	if len(normalizedNS) != 2 {
		t.Fatalf("expected 2 unique nameservers, got %d", len(normalizedNS))
	}
	if normalizedNS[0].Name != "ns1.example.com" || normalizedNS[0].IP != "192.0.2.1" {
		t.Fatalf("unexpected first nameserver: %+v", normalizedNS[0])
	}
	if normalizedNS[1].Name != "ns2.example.com" || normalizedNS[1].IP != "" {
		t.Fatalf("unexpected second nameserver: %+v", normalizedNS[1])
	}

	if len(normalizedDS) != 1 {
		t.Fatalf("expected 1 unique DS record, got %d", len(normalizedDS))
	}
	if normalizedDS[0].Digest != strings.Repeat("A", 64) {
		t.Fatalf("expected uppercase digest, got %q", normalizedDS[0].Digest)
	}
}

func TestRunRejectsInvalidUndelegatedInput(t *testing.T) {
	_, err := Run(RunRequest{
		Domain:   ".",
		Testcase: "basic01",
		UndelegatedNameservers: []UndelegatedNameserver{
			{Name: "bad!name.example"},
		},
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "undelegated nameserver") {
		t.Fatalf("unexpected error: %v", err)
	}
}
