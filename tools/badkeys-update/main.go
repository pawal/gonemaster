// Command badkeys-update downloads the badkeys blocklist data files.
//
// Usage:
//
//	go run ./tools/badkeys-update [--output DIR]
//
// The default output directory is share/badkeys/.
package main

import (
	"flag"
	"fmt"
	"os"

	"codeberg.org/pawal/gonemaster/engine/badkeys"
)

func main() {
	outputDir := flag.String("output", "share/badkeys", "Output directory for blocklist files")
	flag.Parse()

	if err := badkeys.Update(*outputDir, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "badkeys-update: %v\n", err)
		os.Exit(1)
	}
}
