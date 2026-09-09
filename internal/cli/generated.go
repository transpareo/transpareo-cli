package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"sort"

	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/internal/registry"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

// annotationOperation is the cobra annotation that names the
// operation behind a generated command.
const annotationOperation = "operationId"

// addGeneratedCommands builds one command per exposed operation
// of the registry under its group: transpareo <group> <verb>.
func (a *App) addGeneratedCommands(root *cobra.Command) {
	reg := registry.Default()
	groups := map[string]*cobra.Command{}
	for _, op := range exposedOperations(reg) {
		words := CommandWords(op)
		parent := groups[words[0]]
		if parent == nil {
			parent = &cobra.Command{
				Use:   words[0],
				Short: groupSummary(reg, op.Group),
			}
			groups[words[0]] = parent
			root.AddCommand(parent)
		}
		for _, word := range words[1 : len(words)-1] {
			parent = childCommand(parent, word)
		}
		parent.AddCommand(a.operationCommand(op, words))
	}
}

func groupSummary(reg *registry.Registry, group string) string {
	for _, op := range reg.Operations {
		if op.Group == group {
			return op.Tag
		}
	}
	return group
}

// childCommand finds or creates the intermediate command for a
// qualifier word such as "bulk" in "dpps bulk create".
func childCommand(parent *cobra.Command, word string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == word {
			return child
		}
	}
	child := &cobra.Command{Use: word, Short: word}
	parent.AddCommand(child)
	return child
}

// operationCall holds the option values of one invocation.
type operationCall struct {
	op      *registry.Operation
	file    string
	sets    []string
	wait    bool
	output  string
	query   map[string]pflag.Value
	headers map[string]*string
	parts   map[string]*string
}

func (a *App) operationCommand(op *registry.Operation,
	words []string) *cobra.Command {
	call := &operationCall{op: op, query: map[string]pflag.Value{},
		headers: map[string]*string{}, parts: map[string]*string{}}
	use := words[len(words)-1]
	for _, p := range op.PathParams {
		use += " <" + p.Name + ">"
	}
	cmd := &cobra.Command{
		Use:         use,
		Short:       op.Summary,
		Long:        longHelp(op),
		Example:     exampleFor(op, words),
		Args:        cobra.ExactArgs(len(op.PathParams)),
		Annotations: map[string]string{annotationOperation: op.ID},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runOperation(cmd.Context(), call, args)
		},
	}
	f := cmd.Flags()
	for _, p := range op.QueryParams {
		call.query[p.Name] = addParamFlag(f, p)
	}
	for _, p := range op.HeaderParams {
		call.headers[p.Name] = f.String(flagName(p.Name), "",
			usage(p.Description))
	}
	switch {
	case op.RequestContentType == "multipart/form-data":
		for _, field := range multipartFields(op) {
			call.parts[field.name] = f.String(field.flag, "",
				usage(field.usage))
		}
	case op.RequestContentType != "":
		f.StringVar(&call.file, "file", "",
			"request body from a file, or - for standard input")
		if op.RequestContentType == "application/json" {
			f.StringArrayVar(&call.sets, "set", nil,
				"body field as key=value, nested with dots (repeatable)")
		}
	}
	if op.Task {
		f.BoolVar(&call.wait, "wait", false,
			"poll the statusUrl until the work is done")
	}
	f.StringVarP(&call.output, "output", "o", "",
		"write the answer to this file instead of standard output")
	return cmd
}

// addParamFlag registers a query parameter as a typed option.
func addParamFlag(f *pflag.FlagSet, p registry.Param) pflag.Value {
	var schema struct {
		Type string `json:"type"`
	}
	json.Unmarshal(p.Schema, &schema)
	name := flagName(p.Name)
	switch schema.Type {
	case "integer":
		f.Int(name, 0, usage(p.Description))
	case "boolean":
		f.Bool(name, false, usage(p.Description))
	default:
		f.String(name, "", usage(p.Description))
	}
	return f.Lookup(name).Value
}

func longHelp(op *registry.Operation) string {
	var b strings.Builder
	if op.Description != "" {
		b.WriteString(op.Description)
	} else {
		b.WriteString(op.Summary)
	}
	b.WriteString("\n\nOperation: " + op.ID + " (" + op.Method + " " +
		op.Path + ")")
	switch {
	case len(op.Permission) == 1:
		b.WriteString("\nPermission: " + op.Permission[0])
	case len(op.Permission) > 1:
		b.WriteString("\nPermission: one of " + strings.Join(op.Permission,
			", "))
	case op.Public:
		b.WriteString("\nPermission: none, the endpoint is public")
	}
	if op.Destructive {
		b.WriteString("\nThis operation cannot be undone; it requires --yes.")
	}
	return b.String()
}

