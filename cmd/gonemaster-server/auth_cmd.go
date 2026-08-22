package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"codeberg.org/pawal/gonemaster/server"
)

// parseAdminTokenHashes parses a comma-separated list of "label=sha256:hex" (or
// bare "sha256:hex") entries into admin tokens. Hash validity is checked later
// by server.ValidateAuthConfig.
func parseAdminTokenHashes(s string) []server.AdminToken {
	var toks []server.AdminToken
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		label, hash := "", part
		if i := strings.Index(part, "="); i >= 0 {
			label, hash = part[:i], part[i+1:]
		}
		toks = append(toks, server.AdminToken{Label: label, Hash: hash})
	}
	return toks
}

// resolveAuthConfig rebuilds the auth config from its sources (file < env <
// flag) for SIGHUP reload, mirroring startup precedence.
func resolveAuthConfig(configPath, envHashes, flagHashes string) (server.AuthConfig, error) {
	var auth server.AuthConfig
	if configPath != "" {
		fileCfg, err := server.LoadFileConfig(configPath)
		if err != nil {
			return auth, err
		}
		cfg := server.DefaultConfig()
		cfg.ApplyFileConfig(fileCfg)
		auth = cfg.Auth
	}
	if envHashes != "" {
		auth.AdminTokens = parseAdminTokenHashes(envHashes)
	}
	if flagHashes != "" {
		auth.AdminTokens = parseAdminTokenHashes(flagHashes)
	}
	return auth, nil
}

// runAuthCommand handles the "auth" subcommand group (currently add-token).
func runAuthCommand(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] != "add-token" {
		fmt.Fprintln(errOut, "usage: gonemaster-server auth add-token [--label NAME]")
		return 2
	}
	fs := flag.NewFlagSet("auth add-token", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var label string
	fs.StringVar(&label, "label", "", "optional non-secret label for the token")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	tok, err := generateToken()
	if err != nil {
		fmt.Fprintf(errOut, "generate token: %v\n", err)
		return 1
	}
	hash := server.HashToken(tok)
	snippet, err := json.MarshalIndent(server.AdminToken{Label: label, Hash: hash}, "    ", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "encode token: %v\n", err)
		return 1
	}
	envValue := hash
	if label != "" {
		envValue = label + "=" + hash
	}

	fmt.Fprintf(out, "Admin token (shown once, copy it now):\n\n    %s\n\n", tok)
	fmt.Fprintf(out, "Add to your config file under auth.admin_tokens:\n\n    %s\n\n", snippet)
	fmt.Fprintf(out, "Or via env: GONEMASTER_ADMIN_TOKEN_HASHES=%s\n", envValue)
	return 0
}

// generateToken returns a new "gm_"-prefixed high-entropy token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "gm_" + base64.RawURLEncoding.EncodeToString(b), nil
}
