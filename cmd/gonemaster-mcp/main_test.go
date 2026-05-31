package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHandleInfoFlagsVersion(t *testing.T) {
	for _, a := range []string{"--version", "-v", "-version", "-V"} {
		var buf bytes.Buffer
		if !handleInfoFlags([]string{a}, &buf) {
			t.Fatalf("%q should be handled", a)
		}
		out := buf.String()
		if !strings.Contains(out, serverName) || !strings.Contains(out, version) {
			t.Errorf("%q output missing name/version: %q", a, out)
		}
	}
}

func TestHandleInfoFlagsHelp(t *testing.T) {
	for _, a := range []string{"--help", "-h", "-help"} {
		var buf bytes.Buffer
		if !handleInfoFlags([]string{a}, &buf) {
			t.Fatalf("%q should be handled", a)
		}
		out := buf.String()
		for _, want := range []string{"GONEMASTER_URL", "GONEMASTER_MCP_ALLOW_WRITE", "--version"} {
			if !strings.Contains(out, want) {
				t.Errorf("%q help is missing %q", a, want)
			}
		}
	}
}

func TestHandleInfoFlagsIgnoresOther(t *testing.T) {
	for _, args := range [][]string{{}, {"--unknown"}, {"some-arg"}} {
		var buf bytes.Buffer
		if handleInfoFlags(args, &buf) {
			t.Errorf("args %v should not be handled", args)
		}
		if buf.Len() != 0 {
			t.Errorf("args %v should produce no output, got %q", args, buf.String())
		}
	}
}

func TestEnvBool(t *testing.T) {
	t.Setenv("GM_TEST_BOOL", "1")
	if !envBool("GM_TEST_BOOL") {
		t.Errorf("1 should be enabling")
	}
	t.Setenv("GM_TEST_BOOL", "off")
	if envBool("GM_TEST_BOOL") {
		t.Errorf("off should not be enabling")
	}
	if envBool("GM_TEST_BOOL_UNSET") {
		t.Errorf("unset should not be enabling")
	}
}
