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

// CatalogueDoc is the vendored catalogue: the specification
// version it was built from, the instructions an assistant reads
// on connect, and the tools by name.
type CatalogueDoc struct {
	Version      string
	Instructions string
	Tools        map[string]CatalogueTool
}

// CatalogueTool is one tool of the hosted catalogue. The first
// block is the declaration, what a tool is, and both servers build
// their own tool from it. The second is what that building
// produced on the hosted side, which is what an assistant is
// shown. Comparing the second is how two renderings of one
// declaration are held together.
type CatalogueTool struct {
	Name        string
	Group       string
	Operation   string
	Kind        string
	Sentence    string
	Confirm     string
	Destructive bool
	Safe        bool

	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
}

// Catalogue parses the vendored catalogue.
func Catalogue() CatalogueDoc {
	var doc struct {
		Version      string `json:"version"`
		Instructions string `json:"instructions"`
		Tools        []struct {
			Name         string          `json:"name"`
			Description  string          `json:"description"`
			OutputSchema json.RawMessage `json:"outputSchema"`
			Extension    struct {
				OperationID string `json:"operationId"`
				Group       string `json:"group"`
				Kind        string `json:"kind"`
				Sentence    string `json:"sentence"`
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
			Kind:      t.Extension.Kind,
			Sentence:  t.Extension.Sentence,
			Confirm: confirmPhrase(t.Extension.Confirm,
				declaredConfirm(t.InputSchema)),
			Destructive:  t.Extension.Destructive,
			Safe:         t.Extension.Safe,
			Description:  t.Description,
			InputSchema:  t.InputSchema,
			OutputSchema: t.OutputSchema,
		}
	}
	return CatalogueDoc{Version: doc.Version,
		Instructions: doc.Instructions, Tools: tools}
}

// CuratedNames lists, in the order the document gives them, the
// tools the catalogue declares over an operation. The rest are
// composed by hand on one side or the other.
func (d CatalogueDoc) CuratedNames() []string {
	var doc struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	json.Unmarshal(CatalogueJSON, &doc)
	var names []string
	for _, t := range doc.Tools {
		if d.Tools[t.Name].Operation != "" {
			names = append(names, t.Name)
		}
	}
	return names
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
