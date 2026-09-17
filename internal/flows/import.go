package flows

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

// Import is the state of one import as the API answers it, with
// the fields the flow reads.
type Import struct {
	ID          json.Number  `json:"id"`
	DataType    string       `json:"dataType"`
	Status      string       `json:"status"`
	StatusURL   string       `json:"statusUrl"`
	Preview     *Preview     `json:"preview"`
	RowErrors   []RowError   `json:"rowErrors"`
	ErrorGroups []ErrorGroup `json:"errorGroups"`
	FailedCount int          `json:"failedCount"`
	Body        json.RawMessage
}

// Preview describes the upload and what a mapping may target,
// present while the import is fresh.
type Preview struct {
	Columns            []Column       `json:"columns"`
	CoreAttributes     []string       `json:"coreAttributes"`
	RequiredAttributes []string       `json:"requiredAttributes"`
	PropertyTypes      []PropertyType `json:"propertyTypes"`
}

// Column is one column of the upload with the platform's
// suggestion.
type Column struct {
	Header          string   `json:"header"`
	Column          string   `json:"column"`
	SampleValues    []string `json:"sampleValues"`
	SingleValue     bool     `json:"singleValue"`
	SuggestedAction string   `json:"suggestedAction"`
	CoreAttribute   string   `json:"coreAttribute"`
	TypeID          string   `json:"typeId"`
	TypeName        string   `json:"typeName"`
	MatchType       string   `json:"matchType"`
	Similarity      float64  `json:"similarity"`
}

// PropertyType is a property type of the workspace a mapping may
// target with use_existing.
type PropertyType struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	InputType string `json:"inputType"`
	Unit      string `json:"unit"`
}

