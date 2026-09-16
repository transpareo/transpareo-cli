# Signing endpoint

A workspace that brings its own keys (BYOK) signs its passports
itself. The platform holds no private half: at publish time it
sends what is to be signed to an HTTPS endpoint the workspace
runs, reads the signatures back, assembles the proof and
verifies it against the public keys the workspace registered.
`transpareo signer` is the reference implementation of that
endpoint, so a workspace does not have to write one. The keys
never leave the machine it runs on.

## Keys

```
transpareo signer keygen
```

writes `p256.pem` and `ed25519.pem` (PKCS#8, readable by their
owner only) under `~/.config/transpareo/signer`, or under
`--dir`, and prints the two public keys as PEM. Paste them into
the BYOK form of the application manager, one per curve: the
P-256 key signs the passport's selective-disclosure proof, the
Ed25519 key the whole-document proofs of component library
entries and the other artefacts. An existing key file is kept
unless `--force` is given. Keys are files, because the endpoint
is a daemon on a server, where the
desktop keyrings the tool otherwise uses are not available.

## Serving

```
transpareo signer serve --platform-key platform.pem
```

listens on `127.0.0.1:8443` and serves `/sign`. Every request is
checked before a key is touched: the platform's Ed25519 signature
over the request (`X-Master-Signature`, with `X-Master-Timestamp`
and `X-Master-Nonce`), the timestamp within five minutes, the
nonce never seen before. A request that fails answers 401 and is
logged; nothing is signed.

`--platform-key` is the public key of the platform host that
signs for the workspace: in a cluster the node hosting it, so
the key the workspace host publishes. It is a PEM file or a URL
fetched at start.

With a URL the endpoint follows a rotation of that key. Every
signed request names the key it was signed with in
`X-Master-Key-Fingerprint`. While that is the pinned key nothing
is fetched, so a flood of forged requests cannot turn the
endpoint into a client of the platform. When it names another
key, the endpoint reads the host's current key and the hand-over
statement published beside it, at
`/.well-known/transpareo-signing-key-rotation.json`, and takes
the new key only when the pinned key signed that statement and
the statement names the key the host now serves. A host that
publishes no statement, which is one that never rotated and
today every host that is not the master of its cluster, is taken
at its key URL as before. Either way the endpoint looks at the
platform at most once a minute, so a host that rotates twice
inside one minute leaves it refusing requests until that budget
refills. A statement
that did not come from the pinned key is refused, and the
journal says so: restarting the endpoint pins the key the
platform signs with now.

Without `--tls-cert` and `--tls-key` the endpoint speaks plain
HTTP, for a reverse proxy that terminates TLS; with them it
terminates TLS itself. The platform registers an `https` URL that
resolves to a public address, pins that address for the call,
and gives the whole call fifteen seconds; the endpoint answers
one request in well under a second and bounds each at ten.

The platform signs the request for the host and path of the
registered URL, so both must reach the endpoint unchanged. A
reverse proxy that replaces the Host header with the upstream
address (nginx does, unless `proxy_set_header Host $host` is
set) makes every request fail verification; `--host` names the
registered host in that case. Register the URL with the path the
endpoint serves, `/sign` by default, and keep the clocks of the
signer host in step with the world: a timestamp more than five
minutes off is refused.

`--listen` and `--path` change the address and the route;
`--dir` is the directory the key files sit in, and `--p256-key`
and `--ed25519-key` name them one by one when they are not
under one directory. One line per request goes to standard
error: the kind, the elapsed time and the outcome, never a key
and never a body.

A setting the command line leaves out is read from the
environment: `TRANSPAREO_PLATFORM_KEY`, `TRANSPAREO_SIGNER_HOST`,
`TRANSPAREO_SIGNER_LISTEN` and `TRANSPAREO_SIGNER_DIR`. An
option always wins over the variable. This is how the systemd
unit of the packages and the container image are configured, and
it keeps the endpoint's settings out of the process list.

`--allow-unsigned` accepts requests without the platform
signature. It exists for a development platform whose request
signing is not seeded and must never be set on an endpoint a
real workspace registered; the platform never sends an unsigned
request in production.

## Running it on a server

Two shapes, both configured by those environment variables.

The deb, rpm and apk packages carry a systemd unit,
`transpareo-signer`, an environment file with the three
settings, and a post-install script that creates the
`transpareo-signer` account and the key directory
`/etc/transpareo/signer`. The package neither enables nor
starts the unit: the keys come first.
[Running the signer on Ubuntu](signer-ubuntu.md) is the
runbook, from the package to the test button, with nginx in
front terminating TLS. The systemd and nginx parts are the same
on Debian.

`ghcr.io/transpareo/transpareo-signer` is the same endpoint as a
container image, built from the same release for amd64 and
arm64 on a distroless base, signed with Sigstore like the
release checksums. The keys are a mounted volume and the
settings are the same variables.
[Running the signer in a container](signer-docker.md) covers
the run, a compose file, the Kubernetes details and the file
ownership a non-root image needs.

Either way something has to terminate TLS in front: the
platform registers an `https` URL and signs each request for
the host name of it.

## What is signed

Two request shapes arrive on the one route, both JSON.

Whole-document signatures (`eddsa-jcs-2022`): the platform sends
snapshots, each a canonical document and one or more proof
configurations. For every configuration the endpoint signs
SHA-256 of the configuration in JSON Canonicalization Scheme
form (RFC 8785) followed by SHA-256 of the document, with the
Ed25519 key, and answers the signatures base58btc behind a `z`,
in order.

A base proof (`ecdsa-sd-2023`): the platform sends the proof
hash, the mandatory hash and the non-mandatory statements of a
passport. The endpoint generates a P-256 key for this one proof,
signs every statement with it, signs the tuple of proof hash,
proof-scoped public key and mandatory hash with the workspace's
P-256 key, and answers the three components base64. The
proof-scoped key is discarded after the answer. The platform
does the canonicalisation and the proof assembly; the endpoint
never sees the passport.

The Go package `pkg/transpareo/signer` carries both, with the
request verifier and an `http.Handler`, for a workspace that
embeds the endpoint in a program of its own.

## Checking the setup

The signing keys page of the application manager has a test
that round-trips both shapes against the registered endpoint and
verifies the answers against the registered public keys. A pass
proves the endpoint is reachable and that the public keys on
file match the private keys behind it, so a publish will not
fail verification later.

## Not a tool for assistants

The signer is not an MCP tool and is not part of the agent skill.
It is a daemon an operator runs; an assistant has no business
holding signing keys. The two commands are for people.
