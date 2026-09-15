// Command drift refuses a release whose embedded specification
// has fallen behind the one a live host serves.
//
//	go run ./spec/drift -host apicheck.transpareo.dev
//
// The nightly job already opens a pull request on every
// difference, but a pull request nobody merges is not a guard:
// four releases went out on a specification four versions old
// while that job reported the drift every night. This runs
// before the archives are built and stops the tag instead.
//
// It exits 0 when the binary may go out, 1 when the embedded
// document is too far behind, and 2 when it could not find out.
// Not finding out is a failure on purpose: a check that passes
// when it cannot reach the host is the hole it is here to close.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/transpareo/transpareo-cli/spec"
)

const (
	exitStale     = 1
	exitUndecided = 2
)

func main() {
	host := flag.String("host", os.Getenv("TRANSPAREO_TEST_HOST"),
		"the host whose specification the release is measured against")
	flag.Parse()

	if *host == "" {
		fail(exitUndecided, "no host to measure against: pass -host or set "+
			"TRANSPAREO_TEST_HOST")
	}
	live, err := liveVersion(*host)
	if err != nil {
		fail(exitUndecided, fmt.Sprintf("%s: %v", *host, err))
	}
	builtIn := spec.Version()
	if tooStale, reason := stale(builtIn, live); tooStale {
		fail(exitStale, reason)
	}
	fmt.Printf("specification %s is current enough to release; %s serves "+
		"%s\n", builtIn, *host, live)
}

// stale reports whether a binary built from builtIn may go out
// against a host serving live, and why not. One minor version
// behind is what a release in the days after an API release
// looks like; two means the vendored document was left behind.
// A binary built from a newer document than the host serves is
// an API release that has not reached that host yet, which is
// not a reason to hold the tag.
func stale(builtIn, live string) (bool, string) {
	drift, ok := spec.Compare(builtIn, live)
	switch {
	case !ok:
		return true, fmt.Sprintf("the embedded %q and the live %q cannot be "+
			"compared as versions", builtIn, live)
	case drift.Major > 0:
		return true, fmt.Sprintf("this binary embeds specification %s and "+
			"the host serves %s, a new major version: vendor it before "+
			"tagging", builtIn, live)
	case drift.Major == 0 && drift.Minor > 1:
		return true, fmt.Sprintf("this binary embeds specification %s and "+
			"the host serves %s, %d minor versions on: run the nightly "+
			"specification job and merge its pull request before tagging",
			builtIn, live, drift.Minor)
	default:
		return false, ""
	}
}

// liveVersion reads info.version off the host's specification. A
// release must not die on one flaky request, so it asks three
// times before giving up.
func liveVersion(host string) (string, error) {
	url := "https://" + host + "/apidocs/openapi.json"
	client := &http.Client{Timeout: 30 * time.Second}
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		var version string
		version, err = fetchVersion(client, url)
		if err == nil {
			return version, nil
		}
		fmt.Fprintf(os.Stderr, "attempt %d of 3 failed: %v\n", attempt, err)
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 5 * time.Second)
		}
	}
	return "", err
}

func fetchVersion(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
		return "", fmt.Errorf("answered %s", resp.Status)
	}
	var doc struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", fmt.Errorf("answered no readable specification: %w", err)
	}
	if doc.Info.Version == "" {
		return "", fmt.Errorf("answered a specification with no info.version")
	}
	return doc.Info.Version, nil
}

func fail(code int, reason string) {
	fmt.Fprintln(os.Stderr, "refusing the release: "+reason)
	os.Exit(code)
}
