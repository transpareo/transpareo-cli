package transpareo

import (
	"context"
	"time"
)

// Identity is the consumer behind a bearer token and what that
// token allows, as GET /me answers it.
type Identity struct {
	Name               string              `json:"name"`
	Key                string              `json:"key"`
	Status             string              `json:"status"`
	Authority          bool                `json:"authority"`
	Permissions        []string            `json:"permissions"`
	Scope              []string            `json:"scope"`
	ResourceScope      map[string][]string `json:"resourceScope"`
	ExpiresAt          *time.Time          `json:"expiresAt"`
	TokenExpiresAt     time.Time           `json:"tokenExpiresAt"`
	RateLimit          IdentityRateLimit   `json:"rateLimit"`
	BulkItemsPerMinute int                 `json:"bulkItemsPerMinute"`
}

// IdentityRateLimit is the hourly allowance of a consumer and
// how much of it the current window has used.
type IdentityRateLimit struct {
	Limit           int        `json:"limit"`
	Used            int        `json:"used"`
	WindowStartedAt *time.Time `json:"windowStartedAt"`
}

// Me answers what the presented token allows. It is the cheapest
// way to check that a credential works.
func (c *Client) Me(ctx context.Context) (*Identity, error) {
	var id Identity
	if _, err := c.Get(ctx, "/me", nil, &id); err != nil {
		return nil, err
	}
	return &id, nil
}

// GrantInput names the passport a partner grant is scoped to:
// either the scanned code, or GS1 identifiers.
type GrantInput struct {
	DppCode string `json:"dppCode,omitempty"`
	GTIN    string `json:"gtin,omitempty"`
	Batch   string `json:"batch,omitempty"`
	Serial  string `json:"serial,omitempty"`
}

// Grant is the client credentials of the short-lived child
// consumer a grant issued. The secret is shown once, here.
type Grant struct {
	Key       string    `json:"key"`
	Secret    string    `json:"secret"`
	ExpiresAt time.Time `json:"expiresAt"`
	Scope     struct {
		Code string `json:"code"`
	} `json:"scope"`
}

// CreateGrant turns a scanned passport code into credentials
// that can write events to that one passport for a day.
func (c *Client) CreateGrant(ctx context.Context,
	in GrantInput) (*Grant, error) {
	var grant Grant
	if _, err := c.Post(ctx, "/grant", in, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}
