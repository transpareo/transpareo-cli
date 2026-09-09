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
// lines for the dev tenant. TRANSPAREO_TEST_SECOND_HOST names a
// second tenant host for the cross-tenant check, when one exists.
//
// The suite creates a brand and two webhook subscriptions named
// with a run id and deletes them at the end, so a tenant it ran
// on carries nothing of it. The consumer needs brand_access and
// webhook_access.
package e2e
