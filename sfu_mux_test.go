package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestM7HSFUMuxRegistersSharedRoutes(t *testing.T) {
	overrides := m7hSFURouteOverrides{
		handlers: make(map[string]http.Handler),
		metrics: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	for _, path := range m7HSFURoutePaths {
		if path == "/metrics" {
			continue
		}
		overrides.handlers[path] = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}
	mux := newM7HSFUMux(nil, "", overrides)
	for _, path := range m7HSFURoutePaths {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code == http.StatusNotFound {
				t.Fatalf("registered route returned 404")
			}
		})
	}
}
