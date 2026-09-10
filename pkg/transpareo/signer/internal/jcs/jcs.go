// Package jcs serialises a JSON value in the canonical form of
// RFC 8785, so that SHA-256 over the output is a stable hash
// for signing. It mirrors the platform's serialiser: object
// keys ordered by their UTF-16 code units, numbers rendered as
// ECMAScript renders them, and only the characters below 0x20
// plus the quote and the backslash escaped.
package jcs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Marshal answers the canonical bytes of v, which is a value as
// encoding/json decodes it: nil, bool, float64, json.Number,
// string, []any or map[string]any. Go's integer and float
// types are accepted too.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := encode(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Canonical decodes JSON text and answers it in canonical form,
// for a document that arrives as bytes.
func Canonical(data []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("jcs: trailing data after the document")
	}
	return Marshal(v)
}

func encode(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		buf.WriteString(strconv.FormatBool(x))
	case string:
		encodeString(buf, x)
	case float64:
		return encodeNumber(buf, x)
	case float32:
		return encodeNumber(buf, float64(x))
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return fmt.Errorf("jcs: number %q: %w", x, err)
		}
		return encodeNumber(buf, f)
	case int:
		return encodeNumber(buf, float64(x))
	case int64:
		return encodeNumber(buf, float64(x))
	case []any:
		buf.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encode(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		return encodeObject(buf, x)
	default:
		return fmt.Errorf("jcs: unsupported value of type %T", v)
	}
	return nil
}

func encodeObject(buf *bytes.Buffer, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return lessUTF16(keys[i], keys[j]) })
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		encodeString(buf, k)
		buf.WriteByte(':')
		if err := encode(buf, m[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

// lessUTF16 orders keys by their UTF-16 code units, as RFC 8785
// asks, which differs from byte order for characters outside
// the basic multilingual plane.
func lessUTF16(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

func encodeString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\b':
			buf.WriteString(`\b`)
		case '\t':
			buf.WriteString(`\t`)
		case '\n':
			buf.WriteString(`\n`)
		case '\f':
			buf.WriteString(`\f`)
		case '\r':
			buf.WriteString(`\r`)
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\u%04x`, r)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}

// encodeNumber renders f the way ECMAScript's Number::toString
// does: the shortest digits that round-trip, positional for
// exponents between -6 and 21, exponential outside.
func encodeNumber(buf *bytes.Buffer, f float64) error {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return fmt.Errorf("jcs: non-finite number %v", f)
	}
	if f == 0 {
		buf.WriteByte('0')
		return nil
	}
	if f < 0 {
		buf.WriteByte('-')
		f = -f
	}
	// Go's shortest exponential form is "d.ddde±xx"; the
	// significant digits and the decimal exponent come from it.
	mantissa, exp, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	digits := strings.TrimRight(strings.Replace(mantissa, ".", "", 1), "0")
	e, _ := strconv.Atoi(exp)
	n := e + 1
	k := len(digits)
	switch {
	case k <= n && n <= 21:
		buf.WriteString(digits + strings.Repeat("0", n-k))
	case 0 < n && n <= 21:
		buf.WriteString(digits[:n] + "." + digits[n:])
	case -6 < n && n <= 0:
		buf.WriteString("0." + strings.Repeat("0", -n) + digits)
	default:
		if k == 1 {
			buf.WriteString(digits)
		} else {
			buf.WriteString(digits[:1] + "." + digits[1:])
		}
		buf.WriteByte('e')
		if n-1 < 0 {
			buf.WriteByte('-')
		} else {
			buf.WriteByte('+')
		}
		buf.WriteString(strconv.Itoa(abs(n - 1)))
	}
	return nil
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}
