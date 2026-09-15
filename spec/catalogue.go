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

// CatalogueTool is one tool of the hosted catalogue: what it is
// called and what it answers to, and the three documents an
// assistant actually reads. The guide link the hosted side adds
// to every description is taken out when the catalogue is
// vendored, so the description here is the comparable part.
type CatalogueTool struct {
	Name        string
	Group       string
	Operation   string
	Confirm     string
	Destructive bool
	Safe        bool

	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
}

// Catalogue parses the vendored catalogue into its version and
// its tools by name.
func Catalogue() (string, map[string]CatalogueTool) {
	var doc struct {
		Version string `json:"version"`
		Tools   []struct {
			Name         string          `json:"name"`
			Description  string          `json:"description"`
			OutputSchema json.RawMessage `json:"outputSchema"`
			Extension    struct {
				OperationID string `json:"operationId"`
				Group       string `json:"group"`
				Destructive bool   `json:"destructive"`
				Safe        bool   `json:"safe"`
				Confirm     string `json:"confirm"`
			} `json:"x-transpareo"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(CatalogueJSON, &doc); err != nil {
		panic(fmt.Sprintf("spec: vendored mcp-tools.json is invalid: %v", err))
	}
	tools := make(map[string]CatalogueTool, len(doc.Tools))
	for _, t := range doc.Tools {
		tools[t.Name] = CatalogueTool{
			Name:      t.Name,
			Group:     t.Extension.Group,
			Operation: t.Extension.OperationID,
			Confirm: confirmPhrase(t.Extension.Confirm,
				declaredConfirm(t.InputSchema)),
			Destructive:  t.Extension.Destructive,
			Safe:         t.Extension.Safe,
			Description:  t.Description,
			InputSchema:  t.InputSchema,
			OutputSchema: t.OutputSchema,
		}
	}
	return doc.Version, tools
}

// declaredConfirm reads the description of the confirm argument
// out of an input schema.
func declaredConfirm(schema json.RawMessage) string {
	var doc struct {
		Properties struct {
			Confirm struct {
				Description string `json:"description"`
			} `json:"confirm"`
		} `json:"properties"`
	}
	json.Unmarshal(schema, &doc)
	return doc.Properties.Confirm.Description
}

// confirmPhrase takes the phrase the extension states, and falls
// back to reading it out of the argument's description, `Must be
// "void <id>"`. The description is prose an editor may reword,
// which would leave the comparison agreeing on two empty strings,
// so the extension is where the phrase belongs.
func confirmPhrase(stated, description string) string {
	if stated != "" {
		return stated
	}
	var phrase string
	if _, err := fmt.Sscanf(description, "Must be %q", &phrase); err != nil {
		return ""
	}
	return phrase
}
