# Security

## Reporting a vulnerability

Report vulnerabilities in this tool or in the Transpareo platform to
<security@transpareo.com>. Do not open a public issue for a vulnerability;
issues are visible to everyone before a fix exists.

The platform's disclosure details, including encryption keys and the
preferred languages, are published at
<https://transpareo.com/.well-known/security.txt>.

We confirm reports within a few working days and keep you informed until
the fix is released.

## Supported versions

The latest release is the supported version. Fixes are published as a new
release; older releases are not patched.

## Verifying a download

Every release is built by the `release` workflow in this repository and
signed with Sigstore. The signature covers the checksum file; the
checksum file covers every archive. Both the checksum file and its
Sigstore bundle are attached to the GitHub release.

1. Download the archive, the checksum file
   `transpareo_<version>_checksums.txt` and the bundle
   `transpareo_<version>_checksums.txt.sigstore.json`.

2. Verify the checksum file with [cosign](https://docs.sigstore.dev):

   ```sh
   cosign verify-blob \
     --bundle transpareo_<version>_checksums.txt.sigstore.json \
     --certificate-identity-regexp \
       'https://github.com/transpareo/transpareo-cli/.github/workflows/release.yaml@refs/tags/v.*' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     transpareo_<version>_checksums.txt
   ```

   This confirms that the checksum file was produced by the release
   workflow of this repository for a version tag.

3. Verify the archive against the checksum file:

   ```sh
   sha256sum --check --ignore-missing transpareo_<version>_checksums.txt
   ```

The release also carries a SLSA provenance statement
(`multiple.intoto.jsonl`) for the archives, produced by the SLSA
GitHub generator, and an SPDX SBOM per archive.
