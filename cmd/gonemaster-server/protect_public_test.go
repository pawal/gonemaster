package main

import (
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
	"codeberg.org/pawal/gonemaster/server"
)

func TestApplyProtectPublic(t *testing.T) {
	tests := []struct {
		name      string
		file      bool
		env       string
		flagSet   bool
		flagValue bool
		want      bool
	}{
		{name: "file kept without env or flag", file: true, want: true},
		{name: "env overrides file", file: false, env: "true", want: true},
		{name: "env false overrides file", file: true, env: "false", want: false},
		{name: "invalid env keeps file", file: true, env: "maybe", want: true},
		{name: "flag overrides env", env: "true", flagSet: true, flagValue: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := server.AuthConfig{ProtectPublic: tt.file}
			applyProtectPublic(&auth, tt.env, tt.flagSet, tt.flagValue)
			if auth.ProtectPublic != tt.want {
				t.Fatalf("ProtectPublic = %v, want %v", auth.ProtectPublic, tt.want)
			}
		})
	}
}

func TestRunProtectPublicWithoutTokensFails(t *testing.T) {
	res := clitest.Run(t, run, "--dump-config", "--auth-protect-public")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "invalid auth config: protect_public requires admin_tokens")
}

func TestRunProtectPublicFlagDumpConfig(t *testing.T) {
	res := clitest.Run(t, run, "--dump-config", "--auth-protect-public", "--admin-token-hashes", server.HashToken("gm_x"))
	res.RequireCode(t, 0)
	res.RequireOutContains(t, `"protect_public": true`)
}

func TestRunProtectPublicEnvDumpConfig(t *testing.T) {
	t.Setenv("GONEMASTER_AUTH_PROTECT_PUBLIC", "true")
	t.Setenv("GONEMASTER_ADMIN_TOKEN_HASHES", server.HashToken("gm_x"))
	res := clitest.Run(t, run, "--dump-config")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, `"protect_public": true`)
}

func TestRunProtectPublicHelpText(t *testing.T) {
	res := clitest.Run(t, run, "-h")
	res.RequireErrContains(t, "--auth-protect-public", "GONEMASTER_AUTH_PROTECT_PUBLIC")
}
