package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/auth"
	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/internal/version"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
	"github.com/transpareo/transpareo-cli/spec"
)

// Check is one line of the doctor's report.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Report is the doctor's whole report.
type Report struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}

const (
	statusOK    = "ok"
	statusWarn  = "warn"
	statusError = "error"
	statusSkip  = "skipped"
)

func (a *App) doctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the setup: profile, host, token endpoint, credential, keyring",
		Long: `Runs the checks a support request would ask for: which profile is
in use and where its secret lives, whether the host answers and
which specification version it serves against the one built in,
whether the token endpoint answers, whether the credential is
valid, and whether a keyring is available. Exits with 1 when a
check fails.`,
		Example: "  transpareo doctor\n  transpareo doctor --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := a.doctor(cmd.Context())
			printer := a.Printer()
			if printer.JSONMode() {
				if err := printer.Print(report); err != nil {
					return err
				}
			} else {
				for _, check := range report.Checks {
					fmt.Fprintf(a.Stdout, "%-8s %-16s %s\n",
						check.Status, check.Name, check.Detail)
				}
			}
			if !report.OK {
				return output.Exit(output.ExitAPI, nil)
			}
			return nil
		},
	}
}

func (a *App) doctor(ctx context.Context) *Report {
	report := &Report{OK: true}
	add := func(name, status, detail string) {
		check := Check{Name: name, Status: status, Detail: detail}
		report.Checks = append(report.Checks, check)
		if status == statusError {
			report.OK = false
		}
	}
	add("binary", statusOK, fmt.Sprintf("transpareo %s, specification %s; "+
		"transpareo upgrade --verify checks the signature of the binary",
		version.String(), spec.Version()))

	resolver := a.Resolver()
	name, source, err := resolver.ProfileName(a.Profile)
	profile, resolveErr := resolver.Resolve(a.Profile)
	switch {
	case err != nil:
		add("profile", statusError, err.Error())
	case resolveErr != nil && errors.Is(resolveErr, auth.ErrNoProfile):
		add("profile", statusError, "none configured; run `transpareo auth login`")
	case resolveErr != nil:
		add("profile", statusError, resolveErr.Error())
	case profile.Name == "":
		add("profile", statusOK, "credentials from the environment")
	default:
		add("profile", statusOK, fmt.Sprintf("%s (%s), secret in the %s",
			name, source, storeLabel(profile.Store)))
	}

	host, hostErr := resolver.Host(a.Profile)
	if hostErr != nil {
		add("host", statusSkip, "no host to check")
		add("token endpoint", statusSkip, "no host to check")
	} else {
		a.checkHost(ctx, host, add)
		a.checkTokenEndpoint(ctx, host, add)
	}

	if resolveErr != nil {
		add("credential", statusSkip, "no credential to check")
	} else {
		a.checkCredential(ctx, profile, add)
	}

	stores := a.Stores
	if stores == nil {
		stores = auth.NewStores(a.ConfigDir)
	}
	if stores.ForNewSecret().Name() == "keyring" {
		add("keyring", statusOK, "available; new logins are stored there")
	} else {
		add("keyring", statusWarn, "not available; new logins go to "+
			"credentials.json, readable only by you")
	}
	return report
}

// addCheck records one line of the report.
type addCheck func(name, status, detail string)

func (a *App) checkHost(ctx context.Context, host string, add addCheck) {
	data, err := a.fetchPublic(ctx, host, "/apidocs/openapi.json",
		"application/json")
	if err != nil {
		add("host", statusError, fmt.Sprintf("%s: %v", host, err))
		return
	}
	var doc struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.Info.Version == "" {
		add("host", statusError, host+" answered, but not with an API specification")
		return
	}
	live, builtIn := doc.Info.Version, spec.Version()
	status, detail := compareVersions(builtIn, live)
	add("host", status, fmt.Sprintf("%s serves specification %s; %s",
		host, live, detail))
}

// compareVersions rates the live specification against the one
// built in: a newer major version is an error, a newer minor one
// a warning, everything else fine.
func compareVersions(builtIn, live string) (string, string) {
	b, okB := parseVersion(builtIn)
	l, okL := parseVersion(live)
	switch {
	case !okB || !okL:
		return statusWarn, "the versions cannot be compared"
	case l[0] > b[0]:
		return statusError, fmt.Sprintf(
			"this binary was built for %s and needs an upgrade", builtIn)
	case l[0] < b[0]:
		return statusWarn, fmt.Sprintf(
			"this binary was built for the newer %s", builtIn)
	case l[1] > b[1]:
		return statusWarn, fmt.Sprintf("this binary was built for %s; "+
			"newer operations are reachable with `transpareo api`", builtIn)
	default:
		return statusOK, "the built-in specification is current"
	}
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) < 2 {
		return out, false
	}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func (a *App) checkTokenEndpoint(ctx context.Context, host string,
	add addCheck) {
	data, err := a.fetchPublic(ctx, host,
		"/.well-known/oauth-authorization-server", "application/json")
	if err != nil {
		add("token endpoint", statusError, err.Error())
		return
	}
	var meta struct {
		TokenEndpoint string   `json:"token_endpoint"`
		GrantTypes    []string `json:"grant_types_supported"`
	}
	if err := json.Unmarshal(data, &meta); err != nil || meta.TokenEndpoint == "" {
		add("token endpoint", statusError,
			"the authorization server metadata is missing")
		return
	}
	add("token endpoint", statusOK, meta.TokenEndpoint)
}

func (a *App) checkCredential(ctx context.Context, profile *auth.Profile,
	add addCheck) {
	client, err := transpareo.FromResolvedProfile(profile, a.clientOptions()...)
	if err != nil {
		add("credential", statusError, err.Error())
		return
	}
	me, err := client.Me(ctx)
	if err != nil {
		add("credential", statusError, strings.ReplaceAll(err.Error(), "\n", "; "))
		return
	}
	until := me.TokenExpiresAt.UTC().Format("2006-01-02 15:04 UTC")
	add("credential", statusOK, fmt.Sprintf(
		"%q (%s), scope %s, token valid until %s",
		me.Name, me.Status, strings.Join(me.Scope, " "), until))
}
