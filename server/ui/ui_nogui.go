//go:build nogui
// +build nogui

package ui

import "net/http"

// Handler returns a not-found handler for API-only builds.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "ui not available", http.StatusNotFound)
	})
}
