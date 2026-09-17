// Package registry turns the OpenAPI document into one table of
// operations. The command tree, the schema command, the MCP
// server's catalogue and the documentation all read from it.
package registry

//go:generate go run ./gen

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Param is a path or query parameter of an operation.
type Param struct {
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Description string          `json:"description,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// Operation is one entry of the registry.
type Operation struct {
	ID          string `json:"operationId"`
	Group       string `json:"group"`
	Tag         string `json:"tag"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`

	PathParams   []Param `json:"pathParams,omitempty"`
	QueryParams  []Param `json:"queryParams,omitempty"`
	HeaderParams []Param `json:"headerParams,omitempty"`

	// RequestBodies holds every media type the operation accepts,
	// in name order. Which one a caller sends is the caller's to
	// decide: an assistant has nothing but JSON, the command line
	// has a path to read.
	RequestBodies   []RequestBody `json:"requestBodies,omitempty"`
	RequestRequired bool          `json:"requestRequired,omitempty"`

	// Response describes the first success response.
	ResponseStatus      string          `json:"responseStatus,omitempty"`
	ResponseContentType string          `json:"responseContentType,omitempty"`
	ResponseSchema      json.RawMessage `json:"responseSchema,omitempty"`
	ResponseExample     json.RawMessage `json:"responseExample,omitempty"`

	// Permission lists the keys of which the consumer needs one.
	Permission []string `json:"permission"`

	// Destructive marks an operation that cannot be undone; Safe
	// marks a POST that changes nothing; NDJSON marks a response
	// of one JSON object per line; Task marks an operation whose
	// answer can carry a statusUrl to poll; Idempotent marks one
	// that takes an Idempotency-Key, so a caller repeating it
	// with the same key gets the first answer back, and no second
	// record.
	Destructive bool `json:"destructive,omitempty"`
	Safe        bool `json:"safe,omitempty"`
	NDJSON      bool `json:"ndjson,omitempty"`
	Task        bool `json:"task,omitempty"`
	Idempotent  bool `json:"idempotent,omitempty"`

	// Security lists the schemes that can authenticate the call;
	// Public is set when the call needs none. UserOnly is set
	// when a consumer token cannot make the call at all.
	Security []string `json:"security"`
	Public   bool     `json:"public,omitempty"`
	UserOnly bool     `json:"userOnly,omitempty"`
}

// ReadOnly reports whether a read-only client may call the
// operation: a GET, or a POST marked safe.
func (o *Operation) ReadOnly() bool {
	return o.Method == "GET" || o.Safe
}

// RequestBody is one media type an operation accepts: the JSON
// Schema of the body with every reference resolved, and the
// example the document gives for it.
type RequestBody struct {
	ContentType string          `json:"contentType"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Example     json.RawMessage `json:"example,omitempty"`
}

// carriesFile reports whether the body declares a binary field,
// which marks bytes the caller sends as a file.
func (b *RequestBody) carriesFile() bool {
	var schema struct {
		Properties map[string]struct {
			Format string `json:"format"`
		} `json:"properties"`
	}
	json.Unmarshal(b.Schema, &schema)
	for _, prop := range schema.Properties {
		if prop.Format == "binary" {
			return true
		}
	}
	return false
}

// Body returns the body of the media type, or nil.
func (o *Operation) Body(contentType string) *RequestBody {
	for i := range o.RequestBodies {
		if o.RequestBodies[i].ContentType == contentType {
			return &o.RequestBodies[i]
		}
	}
	return nil
}

// JSONBody returns the JSON body, or nil. A caller that can send
// nothing else has no other option than this one.
func (o *Operation) JSONBody() *RequestBody {
	return o.Body("application/json")
}

// AttachmentBody returns the body that carries a file, the one
// declaring a binary field, or nil. An operation can declare it
// beside a JSON body taking the same bytes inline, and then a
// caller holding a path wants this one.
func (o *Operation) AttachmentBody() *RequestBody {
	for i := range o.RequestBodies {
		if o.RequestBodies[i].carriesFile() {
			return &o.RequestBodies[i]
		}
	}
	return nil
}

// ContentTypes names every media type the operation accepts, in
// the order the bodies are kept.
func (o *Operation) ContentTypes() []string {
	if len(o.RequestBodies) == 0 {
		return nil
	}
	types := make([]string, 0, len(o.RequestBodies))
	for _, body := range o.RequestBodies {
		types = append(types, body.ContentType)
	}
	return types
}

