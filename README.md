# transpareo

One binary for the Transpareo API: a command-line tool for people
and scripts, an MCP server and an agent skill for assistants, and
an importable Go client. It logs in with the client credentials of
an API consumer, reaches every endpoint of a workspace, and prints
JSON that pipelines and assistants can read.

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

With the installer script, which detects the platform, verifies
the checksum and installs into `~/.local/bin` or `/usr/local/bin`:

```
curl -fsSL https://transpareo.com/install.sh | sh
```

From source, with Go:

```
go install github.com/transpareo/transpareo-cli/cmd/transpareo@latest
```

Every release carries a checksum file signed with Sigstore, a
software bill of materials per archive and a SLSA provenance
statement. `SECURITY.md` says how to verify a download.
`transpareo upgrade` replaces the binary with the latest release
after verifying the signature against the release workflow's
identity and the embedded Sigstore trusted root, and the
archive's checksum; nothing is installed when either check
fails.

## First five minutes

An API consumer is created in the application manager of the
workspace, which shows its secret once. Log in with it; the secret
is read from standard input or from `TRANSPAREO_CLIENT_SECRET`,
never from an option, so it stays out of the shell history:

```
transpareo auth login --host acme.example.com --client-id <key>
transpareo me
```

`me` answers what the credential allows. Every operation of the
API is a command, `transpareo <group> <verb>`, generated from the
API specification the binary carries:

```
transpareo products list --per-page 5
transpareo dpps requirements --product-id <id> --granularity serial
transpareo dpps validate --file passport.json
transpareo dpps create --file passport.json
transpareo dpps publish <code>
```

Path parameters are positional, query parameters are options, and
a request body comes from `--file <path>` (`-` for standard input)
or, for flat bodies, from `--set key=value`. The full list is in
[docs/cli.md](docs/cli.md); `--help` on any command shows an
example and the permission key it needs.

A few commands span several calls:

```
transpareo imports run --file catalogue.xlsx --type products --accept-suggestions
transpareo imports map 12 --map "Farbe=new:Colour" --map "Intern=skip"
transpareo exports create --format jsonld --wait --download catalogue.tar.gz
transpareo events tail --follow
```

`imports run` uploads, maps, validates and, with `--execute` after
a clean validation, writes; it stops with exit code 5 and the
unresolved columns when a mapping is needed, and with 3 when the
validation found failing rows.

`transpareo api <METHOD> <path>` reaches any endpoint by hand.
`transpareo commands --json` lists every command and every API
operation; `transpareo schema create_dpp` prints the request
schema and an example of one operation; `transpareo guide` prints
the workspace's API guide; `transpareo doctor` checks the setup.

## With an assistant

The same binary is an MCP server. `transpareo setup claude` installs
the agent skill and registers the server for Claude Code;
`transpareo setup codex` does the same for Codex. The registration
holds no secret, only the profile name:

```json
{ "mcpServers": { "transpareo": { "command": "transpareo",
  "args": ["mcp", "--profile", "acme"] } } }
```

That snippet works in Claude Desktop (`claude_desktop_config.json`),
Claude Code (`~/.claude.json`) and Cursor (`.cursor/mcp.json`).
`--read-only` after the profile keeps the assistant to reads and
validations; `--tools dpps,products` narrows the tools. The skill
and the plugin manifest live under `skills/transpareo`; the
repository is a plugin marketplace, so Claude Code can also install
them with `/plugin marketplace add transpareo/transpareo-cli` and
`/plugin install transpareo@transpareo`. See [docs/mcp.md](docs/mcp.md).

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
- `--wait` on every command that starts background work polls
  the `statusUrl` it answers, with progress on stderr and the
  final document on stdout.

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

`client.API()` returns the typed call per operation, generated
from the specification into `pkg/transpareo/api` and sent through
the same transport:

```go
resp, err := client.API().ListDppsWithResponse(ctx, &api.ListDppsParams{})
```

## Privacy

The tool sends no telemetry, checks for no updates, and makes no
network call other than to the configured workspace host. The one
exception is the release download that an explicit
`transpareo upgrade` fetches from GitHub.

## Licence

MIT, see `LICENSE`. The Transpareo name and logo are not part of
the licence, see `TRADEMARKS.md`. Contributions are signed off
under the Developer Certificate of Origin, see `CONTRIBUTING.md`.
