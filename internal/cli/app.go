// Package cli is the command tree of the transpareo binary.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/transpareo/transpareo-cli/internal/auth"
	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/internal/registry"
	"github.com/transpareo/transpareo-cli/internal/version"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
	"github.com/transpareo/transpareo-cli/spec"
)

// App holds what every command needs: the streams, the
// environment, the profile resolver and the output options.
type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string

	// Terminal says whether stdout is a terminal, which selects
	// tables over JSON. StdinTerminal says whether a secret can
	// be prompted for.
	Terminal      bool
	StdinTerminal bool

	// ConfigDir and WorkDir feed the profile resolver.
	ConfigDir string
	WorkDir   string

	// Stores replaces the credential stores, in tests.
	Stores *auth.Stores

	// HTTPClient is used for every request; nil means a default
	// client with a timeout.
	HTTPClient *http.Client

	// LookPath and Run find and start an assistant's command line
	// for setup; nil uses os/exec.
	LookPath func(string) (string, error)
	Run      func(string, ...string) error

	// Flags set from the persistent options.
	Profile  string
	ReadOnly bool
	Yes      bool
	Output   output.Options

	registryOnce sync.Once
	registry     *registry.Registry
	registryErr  error
}

// FromOS builds the App for a real process.
func FromOS() (*App, error) {
	dir, err := auth.ConfigDir(os.Getenv)
	if err != nil {
		return nil, err
	}
	workDir, _ := os.Getwd()
	return &App{
		Stdin:         os.Stdin,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
		Getenv:        os.Getenv,
		Terminal:      term.IsTerminal(int(os.Stdout.Fd())),
		StdinTerminal: term.IsTerminal(int(os.Stdin.Fd())),
		ConfigDir:     dir,
		WorkDir:       workDir,
	}, nil
}

// Main runs the command line and returns the exit code.
func Main(ctx context.Context, app *App, args []string) int {
	root := app.Root()
	root.SetArgs(args)
	root.SetIn(app.Stdin)
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return output.ExitOK
	}
	code := output.ExitCode(err)
	var exit *output.ExitError
	if errors.As(err, &exit) && exit.Err == nil {
		// The command printed its own report; only the code is left.
		return code
	}
	if code == output.ExitUsage && exit == nil {
		// A usage error from the parser: print it with the usage line.
		fmt.Fprintf(app.Stderr, "Error: %v\n", err)
		if cmd, _, findErr := root.Find(args); findErr == nil && cmd != nil {
			fmt.Fprintln(app.Stderr, cmd.UsageString())
		}
		return code
	}
	app.Printer().PrintError(err)
	return code
}

// Printer returns the printer for the current options.
func (a *App) Printer() *output.Printer {
	return &output.Printer{
		Out:      a.Stdout,
		Err:      a.Stderr,
		Terminal: a.Terminal,
		Options:  a.Output,
	}
}

// Resolver returns the profile resolver.
func (a *App) Resolver() *auth.Resolver {
	return &auth.Resolver{
		Getenv:    a.Getenv,
		ConfigDir: a.ConfigDir,
		WorkDir:   a.WorkDir,
		Stores:    a.Stores,
	}
}

// Registry loads the embedded specification once.
func (a *App) Registry() (*registry.Registry, error) {
	a.registryOnce.Do(func() {
		a.registry, a.registryErr = registry.Load(spec.JSON)
	})
	return a.registry, a.registryErr
}