// DefaultBody returns the body for a caller that states no
// preference: the JSON one when the operation declares it, else
// the first in name order. Nil when it takes no body.
func (o *Operation) DefaultBody() *RequestBody {
	if body := o.JSONBody(); body != nil {
		return body
	}
	if len(o.RequestBodies) == 0 {
		return nil
	}
	return &o.RequestBodies[0]
}

// MarshalJSON prints the default body under the single-body keys
// beside the bodies themselves. The operation table is a
// published interface: `transpareo commands --json` prints it,
// and a reader of either shape keeps working.
func (o Operation) MarshalJSON() ([]byte, error) {
	type operation Operation
	out := struct {
		operation
		RequestBody        json.RawMessage `json:"requestBody,omitempty"`
		RequestContentType string          `json:"requestContentType,omitempty"`
		RequestExample     json.RawMessage `json:"requestExample,omitempty"`
	}{operation: operation(o)}
	if body := o.DefaultBody(); body != nil {
		out.RequestBody = body.Schema
		out.RequestContentType = body.ContentType
		out.RequestExample = body.Example
	}
	return json.Marshal(out)
}

// Registry is the loaded operation table.
type Registry struct {
	Version    string      `json:"version"`
	Operations []Operation `json:"operations"`
	byID       map[string]*Operation
}

// Default returns the registry generated from the embedded
// specification.
func Default() *Registry {
	reg := &Registry{Version: generatedVersion,
		Operations: generatedOperations}
	reg.index()
	return reg
}

func (r *Registry) index() {
	r.byID = make(map[string]*Operation, len(r.Operations))
	for i := range r.Operations {
		r.byID[r.Operations[i].ID] = &r.Operations[i]
	}
}

// Find returns the operation with the id, or nil.
func (r *Registry) Find(id string) *Operation {
	return r.byID[id]
}

// Groups returns the group names in order.
func (r *Registry) Groups() []string {
	seen := map[string]bool{}
	var groups []string
	for _, op := range r.Operations {
		if !seen[op.Group] {
			seen[op.Group] = true
			groups = append(groups, op.Group)
		}
	}
	sort.Strings(groups)
	return groups
}

var methods = []string{"get", "post", "put", "patch", "delete"}

// Load parses an OpenAPI 3 document.
func Load(doc []byte) (*Registry, error) {
	var root map[string]any
	if err := json.Unmarshal(doc, &root); err != nil {
		return nil, fmt.Errorf("registry: parsing the specification: %w", err)
	}
	res := &resolver{root: root}
	info, _ := root["info"].(map[string]any)
	reg := &Registry{Version: str(info["version"])}
	paths, _ := root["paths"].(map[string]any)
	pathKeys := make([]string, 0, len(paths))
	for path := range paths {
		pathKeys = append(pathKeys, path)
	}
	sort.Strings(pathKeys)
	globalSecurity, _ := root["security"].([]any)
	for _, path := range pathKeys {
		item, _ := paths[path].(map[string]any)
		shared, _ := item["parameters"].([]any)
		for _, method := range methods {
			raw, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			op, err := res.operation(method, path, raw, shared, globalSecurity)
			if err != nil {
				return nil, err
			}
			reg.Operations = append(reg.Operations, *op)
		}
	}
	seen := map[string]bool{}
	for _, op := range reg.Operations {
		if seen[op.ID] {
			return nil, fmt.Errorf("registry: operationId %q appears twice",
				op.ID)
		}
		seen[op.ID] = true
	}
	reg.index()
	return reg, nil
}

type resolver struct {
	root map[string]any
}

