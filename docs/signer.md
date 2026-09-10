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
unless `--force` is given. Keys are files rather than keyring
entries because the endpoint is a daemon on a server, where the
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
fetched at start; with a URL a key rotation is picked up by
fetching once more when a signature stops verifying, at most
once a minute, so a forged request cannot turn the endpoint into
a client of the platform.

Without `--tls-cert` and `--tls-key` the endpoint speaks plain
HTTP, for a reverse proxy that terminates TLS; with them it
terminates TLS itself. The platform registers an `https` URL that
resolves to a public address, pins that address for the call,
and gives the whole call fifteen seconds; the endpoint answers
one request in well under a second and bounds each at ten.

`--listen` and `--path` change the address and the route;
`--p256-key` and `--ed25519-key` name the key files when they
are not under the signer directory. One line per request goes to
standard error: the kind, the elapsed time and the outcome, never
a key and never a body.

`--allow-unsigned` accepts requests without the platform
signature. It exists for a development platform whose request
signing is not seeded and must never be set on an endpoint a
real workspace registered; the platform never sends an unsigned
request in production.

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
