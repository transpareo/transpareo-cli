package cli

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/pkg/transpareo/signer"
)

// The two key files keygen writes and serve reads by default.
const (
	signerP256File    = "p256.pem"
	signerEd25519File = "ed25519.pem"
)

func (a *App) signerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "signer",
		Short: "Run the signing endpoint of a workspace that holds its own keys",
		Long: `A workspace that brings its own keys signs its passports itself:
the platform sends what is to be signed to an HTTPS endpoint the
workspace runs and reads the signatures back. These commands make
the keys and run that endpoint. The keys never leave the machine.`,
		Example: "  transpareo signer keygen\n" +
			"  transpareo signer serve --platform-key platform.pem",
	}
	cmd.Annotations = map[string]string{annotationOperator: "true"}
	cmd.AddCommand(a.signerKeygenCommand(), a.signerServeCommand())
	return cmd
}

// signerDir is where keygen writes and serve reads by default.
func (a *App) signerDir() string {
	return filepath.Join(a.ConfigDir, "signer")
}

func (a *App) signerKeygenCommand() *cobra.Command {
	var dir string
	var force bool
	cmd := &cobra.Command{
		Use:   "keygen [--dir <path>] [--force]",
		Short: "Write the P-256 and Ed25519 keys and print their public halves",
		Long: `Writes p256.pem and ed25519.pem, readable by their owner only, and
prints the two public keys as PEM. Paste those into the BYOK form
of the application manager, one per curve. An existing key file is
kept unless --force is given.`,
		Example: "  transpareo signer keygen\n" +
			"  transpareo signer keygen --dir /etc/transpareo/signer --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				dir = a.signerDir()
			}
			return a.signerKeygen(dir, force)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "",
		"directory for the key files (default: signer under the config dir)")
	cmd.Flags().BoolVar(&force, "force", false, "replace existing key files")
	return cmd
}

func (a *App) signerKeygen(dir string, force bool) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	keys, err := signer.Generate()
	if err != nil {
		return err
	}
	p256Path := filepath.Join(dir, signerP256File)
	edPath := filepath.Join(dir, signerEd25519File)
	for _, k := range []struct {
		path string
		key  any
	}{{p256Path, keys.P256}, {edPath, keys.Ed25519}} {
		if err := signer.WritePrivateKey(k.path, k.key, force); err != nil {
			if errors.Is(err, os.ErrExist) {
				return output.Exit(output.ExitUsage, fmt.Errorf("%s exists; "+
					"repeat with --force to replace it", k.path))
			}
			return err
		}
	}
	p256Public, err := signer.PublicPEM(keys.P256)
	if err != nil {
		return err
	}
	edPublic, err := signer.PublicPEM(keys.Ed25519)
	if err != nil {
		return err
	}
	printer := a.Printer()
	if printer.JSONMode() {
		return printer.Print(map[string]any{
			"p256": map[string]string{"path": p256Path,
				"publicKey": string(p256Public)},
			"ed25519": map[string]string{"path": edPath,
				"publicKey": string(edPublic)},
		})
	}
	_, err = fmt.Fprintf(a.Stdout, "Keys written to %s and %s.\n\n"+
		"P-256 public key, for the passport proof:\n%s\n"+
		"Ed25519 public key, for whole-document proofs:\n%s",
		p256Path, edPath, p256Public, edPublic)
	return err
}

