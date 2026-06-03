package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunSucceedsForHealthyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var stderr bytes.Buffer
	if code := run([]string{server.URL}, &stderr); code != 0 {
		t.Fatalf("run exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
}

func TestRunFailsForUnhealthyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	var stderr bytes.Buffer
	if code := run([]string{server.URL}, &stderr); code != 1 {
		t.Fatalf("run exit code = %d, want 1", code)
	}
}

func TestRunRejectsTooManyArguments(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"http://127.0.0.1:9090/health", "extra"}, &stderr); code != 2 {
		t.Fatalf("run exit code = %d, want 2", code)
	}
}
