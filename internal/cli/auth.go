package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/transpareo/transpareo-cli/internal/auth"
	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

func (a *App) authCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in, check, and hand out credentials",
	}
	cmd.AddCommand(
		a.loginCommand(),
		a.statusCommand(),
		a.logoutCommand(),
		a.tokenCommand(),
		a.grantCommand(),
	)
	return cmd
}

func (a *App) loginCommand() *cobra.Command {
	var host, clientID, scope, name string
	var setDefault bool
	cmd := &cobra.Command{
		Use:   "login --host <workspace host> --client-id <key>",
		Short: "Store a credential after checking it at the token endpoint",
		Long: `Reads the client secret from TRANSPAREO_CLIENT_SECRET, from
standard input when it is not a terminal, or from a prompt; never
from an option, so it stays out of the shell history. The
credential is checked at the token endpoint and stored in the
operating system's keyring, or in a file readable only by you
when no keyring is available.`,
		Example: `  echo "$SECRET" | transpareo auth login \
      --host acme.example.com --client-id 3f6a...
  transpareo auth login --host acme.example.com --client-id 3f6a... \
      --scope dpp_read,dpp_write --name acme-read`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.login(cmd.Context(), host, clientID, scope, name, setDefault)
		},
	}
	f := cmd.Flags()
	f.StringVar(&host, "host", "", "workspace host, such as acme.example.com")
	f.StringVar(&clientID, "client-id", "", "the consumer's key")
	f.StringVar(&scope, "scope", "",
		"permission keys to narrow tokens to, comma separated")
	f.StringVar(&name, "name", "", "profile name (default: the host)")
	f.BoolVar(&setDefault, "default", false, "make this the default profile")
	cmd.MarkFlagRequired("host")
	cmd.MarkFlagRequired("client-id")
	return cmd
}

func (a *App) login(ctx context.Context, host, clientID, scope, name string,
	setDefault bool) error {
	secret, err := a.readSecret()
	if err != nil {
		return err
	}
	if name == "" {
		name = profileNameFor(host)
	}
	if err := auth.ValidateProfileName(name); err != nil {
		return output.Exit(output.ExitUsage, err)
	}
	profile := &auth.Profile{
		Name:         name,
		Host:         host,
		ClientID:     clientID,
		ClientSecret: secret,
		Scope:        auth.ParseScope(scope),
	}
	client, err := transpareo.FromResolvedProfile(profile, a.clientOptions()...)
	if err != nil {
		return err
	}
	me, err := client.Me(ctx)
	if err != nil {
		return err
	}
	resolver := a.Resolver()
	if err := resolver.Save(profile); err != nil {
		return err
	}
	if setDefault {
		if err := resolver.SetDefault(name); err != nil {
			return err
		}
	}
	printer := a.Printer()
	printer.Message("Logged in to %s as %q; profile %q stored in the %s.",
		client.Host(), me.Name, name, storeLabel(profile.Store))
	return printer.Print(me)
}

// profileNameFor derives a profile name from a host: the first
// label of the host name, or the whole address of an IP.
func profileNameFor(host string) string {
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	host, _, _ = strings.Cut(host, "/")
	host, _, _ = strings.Cut(host, ":")
	name, _, _ := strings.Cut(host, ".")
	if strings.Trim(name, "0123456789") == "" {
		return host
	}
	return name
}

func storeLabel(store string) string {
	switch store {
	case "keyring":
		return "system keyring"
	case "file":
		return "credentials file"
	default:
		return store + " store"
	}
}

// readSecret takes the secret from the environment, from piped
// standard input, or from a prompt that does not echo.
func (a *App) readSecret() (string, error) {
	if secret := a.Getenv(auth.EnvClientSecret); secret != "" {
		return secret, nil
	}
	if !a.StdinTerminal {
		data, err := io.ReadAll(io.LimitReader(a.Stdin, 64<<10))
		if err != nil {
			return "", err
		}
		secret := strings.TrimSpace(string(data))
		if secret == "" {
			return "", output.Exit(output.ExitUsage, errors.New(
				"no secret: pipe it into standard input or set TRANSPAREO_CLIENT_SECRET"))
		}
		return secret, nil
	}
	fmt.Fprint(a.Stderr, "Client secret: ")
	file, ok := a.Stdin.(*os.File)
	if !ok {
		reader := bufio.NewReader(a.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
	data, err := term.ReadPassword(int(file.Fd()))
	fmt.Fprintln(a.Stderr)
	if err != nil {
		return "", err
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", output.Exit(output.ExitUsage, errors.New("no secret was entered"))
	}
	return secret, nil
}

func (a *App) statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "Show what the current credential allows (GET /me)",
		Example: "  transpareo auth status\n  transpareo auth status --profile acme",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.status(cmd.Context())
		},
	}
}

