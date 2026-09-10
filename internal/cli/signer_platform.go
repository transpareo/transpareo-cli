package cli

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/transpareo/transpareo-cli/pkg/transpareo/signer"
)

// platformKeyTimeout bounds a fetch of the platform's key, which
// may run while a request waits.
const platformKeyTimeout = 5 * time.Second

// signerVerifier builds the verifier from the platform key, read
// from a file or fetched from a URL; a URL also serves the
// refetch on a rotation.
func (a *App) signerVerifier(ctx context.Context,
	opts serveOptions) (*signer.Verifier, error) {
	var verifier *signer.Verifier
	switch {
	case opts.platformKey == "":
		verifier = signer.NewVerifier(nil)
	case strings.HasPrefix(opts.platformKey, "https://") ||
		strings.HasPrefix(opts.platformKey, "http://"):
		fetch := func() (ed25519.PublicKey, error) {
			return a.fetchPlatformKey(ctx, opts.platformKey)
		}
		key, err := fetch()
		if err != nil {
			return nil, err
		}
		verifier = signer.NewVerifier(key)
		verifier.Refetch = fetch
		verifier.Rotation = func() (*signer.Rotation, error) {
			return a.fetchRotation(ctx, rotationURL(opts.platformKey))
		}
	default:
		data, err := os.ReadFile(opts.platformKey)
		if err != nil {
			return nil, err
		}
		key, err := signer.ParseEd25519PublicKey(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", opts.platformKey, err)
		}
		verifier = signer.NewVerifier(key)
	}
	verifier.AllowUnsigned = opts.allowUnsigned
	verifier.Host = opts.host
	return verifier, nil
}

func (a *App) fetchPlatformKey(ctx context.Context,
	url string) (ed25519.PublicKey, error) {
	data, status, err := a.published(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetching the platform key: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("fetching the platform key: %s answered %d",
			url, status)
	}
	key, err := signer.ParseEd25519PublicKey(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	return key, nil
}

// rotationURL is where the host publishing the key at keyURL
// publishes the statement that hands over to it: the same name
// carrying -rotation.json.
func rotationURL(keyURL string) string {
	return strings.TrimSuffix(keyURL, ".pem") + "-rotation.json"
}

// fetchRotation reads the hand-over statement of the platform's
// current key. A host that never rotated, and every host that is
// not its cluster's master, answers 404 and publishes none.
func (a *App) fetchRotation(ctx context.Context,
	url string) (*signer.Rotation, error) {
	data, status, err := a.published(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetching the rotation statement: %w", err)
	}
	if status == http.StatusNotFound {
		return nil, signer.ErrNoRotation
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("fetching the rotation statement: %s "+
			"answered %d", url, status)
	}
	return signer.ParseRotation(data)
}

// published reads a document the workspace host serves, bounded
// in time and in size, and answers it with its status.
func (a *App) published(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	client := &http.Client{Timeout: platformKeyTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	return data, resp.StatusCode, err
}
