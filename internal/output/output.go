// Package output shapes what commands print: JSON by default off
// a terminal, tables on one, ids only under --quiet, and the
// error envelope with the exit codes that have fixed meanings.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

// Exit codes with fixed meanings, so a pipeline can branch on
// them.
const (
	ExitOK         = 0
	ExitAPI        = 1
	ExitUsage      = 2
	ExitValidation = 3
	ExitRefused    = 4
	ExitMapping    = 5
)

// ExitError carries an exit code with an error.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// Exit wraps err with a code.
func Exit(code int, err error) error {
	return &ExitError{Code: code, Err: err}
}

// ExitCode returns the code an error maps to: an ExitError's
// own, one for an API or transport error, two for anything
// else, which is a usage error.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	var apiErr *transpareo.Error
	if errors.As(err, &apiErr) {
		return ExitAPI
	}
	return ExitUsage
}

// Options are the output flags every command accepts.
type Options struct {
	JSON   bool
	JSONL  bool
	Quiet  bool
	Fields []string
}

// Printer writes results and errors in the chosen shape.
type Printer struct {
	Out      io.Writer
	Err      io.Writer
	Terminal bool
	Options
}

// JSONMode reports whether output is machine-shaped: asked for,
// or not on a terminal.
func (p *Printer) JSONMode() bool {
	return p.JSON || p.JSONL || !p.Terminal
}

// Print writes a result. Lists become one line per item under
// --jsonl, ids only under --quiet, a table on a terminal, and
// pretty JSON otherwise. Objects become key/value lines on a
// terminal and pretty JSON otherwise.
func (p *Printer) Print(v any) error {
	value, err := normalise(v)
	if err != nil {
		return err
	}
	value = project(value, p.Fields)
	switch {
	case p.Quiet:
		return p.printIDs(value)
	case p.JSONL:
		return p.printLines(value)
	case p.JSONMode():
		return p.printJSON(value)
	default:
		return p.printHuman(value)
	}
}

// PrintRaw writes bytes as they are, with a trailing newline
// when they lack one.
func (p *Printer) PrintRaw(data []byte) error {
	if _, err := p.Out.Write(data); err != nil {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		_, err := io.WriteString(p.Out, "\n")
		return err
	}
	return nil
}

// Message writes a line for a person, to stderr, so it never
// mixes with a result on stdout.
func (p *Printer) Message(format string, args ...any) {
	fmt.Fprintf(p.Err, format+"\n", args...)
}

// Envelope is the error shape on stdout in JSON mode.
type Envelope struct {
	OK    bool          `json:"ok"`
	Error EnvelopeError `json:"error"`
}

// EnvelopeError is the error part of the envelope.
type EnvelopeError struct {
	Code      string                           `json:"code"`
	Message   string                           `json:"message"`
	Hint      string                           `json:"hint,omitempty"`
	DocsURL   string                           `json:"docsUrl,omitempty"`
	Fields    map[string]transpareo.FieldError `json:"fields,omitempty"`
	Retryable bool                             `json:"retryable"`
	Status    int                              `json:"status,omitempty"`
}

// PrintError writes an error: the envelope on stdout in JSON
// mode, "CODE: message" and the hint on stderr otherwise.
func (p *Printer) PrintError(err error) {
	env := envelope(err)
	if p.JSONMode() {
		writeJSON(p.Out, env, "  ")
		return
	}
	fmt.Fprintf(p.Err, "%s: %s\n", env.Error.Code, env.Error.Message)
	if env.Error.Hint != "" {
		fmt.Fprintln(p.Err, env.Error.Hint)
	}
	for name, field := range sortedFields(env.Error.Fields) {
		text := field.FullMessage
		if text == "" {
			text = field.Message
		}
		if len(field.Missing) > 0 {
			text = "missing " + strings.Join(field.Missing, ", ")
		}
		fmt.Fprintf(p.Err, "  %s: %s\n", name, text)
	}
}