func (a *App) signerServeCommand() *cobra.Command {
	var opts serveOptions
	cmd := &cobra.Command{
		Use: "serve --platform-key <pem | url> [--listen <addr>] " +
			"[--dir <path>] [--p256-key <pem>] [--ed25519-key <pem>]",
		Short: "Serve the signing endpoint",
		Long: `Serves the route the platform calls at publish time. Every request
is checked against the platform's request-signing key before a key
of the workspace is touched: the signature over the request, the
timestamp within five minutes, the nonce never seen before.

--platform-key is the Ed25519 public key of the host that signs for
the workspace, as a PEM file or a URL fetched at start; on a URL a
rotation is picked up by fetching once more when a signature stops
verifying. Without --tls-cert and --tls-key the endpoint speaks
plain HTTP for a reverse proxy that terminates TLS. The platform
needs an https URL that resolves to a public address either way.

--dir is the directory keygen wrote the two key files to;
--p256-key and --ed25519-key name them one by one instead.

Behind a reverse proxy the Host header and the path must reach the
endpoint as the platform signed them: the host of the registered
URL, or --host names it, and the path the URL carries as --path.

--allow-unsigned accepts requests without the platform signature.
It exists for a development platform and must never be set on an
endpoint a real workspace registered.`,
		Example: "  transpareo signer serve --platform-key platform.pem\n" +
			"  transpareo signer serve --dir /etc/transpareo/signer " +
			"--platform-key " +
			"https://acme.example.com/.well-known/transpareo-signing-key.pem " +
			"--listen :8443 --tls-cert cert.pem --tls-key key.pem",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.signerServe(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.dir, "dir", "",
		"directory of the key files (default: signer under the config dir)")
	f.StringVar(&opts.p256Key, "p256-key", "",
		"P-256 private key PEM (default: p256.pem under the signer dir)")
	f.StringVar(&opts.ed25519Key, "ed25519-key", "",
		"Ed25519 private key PEM (default: ed25519.pem under the signer dir)")
	f.StringVar(&opts.platformKey, "platform-key", "",
		"the platform's request-signing public key: a PEM file or a URL")
	f.StringVar(&opts.host, "host", "", "host name of the registered "+
		"endpoint URL, when the proxy rewrites the Host header")
	f.StringVar(&opts.listen, "listen", "127.0.0.1:8443", "address to listen on")
	f.StringVar(&opts.path, "path", "/sign", "the one route served")
	f.StringVar(&opts.tlsCert, "tls-cert", "", "TLS certificate PEM")
	f.StringVar(&opts.tlsKey, "tls-key", "", "TLS private key PEM")
	f.BoolVar(&opts.allowUnsigned, "allow-unsigned", false,
		"accept requests without the platform signature (development only)")
	return cmd
}

type serveOptions struct {
	dir                              string
	p256Key, ed25519Key, platformKey string
	host, listen, path               string
	tlsCert, tlsKey                  string
	allowUnsigned                    bool
}

// platformKeyTimeout bounds a fetch of the platform's key, which
// may run while a request waits.
const platformKeyTimeout = 5 * time.Second

// handlerTimeout bounds one request, inside the platform's
// fifteen-second budget for the whole call.
const handlerTimeout = 10 * time.Second

func (a *App) signerServe(ctx context.Context, opts serveOptions) error {
	dir := opts.dir
	if dir == "" {
		dir = a.signerDir()
	}
	if opts.p256Key == "" {
		opts.p256Key = filepath.Join(dir, signerP256File)
	}
	if opts.ed25519Key == "" {
		opts.ed25519Key = filepath.Join(dir, signerEd25519File)
	}
	if (opts.tlsCert == "") != (opts.tlsKey == "") {
		return output.Exit(output.ExitUsage,
			errors.New("--tls-cert and --tls-key go together"))
	}
	if opts.platformKey == "" && !opts.allowUnsigned {
		return output.Exit(output.ExitUsage, errors.New("--platform-key is "+
			"required; --allow-unsigned serves a development platform without it"))
	}
	keys, err := loadSignerKeys(opts)
	if err != nil {
		return output.Exit(output.ExitAPI, err)
	}
	verifier, err := a.signerVerifier(ctx, opts)
	if err != nil {
		return output.Exit(output.ExitAPI, err)
	}
	logger := slog.New(slog.NewTextHandler(a.Stderr, nil))
	handler := signer.Handler(signer.New(keys), verifier,
		signer.HandlerOptions{Path: opts.path, Logger: logger})
	server := &http.Server{
		Addr:              opts.listen,
		Handler:           http.TimeoutHandler(handler, handlerTimeout, ""),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       handlerTimeout,
		WriteTimeout:      handlerTimeout + time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	listener, err := net.Listen("tcp", opts.listen)
	if err != nil {
		return output.Exit(output.ExitAPI, err)
	}
	scheme := "http"
	if opts.tlsCert != "" {
		scheme = "https"
	}
	logger.Info("signing endpoint ready", "url",
		scheme+"://"+listener.Addr().String()+opts.path,
		"unsigned", opts.allowUnsigned)
	done := make(chan error, 1)
	go func() {
		if opts.tlsCert != "" {
			done <- server.ServeTLS(listener, opts.tlsCert, opts.tlsKey)
			return
		}
		done <- server.Serve(listener)
	}()
	select {
	case err := <-done:
		return output.Exit(output.ExitAPI, err)
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(),
			handlerTimeout)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

// loadSignerKeys reads the keys the flags name; a missing file
// says which and how to make it.
func loadSignerKeys(opts serveOptions) (*signer.Keys, error) {
	p256, err := signer.LoadP256(opts.p256Key)
	if err != nil {
		return nil, keyLoadError(err)
	}
	ed, err := signer.LoadEd25519(opts.ed25519Key)
	if err != nil {
		return nil, keyLoadError(err)
	}
	return &signer.Keys{P256: p256, Ed25519: ed}, nil
}

func keyLoadError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w; run `transpareo signer keygen` first or name "+
			"the file with --p256-key and --ed25519-key", err)
	}
	return err
}

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: platformKeyTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching the platform key: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching the platform key: %s answered %d",
			url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if err != nil {
		return nil, err
	}
	key, err := signer.ParseEd25519PublicKey(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	return key, nil
}
