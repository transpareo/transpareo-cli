#!/bin/sh
# Installs the transpareo command line from the latest GitHub
# release: detects the platform, downloads the archive and the
# checksum file, verifies the checksum, and puts the binary into
# a directory on the PATH. Verify the checksum file's Sigstore
# bundle too when cosign is installed.
#
#   curl -fsSL https://transpareo.com/cli/install.sh | sh
#
# Environment: TRANSPAREO_INSTALL_DIR (default ~/.local/bin, or
# /usr/local/bin when writable), TRANSPAREO_VERSION (default:
# latest).
set -eu

repo="transpareo/transpareo-cli"
version="${TRANSPAREO_VERSION:-}"

say() { printf '%s\n' "$*" >&2; }
fail() { say "install: $*"; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }
need curl
need tar

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux|darwin) ;;
  *) fail "unsupported operating system $os; download an archive from https://github.com/$repo/releases" ;;
esac
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail "unsupported architecture $arch" ;;
esac

if [ -z "$version" ]; then
  version=$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" \
    | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p' | head -n 1)
  [ -n "$version" ] || fail "could not read the latest release"
fi
version="${version#v}"

archive="transpareo_${version}_${os}_${arch}.tar.gz"
checksums="transpareo_${version}_checksums.txt"
base="https://github.com/$repo/releases/download/v$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "Downloading transpareo $version for $os/$arch"
curl -fsSL -o "$tmp/$archive" "$base/$archive"
curl -fsSL -o "$tmp/$checksums" "$base/$checksums"

if command -v cosign >/dev/null 2>&1; then
  curl -fsSL -o "$tmp/$checksums.sigstore.json" "$base/$checksums.sigstore.json"
  cosign verify-blob --bundle "$tmp/$checksums.sigstore.json" \
    --certificate-identity-regexp "https://github.com/$repo/.github/workflows/release.yaml@refs/tags/v.*" \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    "$tmp/$checksums" >/dev/null 2>&1 || fail "the Sigstore signature of the checksum file does not verify"
  say "Sigstore signature verified"
else
  say "cosign is not installed; the checksum file's signature was not verified"
fi

expected=$(grep " $archive\$" "$tmp/$checksums" | cut -d ' ' -f 1)
[ -n "$expected" ] || fail "no checksum for $archive"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | cut -d ' ' -f 1)
else
  actual=$(shasum -a 256 "$tmp/$archive" | cut -d ' ' -f 1)
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" transpareo

dir="${TRANSPAREO_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then dir=/usr/local/bin; else dir="$HOME/.local/bin"; fi
fi
mkdir -p "$dir"
install -m 0755 "$tmp/transpareo" "$dir/transpareo"
say "Installed $dir/transpareo"

case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "Add $dir to your PATH." ;;
esac
say "Next: transpareo auth login --host <workspace host> --client-id <key>"
say "Then: transpareo me, and transpareo setup claude for an assistant."
