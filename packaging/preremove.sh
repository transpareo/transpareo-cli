#!/bin/sh
# Stops the signing endpoint and takes it out of the boot sequence
# when the package goes. The keys under /etc/transpareo/signer
# stay: they are the workspace's own and only the operator says
# when they go.
set -e

# An upgrade runs this too, and there the service keeps running:
# dpkg passes "upgrade", rpm the number of packages left behind.
case "$1" in
  upgrade | failed-upgrade | 1) exit 0 ;;
esac

if command -v systemctl >/dev/null 2>&1; then
  systemctl stop transpareo-signer >/dev/null 2>&1 || true
  systemctl disable transpareo-signer >/dev/null 2>&1 || true
fi
