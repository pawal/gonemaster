// Package apitest serves a stand-in gonemaster-server for the CLI and tooling
// tests, over a real socket or in-process. It lives at the module root rather
// than under cmd/internal because tools/server-perf-kit is outside cmd/ and
// could not import it from there.
package apitest
