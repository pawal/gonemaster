// Package dnstest provides shared setup helpers for engine package tests. It
// must not import nameserver, so nameserver and the packages it depends on can
// use it too; testhelpers wraps it and adds the nameserver cache.
//
// It does import logger, so package logger's own tests cannot import it back
// and build their profiles by hand instead.
package dnstest
