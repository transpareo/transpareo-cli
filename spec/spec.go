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
