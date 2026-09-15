// Package spec embeds the OpenAPI document the binary was built
// from. The command tree, the operation registry, the schema
// command and the doctor's version check all read it from here,
// so the binary never depends on a network fetch to know its
// own surface.
package spec

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// JSON is the vendored specification, byte for byte as committed
// under spec/openapi.json.
//
//go:embed openapi.json
var JSON []byte

// Version returns the specification version (info.version) of the
// embedded document.
func Version() string {
	var doc struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.Unmarshal(JSON, &doc); err != nil {
		panic(fmt.Sprintf("spec: embedded openapi.json is invalid: %v", err))
	}
	return doc.Info.Version
}

// Drift is how far a live specification is ahead of the one a
// binary was built from. Minor counts only within a major
// version, because a new major renumbers the minors.
type Drift struct {
	Major int
	Minor int
}

// Compare measures a live specification version against the one
// built in. It reports false when either version is not a dotted
// number, which is the one case a caller cannot rate.
func Compare(builtIn, live string) (Drift, bool) {
	b, okBuiltIn := parseVersion(builtIn)
	l, okLive := parseVersion(live)
	if !okBuiltIn || !okLive {
		return Drift{}, false
	}
	return Drift{Major: l[0] - b[0], Minor: l[1] - b[1]}, true
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) < 2 {
		return out, false
	}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
