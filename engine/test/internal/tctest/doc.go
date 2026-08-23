// Package tctest provides the shared scaffolding for the engine testcase
// suites: log-entry assertions and arg decoding, a per-test context with a
// nameserver and recursor factory, generic seam stubbing, packet and DNSSEC
// record fixtures, and a synctest gate for parallel-query tests. It is
// imported from _test.go files only.
package tctest
