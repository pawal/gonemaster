package integration

import (
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
