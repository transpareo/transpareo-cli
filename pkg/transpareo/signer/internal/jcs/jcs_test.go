package jcs

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestMatchesThePlatformSerialiser compares the output with what
// the platform's own serialiser answered for the same inputs;
// testdata/platform_vectors.json was generated with it.
func TestMatchesThePlatformSerialiser(t *testing.T) {
	data, err := os.ReadFile("testdata/platform_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Input     json.RawMessage `json:"input"`
		Canonical string          `json:"canonical"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) < 7 {
		t.Fatalf("only %d vectors", len(vectors))
	}
	for i, v := range vectors {
		got, err := Canonical(v.Input)
		if err != nil {
			t.Errorf("vector %d: %v", i, err)
			continue
		}
		if string(got) != v.Canonical {
			t.Errorf("vector %d:\n got %s\nwant %s", i, got, v.Canonical)
		}
	}
}

// TestNumbersRenderAsECMAScript covers the number forms RFC 8785
// lists, plus the ones Go renders differently on its own.
func TestNumbersRenderAsECMAScript(t *testing.T) {
	cases := map[float64]string{
		0: "0", 1: "1", -1: "-1", 100: "100", 1.5: "1.5", 0.1: "0.1",
		1e21: "1e+21", 1e20: "100000000000000000000", 1e-6: "0.000001",
		1e-7: "1e-7", 333333333.33333329: "333333333.3333333",
		5e-324: "5e-324", 1.7976931348623157e308: "1.7976931348623157e+308",
		123456789012345680000: "123456789012345680000",
		9007199254740992:      "9007199254740992", 2.5e-5: "0.000025",
		1e6: "1000000", -0.002: "-0.002", 1.25e22: "1.25e+22",
	}
	for f, want := range cases {
		got, err := Marshal(f)
		if err != nil || string(got) != want {
			t.Errorf("%v: got %s (%v), want %s", f, got, err, want)
		}
	}
	got, err := Marshal(json.Number("4.50"))
	if err != nil || string(got) != "4.5" {
		t.Errorf("json.Number: %s %v", got, err)
	}
	if _, err := Marshal(map[string]any{"x": make(chan int)}); err == nil {
		t.Error("an unsupported type must be refused")
	}
}

func TestKeysSortByUTF16CodeUnits(t *testing.T) {
	got, err := Marshal(map[string]any{
		"\U0001F602": 1, "\uFB33": 2, "a": 3, "\u20AC": 4, "B": 5})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"B\":5,\"a\":3,\"\u20AC\":4,\"\U0001F602\":1,\"\uFB33\":2}"
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestTrailingDataIsRefused(t *testing.T) {
	if _, err := Canonical([]byte(`{} {}`)); err == nil ||
		!strings.Contains(err.Error(), "trailing") {
		t.Errorf("err = %v", err)
	}
}