// exampleFor builds the one example every command's help
// carries, from the path parameters, the first query parameter
// and the body the operation takes.
func exampleFor(op *registry.Operation, words []string) string {
	parts := append([]string{"transpareo"}, words...)
	for _, p := range op.PathParams {
		parts = append(parts, "<"+p.Name+">")
	}
	switch {
	case op.RequestContentType == "multipart/form-data":
		for _, field := range multipartFields(op) {
			if field.binary {
				parts = append(parts, "--"+field.flag, "<path>")
			}
		}
	case op.RequestContentType != "":
		parts = append(parts, "--file", "body.json")
	}
	if len(op.QueryParams) > 0 && op.RequestContentType == "" {
		p := op.QueryParams[0]
		parts = append(parts, "--"+flagName(p.Name), "<"+p.Name+">")
	}
	if op.Destructive {
		parts = append(parts, "--yes")
	}
	return "  " + strings.Join(parts, " ")
}

type multipartField struct {
	name, flag, usage string
	binary            bool
}

// multipartFields lists the form fields of a multipart body as
// options; a binary field takes a file path.
func multipartFields(op *registry.Operation) []multipartField {
	var schema struct {
		Properties map[string]struct {
			Format      string `json:"format"`
			Description string `json:"description"`
			Type        string `json:"type"`
		} `json:"properties"`
	}
	json.Unmarshal(op.RequestBody, &schema)
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	var fields []multipartField
	for _, name := range names {
		prop := schema.Properties[name]
		field := multipartField{name: name, flag: flagName(formFieldFlag(name)),
			binary: prop.Format == "binary"}
		field.usage = prop.Description
		if field.binary {
			field.usage = "path of the file to upload"
			if prop.Description != "" {
				field.usage += "; " + prop.Description
			}
		} else if prop.Type == "object" {
			field.usage += " (JSON)"
		}
		fields = append(fields, field)
	}
	return fields
}

// formFieldFlag turns "mediafile[file]" into "file".
func formFieldFlag(name string) string {
	if i := strings.Index(name, "["); i >= 0 {
		return strings.TrimSuffix(name[i+1:], "]")
	}
	return name
}

func (a *App) runOperation(ctx context.Context, call *operationCall,
	args []string) error {
	op := call.op
	if err := a.refuseUnlessAllowed(op, op.Method); err != nil {
		return err
	}
	req := &transpareo.Request{
		Method: op.Method,
		Path:   fillPath(op, args),
		Query:  url.Values{},
		Header: http.Header{},
	}
	for name, value := range call.query {
		if value.String() != "" && value.String() != "0" &&
			value.String() != "false" {
			req.Query.Set(name, value.String())
		}
	}
	for name, value := range call.headers {
		if *value != "" {
			req.Header.Set(name, *value)
		}
	}
	if err := a.attachBody(call, req); err != nil {
		return err
	}
	if op.ResponseContentType != "" &&
		!strings.Contains(op.ResponseContentType, "json") {
		req.Accept = op.ResponseContentType
	}
	client, _, err := a.Client()
	if err != nil {
		return err
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return err
	}
	if call.wait {
		return a.waitForTask(ctx, client, resp)
	}
	if call.output != "" {
		return os.WriteFile(call.output, resp.Body, 0o644)
	}
	return a.printOperationResponse(op, resp)
}

// fillPath replaces the path parameters with the positional
// arguments.
func fillPath(op *registry.Operation, args []string) string {
	path := op.Path
	for i, p := range op.PathParams {
		path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(args[i]))
	}
	return path
}

// attachBody reads the body from --file, --set or the multipart
// options, as the operation takes it.
func (a *App) attachBody(call *operationCall, req *transpareo.Request) error {
	op := call.op
	switch {
	case op.RequestContentType == "":
		return nil
	case op.RequestContentType == "multipart/form-data":
		return a.attachMultipart(call, req)
	case call.file != "" && len(call.sets) > 0:
		return output.Exit(output.ExitUsage,
			errors.New("use --file or --set, not both"))
	case call.file != "":
		data, err := a.readBody(call.file)
		if err != nil {
			return err
		}
		req.Body = data
		req.ContentType = op.RequestContentType
		return nil
	case len(call.sets) > 0:
		body, err := bodyFromSets(call.sets)
		if err != nil {
			return err
		}
		req.Body = body
		return nil
	case op.RequestRequired:
		return output.Exit(output.ExitUsage, fmt.Errorf(
			"%s needs a body: --file <path>, --file - for standard input, "+
				"or --set key=value; see `transpareo schema %s`", op.ID, op.ID))
	default:
		return nil
	}
}