func envelope(err error) Envelope {
	env := Envelope{Error: EnvelopeError{Code: "ERROR", Message: err.Error()}}
	var apiErr *transpareo.Error
	if errors.As(err, &apiErr) {
		env.Error = EnvelopeError{
			Code:      apiErr.Code,
			Message:   apiErr.Message,
			Hint:      apiErr.Hint,
			DocsURL:   apiErr.DocsURL,
			Fields:    apiErr.Fields,
			Retryable: apiErr.Retryable,
			Status:    apiErr.Status,
		}
		return env
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		env.Error.Code = codeName(exit.Code)
	}
	return env
}

// fieldSeq iterates field errors by name.
type fieldSeq func(yield func(string, transpareo.FieldError) bool)

func codeName(code int) string {
	switch code {
	case ExitUsage:
		return "USAGE"
	case ExitValidation:
		return "VALIDATION_FAILED"
	case ExitRefused:
		return "CONFIRMATION_REQUIRED"
	case ExitMapping:
		return "MAPPING_REQUIRED"
	default:
		return "ERROR"
	}
}

func sortedFields(fields map[string]transpareo.FieldError) fieldSeq {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return func(yield func(string, transpareo.FieldError) bool) {
		for _, name := range names {
			if !yield(name, fields[name]) {
				return
			}
		}
	}
}

// normalise turns any value into the generic JSON shapes
// (map[string]any, []any, scalars) so one code path formats
// them.
func normalise(v any) (any, error) {
	var data []byte
	switch value := v.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		data = value
	case []byte:
		data = value
	default:
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("the response is not JSON: %w", err)
	}
	return out, nil
}

// project keeps only the named fields of an object or of every
// item of a list. A name may reach into nested objects with a
// dot.
func project(v any, fields []string) any {
	if len(fields) == 0 {
		return v
	}
	switch value := v.(type) {
	case map[string]any:
		return projectObject(value, fields)
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = project(item, fields)
		}
		return out
	default:
		return v
	}
}

func projectObject(obj map[string]any, fields []string) map[string]any {
	out := map[string]any{}
	for _, field := range fields {
		if value, ok := lookup(obj, field); ok {
			out[field] = value
		}
	}
	return out
}

// lookup finds a value by key, or by a dotted path into nested
// objects when no key of that literal name exists.
func lookup(obj map[string]any, path string) (any, bool) {
	if value, ok := obj[path]; ok {
		return value, true
	}
	head, rest, nested := strings.Cut(path, ".")
	value, ok := obj[head]
	if !ok {
		return nil, false
	}
	if !nested {
		return value, true
	}
	child, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return lookup(child, rest)
}

func (p *Printer) printJSON(v any) error {
	return writeJSON(p.Out, v, "  ")
}

// writeJSON encodes without escaping < and >, which JSON does not
// require and which would garble hints and examples for a reader.
func writeJSON(w io.Writer, v any, indent string) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)
	return enc.Encode(v)
}

func (p *Printer) printLines(v any) error {
	items, ok := v.([]any)
	if !ok {
		items = []any{v}
	}
	for _, item := range items {
		if err := writeJSON(p.Out, item, ""); err != nil {
			return err
		}
	}
	return nil
}

// idKeys are tried in order when --quiet asks for ids.
var idKeys = []string{"id", "code", "key", "taskId", "name"}

func (p *Printer) printIDs(v any) error {
	items, ok := v.([]any)
	if !ok {
		items = []any{v}
	}
	for _, item := range items {
		if _, err := fmt.Fprintln(p.Out, identifier(item)); err != nil {
			return err
		}
	}
	return nil
}

func identifier(item any) string {
	obj, ok := item.(map[string]any)
	if !ok {
		return scalar(item)
	}
	for _, key := range idKeys {
		if value, ok := obj[key]; ok && value != nil {
			return scalar(value)
		}
	}
	return ""
}

func (p *Printer) printHuman(v any) error {
	switch value := v.(type) {
	case []any:
		return p.printTable(value)
	case map[string]any:
		return p.printObject(value)
	case nil:
		return nil
	default:
		_, err := fmt.Fprintln(p.Out, scalar(value))
		return err
	}
}
