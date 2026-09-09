package transpareo

import (
	"bytes"
	"io"
	"net/http"
	"sync"

	"github.com/transpareo/transpareo-cli/pkg/transpareo/api"
)

// API returns the typed calls generated from the specification,
// sent through this client's transport: the bearer token, the
// idempotency key on a POST and the retries apply to every call.
//
//	resp, err := client.API().ListDppsWithResponse(ctx, nil)
func (c *Client) API() *api.ClientWithResponses {
	c.apiOnce.Do(func() {
		typed, err := api.NewClientWithResponses(c.BaseURL()+"/",
			api.WithHTTPClient(c.HTTPClient()))
		if err != nil {
			panic("transpareo: building the typed client: " + err.Error())
		}
		c.api = typed
	})
	return c.api
}

// HTTPClient returns an *http.Client whose transport is this
// client: every request it sends gets the token, the idempotency
// key and the retries, and must stay on the client's host.
func (c *Client) HTTPClient() *http.Client {
	return &http.Client{Transport: transport{c}}
}

// transport is the http.RoundTripper behind HTTPClient.
type transport struct {
	client *Client
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	c := t.client
	target, err := c.resolve(req.URL.String(), nil)
	if err != nil {
		return nil, err
	}
	var body []byte
	if req.Body != nil {
		body, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, &Error{Code: "REQUEST_INVALID",
				Message: "reading the request body: " + err.Error(), cause: err}
		}
	}
	key := req.Header.Get("Idempotency-Key")
	if key == "" && req.Method == http.MethodPost {
		key = c.newKey()
	}
	resp, apiErr := c.exchange(req.Context(), req.Method, target, body,
		func(httpReq *http.Request, token *Token) {
			for name, values := range req.Header {
				httpReq.Header[name] = values
			}
			if httpReq.Header.Get("Accept") == "" {
				httpReq.Header.Set("Accept", Accept)
			}
			httpReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
			httpReq.Header.Set("User-Agent", c.userAgent)
			if key != "" {
				httpReq.Header.Set("Idempotency-Key", key)
			}
		})
	if apiErr != nil {
		return nil, apiErr
	}
	return &http.Response{
		Status:        http.StatusText(resp.StatusCode),
		StatusCode:    resp.StatusCode,
		Header:        resp.Header,
		Body:          io.NopCloser(bytes.NewReader(resp.Body)),
		ContentLength: int64(len(resp.Body)),
		Request:       req,
	}, nil
}

// apiState is embedded in Client for the memoised typed client.
type apiState struct {
	apiOnce sync.Once
	api     *api.ClientWithResponses
}
