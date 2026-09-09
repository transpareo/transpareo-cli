package transpareo

import (
	"os"

	"github.com/transpareo/transpareo-cli/internal/auth"
)

// FromProfile returns a client for a profile stored by
// `transpareo auth login`. An empty name selects the profile the
// command-line tool would use: TRANSPAREO_PROFILE, the project's
// .transpareo/config.json, or the default profile. The
// environment variables TRANSPAREO_HOST, TRANSPAREO_CLIENT_ID,
// TRANSPAREO_CLIENT_SECRET and TRANSPAREO_TOKEN override the
// stored values.
func FromProfile(name string, opts ...Option) (*Client, error) {
	dir, err := auth.ConfigDir(os.Getenv)
	if err != nil {
		return nil, err
	}
	workDir, _ := os.Getwd()
	resolver := &auth.Resolver{Getenv: os.Getenv, ConfigDir: dir, WorkDir: workDir}
	profile, err := resolver.Resolve(name)
	if err != nil {
		return nil, err
	}
	return FromResolvedProfile(profile, opts...)
}

// FromResolvedProfile builds a client from a profile the auth
// package resolved.
func FromResolvedProfile(p *auth.Profile, opts ...Option) (*Client, error) {
	if p.Token != "" {
		return NewWithToken(p.Host, p.Token, opts...)
	}
	creds := ClientCredentials{
		ID:     p.ClientID,
		Secret: p.ClientSecret,
		Scope:  p.Scope,
	}
	return New(p.Host, creds, opts...)
}