func (r *resolver) operation(method, path string, raw map[string]any,
	shared []any, globalSecurity []any) (*Operation, error) {
	id := str(raw["operationId"])
	if id == "" {
		return nil, fmt.Errorf("registry: %s %s has no operationId",
			strings.ToUpper(method), path)
	}
	op := &Operation{
		ID:          id,
		Method:      strings.ToUpper(method),
		Path:        path,
		Summary:     str(raw["summary"]),
		Description: str(raw["description"]),
		Destructive: raw["x-destructive"] == true,
		Safe:        raw["x-safe"] == true,
		NDJSON:      raw["x-ndjson"] == true,
		Permission:  stringList(raw["x-permission"]),
	}
	tags, _ := raw["tags"].([]any)
	if len(tags) > 0 {
		op.Tag = str(tags[0])
	}
	op.Group = groupName(op.Tag)
	params, _ := raw["parameters"].([]any)
	for _, p := range append(append([]any{}, shared...), params...) {
		param, err := r.param(p)
		if err != nil {
			return nil, fmt.Errorf("registry: %s: %w", id, err)
		}
		switch param.In {
		case "path":
			op.PathParams = append(op.PathParams, param)
		case "query":
			op.QueryParams = append(op.QueryParams, param)
		case "header":
			// The key is not a header a caller sets by hand: the
			// command line has --idempotency-key and the client
			// fills one in on every POST. It is kept as a fact
			// about the operation, because only the operations
			// declaring it replay an answer.
			if param.Name == "Idempotency-Key" {
				op.Idempotent = true
				continue
			}
			op.HeaderParams = append(op.HeaderParams, param)
		}
	}
	if err := r.requestBody(op, raw["requestBody"]); err != nil {
		return nil, fmt.Errorf("registry: %s: %w", id, err)
	}
	if err := r.response(op, raw["responses"]); err != nil {
		return nil, fmt.Errorf("registry: %s: %w", id, err)
	}
	security, ok := raw["security"].([]any)
	if !ok {
		security = globalSecurity
	}
	op.Security, op.Public = schemes(security)
	if ok && len(security) == 0 {
		op.Public = true
	}
	op.UserOnly = !op.Public && len(op.Security) > 0 &&
		!contains(op.Security, "oauth2")
	return op, nil
}

func (r *resolver) param(v any) (Param, error) {
	resolved, err := r.resolve(v, nil)
	if err != nil {
		return Param{}, err
	}
	m, _ := resolved.(map[string]any)
	param := Param{
		Name:        str(m["name"]),
		In:          str(m["in"]),
		Description: str(m["description"]),
		Required:    m["required"] == true,
	}
	if schema, ok := m["schema"]; ok {
		param.Schema = marshal(schema)
	}
	return param, nil
}

func (r *resolver) requestBody(op *Operation, v any) error {
	if v == nil {
		return nil
	}
	resolved, err := r.deref(v)
	if err != nil {
		return err
	}
	body, _ := resolved.(map[string]any)
	op.RequestRequired = body["required"] == true
	content, _ := body["content"].(map[string]any)
	types := make([]string, 0, len(content))
	for contentType := range content {
		types = append(types, contentType)
	}
	sort.Strings(types)
	for _, contentType := range types {
		media, _ := content[contentType].(map[string]any)
		if media == nil {
			continue
		}
		schema, err := r.resolve(media["schema"], nil)
		if err != nil {
			return err
		}
		entry := RequestBody{ContentType: contentType,
			Example: example(media, schema)}
		if schema != nil {
			entry.Schema = marshal(schema)
		}
		op.RequestBodies = append(op.RequestBodies, entry)
	}
	return nil
}

func (r *resolver) response(op *Operation, v any) error {
	responses, _ := v.(map[string]any)
	codes := make([]string, 0, len(responses))
	for code := range responses {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		if !strings.HasPrefix(code, "2") {
			continue
		}
		resolved, err := r.deref(responses[code])
		if err != nil {
			return err
		}
		resp, _ := resolved.(map[string]any)
		if r.answersTask(resp) {
			op.Task = true
		}
		if op.ResponseStatus != "" {
			continue
		}
		op.ResponseStatus = code
		contentType, media := firstMedia(resp["content"])
		if media == nil {
			return nil
		}
		op.ResponseContentType = contentType
		schema, err := r.resolve(media["schema"], nil)
		if err != nil {
			return err
		}
		if schema != nil {
			op.ResponseSchema = marshal(schema)
		}
		op.ResponseExample = example(media, schema)
	}
	return nil
}

// answersTask reports whether a success response's JSON schema
// carries a statusUrl of its own to poll. A list of runs, whose
// rows each name theirs, answers no single task, so the property
// has to sit at the top of the schema.
func (r *resolver) answersTask(resp map[string]any) bool {
	_, media := firstMedia(resp["content"])
	if media == nil {
		return false
	}
	schema, err := r.resolve(media["schema"], nil)
	if err != nil {
		return false
	}
	object, _ := schema.(map[string]any)
	properties, _ := object["properties"].(map[string]any)
	_, found := properties["statusUrl"]
	return found
}

// deref follows a $ref at the top of v only, for request bodies
// and responses that live under components. The schema inside
// is resolved once, by resolve, so a recursive schema keeps one
// level, not two.
func (r *resolver) deref(v any) (any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	ref, ok := m["$ref"].(string)
	if !ok {
		return v, nil
	}
	return r.lookup(ref)
}