// RowError is one failing row of a validation or a run.
type RowError struct {
	Row     int    `json:"row"`
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ErrorGroup is one cause of failure with its count.
type ErrorGroup struct {
	Key     string `json:"key"`
	Message string `json:"message"`
	Count   int    `json:"count"`
}

// Mapping is one action for one column, as ImportMappingsInput
// takes it.
type Mapping struct {
	Action        string `json:"action"`
	CoreAttribute string `json:"coreAttribute,omitempty"`
	TypeID        string `json:"typeId,omitempty"`
	TypeName      string `json:"typeName,omitempty"`
	IsSingleValue *bool  `json:"isSingleValue,omitempty"`
}

// MappingOptions are the options a run reads. Backup left out
// means the platform takes one, which a revert restores from;
// false runs without it.
type MappingOptions struct {
	Published *bool `json:"published,omitempty"`
	Backup    *bool `json:"backup,omitempty"`
}

// Unresolved is one column the flow could not map on its own.
type Unresolved struct {
	Column       string   `json:"column"`
	Header       string   `json:"header"`
	MatchType    string   `json:"matchType"`
	Suggestion   *Mapping `json:"suggestion,omitempty"`
	SampleValues []string `json:"sampleValues,omitempty"`
}

// ErrMappingRequired is returned when columns stay unresolved;
// the command line answers it with exit code 5.
type ErrMappingRequired struct {
	Import     *Import
	Unresolved []Unresolved
}

func (e *ErrMappingRequired) Error() string {
	names := make([]string, len(e.Unresolved))
	for i, u := range e.Unresolved {
		names[i] = u.Header
	}
	return fmt.Sprintf("%d columns need a mapping: %s", len(e.Unresolved),
		strings.Join(names, ", "))
}

// ErrValidationFailed is returned when the dry run found row
// errors; the command line answers it with exit code 3.
type ErrValidationFailed struct {
	Import *Import
}

func (e *ErrValidationFailed) Error() string {
	return fmt.Sprintf("validation found %d failing rows",
		len(e.Import.RowErrors))
}

// Upload sends a file as a new import and answers its state.
func Upload(ctx context.Context, c *transpareo.Client, path, dataType string,
	valueSeparator string) (*Import, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return upload(ctx, c, filepath.Base(path), content, dataType,
		valueSeparator)
}

// UploadRows sends rows a caller already read as a new import,
// one object per row keyed by the column headers. The extension
// picks the reader on the platform's side, so they travel as a
// JSON file.
func UploadRows(ctx context.Context, c *transpareo.Client,
	rows []map[string]any, dataType, valueSeparator string) (*Import, error) {
	content, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	return upload(ctx, c, "rows.json", content, dataType, valueSeparator)
}

// upload posts the bytes as the multipart file every import
// starts from.
func upload(ctx context.Context, c *transpareo.Client, name string,
	content []byte, dataType, valueSeparator string) (*Import, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		return nil, err
	}
	if dataType != "" {
		writer.WriteField("dataType", dataType)
	}
	if valueSeparator != "" {
		writer.WriteField("valueSeparator", valueSeparator)
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	resp, err := c.Do(ctx, &transpareo.Request{Method: http.MethodPost,
		Path: "/imports", Body: buf.Bytes(),
		ContentType: writer.FormDataContentType()})
	if err != nil {
		return nil, err
	}
	return decodeImport(resp.Body)
}

// Get reads an import.
func Get(ctx context.Context, c *transpareo.Client, id string) (*Import,
	error) {
	resp, err := c.Do(ctx, &transpareo.Request{Method: http.MethodGet,
		Path: "/imports/" + id})
	if err != nil {
		return nil, err
	}
	return decodeImport(resp.Body)
}

func decodeImport(body []byte) (*Import, error) {
	var imp Import
	if err := json.Unmarshal(body, &imp); err != nil {
		return nil, &transpareo.Error{Code: transpareo.CodeInvalidResponse,
			Message: "the import answer is not the JSON the client expected: " +
				err.Error()}
	}
	imp.Body = json.RawMessage(body)
	return &imp, nil
}

// ParseMapSpec turns "Column=target" into a mapping: a core
// attribute name, "property:<type name>", "new:<type name>" or
// "skip".
func ParseMapSpec(spec string) (column string, m Mapping, err error) {
	column, target, ok := strings.Cut(spec, "=")
	column = strings.TrimSpace(column)
	target = strings.TrimSpace(target)
	if !ok || column == "" || target == "" {
		return "", Mapping{}, fmt.Errorf("%q is not Column=target", spec)
	}
	switch {
	case target == "skip":
		return column, Mapping{Action: "skip"}, nil
	case strings.HasPrefix(target, "property:"):
		return column, Mapping{Action: "use_existing",
			TypeName: strings.TrimPrefix(target, "property:")}, nil
	case strings.HasPrefix(target, "new:"):
		return column, Mapping{Action: "create_new",
			TypeName: strings.TrimPrefix(target, "new:")}, nil
	default:
		return column, Mapping{Action: "map_to_attribute",
			CoreAttribute: target}, nil
	}
}

// ResolveMappings completes the mappings for a fresh import: the
// explicit ones, then the preview's own suggestions when
// acceptSuggestions is set and the match is exact or an
// attribute. It never creates a property type on its own. The
// columns left over come back as ErrMappingRequired.
func ResolveMappings(imp *Import, explicit map[string]Mapping,
	acceptSuggestions bool) (map[string]Mapping, error) {
	if imp.Preview == nil {
		return explicit, nil
	}
	byHeader := map[string]string{}
	for _, col := range imp.Preview.Columns {
		byHeader[strings.ToLower(col.Header)] = col.Column
		byHeader[strings.ToLower(col.Column)] = col.Column
	}
	resolved := map[string]Mapping{}
	for name, m := range explicit {
		key, ok := byHeader[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("the upload has no column %q", name)
		}
		if m.Action == "use_existing" && m.TypeID == "" {
			id := typeIDByName(imp.Preview, m.TypeName)
			if id == "" {
				return nil, fmt.Errorf("no property type named %q", m.TypeName)
			}
			m.TypeID = id
			m.TypeName = ""
		}
		resolved[key] = m
	}
	var unresolved []Unresolved
	for _, col := range imp.Preview.Columns {
		if _, ok := resolved[col.Column]; ok {
			continue
		}
		suggestion := suggestionOf(col)
		if acceptSuggestions && suggestion != nil &&
			(col.MatchType == "exact" || col.MatchType == "attribute") {
			resolved[col.Column] = *suggestion
			continue
		}
		unresolved = append(unresolved, Unresolved{Column: col.Column,
			Header: col.Header, MatchType: col.MatchType,
			Suggestion:   suggestion,
			SampleValues: col.SampleValues})
	}
	if len(unresolved) > 0 {
		return resolved, &ErrMappingRequired{Import: imp,
			Unresolved: unresolved}
	}
	return resolved, nil
}

// suggestionOf renders the preview's suggestion as a mapping, or
// nil when it suggests creating a type, which the flow never does
// on its own.
func suggestionOf(col Column) *Mapping {
	switch col.SuggestedAction {
	case "map_to_attribute":
		return &Mapping{Action: "map_to_attribute",
			CoreAttribute: col.CoreAttribute}
	case "use_existing":
		return &Mapping{Action: "use_existing", TypeID: col.TypeID}
	case "skip":
		return &Mapping{Action: "skip"}
	case "create_new":
		return &Mapping{Action: "create_new", TypeName: col.TypeName}
	default:
		return nil
	}
}

func typeIDByName(preview *Preview, name string) string {
	for _, pt := range preview.PropertyTypes {
		if strings.EqualFold(pt.Name, name) {
			return pt.ID
		}
	}
	return ""
}

// SendMappings stores the mappings and options on the import.
func SendMappings(ctx context.Context, c *transpareo.Client, id string,
	mappings map[string]Mapping, options *MappingOptions) (*Import, error) {
	body := map[string]any{"mappings": mappings}
	if options != nil {
		body["options"] = options
	}
	resp, err := c.Do(ctx, &transpareo.Request{Method: http.MethodPut,
		Path: "/imports/" + id + "/mappings", Body: body})
	if err != nil {
		return nil, err
	}
	return decodeImport(resp.Body)
}

// Validate runs the dry run and waits for it. Row errors come
// back as ErrValidationFailed with the import attached.
func Validate(ctx context.Context, c *transpareo.Client, id string,
	progress func(*transpareo.Task)) (*Import, error) {
	imp, err := runStep(ctx, c, id, "validate", nil, progress)
	if err != nil {
		return nil, err
	}
	if len(imp.RowErrors) > 0 || imp.FailedCount > 0 {
		return imp, &ErrValidationFailed{Import: imp}
	}
	return imp, nil
}

// Execute writes the rows and waits for the run.
func Execute(ctx context.Context, c *transpareo.Client, id string,
	options *MappingOptions, progress func(*transpareo.Task)) (*Import, error) {
	var body any
	if options != nil {
		body = map[string]any{"options": options}
	}
	imp, err := runStep(ctx, c, id, "execute", body, progress)
	if err != nil {
		return nil, err
	}
	if imp.Status == "failed" {
		return imp, &ErrValidationFailed{Import: imp}
	}
	return imp, nil
}

func runStep(ctx context.Context, c *transpareo.Client, id, step string,
	body any,
	progress func(*transpareo.Task)) (*Import, error) {
	resp, err := c.Do(ctx, &transpareo.Request{Method: http.MethodPost,
		Path: "/imports/" + id + "/" + step, Body: body})
	if err != nil {
		return nil, err
	}
	started, err := decodeImport(resp.Body)
	if err != nil {
		return nil, err
	}
	statusURL := started.StatusURL
	if statusURL == "" {
		statusURL = "/imports/" + id
	}
	task, err := c.WaitForTask(ctx, statusURL,
		&transpareo.WaitOptions{OnPoll: progress})
	if err != nil {
		return nil, err
	}
	return decodeImport(task.Body)
}

// RunOptions drive Run.
type RunOptions struct {
	// Path is a file on this machine, Rows a sheet the caller
	// already read. Rows wins when both are given.
	Path              string
	Rows              []map[string]any
	DataType          string
	ValueSeparator    string
	Mappings          map[string]Mapping
	AcceptSuggestions bool
	Execute           bool
	Options           *MappingOptions
	Progress          func(*transpareo.Task)
}

// upload sends whichever sheet the caller gave.
func (o RunOptions) upload(ctx context.Context,
	c *transpareo.Client) (*Import, error) {
	if len(o.Rows) > 0 {
		return UploadRows(ctx, c, o.Rows, o.DataType, o.ValueSeparator)
	}
	return Upload(ctx, c, o.Path, o.DataType, o.ValueSeparator)
}

// Run uploads, maps, validates and, when asked and clean,
// executes. It stops with ErrMappingRequired when columns stay
// unresolved and with ErrValidationFailed when the dry run found
// problems; the import is attached to both.
func Run(ctx context.Context, c *transpareo.Client, opts RunOptions) (*Import,
	error) {
	imp, err := opts.upload(ctx, c)
	if err != nil {
		return nil, err
	}
	id := imp.ID.String()
	if imp.Status == "fresh" {
		mappings, err := ResolveMappings(imp, opts.Mappings,
			opts.AcceptSuggestions)
		if err != nil {
			return imp, err
		}
		if _, err := SendMappings(ctx, c, id, mappings, opts.Options); err != nil {
			return nil, err
		}
	}
	imp, err = Validate(ctx, c, id, opts.Progress)
	if err != nil {
		return imp, err
	}
	if !opts.Execute {
		return imp, nil
	}
	return Execute(ctx, c, id, opts.Options, opts.Progress)
}

// SortedKeys returns the mapping columns in order, for stable
// output.
func SortedKeys(m map[string]Mapping) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// IsMappingRequired reports whether err is ErrMappingRequired.
func IsMappingRequired(err error) bool {
	var e *ErrMappingRequired
	return errors.As(err, &e)
}
