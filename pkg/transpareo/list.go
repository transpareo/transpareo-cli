package transpareo

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

// Page is one page of a list. Total and PerPage come from the
// API-Total and API-Per-Page headers, NextURL from the Link
// header while a further page exists. Body is the whole answer,
// for the fields some lists carry beside their items, such as
// the nextCursor of the event feed.
type Page[T any] struct {
	Items   []T
	Total   int
	Count   int
	Page    int
	PerPage int
	NextURL string
	Body    json.RawMessage
}

// NextPage returns the number of the following page, or zero on
// the last one.
func (p *Page[T]) NextPage() int {
	if p.NextURL == "" {
		return 0
	}
	return p.Page + 1
}

// List fetches one page of a list endpoint. Every list answers
// an object with the items under one key ({"dpps": [...]}); a
// bare array and an empty body are accepted too.
func List[T any](ctx context.Context, c *Client, path string,
	query url.Values) (*Page[T], error) {
	resp, err := c.Do(ctx, &Request{Method: http.MethodGet, Path: path,
		Query: query})
	if err != nil {
		return nil, err
	}
	items, err := unwrapItems[T](resp)
	if err != nil {
		return nil, err
	}
	page := pageFrom(resp, items)
	page.Body = json.RawMessage(resp.Body)
	return page, nil
}

// unwrapItems finds the items of a list answer: the body itself
// when it is an array, else the first array-valued key of the
// object, in the order the platform wrote them.
func unwrapItems[T any](resp *Response) ([]T, error) {
	body := bytes.TrimSpace(resp.Body)
	if len(body) == 0 || string(body) == "null" {
		return nil, nil
	}
	var items []T
	if body[0] == '[' {
		return items, resp.Decode(&items)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, invalidList(resp, err)
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, invalidList(resp, err)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, invalidList(resp, err)
		}
		if _, ok := key.(string); ok && len(value) > 0 && value[0] == '[' {
			if err := json.Unmarshal(value, &items); err != nil {
				return nil, invalidList(resp, err)
			}
			return items, nil
		}
	}
	return nil, invalidList(resp, nil)
}

func invalidList(resp *Response, cause error) *Error {
	message := "the response carries no list"
	if cause != nil {
		message = "the response is not the list the client expected: " +
			cause.Error()
	}
	return &Error{Code: CodeInvalidResponse, Status: resp.StatusCode,
		Message: message, cause: cause}
}

// ListAll iterates over every item of a list endpoint, following
// the Link header from page to page. The iteration stops at the
// first error, which is yielded with a zero item.
func ListAll[T any](ctx context.Context, c *Client, path string,
	query url.Values) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		next := path
		for next != "" {
			page, err := List[T](ctx, c, next, query)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range page.Items {
				if !yield(item, nil) {
					return
				}
			}
			next = page.NextURL
			query = nil
		}
	}
}

func pageFrom[T any](resp *Response, items []T) *Page[T] {
	header := func(name string) int {
		n, _ := strconv.Atoi(resp.Header.Get(name))
		return n
	}
	return &Page[T]{
		Items:   items,
		Total:   header("API-Total"),
		Count:   header("API-Count"),
		Page:    header("API-Page"),
		PerPage: header("API-Per-Page"),
		NextURL: nextLink(resp.Header),
	}
}

var linkNext = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="?next"?`)

// nextLink extracts the URL with rel="next" from a Link header.
func nextLink(h http.Header) string {
	for _, value := range h.Values("Link") {
		if m := linkNext.FindStringSubmatch(value); m != nil {
			return m[1]
		}
	}
	return ""
}
