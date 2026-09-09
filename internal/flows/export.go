package flows

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

// Export is the state of one export as the API answers it.
type Export struct {
	ID          json.Number `json:"id"`
	Status      string      `json:"status"`
	StatusURL   string      `json:"statusUrl"`
	DownloadURL string      `json:"downloadUrl"`
	Filename    string      `json:"filename"`
	Failure     string      `json:"failure"`
	Body        json.RawMessage
}

// ExportOptions drive StartExport.
type ExportOptions struct {
	Format       string
	Normalize    *bool
	IncludeMedia *bool
	Wait         bool
	Progress     func(*transpareo.Task)
}

// StartExport starts an export and, when asked, waits for it.
func StartExport(ctx context.Context, c *transpareo.Client,
	opts ExportOptions) (*Export, error) {
	input := map[string]any{}
	if opts.Format != "" {
		input["format"] = opts.Format
	}
	if opts.Normalize != nil {
		input["normalize"] = *opts.Normalize
	}
	if opts.IncludeMedia != nil {
		input["includeMedia"] = *opts.IncludeMedia
	}
	resp, err := c.Do(ctx, &transpareo.Request{Method: http.MethodPost,
		Path: "/exports", Body: map[string]any{"export": input}})
	if err != nil {
		return nil, err
	}
	exp, err := decodeExport(resp.Body)
	if err != nil || !opts.Wait {
		return exp, err
	}
	return WaitExport(ctx, c, exp, opts.Progress)
}

// WaitExport polls until the export is done.
func WaitExport(ctx context.Context, c *transpareo.Client, exp *Export,
	progress func(*transpareo.Task)) (*Export, error) {
	statusURL := exp.StatusURL
	if statusURL == "" {
		statusURL = "/exports/" + exp.ID.String()
	}
	task, err := c.WaitForTask(ctx, statusURL,
		&transpareo.WaitOptions{OnPoll: progress})
	if err != nil {
		return nil, err
	}
	done, err := decodeExport(task.Body)
	if err != nil {
		return nil, err
	}
	if done.Status == "failed" {
		return done, &transpareo.Error{Code: "EXPORT_FAILED",
			Message: firstNonEmpty(done.Failure, "the export failed")}
	}
	return done, nil
}

// Download streams the archive of a completed export to w.
func Download(ctx context.Context, c *transpareo.Client, exp *Export,
	w io.Writer) (int64, error) {
	url := exp.DownloadURL
	if url == "" {
		url = "/exports/" + exp.ID.String() + "/download"
	}
	return c.Download(ctx, url, w)
}

func decodeExport(body []byte) (*Export, error) {
	var exp Export
	if err := json.Unmarshal(body, &exp); err != nil {
		return nil, &transpareo.Error{Code: transpareo.CodeInvalidResponse,
			Message: "the export answer is not the JSON the client expected: " +
				err.Error()}
	}
	exp.Body = json.RawMessage(body)
	return &exp, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Summary returns one line about an export.
func (e *Export) Summary() string {
	if e.Filename != "" {
		return fmt.Sprintf("export %s %s (%s)", e.ID, e.Status, e.Filename)
	}
	return fmt.Sprintf("export %s %s", e.ID, e.Status)
}
