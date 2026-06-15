// Package engine provides the public entry points for running Gonemaster test
// plans and collecting normalized log output.
//
// Typical usage is:
//
//	entries, err := engine.Run(engine.RunRequest{
//		Domain:  "example.com",
//		MinLevel: "INFO",
//	})
//
// Use RunWithRunner when the caller needs to inject a prebuilt per-run
// container (profile, logger, limiter, cache) and keep strict control over run
// lifecycle and dependencies.
package engine
