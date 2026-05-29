package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"codeberg.org/pawal/gonemaster/server"
)

// runAuthCommand handles the "auth" subcommand group (currently add-token).
func runAuthCommand(args []string, out, errOut *os.File) int {
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
