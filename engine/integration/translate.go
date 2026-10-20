package integration

import (
	"bytes"
	"encoding/json"

	"codeberg.org/pawal/gonemaster/engine/i18n"
)

// TranslateRequest asks for the message of each entry in one language.
type TranslateRequest struct {
	Language string           `json:"language"`
	Entries  []TranslateEntry `json:"entries"`
}

// TranslateEntry names one message by module, tag and args.
type TranslateEntry struct {
	Module string         `json:"module"`
	Tag    string         `json:"tag"`
	Args   map[string]any `json:"args,omitempty"`
}

// Translation is one rendered message; Found is false when no catalog has the tag.
type Translation struct {
	Message string `json:"message"`
	Found   bool   `json:"found"`
}

// ParseTranslateRequest decodes a translate request; unknown fields are an error.
func ParseTranslateRequest(data []byte) (TranslateRequest, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var req TranslateRequest
	if err := dec.Decode(&req); err != nil {
		return TranslateRequest{}, &Error{Code: CodeInvalidRequest, Message: err.Error()}
	}
	return req, nil
}

// Translate renders each entry in the requested language; an empty language is English.
func Translate(req TranslateRequest) []Translation {
	language := req.Language
	if language == "" {
		language = "en"
	}
	out := make([]Translation, 0, len(req.Entries))
	for _, entry := range req.Entries {
		msg, found := i18n.TranslateWithStatus(language, entry.Module, entry.Tag, entry.Args)
		out = append(out, Translation{Message: msg, Found: found})
	}
	return out
}

// Languages lists the locales the message catalogs cover, English included.
func Languages() []string { return i18n.AvailableLocales() }
