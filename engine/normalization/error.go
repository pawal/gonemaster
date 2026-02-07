package normalization

import (
	"fmt"
	"strings"
)

// Error represents one domain-normalization validation issue.
type Error struct {
	Tag    string
	Params map[string]string
}

type errorSpec struct {
	message string
	args    []string
}

var errorSpecs = map[string]errorSpec{
	"AMBIGUOUS_DOWNCASING": {
		message: "Ambiguous downcasing of character \"{unicode_name}\" in the domain name. Use all lower case instead.",
		args:    []string{"unicode_name"},
	},
	"DOMAIN_NAME_TOO_LONG": {
		message: "Domain name is too long (more than 253 characters with no final dot).",
	},
	"EMPTY_DOMAIN_NAME": {
		message: "Domain name is empty.",
	},
	"INITIAL_DOT": {
		message: "Domain name starts with dot.",
	},
	"INVALID_ASCII": {
		message: "Domain name has an ASCII label (\"{label}\") with a character not permitted.",
		args:    []string{"label"},
	},
	"INVALID_U_LABEL": {
		message: "Domain name has a non-ASCII label (\"{label}\") which is not a valid U-label.",
		args:    []string{"label"},
	},
	"REPEATED_DOTS": {
		message: "Domain name has repeated dots.",
	},
	"LABEL_TOO_LONG": {
		message: "Domain name has a label that is too long (more than 63 characters), \"{label}\".",
		args:    []string{"label"},
	},
}

// NewError builds an Error from a known tag and required parameters.
func NewError(tag string, params map[string]string) (Error, error) {
	spec, ok := errorSpecs[tag]
	if !ok {
		return Error{}, fmt.Errorf("unknown error tag: %s", tag)
	}

	resolved := map[string]string{}
	for _, arg := range spec.args {
		value, ok := params[arg]
		if !ok {
			return Error{}, fmt.Errorf("missing argument %s", arg)
		}
		resolved[arg] = value
	}

	return Error{Tag: tag, Params: resolved}, nil
}

// Message returns the human-readable message for the normalization error.
func (e Error) Message() string {
	spec, ok := errorSpecs[e.Tag]
	if !ok {
		return ""
	}

	msg := spec.message
	for key, value := range e.Params {
		msg = strings.ReplaceAll(msg, "{"+key+"}", value)
	}
	return msg
}

// String implements fmt.Stringer using Message().
func (e Error) String() string {
	return e.Message()
}
