package flows

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

// Feed is one page of the event feed.
type Feed struct {
	Events     []json.RawMessage `json:"events"`
	NextCursor string            `json:"nextCursor"`
}

// FeedOptions select what the feed answers.
type FeedOptions struct {
	Since   string
	Types   string
	DppCode string
	Limit   int
}

func (o FeedOptions) query() url.Values {
	q := url.Values{}
	if o.Since != "" {
		q.Set("since", o.Since)
	}
	if o.Types != "" {
		q.Set("types", o.Types)
	}
	if o.DppCode != "" {
		q.Set("dppCode", o.DppCode)
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	return q
}

// ReadFeed fetches one page of the feed.
func ReadFeed(ctx context.Context, c *transpareo.Client,
	opts FeedOptions) (*Feed, error) {
	resp, err := c.Do(ctx, &transpareo.Request{Method: http.MethodGet,
		Path: "/events", Query: opts.query()})
	if err != nil {
		return nil, err
	}
	var feed Feed
	if err := json.Unmarshal(resp.Body, &feed); err != nil {
		return nil, &transpareo.Error{Code: transpareo.CodeInvalidResponse,
			Message: "the feed answer is not the JSON the client expected: " +
				err.Error()}
	}
	return &feed, nil
}

// Follow polls the feed until ctx ends, handing every page to
// each and waiting interval between empty answers. The cursor
// advances with every answer that carries one.
func Follow(ctx context.Context, c *transpareo.Client, opts FeedOptions,
	interval time.Duration, sleep func(context.Context, time.Duration) error,
	each func(*Feed) error) error {
	if sleep == nil {
		sleep = sleepContext
	}
	for {
		feed, err := ReadFeed(ctx, c, opts)
		if err != nil {
			return err
		}
		if err := each(feed); err != nil {
			return err
		}
		if feed.NextCursor != "" {
			opts.Since = feed.NextCursor
		}
		if err := sleep(ctx, interval); err != nil {
			return err
		}
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
