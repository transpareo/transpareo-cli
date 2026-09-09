package transpareo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"testing"
)

type item struct {
	ID string `json:"id"`
}

func servePages(ts *tokenServer, total, perPage int) {
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter,
		r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		var items []item
		for i := (page-1)*perPage + 1; i <= min(page*perPage, total); i++ {
			items = append(items, item{ID: fmt.Sprint(i)})
		}
		w.Header().Set("API-Total", fmt.Sprint(total))
		w.Header().Set("API-Count", fmt.Sprint(len(items)))
		w.Header().Set("API-Page", fmt.Sprint(page))
		w.Header().Set("API-Per-Page", fmt.Sprint(perPage))
		if page*perPage < total {
			w.Header().Set("Link",
				fmt.Sprintf(`<%s/api/dpps?page=%d&per_page=%d>; rel="next"`,
					ts.URL, page+1, perPage))
		}
		writeJSON(w, 200, map[string]any{"dpps": items, "total": total})
	})
}

func TestListReadsPaginationHeaders(t *testing.T) {
	ts := newTokenServer(t)
	servePages(ts, 5, 2)
	c := newTestClient(t, ts, newFakeClock())
	page, err := List[item](context.Background(), c, "/dpps",
		url.Values{"per_page": {"2"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 5 || page.Count != 2 || page.Page != 1 ||
		page.PerPage != 2 {
		t.Errorf("page = %+v", page)
	}
	if page.NextPage() != 2 || page.NextURL == "" {
		t.Errorf("next = %d %q", page.NextPage(), page.NextURL)
	}
	if len(page.Items) != 2 || page.Items[1].ID != "2" {
		t.Errorf("items = %+v", page.Items)
	}
}

func TestListAllFollowsLinkHeader(t *testing.T) {
	ts := newTokenServer(t)
	servePages(ts, 5, 2)
	c := newTestClient(t, ts, newFakeClock())
	var ids []string
	for it, err := range ListAll[item](context.Background(), c, "/dpps",
		url.Values{"per_page": {"2"}}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, it.ID)
	}
	if fmt.Sprint(ids) != "[1 2 3 4 5]" {
		t.Errorf("ids = %v", ids)
	}
}

func TestListAllStopsEarly(t *testing.T) {
	ts := newTokenServer(t)
	servePages(ts, 5, 2)
	c := newTestClient(t, ts, newFakeClock())
	n := 0
	for _, err := range ListAll[item](context.Background(), c, "/dpps", nil) {
		if err != nil {
			t.Fatal(err)
		}
		n++
		if n == 3 {
			break
		}
	}
	if n != 3 {
		t.Errorf("n = %d", n)
	}
}

func TestListAllYieldsErrors(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 403, map[string]string{"error": "FORBIDDEN",
			"message": "no"})
	})
	c := newTestClient(t, ts, newFakeClock())
	var got error
	for _, err := range ListAll[item](context.Background(), c, "/dpps", nil) {
		got = err
	}
	if !IsCode(got, "FORBIDDEN") {
		t.Errorf("err = %v", got)
	}
}

func TestNextLink(t *testing.T) {
	h := http.Header{}
	if nextLink(h) != "" {
		t.Error("no header, no link")
	}
	h.Set("Link", `<https://x/api/dpps?page=2>; rel="next"`)
	if nextLink(h) != "https://x/api/dpps?page=2" {
		t.Errorf("got %q", nextLink(h))
	}
	h.Set("Link", `<https://x/a>; rel="prev", <https://x/b>; rel=next`)
	if nextLink(h) != "https://x/b" {
		t.Errorf("got %q", nextLink(h))
	}
}

func TestListAcceptsBareArraysAndEmptyBodies(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("/api/bare", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, []item{{ID: "a"}})
	})
	ts.Mux.HandleFunc("/api/empty", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	ts.Mux.HandleFunc("/api/nolist", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"total": 3})
	})
	ts.Mux.HandleFunc("/api/scalar", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, "x")
	})
	c := newTestClient(t, ts, newFakeClock())
	ctx := context.Background()
	page, err := List[item](ctx, c, "/bare", nil)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "a" {
		t.Errorf("bare: %+v %v", page, err)
	}
	page, err = List[item](ctx, c, "/empty", nil)
	if err != nil || len(page.Items) != 0 {
		t.Errorf("empty: %+v %v", page, err)
	}
	_, err = List[item](ctx, c, "/nolist", nil)
	if !IsCode(err, CodeInvalidResponse) {
		t.Errorf("no list: %v", err)
	}
	_, err = List[item](ctx, c, "/scalar", nil)
	if !IsCode(err, CodeInvalidResponse) {
		t.Errorf("scalar: %v", err)
	}
}

func TestListKeepsTheWholeBody(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"events": []item{{ID: "e1"}},
			"nextCursor": "abc"})
	})
	c := newTestClient(t, ts, newFakeClock())
	page, err := List[item](context.Background(), c, "/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	var extra struct {
		NextCursor string `json:"nextCursor"`
	}
	json.Unmarshal(page.Body, &extra)
	if extra.NextCursor != "abc" || page.Items[0].ID != "e1" {
		t.Errorf("page = %+v extra %+v", page, extra)
	}
}
