// Package e2e runs the command-line tool and the client against a
// real workspace host. It needs a consumer on a test tenant and
// skips without one:
//
//	export TRANSPAREO_TEST_HOST=...
//	export TRANSPAREO_TEST_CLIENT_ID=...
//	export TRANSPAREO_TEST_CLIENT_SECRET=...
//	go test -tags e2e ./test/e2e/
//
// The backend's `rake test:api_credentials` prints the three
// lines for the dev tenant. Three more widen the run and each
// skips its own check when unset: TRANSPAREO_TEST_SECOND_HOST
// names a second tenant host, so a token of the first host is
// proven to be refused there, and
// TRANSPAREO_TEST_READONLY_CLIENT_ID with
// TRANSPAREO_TEST_READONLY_CLIENT_SECRET name a consumer without
// write permissions, so a refused write is proven too.
//
// The suite creates a brand, a product and two webhook
// subscriptions named with a run id and deletes them at the end,
// so a tenant it ran on carries nothing of it. A run that dies
// before its cleanup leaves records behind, so the next run
// sweeps the ones older than an hour, which leaves a run beside
// it alone. The consumer needs brand_access, product_access and
// webhook_access.
package e2e
