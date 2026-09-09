package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

func newPrinter(terminal bool, opts Options) (*Printer, *bytes.Buffer,
	*bytes.Buffer) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	return &Printer{Out: out, Err: errOut, Terminal: terminal, Options: opts},
		out, errOut
}

var list = json.RawMessage(`[
  {"id": "1", "name": "Alpha", "status": "draft", "extra": {"a": 1}},
  {"id": "2", "name": "Beta", "status": "published", "extra": {"a": 2}}
]`)

func TestPrintJSONOffTerminal(t *testing.T) {
	p, out, _ := newPrinter(false, Options{})
	if err := p.Print(list); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got) != 2 {
		t.Fatalf("output = %s", out)
	}
	if !strings.HasPrefix(out.String(), "[\n  {") {
		t.Errorf("not pretty printed: %s", out)
	}
}

func TestPrintTableOnTerminal(t *testing.T) {
	p, out, _ := newPrinter(true, Options{})
	if err := p.Print(list); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	if lines[0] != "id  name   status" {
		t.Errorf("header = %q", lines[0])
	}
	if lines[2] != "2   Beta   published" {
		t.Errorf("row = %q", lines[2])
	}
}

func TestPrintObjectOnTerminal(t *testing.T) {
	p, out, _ := newPrinter(true, Options{})
	p.Print(map[string]any{"name": "ERP", "permissions": []string{"a", "b"},
		"n": 3.5})
	want := "n:            3.5\nname:         ERP\npermissions:  " +
		"[\"a\",\"b\"]\n"
	if out.String() != want {
		t.Errorf("output = %q", out.String())
	}
}

func TestPrintJSONLines(t *testing.T) {
	p, out, _ := newPrinter(true, Options{JSONL: true})
	p.Print(list)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 ||
		!strings.HasPrefix(lines[1], `{"extra":{"a":2},"id":"2"`) {
		t.Errorf("lines = %q", lines)
	}
}

func TestPrintQuietIDs(t *testing.T) {
	p, out, _ := newPrinter(true, Options{Quiet: true})
	p.Print(list)
	if out.String() != "1\n2\n" {
		t.Errorf("output = %q", out.String())
	}
	out.Reset()
	p.Print(map[string]any{"code": "ABC", "name": "x"})
	if out.String() != "ABC\n" {
		t.Errorf("output = %q", out.String())
	}
}

func TestFieldsProjection(t *testing.T) {
	p, out, _ := newPrinter(false, Options{Fields: []string{"name", "extra.a",
		"nope"}})
	p.Print(list)
	var got []map[string]any
	json.Unmarshal(out.Bytes(), &got)
	if len(got[0]) != 2 || got[0]["name"] != "Alpha" ||
		got[1]["extra.a"] != 2.0 {
		t.Errorf("projected = %v", got)
	}
	p, out, _ = newPrinter(true, Options{Fields: []string{"name", "extra.a"}})
	p.Print(list)
	if !strings.HasPrefix(out.String(), "name   extra.a\nAlpha  1\n") {
		t.Errorf("table = %q", out.String())
	}
}

func TestEmptyListOnTerminal(t *testing.T) {
	p, out, errOut := newPrinter(true, Options{})
	p.Print([]any{})
	if out.String() != "" || !strings.Contains(errOut.String(), "No records") {
		t.Errorf("out = %q err = %q", out, errOut)
	}
}

func TestScalarList(t *testing.T) {
	p, out, _ := newPrinter(true, Options{})
	p.Print([]string{"a", "b"})
	if out.String() != "a\nb\n" {
		t.Errorf("out = %q", out)
	}
}

func TestPrintRawAndNil(t *testing.T) {
	p, out, _ := newPrinter(true, Options{})
	p.PrintRaw([]byte("guide"))
	if out.String() != "guide\n" {
		t.Errorf("out = %q", out)
	}
	out.Reset()
	if err := p.Print(nil); err != nil || out.String() != "" {
		t.Errorf("nil: %q %v", out, err)
	}
	if err := p.Print(json.RawMessage("not json")); err == nil {
		t.Error("invalid JSON must be reported")
	}
}

func TestPrintErrorEnvelope(t *testing.T) {
	apiErr := &transpareo.Error{Code: "DPP_INVALID", Status: 422,
		Message: "Validation failed",
		Hint:    "Fix it.", DocsURL: "https://x/guide#w",
		Fields: map[string]transpareo.FieldError{
			"name":       {FullMessage: "Name is required"},
			"properties": {Missing: []string{"Weight"}},
		}}
	p, out, errOut := newPrinter(false, Options{})
	p.PrintError(apiErr)
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("envelope: %v: %s", err, out)
	}
	if env.OK || env.Error.Code != "DPP_INVALID" ||
		env.Error.Hint != "Fix it." ||
		env.Error.Status != 422 || env.Error.Retryable ||
		len(env.Error.Fields) != 2 {
		t.Errorf("envelope = %+v", env)
	}
	if errOut.Len() != 0 {
		t.Error("nothing on stderr in JSON mode")
	}

	p, out, errOut = newPrinter(true, Options{})
	p.PrintError(apiErr)
	want := "DPP_INVALID: Validation failed\nFix it.\n" +
		"  name: Name is required\n" +
		"  properties: missing Weight\n"
	if errOut.String() != want || out.Len() != 0 {
		t.Errorf("stderr = %q stdout = %q", errOut, out)
	}
}

func TestPrintErrorPlain(t *testing.T) {
	p, out, _ := newPrinter(false, Options{})
	p.PrintError(Exit(ExitRefused, errors.New("void needs --yes")))
	var env Envelope
	json.Unmarshal(out.Bytes(), &env)
	if env.Error.Code != "CONFIRMATION_REQUIRED" ||
		env.Error.Message != "void needs --yes" {
		t.Errorf("envelope = %+v", env)
	}
	p, out, _ = newPrinter(false, Options{})
	p.PrintError(errors.New("boom"))
	json.Unmarshal(out.Bytes(), &env)
	if env.Error.Code != "ERROR" || env.Error.Message != "boom" {
		t.Errorf("envelope = %+v", env)
	}
}

func TestExitCode(t *testing.T) {
	cases := map[int]error{
		ExitOK:         nil,
		ExitAPI:        &transpareo.Error{Code: "X"},
		ExitUsage:      errors.New("bad flag"),
		ExitValidation: Exit(ExitValidation, nil),
		ExitMapping:    Exit(ExitMapping, errors.New("map")),
	}
	for want, err := range cases {
		if got := ExitCode(err); got != want {
			t.Errorf("%v: code %d, want %d", err, got, want)
		}
	}
	wrapped := Exit(ExitRefused, &transpareo.Error{Code: "X"})
	if ExitCode(wrapped) != ExitRefused {
		t.Error("an explicit code wins over the API error inside")
	}
}

func TestClip(t *testing.T) {
	if clip("short", 10) != "short" || clip("abcdefghij", 6) != "abc..." {
		t.Errorf("clip = %q %q", clip("short", 10), clip("abcdefghij", 6))
	}
}
