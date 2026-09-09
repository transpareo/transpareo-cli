package transpareo

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestMe(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("GET /api/me", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"name": "ERP integration", "key": "3f6a", "status": "active",
			"permissions":    []string{"product_access", "dpp_read"},
			"scope":          []string{"dpp_read"},
			"resourceScope":  map[string]any{},
			"expiresAt":      nil,
			"tokenExpiresAt": "2026-09-08T15:04:05Z",
			"rateLimit": map[string]any{"limit": 1000, "used": 12,
				"windowStartedAt": "2026-09-08T14:10:00Z"},
			"bulkItemsPerMinute": 5000,
		})
	})
	c := newTestClient(t, ts, newFakeClock())
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.Name != "ERP integration" || me.Status != "active" ||
		len(me.Permissions) != 2 {
		t.Errorf("me = %+v", me)
	}
	if me.ExpiresAt != nil || me.TokenExpiresAt.Year() != 2026 {
		t.Errorf("expiry = %v %v", me.ExpiresAt, me.TokenExpiresAt)
	}
	if me.RateLimit.Limit != 1000 || me.RateLimit.Used != 12 ||
		me.BulkItemsPerMinute != 5000 {
		t.Errorf("limits = %+v %d", me.RateLimit, me.BulkItemsPerMinute)
	}
}

func TestCreateGrant(t *testing.T) {
	ts := newTokenServer(t)
	var body map[string]any
	ts.Mux.HandleFunc("POST /api/grant", func(w http.ResponseWriter,
		r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		writeJSON(w, 201, map[string]any{"key": "child", "secret": "shh",
			"expiresAt": "2026-09-10T12:00:00Z",
			"scope":     map[string]string{"code": "A1B2"}})
	})
	c := newTestClient(t, ts, newFakeClock())
	grant, err := c.CreateGrant(context.Background(),
		GrantInput{DppCode: "A1B2"})
	if err != nil {
		t.Fatal(err)
	}
	if body["dppCode"] != "A1B2" || len(body) != 1 {
		t.Errorf("body = %v", body)
	}
	if grant.Key != "child" || grant.Secret != "shh" ||
		grant.Scope.Code != "A1B2" {
		t.Errorf("grant = %+v", grant)
	}
}
