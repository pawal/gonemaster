package main

import (
	"encoding/json"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/integration"
)

func decodeEvent(t *testing.T, data string) integration.Event {
	t.Helper()
	var ev integration.Event
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		t.Fatalf("decode %q: %v", data, err)
	}
	return ev
}

// pull reads events until the stream ends.
func pull(t *testing.T, handle uint64) []integration.Event {
	t.Helper()
	var events []integration.Event
	for {
		ev := decodeEvent(t, runNext(handle))
		events = append(events, ev)
		if ev.Type == integration.EventDone || ev.Type == integration.EventError {
			return events
		}
	}
}

func TestVersionJSON(t *testing.T) {
	var v integration.VersionInfo
	if err := json.Unmarshal([]byte(versionJSON()), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.Schema != integration.Schema || v.EngineVersion == "" || !v.Capabilities["streaming"] {
		t.Fatalf("version = %+v", v)
	}
}

func TestListTestsJSON(t *testing.T) {
	var meta integration.Metadata
	if err := json.Unmarshal([]byte(listTestsJSON()), &meta); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(meta.Modules) == 0 || meta.Modules[0].Name != "basic" || meta.Modules[0].Testcases[0].ID != "basic01" {
		t.Fatalf("metadata = %+v", meta)
	}
}

func TestCheckProfileJSON(t *testing.T) {
	cases := []struct {
		name       string
		profile    string
		wantPrefix string
	}{
		{"valid", `{}`, `{"errors":[],"effective":{"asn_db":`},
		{"unknown property", `{"net":1}`, `{"errors":[{"code":"unknown_property","path":"net","message":"unknown property \"net\""}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkProfileJSON(tc.profile); !strings.HasPrefix(got, tc.wantPrefix) {
				t.Fatalf("checkProfileJSON = %s, want prefix %s", got, tc.wantPrefix)
			}
		})
	}
}

func TestTranslateJSON(t *testing.T) {
	cases := []struct {
		name    string
		request string
		want    string
	}{
		{"english", `{"language":"en","entries":[{"module":"System","tag":"GLOBAL_VERSION","args":{"version":"v1"}}]}`, `{"messages":[{"message":"Using version v1 of the Gonemaster engine.","found":true}]}`},
		{"swedish", `{"language":"sv","entries":[{"module":"System","tag":"GLOBAL_VERSION","args":{"version":"v1"}}]}`, `{"messages":[{"message":"Använder version v1 av Gonemasters testmotor.","found":true}]}`},
		{"unknown tag", `{"language":"en","entries":[{"module":"Basic","tag":"NOPE"}]}`, `{"messages":[{"message":"BASIC:NOPE","found":false}]}`},
		{"no entries", `{"language":"en"}`, `{"messages":[]}`},
		{"bad json", `{"language":`, `{"type":"error","error":{"code":"invalid_request","message":"unexpected EOF"}}`},
		{"unknown field", `{"locale":"en"}`, `{"type":"error","error":{"code":"invalid_request","message":"json: unknown field \"locale\""}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := translateJSON(tc.request); got != tc.want {
				t.Fatalf("translateJSON = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestNormalizeNameJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"valid", "Example.COM", `{"name":"example.com","errors":[]}`},
		{"idn", "räksmörgås.se", `{"name":"xn--rksmrgs-5wao1o.se","errors":[]}`},
		{"empty", "", `{"errors":[{"tag":"EMPTY_DOMAIN_NAME","message":"Domain name is empty."}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeNameJSON(tc.in); got != tc.want {
				t.Fatalf("normalizeNameJSON = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRunLifecycle(t *testing.T) {
	handle := runStart(`{"domain":"example.com","testcases":["syntax01"],"ipv4":false,"ipv6":false,"include_timings":true}`)
	if handle == 0 {
		t.Fatal("runStart returned handle 0")
	}
	events := pull(t, handle)
	var tags []string
	for _, ev := range events {
		if ev.Type == integration.EventEntry {
			tags = append(tags, ev.Entry.Tag)
		}
	}
	if len(tags) == 0 || tags[0] != "GLOBAL_VERSION" || events[len(events)-1].Type != integration.EventDone {
		t.Fatalf("events = %+v", events)
	}
	if events[len(events)-2].Type != integration.EventTimings {
		t.Fatalf("event before done = %s, want timings", events[len(events)-2].Type)
	}
	runFree(handle)
	if ev := decodeEvent(t, runNext(handle)); ev.Type != integration.EventError || ev.Error.Code != integration.CodeInvalidRequest || !strings.Contains(ev.Error.Message, "unknown run handle") {
		t.Fatalf("Next after free = %+v", ev)
	}
}

func TestRunStartRejectsBadJSON(t *testing.T) {
	handle := runStart(`{"domain":`)
	events := pull(t, handle)
	if len(events) != 1 || events[0].Error == nil || events[0].Error.Code != integration.CodeInvalidRequest {
		t.Fatalf("events = %+v", events)
	}
	runFree(handle)
}

func TestRunCancelOnUnknownHandleIsANoop(t *testing.T) {
	runCancel(1 << 40)
	runFree(1 << 40)
}

func TestHandlesAreDistinct(t *testing.T) {
	seen := map[uint64]bool{}
	for range 5 {
		handle := runStart(`{}`)
		if seen[handle] {
			t.Fatalf("handle %d issued twice", handle)
		}
		seen[handle] = true
		runFree(handle)
	}
}

func TestEncodeFallsBackToAnErrorEvent(t *testing.T) {
	ev := decodeEvent(t, encode(make(chan int)))
	if ev.Type != integration.EventError || ev.Error.Code != integration.CodeRunFailed || !strings.Contains(ev.Error.Message, "unsupported type") {
		t.Fatalf("event = %+v", ev)
	}
}

func TestGuardTurnsAPanicIntoAnErrorEvent(t *testing.T) {
	ev := decodeEvent(t, guard(func() string { panic("boom") }))
	if ev.Type != integration.EventError || ev.Error.Message != "panic: boom" {
		t.Fatalf("event = %+v", ev)
	}
}
