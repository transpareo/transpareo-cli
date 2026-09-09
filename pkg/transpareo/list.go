package transpareo

import (
	"context"
	"iter"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

// Page is one page of a list. Total and PerPage come from the
// API-Total and API-Per-Page headers, NextURL from the Link
// header while a further page exists.
type Page[T any] struct {
	Items   []T
	Total   int
	Count   int
	Page    int
	PerPage int
	NextURL string
}

// NextPage returns the number of the following page, or zero on
// the last one.
func (p *Page[T]) NextPage() int {
	if p.NextURL == "" {
		return 0
	}
	return p.Page + 1
}

// List fetches one page of a list endpoint.
func List[T any](ctx context.Context, c *Client, path string,
	query url.Values) (*Page[T], error) {
	var items []T
	resp, err := c.Get(ctx, path, query, &items)
	if err != nil {
		return nil, err
	}
	return pageFrom(resp, items), nil
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
