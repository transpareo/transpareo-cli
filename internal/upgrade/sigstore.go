package upgrade

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// trustedRoot is the Sigstore public-good trusted root, fetched
// through TUF by internal/upgrade/fetchroot and embedded so that
// verification needs no further network call.
//
//go:embed trusted_root.json
var trustedRoot []byte

// OIDCIssuer is the identity provider of the release workflow.
const OIDCIssuer = "https://token.actions.githubusercontent.com"

// SigstoreVerifier verifies the keyless signature of the release
// workflow on the checksum file: the certificate must name the
// release workflow of the repository on a version tag and be
// issued for GitHub Actions.
type SigstoreVerifier struct {
	Repo string
}

// IdentityRegexp is the certificate identity the release workflow
// signs with.
func (v SigstoreVerifier) IdentityRegexp() string {
	return fmt.Sprintf(`^https://github\.com/%s/\.github/workflows/`+
		`release\.yaml@refs/tags/v.*$`, v.Repo)
}

// Verify checks bundle against artifact.
func (v SigstoreVerifier) Verify(bundleJSON, artifact []byte) error {
	trusted, err := root.NewTrustedRootFromJSON(trustedRoot)
	if err != nil {
		return fmt.Errorf("embedded trusted root: %w", err)
	}
	tmp, err := os.CreateTemp("", "transpareo-bundle-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bundleJSON); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	b, err := bundle.LoadJSONFromPath(tmp.Name())
	if err != nil {
		return fmt.Errorf("reading the bundle: %w", err)
	}
	verifier, err := verify.NewVerifier(trusted,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1))
	if err != nil {
		return err
	}
	identity, err := verify.NewShortCertificateIdentity(OIDCIssuer, "", "",
		v.IdentityRegexp())
	if err != nil {
		return err
	}
	policy := verify.NewPolicy(verify.WithArtifact(bytesReader(artifact)),
		verify.WithCertificateIdentity(identity))
	if _, err := verifier.Verify(b, policy); err != nil {
		return err
	}
	return nil
}
