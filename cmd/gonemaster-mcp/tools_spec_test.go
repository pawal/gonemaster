package main

import (
	"testing"
)

func TestSpecListTool(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{specList: &specTestcaseListView{
		Items: []specTestcaseView{
			{ID: "dnssec09", Module: "dnssec", Description: "RRSIG validity"},
			{ID: "dnssec10", Module: "dnssec", Description: "Zone signed"},
		},
		Total: 2,
	}})
	defer ts.Close()

	var out specListOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "spec_list_testcases", map[string]any{"category": "dnssec"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Count != 2 || len(out.Testcases) != 2 {
		t.Fatalf("count = %d", out.Count)
	}
	if out.Testcases[0].ID != "dnssec09" || out.Testcases[0].Module != "dnssec" {
		t.Errorf("first testcase wrong: %+v", out.Testcases[0])
	}
}

func TestSpecGetTool(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{specDetail: &specTestcaseDetailView{
		ID:          "consistency03",
		Module:      "consistency",
		Description: "SOA timers consistency",
		Locale:      "en",
		Tags: []specTagView{
			{Tag: "SOATIME", Message: "SOA timers are consistent"},
			{Tag: "MULTIPLE_SOA_TIME", Message: "SOA timers differ"},
		},
	}})
	defer ts.Close()

	var out specGetOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "spec_get_testcase", map[string]any{"testcase": "consistency03"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.ID != "consistency03" || out.Module != "consistency" {
		t.Errorf("id/module wrong: %+v", out)
	}
	if out.Description == "" {
		t.Errorf("expected a description")
	}
	if len(out.Tags) != 2 || out.Tags[0].Tag != "SOATIME" || out.Tags[0].Message == "" {
		t.Errorf("tags wrong: %+v", out.Tags)
	}
}

func TestSpecGetToolNotFound(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{specDetail: nil}) // 404
	defer ts.Close()

	res := callTool(t, clientFor(t, ts.URL, ""), "spec_get_testcase", map[string]any{"testcase": "nope99"}, nil)
	if !res.IsError {
		t.Fatalf("expected a not-found tool error")
	}
	if got := errorText(res); got == "" {
		t.Errorf("expected an error message")
	}
}
