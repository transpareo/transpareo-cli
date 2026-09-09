# Contributing

Thank you for helping with the Transpareo command-line tool. This page
covers the build, the checks that run on every pull request, the sign-off
every commit needs, and the style rules the repository follows.

## Build

```sh
go build ./cmd/transpareo
```

Go 1.27 or later is required. `mise.toml` pins the version used by the
maintainers; `mise install` sets it up.

## Test

```sh
go test ./...
```

Write the test first, then the change that makes it pass. Every new
behaviour and every bug fix comes with a test in the same pull request.

## End-to-end checks

`go test ./...` never touches the network. The suite under
`test/e2e` runs the tool against a real workspace host and skips
unless a consumer is configured:

```sh
export TRANSPAREO_TEST_HOST=<workspace host>
export TRANSPAREO_TEST_CLIENT_ID=<key>
export TRANSPAREO_TEST_CLIENT_SECRET=<secret>
go test -tags e2e ./test/e2e/
```

It creates a brand and two webhook subscriptions named after the
run and deletes them when it is done. The nightly workflow runs it against the test tenant.

## Regenerate

```sh
go generate ./...
```

Generated code is committed. The `test` workflow regenerates it on every
pull request and fails when the committed files are stale, so run
`go generate ./...` after changing anything it depends on and commit the
result together with your change.

## The specification

The OpenAPI document is vendored in `spec/openapi.json` and embedded into
the binary. It is not edited by hand: the `nightly` workflow downloads the
live document, neutralises the tenant-specific fields, and fails when the
result differs from the vendored file. The API itself lives in the
Transpareo platform and is not changed from this repository; if you need
an API change, open an issue that describes it and it will be routed to
the platform team.

## Sign your commits

Every commit needs a Developer Certificate of Origin sign-off:

```sh
git commit -s
```

This adds a `Signed-off-by:` line with your name and email address. By
adding it you certify the statements at
<https://developercertificate.org>, in short: you wrote the change or have
the right to submit it under the repository's licence. Pull requests with
unsigned commits cannot be merged.

## Style

- No em-dashes, en-dashes used as dashes, or arrow glyphs in code,
  comments, documentation or commit messages. Use a spaced hyphen.
- Comments describe what the code does now, not the history that led to
  it. No phase names, ticket numbers or "since version X" notes.
- Tests first: a pull request that adds behaviour without a test is not
  ready for review.
- No secrets and no tenant host names anywhere in the repository,
  including tests, fixtures and workflow files. CI reads them from
  GitHub Actions secrets only.
- The platform's administration tool is called the "application
  manager" in every user-readable string and document.
- Keep lines at or under 80 characters where feasible.

## Pull requests

Fill in the pull request template. The checklist there mirrors what the
review looks for: tests, regenerated code, the DCO sign-off and the
absence of secrets.
