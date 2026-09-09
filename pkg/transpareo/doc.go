// Package transpareo is the Go client for the Transpareo API.
//
// A client authenticates with the OAuth 2.0 client credentials
// grant. It exchanges the secret at the token endpoint of the
// workspace host on first use, refreshes the token shortly before
// it expires, and retries once when the API reports an expired
// token. The secret never leaves the process except towards the
// token endpoint.
//
//	client, err := transpareo.New("acme.example.com",
//		transpareo.ClientCredentials{ID: id, Secret: secret})
//	me, err := client.Me(ctx)
//
// Every POST carries an Idempotency-Key, so the client retries
// timeouts and server errors for every method with growing,
// jittered waits, honouring Retry-After on a 429.
//
// Errors from the API are returned as *Error and carry the
// stable code, the message, the hint the platform attaches, and
// the rate-limit headers of the response.
package transpareo