func (a *App) httpClient() *http.Client {
	if a.HTTPClient != nil {
		return a.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// clientOptions wires the client to the tool: its HTTP client,
// its name, and a log on stderr for every attempt the client
// repeats, so a request that stalled and was retried leaves a
// trace instead of only a slow command.
func (a *App) clientOptions() []transpareo.Option {
	return []transpareo.Option{
		transpareo.WithHTTPClient(a.httpClient()),
		transpareo.WithUserAgent("transpareo-cli/" + version.Version),
		transpareo.WithLogger(slog.New(slog.NewTextHandler(a.Stderr, nil))),
	}
}

// Client returns an API client for the selected profile. A
// stored profile keeps its token beside its secret, so every
// invocation reuses the token the last one minted and the secret
// is exchanged once an hour, not once a command.
func (a *App) Client() (*transpareo.Client, *auth.Profile, error) {
	profile, err := a.Resolver().Resolve(a.Profile)
	if err != nil {
		return nil, nil, noProfileHint(err)
	}
	opts := a.clientOptions()
	if store := a.tokenStore(profile); store != nil {
		source, err := transpareo.NewClientCredentialsSource(profile.Host,
			credentialsOf(profile), a.httpClient())
		if err != nil {
			return nil, nil, err
		}
		opts = append(opts, transpareo.WithTokenSource(
			transpareo.NewCachedTokenSource(source, store)))
	}
	client, err := transpareo.FromResolvedProfile(profile, opts...)
	if err != nil {
		return nil, nil, err
	}
	return client, profile, nil
}

// tokenStore answers where a profile's token is kept, or nil for
// a run on environment credentials or a token, which keep
// nothing.
func (a *App) tokenStore(profile *auth.Profile) *profileTokens {
	if profile.Name == "" || profile.Token != "" ||
		profile.Store == "environment" {
		return nil
	}
	stores := a.Stores
	if stores == nil {
		stores = auth.NewStores(a.ConfigDir)
	}
	return &profileTokens{stores.TokenStore(profile.Name, profile.Store)}
}

func credentialsOf(profile *auth.Profile) transpareo.ClientCredentials {
	return transpareo.ClientCredentials{ID: profile.ClientID,
		Secret: profile.ClientSecret, Scope: profile.Scope}
}

// profileTokens adapts the profile's token store to the client.
type profileTokens struct {
	store *auth.TokenStore
}

func (p *profileTokens) Load() (*transpareo.Token, error) {
	stored, err := p.store.Load()
	if err != nil || stored == nil {
		return nil, err
	}
	return &transpareo.Token{AccessToken: stored.AccessToken,
		Scope: stored.Scope, ExpiresAt: stored.ExpiresAt}, nil
}

func (p *profileTokens) Save(token *transpareo.Token) error {
	return p.store.Save(&auth.StoredToken{AccessToken: token.AccessToken,
		Scope: token.Scope, ExpiresAt: token.ExpiresAt})
}

func (p *profileTokens) Clear() error {
	return p.store.Clear()
}

// noProfileHint turns the resolver's error into one that says
// what to do.
func noProfileHint(err error) error {
	if errors.Is(err, auth.ErrNoProfile) {
		return &transpareo.Error{
			Code:    "NO_PROFILE",
			Message: "no workspace is configured",
			Hint: "Run `transpareo auth login --host <workspace host> " +
				"--client-id <key>` or set TRANSPAREO_HOST, " +
				"TRANSPAREO_CLIENT_ID and TRANSPAREO_CLIENT_SECRET.",
		}
	}
	return err
}

// refuseUnlessAllowed applies --read-only and --yes to an
// operation before it is sent.
func (a *App) refuseUnlessAllowed(op *registry.Operation, method string) error {
	if a.ReadOnly {
		readOnly := strings.EqualFold(method, http.MethodGet)
		if op != nil {
			readOnly = op.ReadOnly()
		}
		if !readOnly {
			return output.Exit(output.ExitRefused,
				fmt.Errorf("%s is refused under --read-only", describe(op, method)))
		}
	}
	if op != nil && op.Destructive && !a.Yes {
		return output.Exit(output.ExitRefused,
			fmt.Errorf("%s cannot be undone; repeat the command with --yes",
				describe(op, method)))
	}
	return nil
}

func describe(op *registry.Operation, method string) string {
	if op == nil {
		return strings.ToUpper(method)
	}
	return op.ID
}

// addOutputFlags registers the output options every command
// accepts.
func (a *App) addOutputFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.StringVar(&a.Profile, "profile", "",
		"profile to use, as stored by auth login")
	f.BoolVar(&a.Output.JSON, "json", false,
		"print JSON (the default off a terminal)")
	f.BoolVar(&a.Output.JSONL, "jsonl", false,
		"print lists as one JSON object per line")
	f.BoolVarP(&a.Output.Quiet, "quiet", "q", false,
		"print ids only, one per line")
	f.StringSliceVar(&a.Output.Fields, "fields", nil,
		"fields to keep, comma separated")
	f.BoolVar(&a.ReadOnly, "read-only", false,
		"refuse every operation that changes data")
	f.BoolVar(&a.Yes, "yes", false, "confirm an operation that cannot be undone")
}
