// Package dnstest provides shared setup helpers for engine package tests. It
// must not import nameserver, so nameserver and the packages it depends on can
// use it too; testhelpers wraps it and adds the nameserver cache.
package dnstest
