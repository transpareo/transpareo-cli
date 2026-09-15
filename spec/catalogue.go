package spec

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// CatalogueJSON is the tool catalogue the hosted assistant server
// publishes, vendored from /apidocs/mcp-tools.json with the guide
// links taken out of the descriptions. Those links name whichever
// host answered, so the stable document is the host-neutral one.
//
//go:embed mcp-tools.json
var CatalogueJSON []byte

// CatalogueTool is one tool of the hosted catalogue, reduced to
// the fields worth agreeing on. Descriptions are left out: the
// hosted side adds a guide link this binary cannot know.
type CatalogueTool struct {
	Name        string
	Group       string
	Operation   string
	Confirm     string
	Destructive bool
	Safe        bool
}

// Catalogue parses the vendored catalogue into its version and
// its tools by name.
func Catalogue() (string, map[string]CatalogueTool) {
	var doc struct {
		Version string `json:"version"`
		Tools   []struct {
			Name      string `json:"name"`
			Extension struct {
				OperationID string `json:"operationId"`
				Group       string `json:"group"`
				Destructive bool   `json:"destructive"`
				Safe        bool   `json:"safe"`
			} `json:"x-transpareo"`
			InputSchema struct {
				Properties struct {
					Confirm struct {
						Description string `json:"description"`
					} `json:"confirm"`
				} `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(CatalogueJSON, &doc); err != nil {
		panic(fmt.Sprintf("spec: vendored mcp-tools.json is invalid: %v", err))
	}
	tools := make(map[string]CatalogueTool, len(doc.Tools))
	for _, t := range doc.Tools {
		confirm := t.InputSchema.Properties.Confirm.Description
		tools[t.Name] = CatalogueTool{
			Name:        t.Name,
			Group:       t.Extension.Group,
			Operation:   t.Extension.OperationID,
			Confirm:     confirmPhrase(confirm),
			Destructive: t.Extension.Destructive,
			Safe:        t.Extension.Safe,
		}
	}
	return doc.Version, tools
}

// confirmPhrase reads the phrase out of the schema description
// the hosted side writes, `Must be "void <id>"`.
func confirmPhrase(description string) string {
	var phrase string
	if _, err := fmt.Sscanf(description, "Must be %q", &phrase); err != nil {
		return ""
	}
	return phrase
}
