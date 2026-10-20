package integration

import "codeberg.org/pawal/gonemaster/engine/normalization"

// NameError is one problem found in a domain name.
type NameError struct {
	Tag     string            `json:"tag"`
	Params  map[string]string `json:"params,omitempty"`
	Message string            `json:"message"`
}

// NormalizedName is the outcome of NormalizeName; Name is empty when Errors is not.
type NormalizedName struct {
	Name   string      `json:"name,omitempty"`
	Errors []NameError `json:"errors"`
}

// NormalizeName applies the engine's domain name normalisation, IDN included.
func NormalizeName(name string) NormalizedName {
	errs, normalized := normalization.NormalizeName(name)
	out := NormalizedName{Name: normalized, Errors: make([]NameError, 0, len(errs))}
	for _, e := range errs {
		out.Errors = append(out.Errors, NameError{Tag: e.Tag, Params: e.Params, Message: e.Message()})
	}
	return out
}
