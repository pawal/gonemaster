package main

// Register the pure-Go SQLite driver so that "sqlite" can be used as the
// database driver without requiring CGO.
import _ "modernc.org/sqlite"
