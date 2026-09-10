#!/bin/sh
# Prepares the host for `systemctl enable --now transpareo-signer`:
# the service account, the key directory only that account can
# read, and a systemd that knows the unit. The unit is neither
# enabled nor started here; the keys have to exist first, and
# `transpareo signer keygen` is the operator's step.
set -e

user=transpareo-signer
keydir=/etc/transpareo/signer

exists() {
  if command -v getent >/dev/null 2>&1; then
    getent "$1" "$2" >/dev/null 2>&1
  else
    grep -q "^$2:" "/etc/$1"
  fi
}

if ! exists group "$user"; then
  if command -v groupadd >/dev/null 2>&1; then
    groupadd --system "$user"
  else
    addgroup -S "$user"
  fi
fi

if ! exists passwd "$user"; then
  if command -v useradd >/dev/null 2>&1; then
    useradd --system --gid "$user" --home-dir /nonexistent \
      --no-create-home --shell /usr/sbin/nologin \
      --comment 'Transpareo signing endpoint' "$user"
  else
    adduser -S -G "$user" -H -h /nonexistent -s /sbin/nologin "$user"
  fi
fi

mkdir -p "$keydir"
chown "$user":"$user" "$keydir"
chmod 0700 "$keydir"

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload >/dev/null 2>&1 || true
fi
