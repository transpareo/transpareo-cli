package output

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// summaryKeys are the columns a list shows on a terminal when
// no --fields narrows it, in this order, each when present.
var summaryKeys = []string{"id", "code", "identifier", "name", "title",
	"status", "granularity", "dataType", "url", "updatedAt", "createdAt"}

// maxCell bounds a table cell so one long value does not push
// the rest off screen.
const maxCell = 48

func (p *Printer) printTable(items []any) error {
	if len(items) == 0 {
		_, err := fmt.Fprintln(p.Err, "No records.")
		return err
	}
	columns := p.columns(items)
	if columns == nil {
		for _, item := range items {
			if _, err := fmt.Fprintln(p.Out, scalar(item)); err != nil {
				return err
			}
		}
		return nil
	}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		obj, _ := item.(map[string]any)
		row := make([]string, len(columns))
		for i, column := range columns {
			value, _ := lookup(obj, column)
			row[i] = clip(scalar(value), maxCell)
		}
		rows = append(rows, row)
	}
	widths := make([]int, len(columns))
	for i, column := range columns {
		widths[i] = utf8.RuneCountInString(column)
	}
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}
	if err := p.writeRow(columns, widths); err != nil {
		return err
	}
	for _, row := range rows {
		if err := p.writeRow(row, widths); err != nil {
			return err
		}
	}
	return nil
}

// columns picks the table columns: the requested fields, else
// the summary keys the items carry, else every key of the first
// item. Items that are not objects get no columns.
func (p *Printer) columns(items []any) []string {
	first, ok := items[0].(map[string]any)
	if !ok {
		return nil
	}
	if len(p.Fields) > 0 {
		return p.Fields
	}
	present := map[string]bool{}
	for _, item := range items {
		obj, _ := item.(map[string]any)
		for key := range obj {
			present[key] = true
		}
	}
	var columns []string
	for _, key := range summaryKeys {
		if present[key] {
			columns = append(columns, key)
		}
	}
	if len(columns) > 0 {
		return columns
	}
	for key := range first {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	return columns
}

func (p *Printer) writeRow(cells []string, widths []int) error {
	var b strings.Builder
	for i, cell := range cells {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(cell)
		if i < len(cells)-1 {
			b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)))
		}
	}
	_, err := fmt.Fprintln(p.Out, strings.TrimRight(b.String(), " "))
	return err
}

func (p *Printer) printObject(obj map[string]any) error {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	width := 0
	for _, key := range keys {
		width = max(width, utf8.RuneCountInString(key))
	}
	for _, key := range keys {
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(key))
		_, err := fmt.Fprintf(p.Out, "%s:%s  %s\n", key, pad, scalar(obj[key]))
		if err != nil {
			return err
		}
	}
	return nil
}

// scalar renders a value in one line: strings as they are,
// numbers without an exponent, nested values as compact JSON.
func scalar(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case json.Number:
		return value.String()
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n-3]) + "..."
}