// resolve replaces every $ref below v with the object it names.
// A reference met again on the way down is left in place, so a
// recursive schema stays finite.
func (r *resolver) resolve(v any, stack []string) (any, error) {
	switch value := v.(type) {
	case map[string]any:
		if ref, ok := value["$ref"].(string); ok {
			if contains(stack, ref) {
				return map[string]any{"$ref": ref}, nil
			}
			target, err := r.lookup(ref)
			if err != nil {
				return nil, err
			}
			return r.resolve(target, append(stack, ref))
		}
		out := make(map[string]any, len(value))
		for key, child := range value {
			resolved, err := r.resolve(child, stack)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(value))
		for i, child := range value {
			resolved, err := r.resolve(child, stack)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return v, nil
	}
}

func (r *resolver) lookup(ref string) (any, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("unsupported reference %q", ref)
	}
	var node any = r.root
	for _, part := range strings.Split(ref[2:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0",
			"~")
		m, ok := node.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("dangling reference %q", ref)
		}
		node, ok = m[part]
		if !ok {
			return nil, fmt.Errorf("dangling reference %q", ref)
		}
	}
	return node, nil
}

// firstMedia returns the JSON media type of a content map when
// there is one, else the first media type in sorted order. It
// reads a response, where the server picks the media type; a
// request keeps every one of them, for the caller to pick.
func firstMedia(v any) (string, map[string]any) {
	content, _ := v.(map[string]any)
	if len(content) == 0 {
		return "", nil
	}
	if media, ok := content["application/json"].(map[string]any); ok {
		return "application/json", media
	}
	types := make([]string, 0, len(content))
	for t := range content {
		types = append(types, t)
	}
	sort.Strings(types)
	media, _ := content[types[0]].(map[string]any)
	return types[0], media
}

// example takes the media type's example, else the schema's.
func example(media map[string]any, schema any) json.RawMessage {
	if ex, ok := media["example"]; ok {
		return marshal(ex)
	}
	if examples, ok := media["examples"].(map[string]any); ok {
		names := make([]string, 0, len(examples))
		for name := range examples {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if ex, ok := examples[name].(map[string]any); ok {
				if value, ok := ex["value"]; ok {
					return marshal(value)
				}
			}
		}
	}
	if s, ok := schema.(map[string]any); ok {
		if ex, ok := s["example"]; ok {
			return marshal(ex)
		}
	}
	return nil
}

// schemes lists the security scheme names an operation accepts
// and whether an empty requirement (no authentication) is among
// them.
func schemes(security []any) ([]string, bool) {
	names := []string{}
	public := false
	for _, req := range security {
		m, _ := req.(map[string]any)
		if len(m) == 0 {
			public = true
			continue
		}
		for name := range m {
			if !contains(names, name) {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names, public
}

// groupName turns a tag into a command group: "DPPs" becomes
// "dpps", "Reference Data" becomes "reference-data".
func groupName(tag string) string {
	return strings.ToLower(strings.Join(strings.Fields(tag), "-"))
}

func stringList(v any) []string {
	switch value := v.(type) {
	case string:
		return []string{value}
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			out = append(out, str(item))
		}
		return out
	default:
		return []string{}
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func marshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// Match finds the operation a concrete request would hit, by
// method and by the path template its path fits. The path is
// relative to the API root, with or without the leading /api.
func (r *Registry) Match(method, path string) *Operation {
	method = strings.ToUpper(method)
	path = "/" + strings.Trim(strings.TrimPrefix(path, "/api"), "/")
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	segments := strings.Split(path, "/")
	var best *Operation
	bestLiterals := -1
	for i := range r.Operations {
		op := &r.Operations[i]
		if op.Method != method {
			continue
		}
		literals, ok := matchTemplate(op.Path, segments)
		if ok && literals > bestLiterals {
			best, bestLiterals = op, literals
		}
	}
	return best
}

// matchTemplate reports whether the segments fit the template
// and how many of its segments are literal, so /dpps/validate
// wins over /dpps/{id}.
func matchTemplate(template string, segments []string) (int, bool) {
	parts := strings.Split(template, "/")
	if len(parts) != len(segments) {
		return 0, false
	}
	literals := 0
	for i, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			if segments[i] == "" {
				return 0, false
			}
			continue
		}
		if part != segments[i] {
			return 0, false
		}
		literals++
	}
	return literals, true
}
