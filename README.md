# transpareo

One binary for the Transpareo API: a command-line tool for people
and scripts, and an importable Go client. It logs in with the
client credentials of an API consumer, reaches every endpoint of a
workspace, and prints JSON that pipelines and AI assistants can
read. An MCP server and an agent skill follow in the same binary.

## Install

macOS, with Homebrew:

```
brew install transpareo/tap/transpareo
```

Linux: the deb, rpm and apk packages and the tar.gz archives are
on the [releases page](https://github.com/transpareo/transpareo-cli/releases).

Windows, with Scoop:

```
scoop bucket add transpareo https://github.com/transpareo/scoop-bucket
scoop install transpareo
```

From source, with Go:

```
go install github.com/transpareo/transpareo-cli/cmd/transpareo@latest
```

Every release carries a checksum file signed with Sigstore, a
software bill of materials per archive and a SLSA provenance
statement. `SECURITY.md` says how to verify a download.

## First five minutes

An API consumer is created in the application manager of the
workspace, which shows its secret once. Log in with it; the secret
is read from standard input or from `TRANSPAREO_CLIENT_SECRET`,
never from an option, so it stays out of the shell history:

```
transpareo auth login --host acme.example.com --client-id <key>
transpareo me
```

`me` answers what the credential allows. Then anything the API
does is one call away:

```
transpareo api GET /products --query per_page=5
transpareo api GET /dpps/requirements --query productId=<id> --query granularity=serial
transpareo api POST /dpps/validate --body @passport.json
```

`transpareo commands --json` lists every command and every API
operation; `transpareo schema create_dpp` prints the request
schema and an example of one operation; `transpareo guide` prints
the workspace's API guide; `transpareo doctor` checks the setup.

## Output, exit codes and safety

- JSON is the default when output is not a terminal, `--json`
  forces it, `--jsonl` prints lists one object per line, `-q`
  prints ids only, `--fields a,b` narrows any read. Tables appear
  on a terminal only.
- On an error the output is `{"ok": false, "error": {code,
  message, hint, docsUrl, retryable}}`. The hint is the text to
  read first.
- Exit codes: 0 success, 1 an API or transport error, 2 wrong
  usage, 3 validation failed, 4 an operation that cannot be undone
  was refused for lack of `--yes`, 5 a column mapping is required.
- `--read-only` refuses every operation that changes data and
  keeps the validations available, for a profile handed to an
  assistant. Operations that cannot be undone need `--yes`.

## Credentials

- The secret lives in the operating system's keyring: the macOS
  Keychain, the Windows Credential Manager, or the Secret Service
  on Linux. Without a keyring, on a server or in a container, it
  goes to `~/.config/transpareo/credentials.json`, readable only
  by you.
- Non-secret settings live in `~/.config/transpareo/config.json`.
  `--profile <name>` and `TRANSPAREO_PROFILE` select between
  workspaces; a `.transpareo/config.json` in a project directory
  with `{"profile": "acme"}` selects one for that directory.
- `TRANSPAREO_HOST`, `TRANSPAREO_CLIENT_ID`,
  `TRANSPAREO_CLIENT_SECRET` and `TRANSPAREO_TOKEN` override the
  stored profile, for automated runs.
- `transpareo auth token --scope dpp_read` prints a token that
  lives an hour, for a script or a subprocess that should never
  see the secret. `transpareo auth grant --code <passport code>`
  issues child credentials confined to one passport for a day.

## Go client

```go
import "github.com/transpareo/transpareo-cli/pkg/transpareo"

client, err := transpareo.New("acme.example.com",
	transpareo.ClientCredentials{ID: id, Secret: secret})
me, err := client.Me(ctx)
```

The client exchanges the secret on first use and again shortly
before the token expires, sends an `Idempotency-Key` on every
POST, retries timeouts and server errors with growing waits, and
follows `Retry-After` on a rate limit. Lists are read with
`transpareo.List` and `transpareo.ListAll`, background work with
`client.WaitForTask`. `transpareo.FromProfile("acme")` reuses a
stored login.

## Privacy

The tool sends no telemetry, checks for no updates, and makes no
network call other than to the configured workspace host.

## Licence

MIT, see `LICENSE`. The Transpareo name and logo are not part of
the licence, see `TRADEMARKS.md`. Contributions are signed off
under the Developer Certificate of Origin, see `CONTRIBUTING.md`.
