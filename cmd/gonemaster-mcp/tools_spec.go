package main

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSpecTools(srv *mcp.Server, api *apiClient) {
	registerSpecList(srv, api)
	registerSpecGet(srv, api)
}

type specListInput struct {
	Category string `json:"category,omitempty" jsonschema:"optional module to filter by, e.g. dnssec or consistency"`
}

type specTestcaseOut struct {
	ID          string `json:"id" jsonschema:"testcase id, e.g. dnssec09"`
	Module      string `json:"module"`
	Description string `json:"description,omitempty"`
}

type specListOutput struct {
	Count     int               `json:"count"`
	Testcases []specTestcaseOut `json:"testcases"`
}

func registerSpecList(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "spec_list_testcases",
		Description: "List gonemaster's implemented testcases, optionally filtered to one module.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in specListInput) (*mcp.CallToolResult, specListOutput, error) {
		list, err := api.listSpecTestcases(ctx, strings.TrimSpace(in.Category))
		if err != nil {
			return nil, specListOutput{}, toolError("list testcases", err)
		}
		out := specListOutput{Testcases: []specTestcaseOut{}}
		for _, t := range list.Items {
			out.Testcases = append(out.Testcases, specTestcaseOut{ID: t.ID, Module: t.Module, Description: t.Description})
		}
		out.Count = len(out.Testcases)
		return nil, out, nil
	})
}

type specGetInput struct {
	Testcase string `json:"testcase" jsonschema:"the testcase id, e.g. dnssec09 (a testcase identifier, not a message tag)"`
	Lang     string `json:"lang,omitempty" jsonschema:"language for rendered messages (default en)"`
}

type specTagOut struct {
	Tag     string `json:"tag"`
	Message string `json:"message,omitempty" jsonschema:"the rendered message template for this tag"`
}

type specGetOutput struct {
	ID          string       `json:"id"`
	Module      string       `json:"module"`
	Description string       `json:"description,omitempty"`
	Locale      string       `json:"locale,omitempty"`
	Tags        []specTagOut `json:"tags" jsonschema:"tags this testcase can emit, each with its rendered message"`
}

func registerSpecGet(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "spec_get_testcase",
		Description: "Get one testcase's module, description, and the tags it can emit with their rendered messages.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in specGetInput) (*mcp.CallToolResult, specGetOutput, error) {
		id := strings.TrimSpace(in.Testcase)
		if id == "" {
			return nil, specGetOutput{}, errors.New("testcase is required")
		}
		lang := strings.TrimSpace(in.Lang)
		if lang == "" {
			lang = "en"
		}
		d, err := api.getSpecTestcase(ctx, id, lang)
		if err != nil {
			return nil, specGetOutput{}, toolError("get testcase", err)
		}
		out := specGetOutput{ID: d.ID, Module: d.Module, Description: d.Description, Locale: d.Locale, Tags: []specTagOut{}}
		for _, tg := range d.Tags {
			out.Tags = append(out.Tags, specTagOut{Tag: tg.Tag, Message: tg.Message})
		}
		return nil, out, nil
	})
}
