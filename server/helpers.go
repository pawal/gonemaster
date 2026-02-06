package server

import (
	"encoding/json"
	"io"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func readJSON(r *http.Request, maxBodySize int64, dst any) error {
	if maxBodySize > 0 {
		r.Body = io.NopCloser(io.LimitReader(r.Body, maxBodySize))
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, code string, message string, details map[string]any) {
	if metricsWriter, ok := w.(metricsAwareResponseWriter); ok {
		metricsWriter.SetErrorCode(code)
	}
	resp := ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
	writeJSON(w, status, resp)
}