func (a *App) meCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "me",
		Short:   "Show what the current credential allows (GET /me)",
		Example: "  transpareo me\n  transpareo me --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.status(cmd.Context())
		},
	}
}

func (a *App) status(ctx context.Context) error {
	client, profile, err := a.Client()
	if err != nil {
		return err
	}
	me, err := client.Me(ctx)
	if err != nil {
		return err
	}
	printer := a.Printer()
	if !printer.JSONMode() {
		printer.Message("%s on %s (%s)", profileLabel(profile), client.Host(),
			storeLabel(profile.Store))
	}
	return printer.Print(me)
}

func profileLabel(p *auth.Profile) string {
	if p.Name == "" {
		return "Environment credentials"
	}
	return "Profile " + p.Name
}

func (a *App) logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Short:   "Remove the stored profile and its secret",
		Example: "  transpareo auth logout\n  transpareo auth logout --profile acme",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolver := a.Resolver()
			name, _, err := resolver.ProfileName(a.Profile)
			if err != nil {
				return err
			}
			if name == "" {
				return output.Exit(output.ExitUsage, errors.New("no profile is selected"))
			}
			if err := resolver.Delete(name); err != nil {
				return err
			}
			a.Printer().Message("Profile %q removed.", name)
			return nil
		},
	}
}

func (a *App) tokenCommand() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "token [--scope a,b]",
		Short: "Print a fresh short-lived bearer token for scripts and subagents",
		Long: `Exchanges the stored credential for a token and prints it, so a
script or a subprocess can call the API with TRANSPAREO_TOKEN and
never sees the secret. With --json the expiry and scope come too.`,
		Example: "  export TRANSPAREO_TOKEN=$(transpareo auth token " +
			"--scope dpp_read)",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.token(cmd.Context(), scope)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "",
		"permission keys to narrow the token to, comma separated")
	return cmd
}

func (a *App) token(ctx context.Context, scope string) error {
	profile, err := a.Resolver().Resolve(a.Profile)
	if err != nil {
		return noProfileHint(err)
	}
	if profile.Token != "" {
		return output.Exit(output.ExitUsage, errors.New(
			"TRANSPAREO_TOKEN is set; a token cannot mint another token"))
	}
	if scope != "" {
		profile.Scope = auth.ParseScope(scope)
	}
	client, err := transpareo.FromResolvedProfile(profile, a.clientOptions()...)
	if err != nil {
		return err
	}
	token, err := client.TokenSource().Token(ctx)
	if err != nil {
		return err
	}
	if !a.Output.JSON {
		_, err := fmt.Fprintln(a.Stdout, token.AccessToken)
		return err
	}
	return a.Printer().Print(map[string]any{
		"accessToken": token.AccessToken,
		"tokenType":   "Bearer",
		"expiresAt":   token.ExpiresAt.UTC().Format(time.RFC3339),
		"scope":       token.Scope,
		"host":        client.Host(),
	})
}

func (a *App) grantCommand() *cobra.Command {
	var in transpareo.GrantInput
	cmd := &cobra.Command{
		Use:   "grant --code <passport code>",
		Short: "Issue a child credential scoped to one passport (POST /grant)",
		Long: `Turns a scanned passport code into the client credentials of a
child consumer that may write events to that one passport for 24
hours. The secret is printed once and never again.`,
		Example: `  transpareo auth grant --code A1B2C3D4E
  transpareo auth grant --gtin 04012345678901 --serial 000412`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if in.DppCode == "" && in.GTIN == "" {
				return output.Exit(output.ExitUsage,
					errors.New("name the passport with --code or --gtin"))
			}
			reg, err := a.Registry()
			if err != nil {
				return err
			}
			op := reg.Find("create_grant")
			if err := a.refuseUnlessAllowed(op, "POST"); err != nil {
				return err
			}
			client, _, err := a.Client()
			if err != nil {
				return err
			}
			grant, err := client.CreateGrant(cmd.Context(), in)
			if err != nil {
				return err
			}
			return a.Printer().Print(grant)
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.DppCode, "code", "",
		"the passport code printed on the QR code")
	f.StringVar(&in.GTIN, "gtin", "",
		"GTIN of the product, for a lookup by GS1 identifiers")
	f.StringVar(&in.Batch, "batch", "", "batch identifier, with --gtin")
	f.StringVar(&in.Serial, "serial", "", "serial identifier, with --gtin")
	return cmd
}
