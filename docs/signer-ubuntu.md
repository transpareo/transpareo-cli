# Running the signer on Ubuntu

The worked example: Ubuntu 22.04 or 24.04, nginx in front
terminating TLS, the signing endpoint on loopback behind it, and
the whole thing proved by the test button on the signing keys
page of the application manager. Every systemd and nginx piece
is the same on Debian. What the endpoint is and what it signs is
in [the signing endpoint](signer.md); this page is the
deployment.

## Before you start

- A server reachable from the internet on port 443.
- A DNS name pointing at it. `signer.example.com` stands in for
  it below.
- nginx and certbot installed
  (`sudo apt install nginx python3-certbot-nginx`).
- The clock in step with the world: `timedatectl` says
  `System clock synchronized: yes`. A request whose timestamp is
  more than five minutes off is refused, so a drifting clock
  makes every request fail.
- The host name of the workspace, from the address bar of the
  application manager, `acme.transpareo.com` for instance. The
  key that signs the platform's requests is published there.

## Install

```
curl -fsSLO https://github.com/transpareo/transpareo-cli/releases/latest/download/transpareo_<version>_amd64.deb
sudo apt install ./transpareo_<version>_amd64.deb
```

The package brings the `transpareo` binary and, for the signing
endpoint, four things: the system account `transpareo-signer`,
the key directory `/etc/transpareo/signer` readable by that
account only, the systemd unit `transpareo-signer`, and the
settings file `/etc/transpareo/signer.env`. The unit is not
enabled and not started, because the keys do not exist yet. An
upgrade keeps the settings file as you edited it.

## Keys

```
sudo -u transpareo-signer transpareo signer keygen --dir /etc/transpareo/signer
```

This writes `p256.pem` and `ed25519.pem` into that directory,
readable by the service account only, and prints the two public
halves. Paste them into the BYOK form on the signing keys page,
the P-256 key in the P-256 field and the Ed25519 key in the
Ed25519 field. The private halves stay on this server. Nothing
uploads them, and the platform never asks for them.

## Configure

Edit `/etc/transpareo/signer.env`:

```
TRANSPAREO_PLATFORM_KEY=https://acme.transpareo.com/.well-known/transpareo-signing-key.pem
TRANSPAREO_SIGNER_HOST=signer.example.com
TRANSPAREO_SIGNER_LISTEN=127.0.0.1:8443
```

`TRANSPAREO_PLATFORM_KEY` carries the workspace host, the one
from the address bar. `TRANSPAREO_SIGNER_HOST` is the host name
you are about to register in the BYOK form. Leave the listen
address alone; nginx reaches the endpoint over loopback. The
endpoint reads these three from its environment, which is how
the unit passes them; an option on the command line would win
over them. The container image takes the same three, in
[Running the signer in a container](signer-docker.md).

## nginx

`/etc/nginx/sites-available/signer.example.com`, symlinked into
`sites-enabled`:

```
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name signer.example.com;

    ssl_certificate /etc/letsencrypt/live/signer.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/signer.example.com/privkey.pem;

    location = /sign {
        proxy_pass http://127.0.0.1:8443;
        proxy_set_header Host $host;
        proxy_read_timeout 15s;
        client_max_body_size 2m;
    }

    location / {
        return 404;
    }
}
```

Three lines carry weight. `proxy_set_header Host $host` passes
the registered host name through: the platform signs each
request for that host, and nginx would otherwise hand the
upstream address to the endpoint and every signature would fail.
`proxy_read_timeout 15s` matches the budget the platform gives
the whole call. `client_max_body_size 2m` keeps nginx from
refusing a request before the endpoint's own limit of 1 MiB
does; the canonical statements of a passport sit well under
that.

Then the certificate and the reload:

```
sudo certbot --nginx -d signer.example.com
sudo nginx -t
sudo systemctl reload nginx
```

## Firewall

```
sudo ufw allow 443/tcp
```

Plus the security group of the cloud provider, if there is one.
Port 8443 stays closed to the world: the endpoint listens on
loopback and only nginx talks to it.

## Start

```
sudo systemctl enable --now transpareo-signer
sudo systemctl status transpareo-signer
sudo journalctl -u transpareo-signer -f
```

The journal shows one line per request: the kind, the elapsed
time and the outcome. Never a key, never a body.

## Register and test

Enter `https://signer.example.com/sign` in the BYOK form and
press the test button. It round-trips both request shapes and
verifies the answers against the public keys you pasted in. A
pass means the endpoint is reachable and that those public keys
belong to the private keys on this server, so a publish will not
fail verification later.

## Operating it

The endpoint sits in the publish path. If it is down, publishing
fails and the application manager says so. Point the uptime
monitor at `https://signer.example.com/sign` and expect 405: the
endpoint refuses anything but POST before it looks at a
signature, so the check needs no credentials.

Rotating the keys: stage a new pair beside the old one, paste
the new public halves into the BYOK form, move them into place
and restart. Do the last two steps back to back, because between
them the platform holds the new public keys while the endpoint
still signs with the old ones. Passports signed with the old
keys stay verifiable: their public halves are published for
good.

```
sudo -u transpareo-signer transpareo signer keygen --dir /etc/transpareo/signer-next
sudo mv /etc/transpareo/signer-next/*.pem /etc/transpareo/signer/
sudo systemctl restart transpareo-signer
```

The old pair is gone once you overwrite it, so keep a copy
until the test button passes on the new keys.

Upgrading: install the new package and restart the service. The
settings file survives.

```
sudo apt install ./transpareo_<version>_amd64.deb
sudo systemctl restart transpareo-signer
```

## When something fails

- **401 in the journal, the test fails.** The Host header or the
  platform key. Check `TRANSPAREO_SIGNER_HOST` against the
  registered URL, check `proxy_set_header Host $host` in the
  nginx block, and check the workspace host in
  `TRANSPAREO_PLATFORM_KEY`. Reload nginx and restart the
  service after a change.
- **"rotation statement" in the journal.** The platform signs
  with a key this endpoint cannot follow from the one it pinned,
  which happens when it was pinned across two rotations.
  `sudo systemctl restart transpareo-signer` pins the key the
  platform signs with now.
- **"timestamp" in the journal.** The clock. `timedatectl` and
  then `sudo timedatectl set-ntp true`.
- **The test says the endpoint is unreachable.** DNS, the
  firewall or the certificate, in that order.
  `curl -i https://signer.example.com/sign` from elsewhere
  should answer 405.
- **The test says the keys do not match.** The public keys in
  the form are not the halves of the private keys the service
  uses. Print the halves it holds and paste them again:

  ```
  sudo -u transpareo-signer openssl pkey -pubout -in /etc/transpareo/signer/p256.pem
  sudo -u transpareo-signer openssl pkey -pubout -in /etc/transpareo/signer/ed25519.pem
  ```

  `keygen` prints them only when it makes them, and it refuses
  to overwrite an existing key without `--force`, so this is the
  way to read them back.
