package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/profile"
)

// Profile error codes.
const (
	ProfileInvalidJSON     = "invalid_json"
	ProfileUnknownProperty = "unknown_property"
	ProfileInvalidValue    = "invalid_value"
	ProfileUnknownTestcase = "unknown_testcase"
)

// ProfileError is one problem found in a profile.
type ProfileError struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// CheckProfile validates a JSON profile; nil means it applies as given.
// An unknown testcase in test_cases is reported but does not stop a run.
func CheckProfile(data []byte) []ProfileError {
	if len(data) == 0 {
		return nil
	}
	p, err := profile.FromJSON(string(data))
	if err != nil {
		var pe *profile.PropertyError
		if errors.As(err, &pe) {
			code := ProfileInvalidValue
			if pe.Unknown {
				code = ProfileUnknownProperty
			}
			return []ProfileError{{Code: code, Path: pe.Path, Message: pe.Error()}}
		}
		return []ProfileError{{Code: ProfileInvalidJSON, Message: err.Error()}}
	}
	var out []ProfileError
	for _, item := range p.TestCases {
		name, _ := item.(string)
		if !knownTestcase(strings.ToLower(strings.TrimSpace(name))) {
			out = append(out, ProfileError{Code: ProfileUnknownTestcase, Path: "test_cases", Message: fmt.Sprintf("unknown testcase %v", item)})
		}
	}
	return out
}

// ProfileReport is the outcome of a profile check with the profile a run would use.
type ProfileReport struct {
	Errors []ProfileError `json:"errors"`
	// Effective is the defaults with the checked profile applied; absent when the profile does not apply.
	Effective json.RawMessage `json:"effective,omitempty"`
}

// ReportProfile checks data and adds the effective profile when a run would accept it.
func ReportProfile(data []byte) ProfileReport {
	report := ProfileReport{Errors: CheckProfile(data)}
	if report.Errors == nil {
		report.Errors = []ProfileError{}
	}
	for _, pe := range report.Errors {
		if pe.Code != ProfileUnknownTestcase {
			return report
		}
	}
	if effective, err := EffectiveProfile(data); err == nil {
		report.Effective = effective
	}
	return report
}

// EffectiveProfile returns the defaults with data applied, as the engine applies them.
func EffectiveProfile(data []byte) (json.RawMessage, error) {
	base, err := profile.Default()
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		override, err := profile.FromJSON(string(data))
		if err != nil {
			return nil, err
		}
		if err := base.Merge(override); err != nil {
			return nil, err
		}
	}
	text, err := base.ToJSON()
	if err != nil {
		return nil, err
	}
	return json.RawMessage(text), nil
}
