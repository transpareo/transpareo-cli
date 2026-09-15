package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStale(t *testing.T) {
	for _, tc := range []struct {
		name          string
		builtIn, live string
		want          bool
	}{
		{"the same version", "1.18.0", "1.18.0", false},
		{"a newer patch on the host", "1.18.0", "1.18.3", false},
		{"one minor behind", "1.18.0", "1.19.0", false},
		{"two minors behind", "1.18.0", "1.20.0", true},
		{"four minors behind, the release that failed", "1.14.0", "1.18.0",
			true},
		{"a new major on the host", "1.18.0", "2.0.0", true},
		{"built from a document the host has not got yet", "1.19.0", "1.18.0",
			false},
		{"a major the host has not got yet", "2.0.0", "1.18.0", false},
		{"an unreadable live version", "1.18.0", "unreleased", true},
		{"an unreadable embedded version", "", "1.18.0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := stale(tc.builtIn, tc.live)
			if got != tc.want {
				t.Errorf("stale(%q, %q) = %t, want %t", tc.builtIn, tc.live,
					got, tc.want)
			}
			if got && reason == "" {
				t.Error("a refusal with no reason in it")
			}
			if !got && reason != "" {
				t.Errorf("a pass carrying the reason %q", reason)
			}
		})
	}
}

func TestFetchVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			json.NewEncoder(w).Encode(map[string]any{
				"info": map[string]any{"version": "1.18.0"}})
		case "/empty":
			json.NewEncoder(w).Encode(map[string]any{"info": map[string]any{}})
		case "/html":
			w.Write([]byte("<html>not a specification</html>"))
		default:
			http.Error(w, "gone", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := server.Client()
	version, err := fetchVersion(client, server.URL+"/ok")
	if err != nil || version != "1.18.0" {
		t.Errorf("fetchVersion = %q, %v; want the version of the document",
			version, err)
	}
	for _, tc := range []struct{ path, wants string }{
		{"/empty", "no info.version"},
		{"/html", "no readable specification"},
		{"/missing", "404"},
	} {
		if _, err := fetchVersion(client, server.URL+tc.path); err == nil {
			t.Errorf("%s was accepted", tc.path)
		} else if !strings.Contains(err.Error(), tc.wants) {
			t.Errorf("%s answered %q, want it to mention %q", tc.path, err,
				tc.wants)
		}
	}
}