// bodyFromSets builds a JSON object from key=value pairs. Keys
// nest with dots; values that parse as JSON are kept as such,
// anything else is a string.
func bodyFromSets(sets []string) (map[string]any, error) {
	body := map[string]any{}
	for _, set := range sets {
		key, value, ok := strings.Cut(set, "=")
		if !ok || key == "" {
			return nil, output.Exit(output.ExitUsage,
				fmt.Errorf("%q is not key=value", set))
		}
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			parsed = value
		}
		node := body
		path := strings.Split(key, ".")
		for _, part := range path[:len(path)-1] {
			child, ok := node[part].(map[string]any)
			if !ok {
				child = map[string]any{}
				node[part] = child
			}
			node = child
		}
		node[path[len(path)-1]] = parsed
	}
	return body, nil
}

func (a *App) attachMultipart(call *operationCall,
	req *transpareo.Request) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, field := range multipartFields(call.op) {
		value := *call.parts[field.name]
		if value == "" {
			continue
		}
		if field.binary {
			if err := writeFilePart(writer, field.name, value); err != nil {
				return err
			}
			continue
		}
		if err := writeFormFields(writer, field.name, value); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	req.Body = buf.Bytes()
	req.ContentType = writer.FormDataContentType()
	return nil
}

func writeFilePart(writer *multipart.Writer, field, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return output.Exit(output.ExitUsage, err)
	}
	defer file.Close()
	part, err := writer.CreateFormFile(field, file.Name())
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

// writeFormFields writes a value as form fields; a JSON object
// becomes nested fields (mappings[column][action]), a JSON
// string or anything else one field.
func writeFormFields(writer *multipart.Writer, field, value string) error {
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return writer.WriteField(field, value)
	}
	var walk func(name string, v any) error
	walk = func(name string, v any) error {
		switch node := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(node))
			for key := range node {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if err := walk(name+"["+key+"]", node[key]); err != nil {
					return err
				}
			}
			return nil
		case []any:
			for _, item := range node {
				if err := walk(name+"[]", item); err != nil {
					return err
				}
			}
			return nil
		case string:
			return writer.WriteField(name, node)
		default:
			data, _ := json.Marshal(node)
			return writer.WriteField(name, string(data))
		}
	}
	return walk(field, parsed)
}

// waitForTask follows the statusUrl of the answer, printing
// progress to stderr and the final document to stdout.
func (a *App) waitForTask(ctx context.Context, client *transpareo.Client,
	resp *transpareo.Response) error {
	var doc struct {
		StatusURL string `json:"statusUrl"`
	}
	if err := json.Unmarshal(resp.Body, &doc); err != nil ||
		doc.StatusURL == "" {
		return a.Printer().Print(resp.Body)
	}
	printer := a.Printer()
	task, err := client.WaitForTask(ctx, doc.StatusURL, &transpareo.WaitOptions{
		OnPoll: func(task *transpareo.Task) {
			printer.Message("%s %d%%", task.Status, task.Progress)
		},
	})
	if err != nil {
		return err
	}
	if err := printer.Print(task.Body); err != nil {
		return err
	}
	if task.Failed() {
		return output.Exit(output.ExitAPI, nil)
	}
	return nil
}

// printOperationResponse prints the answer. A list answers an
// object with the items under one key; for tables, ids and
// field projection the items are what matters, so those modes
// unwrap them.
func (a *App) printOperationResponse(op *registry.Operation,
	resp *transpareo.Response) error {
	printer := a.Printer()
	contentType := resp.Header.Get("Content-Type")
	if len(resp.Body) == 0 {
		if !printer.JSONMode() {
			printer.Message("HTTP %d, no content.", resp.StatusCode)
		}
		return nil
	}
	if !strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "ndjson") {
		return printer.PrintRaw(resp.Body)
	}
	unwrap := printer.Quiet || printer.JSONL || len(printer.Fields) > 0 ||
		!printer.JSONMode()
	if unwrap && strings.HasPrefix(op.ID, "list_") {
		if items := listItems(resp.Body); items != nil {
			return printer.Print(items)
		}
	}
	return printer.Print(resp.Body)
}

// listItems returns the first array-valued key of a list answer,
// or nil when the body is not such an object.
func listItems(body []byte) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := bytes.TrimSpace(obj[key])
		if len(value) > 0 && value[0] == '[' {
			return value
		}
	}
	return nil
}
