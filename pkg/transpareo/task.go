package transpareo

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// Task is the polled document of background work: a bulk create,
// an export or an import. Body is the whole document, so the
// caller can read the fields of the kind of task it started.
type Task struct {
	Status   string
	Progress int
	URL      string
	Body     json.RawMessage
}

// runningStatuses are the states in which the work is still in
// progress. Every other status is final for the purpose of
// waiting: completed, failed, cancelled, validated, reverted,
// and the resting states of an import.
var runningStatuses = map[string]bool{
	"pending":    true,
	"running":    true,
	"validating": true,
	"importing":  true,
	"restoring":  true,
}

// Done reports whether the task has stopped running.
func (t *Task) Done() bool {
	return !runningStatuses[t.Status]
}

// Failed reports whether the task ended without doing its work.
func (t *Task) Failed() bool {
	return t.Status == "failed" || t.Status == "cancelled"
}

// WaitOptions tunes WaitForTask. Zero values take the defaults:
// a first wait of one second, doubling up to thirty seconds.
type WaitOptions struct {
	Interval    time.Duration
	MaxInterval time.Duration

	// OnPoll is called after every poll with the current state,
	// for progress output.
	OnPoll func(*Task)
}

// WaitForTask polls the status URL that a bulk create, export or
// import answered until the task stops running or ctx ends. A
// task that ended in failure is returned without an error; check
// Failed.
func (c *Client) WaitForTask(ctx context.Context, statusURL string,
	opts *WaitOptions) (*Task, error) {
	if opts == nil {
		opts = &WaitOptions{}
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = time.Second
	}
	maxInterval := opts.MaxInterval
	if maxInterval <= 0 {
		maxInterval = 30 * time.Second
	}
	for {
		task, err := c.GetTask(ctx, statusURL)
		if err != nil {
			return nil, err
		}
		if opts.OnPoll != nil {
			opts.OnPoll(task)
		}
		if task.Done() {
			return task, nil
		}
		if err := c.sleep(ctx, interval); err != nil {
			return task, err
		}
		interval = min(interval*2, maxInterval)
	}
}

// GetTask fetches the status document once. The URL may be
// absolute, as the API hands it out, or relative to the API
// root.
func (c *Client) GetTask(ctx context.Context, statusURL string) (*Task, error) {
	var doc struct {
		Status   string `json:"status"`
		Progress int    `json:"progress"`
	}
	resp, err := c.Get(ctx, statusURL, url.Values{"reload": {"1"}}, &doc)
	if err != nil {
		return nil, err
	}
	return &Task{
		Status:   doc.Status,
		Progress: doc.Progress,
		URL:      statusURL,
		Body:     json.RawMessage(resp.Body),
	}, nil
}
