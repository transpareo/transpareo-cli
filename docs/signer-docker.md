# Running the signer in a container

`ghcr.io/transpareo/transpareo-signer` is the signing endpoint as
an image, for a workspace that runs containers. The deb, rpm and
apk carry the same endpoint as a systemd unit. It is built from the
same release as the binary, for amd64 and arm64, on a distroless
base: no shell, no package manager, nothing in it but the
endpoint and the certificate authorities it needs to fetch the
platform's key. What the endpoint is and what it signs is in
[the signing endpoint](signer.md); the server-with-nginx variant
is [the Ubuntu runbook](signer-ubuntu.md).

## The image

```
docker pull ghcr.io/transpareo/transpareo-signer:latest
```

Every image is signed with Sigstore, like the release checksums.
Verify it before you run it:

```
cosign verify ghcr.io/transpareo/transpareo-signer:latest \
  --certificate-identity-regexp 'https://github.com/transpareo/transpareo-cli/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Pin an exact version in anything that runs unattended, so an
upgrade is a change you make.

## Keys

The image carries `transpareo` itself, so it can write the keys,
but the entry point has to be moved aside for the one command:

```
mkdir keys
docker run --rm --user "$(id -u):$(id -g)" \
  --entrypoint /usr/bin/transpareo \
  -v "$PWD/keys":/keys \
  ghcr.io/transpareo/transpareo-signer:latest \
  signer keygen --dir /keys
```

`--user` gives you the two key files; without it the image's
account owns them. Paste the public halves it prints into the BYOK
form on the signing keys page, P-256 in the P-256 field and
Ed25519 in the Ed25519 field. The private halves stay in
`keys/`, mode 0600, and nothing uploads them.

## Run it

```
docker run -d --name transpareo-signer --restart unless-stopped \
  --user "$(id -u):$(id -g)" \
  -p 127.0.0.1:8443:8443 \
  -v "$PWD/keys":/keys:ro \
  -e TRANSPAREO_PLATFORM_KEY=https://acme.transpareo.com/.well-known/transpareo-signing-key.pem \
  -e TRANSPAREO_SIGNER_HOST=signer.example.com \
  ghcr.io/transpareo/transpareo-signer:latest
```

The three settings are the ones the systemd unit uses.
`TRANSPAREO_PLATFORM_KEY` carries the workspace host, the one
from the address bar of the application manager.
`TRANSPAREO_SIGNER_HOST` is the host name registered in the BYOK
form, which the proxy in front replaces on the way in.
`TRANSPAREO_SIGNER_LISTEN` defaults to `0.0.0.0:8443` in the
image, and the port mapping keeps that reachable from the host
only. The keys are read from `/keys`, which
`TRANSPAREO_SIGNER_DIR` could move elsewhere.

The same as a compose file:

```yaml
services:
  signer:
    image: ghcr.io/transpareo/transpareo-signer:latest
    restart: unless-stopped
    user: "1000:1000"
    ports:
      - "127.0.0.1:8443:8443"
    volumes:
      - ./keys:/keys:ro
    environment:
      TRANSPAREO_PLATFORM_KEY: https://acme.transpareo.com/.well-known/transpareo-signing-key.pem
      TRANSPAREO_SIGNER_HOST: signer.example.com
```

## TLS in front

The platform registers an `https` URL, so something has to
terminate TLS: nginx or Caddy on the host, or the ingress of the
cluster. The nginx block, what each line in it is for, the
certificate and the firewall are in
[the Ubuntu runbook](signer-ubuntu.md); with the container
listening on `127.0.0.1:8443` the block is the same, and
`proxy_set_header Host $host` matters just as much, because the
platform signs each request for the registered host name.

## The keys have to be readable

The image runs as a non-root account, so a key file that only
another account can read stops it before it serves anything:

```
open /keys/p256.pem: permission denied
```

Either run with `--user` naming the owner of the keys, as above,
or give the files to the image's account (uid 65532). In
Kubernetes, mount them as a Secret with `defaultMode: 0400` and
set `runAsUser` and `fsGroup` in the pod's security context to
the same id.

One thing to know about health checks there: the endpoint answers
`405` to anything but POST, and an `httpGet` probe counts that as
a failure. Use a `tcpSocket` probe on 8443, and leave the real
proof to the test button on the signing keys page.

## Operating it

```
docker logs -f transpareo-signer
```

One line per request: the kind, the elapsed time and the
outcome, never a key and never a body. The endpoint is in the
publish path, so an uptime monitor pointed at
`https://signer.example.com/sign` expecting 405 tells you before
a publish does.

Upgrading is a pull and a recreate; the keys are a volume and
stay put. Rotating them: generate a new pair into a second
directory, paste the new public halves into the BYOK form, then
point the volume at the new directory and recreate the
container. Do the last two steps back to back, because between
them the platform holds the new public keys while the endpoint
still signs with the old ones.
