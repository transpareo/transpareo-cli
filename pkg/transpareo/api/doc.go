// Package api holds the request and response types and the typed
// call per operation, generated from spec/openapi.json with
// oapi-codegen. Nothing here is written by hand.
//
// The typed calls are reached through the transport of the
// parent package, which adds the bearer token, the idempotency
// key and the retries:
//
//	typed := api.NewClientWithResponses(client.BaseURL(),
//		api.WithHTTPClient(client))
//	resp, err := typed.ListDppsWithResponse(ctx, &api.ListDppsParams{})
package api

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../spec/openapi.json
