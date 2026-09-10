# Authentication

Programs authenticate with the OAuth 2.0 client credentials grant.
An API consumer is created in the application manager, which shows
its secret once. Two calls are all an integration needs:

```
curl -X POST https://<host>/api/oauth/token \
  -d grant_type=client_credentials \
  -d client_id=<client id> \
  -d client_secret=<client secret>

curl https://<host>/api/me -H "Authorization: Bearer <token>"
```

The token lives one hour and belongs to the host that issued it.
`scope` narrows it to a subset of the consumer's permission keys,
as a space-separated list; without it the token carries all of
them. `GET /api/me` answers what a token allows.

## With the command line

```
transpareo auth login --host acme.example.com --client-id <key>
```

reads the secret from `TRANSPAREO_CLIENT_SECRET`, from standard
input when it is not a terminal, or from a prompt that does not
echo; never from an option. It checks the credential at the token
endpoint, then stores the profile: the secret in the operating
system's keyring (the macOS Keychain, the Windows Credential
Manager, or the Secret Service on Linux), and the host, client id
and scope in `~/.config/transpareo/config.json`. Without a
keyring, on a server or in a container, the secret goes to
`~/.config/transpareo/credentials.json`, readable only by its
owner.

The token the check minted is stored beside the secret, and
every later invocation reuses it until sixty seconds before it
expires, so a script that runs the tool in a loop exchanges the
secret once an hour rather than once a command. The token
endpoint allows ten exchanges a minute per consumer, which a
run on a stored profile never meets. A 401 that reports an
expired token drops the stored token and exchanges again.

`--name` names the profile (default: the first label of the host);
`--scope a,b` narrows every token the profile mints; `--default`
makes it the default profile. `transpareo auth status` and
`transpareo me` show what the credential allows;
`transpareo auth logout` removes the profile and its secret;
`transpareo doctor` checks the setup.

## Several workspaces

`--profile <name>` or `TRANSPAREO_PROFILE` selects a profile. A
project directory may carry `.transpareo/config.json` with
`{"profile": "acme"}`, so an agent working inside that directory
targets the right workspace.

## Automated runs

The environment variables `TRANSPAREO_HOST`,
`TRANSPAREO_CLIENT_ID` and `TRANSPAREO_CLIENT_SECRET` make the tool
work without a stored profile, for pipelines. `TRANSPAREO_TOKEN`
with `TRANSPAREO_HOST` sends a token issued elsewhere as it is.

`transpareo auth token [--scope a,b]` always exchanges the secret
for a fresh token and prints it, leaving the stored one alone, so
a script or a subagent can call the API for an hour without ever
seeing the secret:

```
export TRANSPAREO_TOKEN=$(transpareo auth token --scope dpp_read)
```

## Child grants

`transpareo auth grant --code <passport code>` calls
`POST /api/grant` and prints the client credentials of a child
consumer confined to that one passport for 24 hours, so a
workshop or a recycler can record an event without an account of
its own. The child inherits only the event-write permissions the
caller holds. The secret is printed once.

## Errors

The token endpoint answers the RFC 6749 shape: `invalid_client`
(401) for a wrong secret or a suspended consumer,
`invalid_scope` (400) for a key the consumer was not granted,
`slow_down` (429) when the consumer is over the hourly rate limit
that `GET /api/me` reports. Past ten exchanges a minute, per
address or per consumer, the edge answers 429 in the documented
envelope with `TOO_MANY_REQUESTS` instead. Both carry
`Retry-After`, which the client waits out before it retries. Every
other endpoint answers the documented envelope with `error`,
`message`, `hint` and `docsUrl`; `TOKEN_EXPIRED` makes the client
exchange the secret once and retry.
