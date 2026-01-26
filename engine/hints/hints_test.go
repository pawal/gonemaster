package hints

import (
	"strings"
	"testing"
)

func TestParseHintsValid(t *testing.T) {
	text := `. 3600000 IN NS a.root-servers.net.
a.root-servers.net. 3600000 IN A 198.41.0.4
a.root-servers.net. 3600000 IN AAAA 2001:503:ba3e::2:30
`

	hints, err := ParseHints(text)
	if err != nil {
		t.Fatalf("ParseHints error: %v", err)
	}

	addrs := hints["a.root-servers.net."]
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %v", addrs)
	}
	if addrs[0] != "198.41.0.4" || addrs[1] != "2001:503:ba3e::2:30" {
		t.Fatalf("unexpected addresses: %v", addrs)
	}
}

func TestParseHintsErrors(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantErr string
	}{
		{
			name: "forbidden directive",
			text: `$TTL 3600
. 3600 IN NS a.root-servers.net.
a.root-servers.net. 3600 IN A 198.41.0.4
`,
			wantErr: "Forbidden directive $TTL",
		},
		{
			name: "forbidden class",
			text: `. 3600 CH NS a.root-servers.net.
a.root-servers.net. 3600 IN A 198.41.0.4
`,
			wantErr: "Forbidden RR class CH",
		},
		{
			name:    "bad NS owner",
			text:    `example. 3600 IN NS a.root-servers.net.`,
			wantErr: "Owner name for NS record must be \".\"",
		},
		{
			name:    "forbidden type",
			text:    `. 3600 IN MX 10 mail.example.`,
			wantErr: "Forbidden RR type MX",
		},
		{
			name: "glue mismatch",
			text: `. 3600 IN NS a.root-servers.net.
b.root-servers.net. 3600 IN A 198.51.100.1
`,
			wantErr: "Owner name of A record does not match any NS RDATA",
		},
		{
			name:    "missing glue",
			text:    `. 3600 IN NS a.root-servers.net.`,
			wantErr: "No address record found for NS a.root-servers.net.",
		},
		{
			name:    "no NS records",
			text:    ``,
			wantErr: "No NS record found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseHints(tt.text)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q, got %v", tt.wantErr, err)
			}
		})
	}
}
