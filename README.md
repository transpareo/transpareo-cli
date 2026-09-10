# transpareo-cli

[![test](https://github.com/transpareo/transpareo-cli/actions/workflows/test.yaml/badge.svg)](https://github.com/transpareo/transpareo-cli/actions/workflows/test.yaml)
[![release](https://img.shields.io/github/v/release/transpareo/transpareo-cli?display_name=tag)](https://github.com/transpareo/transpareo-cli/releases)
[![Go reference](https://pkg.go.dev/badge/github.com/transpareo/transpareo-cli/pkg/transpareo.svg)](https://pkg.go.dev/github.com/transpareo/transpareo-cli/pkg/transpareo)
[![licence](https://img.shields.io/badge/licence-MIT-blue)](LICENSE)

The command line, MCP server and Go client for the
[Transpareo](https://transpareo.com) API. One binary that people,
scripts and AI assistants use to work with a Transpareo workspace:
its products, components, brands and Digital Product Passports.

![A terminal session: me, products create, dpps requirements, dpps validate, dpps create, dpps publish, setup claude](docs/demo.svg)

## Transpareo for the agentic age

A Digital Product Passport is data that many parties produce, keep
current and read for years, and most of those parties are
programs. Transpareo was built for programs from the start: every
passport it publishes is a signed, machine-readable document, and
its API covers the whole life cycle of a passport, from the
product's property types through validation, creation and
publication to the events of a unit in the field. Battery
passports become mandatory in the European Union in February 2027;
[other product groups follow](https://transpareo.com/en/industries).

The newest programs are AI agents. Manufacturers have begun to
hand the filling, checking and upkeep of compliance data to
assistants. This tool is how they reach Transpareo: every
operation of the API is a command, the same binary serves the
Model Context Protocol to an assistant, an agent skill teaches the
flows, and the credential stays with the tool. Its commands, its
reference and the operation catalogue an assistant reads are
generated from the API specification, so a platform change is a
generated change.

The API itself is documented on every workspace host: the
[reference](https://demo.transpareo.com/apidocs) and the
[guide](https://demo.transpareo.com/apidocs/guide.md) of the demo
workspace show what it offers. Programs authenticate with the
OAuth 2.0 client credentials grant; two `curl` lines are enough to
start without this tool, and this tool is what makes the rest
safe and quick.

## Quick start

```
curl -fsSL https://transpareo.com/cli/install.sh | sh
transpareo auth login --host acme.example.com --client-id <key>
transpareo me
```

The installer serves Linux and macOS. macOS also has the Homebrew
tap [transpareo/homebrew-tap](https://github.com/transpareo/homebrew-tap),
Windows the Scoop bucket
[transpareo/scoop-bucket](https://github.com/transpareo/scoop-bucket);
the packages and `go install` are listed under Install below.

An API consumer is created in the application manager of the
workspace, which shows its secret once. `auth login` reads the
secret from standard input or from `TRANSPAREO_CLIENT_SECRET`,
never from an option, checks it at the token endpoint and stores
it in the operating system's keyring, with the token beside it,
so later commands reuse the token for its hour instead of
exchanging the secret every time. `me` answers what the
credential allows. Then:

```
transpareo products list --per-page 5
transpareo dpps requirements --product-id <id> --granularity item
transpareo dpps validate --file passport.json
transpareo dpps create --file passport.json
transpareo dpps publish <code>
```

What a call answers, taken from a real workspace. The
requirements of a unit passport:

```
$ transpareo dpps requirements --product-id 8 --granularity item --fields identifiers
{
  "identifiers": {
    "batchIdentifier": { "required": true },
    "modelIdentifier": { "required": true, "source": "product", "value": "4006381333931" },
    "serialIdentifier": { "required": true }
  }
}
```

A validation, which runs the checks a publish runs and writes
nothing:

```
$ transpareo dpps validate --file passport.json
{
  "valid": true,
  "publishBlocked": false,
  "fields": {},
  "validation": { "must": [], "should": [], "validatorVersion": "shacl-v1" }
}
```

A refusal, with the hint to read first and one line per failing
field; the same envelope reaches an assistant through the MCP
tools:

```
$ transpareo products create --set product.name=Sample
PRODUCT_INVALID: Product invalid
Correct the attributes listed under fields and resend the request.
  brand: Brand needs a brand - none was given
  componentsInput: Components input missing
```

For an assistant, one more line:

```
transpareo setup claude
```

The five flows the API guide teaches, as a person runs them and
as an assistant calls them:

| Task | Command | MCP tool |
|---|---|---|
| What does the credential allow | `transpareo me` | `me` |
| What a passport of a product needs | `transpareo dpps requirements --product-id <id>` | `dpp_requirements` |
| Check a passport without writing | `transpareo dpps validate --file passport.json` | `validate_dpp` |
| Write and publish it | `transpareo dpps create --file passport.json`, `transpareo dpps publish <code>` | `create_dpp`, `publish_dpp` |
| Follow what happened | `transpareo events tail --follow` | `tail_events` |
| A new product first | `transpareo products new`, `transpareo products create --file product.json` | `product_property_types`, `create_product` |
| Many passports at once | `transpareo dpps bulk validate --file rows.ndjson`, `transpareo dpps bulk create --file rows.ndjson` | `bulk_validate_dpps`, `bulk_create_dpps` |

## Install

macOS, with Homebrew, from the
[transpareo/homebrew-tap](https://github.com/transpareo/homebrew-tap)
repository:

```
brew install transpareo/tap/transpareo
```

Linux and macOS, with the installer script, which detects the
platform, verifies the checksum and installs into `~/.local/bin`
or `/usr/local/bin`:

```
curl -fsSL https://transpareo.com/cli/install.sh | sh
```

Linux packages: the deb, rpm and apk packages and the tar.gz
archives are on the
[releases page](https://github.com/transpareo/transpareo-cli/releases).
The three packages also carry the signing endpoint as a systemd
service, for a workspace that holds its own keys.

Windows, with Scoop, from the
[transpareo/scoop-bucket](https://github.com/transpareo/scoop-bucket)
repository:

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
statement; `SECURITY.md` says how to verify a download.
`transpareo upgrade` replaces the binary with the latest release
after verifying that signature against the release workflow's
identity and the embedded Sigstore trusted root, and the
archive's checksum. Nothing is installed when either check fails.
`transpareo upgrade --verify` checks the binary you are running
the same way, byte for byte against its release.

## With an assistant

The same binary is an MCP server. `transpareo setup claude`
installs the agent skill and registers the server for Claude Code;
`transpareo setup codex` does the same for Codex. The registration
holds no secret, only the profile name:

```json
{ "mcpServers": { "transpareo": { "command": "transpareo",
  "args": ["mcp", "--profile", "acme"] } } }
```

That snippet works in Claude Desktop (`claude_desktop_config.json`),
Claude Code (`~/.claude.json`) and Cursor (`.cursor/mcp.json`).

The server offers curated tools rather than one per endpoint:
`me`; products, components and brands; the passport flow from
`dpp_requirements` through `validate_dpp`, `create_dpp` and
`publish_dpp` to `void_dpp` and the bulk calls; imports, exports,
tasks and the event feed; webhooks. `search_operations` and
`call_api` reach every other operation. Every tool description
carries the permission it needs, whether the action can be undone,
the data tier of its answer and one example. Tools that cannot be
undone take a `confirm` argument that repeats the record's
identifier, so a host that shows tool calls shows the intent.

`--read-only` after the profile keeps the assistant to reads and
validations; `--tools dpps,products` narrows the tools. The skill
and the plugin manifest live under `skills/transpareo`; the
repository is a plugin marketplace, so Claude Code can also
install them with `/plugin marketplace add transpareo/transpareo-cli`
and `/plugin install transpareo@transpareo`. Details in
[docs/mcp.md](docs/mcp.md).

## Commands

Every operation of the API is a command, `transpareo <group>
<verb>`, generated from the specification the binary carries:
`dpps list`, `dpps bulk create`, `products new`, `webhooks test`.
Path parameters are positional, query parameters are options, and
a request body comes from `--file <path>` (`-` for standard input)
or, for flat bodies, from `--set key=value`. `--help` on any
command shows an example and the permission key it needs.

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

`transpareo tasks wait <statusUrl>` polls the status URL that any
background work answered and prints the final document.

Three commands are for finding your way: `transpareo api <METHOD>
<path>` reaches any endpoint by hand, `transpareo commands --json`
prints every command and every operation for programs, and
`transpareo schema create_dpp` prints the request schema and an
example of one operation. `transpareo guide` prints the
workspace's API guide and `transpareo doctor` checks the setup.

The full reference, one section per group with every command, its
example, options and permission key, is
[docs/cli.md](docs/cli.md).

## Output, exit codes and safety

- JSON is the default when output is not a terminal, `--json`
  forces it, `--jsonl` prints lists one object per line, `-q`
  prints ids only, `--fields a,b` narrows any read. Tables appear
  on a terminal only.
- On an error the output is `{"ok": false, "error": {code,
  message, hint, docsUrl, fields, retryable}}`. The hint is the
  text to read first.
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

The secret lives in the operating system's keyring, or in
`~/.config/transpareo/credentials.json`, readable only by you,
where no keyring exists. `--profile <name>` selects between
workspaces, and a `.transpareo/config.json` in a project directory
selects one for that directory. `TRANSPAREO_HOST`,
`TRANSPAREO_CLIENT_ID`, `TRANSPAREO_CLIENT_SECRET` and
`TRANSPAREO_TOKEN` override the stored profile for automated runs.
`transpareo auth token --scope dpp_read` prints a token that lives
an hour, for a script or a subprocess that should never see the
secret; `transpareo auth grant --code <passport code>` issues child
credentials confined to one passport for a day. All of it in
[docs/auth.md](docs/auth.md).

## Signing endpoint

A workspace that brings its own keys (BYOK) signs its passports
itself. The platform holds no private half: at publish time it
sends what is to be signed to an HTTPS endpoint the workspace
runs, reads the signatures back, assembles the proof and
verifies it against the public keys the workspace registered.
`transpareo signer` is that endpoint, so a workspace does not
have to write one.

`transpareo signer keygen` writes the two keys, P-256 for the
passport's selective-disclosure proof and Ed25519 for the
whole-document proofs, and prints their public halves for the
BYOK form of the application manager. `transpareo signer serve`
runs the endpoint. Every request is checked before a key is
touched: the platform's signature over the request, the
timestamp within five minutes, the nonce never seen before. The
keys never leave the machine, and no assistant can reach them:
the signer is not an MCP tool and not part of the agent skill.

The deb, rpm and apk packages install it as a service. They
bring the systemd unit `transpareo-signer`, the settings file
`/etc/transpareo/signer.env`, the system account
`transpareo-signer`, and the key directory
`/etc/transpareo/signer` that only that account can read. The
package neither enables nor starts the unit, because the keys
come first:

```
sudo apt install ./transpareo_<version>_amd64.deb
sudo -u transpareo-signer transpareo signer keygen --dir /etc/transpareo/signer
sudoedit /etc/transpareo/signer.env
sudo systemctl enable --now transpareo-signer
```

The settings file holds three values: the URL of the key the
workspace host signs its requests with, the host name registered
in the BYOK form, and the loopback address the proxy in front
forwards to. An upgrade keeps the file as you edited it. Paste
the two public keys into the BYOK form, put nginx or another
proxy in front to terminate TLS, and press the test button on
the signing keys page: it round-trips both request shapes and
verifies the answers against the registered public keys, so a
publish will not fail verification later.

[docs/signer-ubuntu.md](docs/signer-ubuntu.md) is the runbook for
Ubuntu with nginx and certbot, from the package to the test
button: the proxy block that passes the registered Host through,
the firewall, the uptime check, key rotation and what each
failure in the journal means. [docs/signer.md](docs/signer.md)
has the protocol, what each of the two request shapes signs, and
the Go package for a workspace that would rather embed the
endpoint in a program of its own.

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

## Development

`go build ./cmd/transpareo`, `go test ./...`, `go generate ./...`
after a change to the specification. Generated code is committed
and a check fails when it is stale. The end-to-end suite under
`test/e2e` runs against a real workspace when
`TRANSPAREO_TEST_HOST`, `TRANSPAREO_TEST_CLIENT_ID` and
`TRANSPAREO_TEST_CLIENT_SECRET` are set. `CONTRIBUTING.md` has the
rest, including the sign-off every commit needs.

## Security and privacy

Report vulnerabilities as `SECURITY.md` says, not in a public
issue. The tool sends no telemetry, checks for no updates, and
makes no network call other than to the configured workspace
host; the one exception is the release download that an explicit
`transpareo upgrade` fetches from GitHub.

## Licence

MIT, see `LICENSE`. The Transpareo name and logo are not part of
the licence, see `TRADEMARKS.md`. Contributions are signed off
under the Developer Certificate of Origin, see `CONTRIBUTING.md`.

## About Transpareo

[Transpareo](https://transpareo.com) is a product information
platform for Digital Product Passports: a workspace per
manufacturer, signed passports served for ten years, an
application manager for people and this API for programs.
