package auth

import (
	"errors"
	"fmt"
	"strings"
)

// Environment variables that override the stored profile, for
// automated runs where no login has happened.
const (
	EnvHost         = "TRANSPAREO_HOST"
	EnvClientID     = "TRANSPAREO_CLIENT_ID"
	EnvClientSecret = "TRANSPAREO_CLIENT_SECRET"
	EnvToken        = "TRANSPAREO_TOKEN"
	EnvProfile      = "TRANSPAREO_PROFILE"
)

// Profile is what a run authenticates with, after the stored
// profile, the project configuration, the environment and the
// --profile option have been reconciled.
type Profile struct {
	// Name is the stored profile's name, or empty when the
	// environment supplied everything.
	Name string

	Host         string
	ClientID     string
	ClientSecret string
	Scope        []string

	// Token is a bearer token from the environment. When set, the
	// client sends it as it is and never exchanges a secret.
	Token string

	// Store names where the secret came from: keyring, file or
	// environment.
	Store string
}

// ErrNoProfile is returned when nothing names a host and a
// credential.
var ErrNoProfile = errors.New("no profile is configured")

// Resolver finds the profile for a run.
type Resolver struct {
	// Getenv reads the environment; os.Getenv outside tests.
	Getenv func(string) string

	// ConfigDir holds config.json and the file store.
	ConfigDir string

	// WorkDir is where the search for .transpareo/config.json
	// starts.
	WorkDir string

	// Stores opens the credential stores; nil uses NewStores.
	Stores *Stores
}

func (r *Resolver) stores() *Stores {
	if r.Stores == nil {
		r.Stores = NewStores(r.ConfigDir)
	}
	return r.Stores
}

// ProfileName returns the name the run would use, in order of
// precedence: the option, TRANSPAREO_PROFILE, the project
// configuration, the default profile of config.json. The second
// value says where the name came from.
func (r *Resolver) ProfileName(flag string) (string, string, error) {
	if flag != "" {
		return flag, "option", nil
	}
	if name := r.Getenv(EnvProfile); name != "" {
		return name, EnvProfile, nil
	}
	if r.WorkDir != "" {
		name, err := FindProjectProfile(r.WorkDir)
		if err != nil {
			return "", "", err
		}
		if name != "" {
			return name, "project", nil
		}
	}
	cfg, err := LoadConfig(r.ConfigDir)
	if err != nil {
		return "", "", err
	}
	if cfg.DefaultProfile != "" {
		return cfg.DefaultProfile, "default", nil
	}
	return "", "", nil
}

// Resolve returns the profile for a run. A stored profile is the
// base; TRANSPAREO_HOST, TRANSPAREO_CLIENT_ID,
// TRANSPAREO_CLIENT_SECRET and TRANSPAREO_TOKEN override its
// fields, and suffice on their own.
func (r *Resolver) Resolve(flag string) (*Profile, error) {
	name, _, err := r.ProfileName(flag)
	if err != nil {
		return nil, err
	}
	p := &Profile{}
	if name != "" {
		stored, err := r.load(name, flag != "")
		if err != nil {
			return nil, err
		}
		if stored != nil {
			p = stored
		}
	}
	r.applyEnv(p)
	if p.Host == "" {
		return nil, ErrNoProfile
	}
	if p.Token == "" && (p.ClientID == "" || p.ClientSecret == "") {
		if p.Name != "" {
			return nil, fmt.Errorf("profile %q has no stored secret; "+
				"run transpareo auth login again", p.Name)
		}
		return nil, ErrNoProfile
	}
	return p, nil
}

// load reads a stored profile. An unknown name is an error when
// it was asked for explicitly and silently absent otherwise, so
// an environment-only run is not blocked by a stale default.
func (r *Resolver) load(name string, explicit bool) (*Profile, error) {
	cfg, err := LoadConfig(r.ConfigDir)
	if err != nil {
		return nil, err
	}
	pc, ok := cfg.Profiles[name]
	if !ok {
		if explicit {
			return nil, fmt.Errorf("profile %q does not exist", name)
		}
		return nil, nil
	}
	p := &Profile{
		Name:     name,
		Host:     pc.Host,
		ClientID: pc.ClientID,
		Scope:    pc.Scope,
		Store:    pc.Store,
	}
	secret, err := r.stores().ByName(pc.Store).Get(name)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("reading the secret of profile %q: %w", name, err)
	}
	p.ClientSecret = secret
	return p, nil
}

func (r *Resolver) applyEnv(p *Profile) {
	if host := r.Getenv(EnvHost); host != "" {
		p.Host = host
	}
	if id := r.Getenv(EnvClientID); id != "" {
		p.ClientID = id
	}
	if secret := r.Getenv(EnvClientSecret); secret != "" {
		p.ClientSecret = secret
		p.Store = "environment"
	}
	if token := r.Getenv(EnvToken); token != "" {
		p.Token = token
		p.Store = "environment"
	}
}

// Save stores a profile: the secret in the store for new secrets,
// the rest in config.json. The first profile becomes the
// default.
func (r *Resolver) Save(p *Profile) error {
	if err := ValidateProfileName(p.Name); err != nil {
		return err
	}
	cfg, err := LoadConfig(r.ConfigDir)
	if err != nil {
		return err
	}
	store := r.stores().ForNewSecret()
	if err := store.Set(p.Name, p.ClientSecret); err != nil {
		return fmt.Errorf("storing the secret: %w", err)
	}
	p.Store = store.Name()
	cfg.Profiles[p.Name] = ProfileConfig{
		Host:     p.Host,
		ClientID: p.ClientID,
		Scope:    p.Scope,
		Store:    p.Store,
	}
	if cfg.DefaultProfile == "" {
		cfg.DefaultProfile = p.Name
	}
	return cfg.Save(r.ConfigDir)
}

// Delete removes a profile and its secret. Deleting the default
// profile promotes the alphabetically first remaining one.
func (r *Resolver) Delete(name string) error {
	cfg, err := LoadConfig(r.ConfigDir)
	if err != nil {
		return err
	}
	pc, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("profile %q does not exist", name)
	}
	if err := r.stores().ByName(pc.Store).Delete(name); err != nil {
		return fmt.Errorf("removing the secret: %w", err)
	}
	delete(cfg.Profiles, name)
	if cfg.DefaultProfile == name {
		cfg.DefaultProfile = firstName(cfg.Profiles)
	}
	return cfg.Save(r.ConfigDir)
}

// SetDefault makes name the default profile.
func (r *Resolver) SetDefault(name string) error {
	cfg, err := LoadConfig(r.ConfigDir)
	if err != nil {
		return err
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return fmt.Errorf("profile %q does not exist", name)
	}
	cfg.DefaultProfile = name
	return cfg.Save(r.ConfigDir)
}

func firstName(profiles map[string]ProfileConfig) string {
	first := ""
	for name := range profiles {
		if first == "" || name < first {
			first = name
		}
	}
	return first
}

// ParseScope splits a scope given as "a,b" or "a b"; nil when
// empty.
func ParseScope(s string) []string {
	scope := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' '
	})
	if len(scope) == 0 {
		return nil
	}
	return scope
}

// Host returns the host a run targets even when no credential is
// configured: the environment's, else the selected profile's.
func (r *Resolver) Host(flag string) (string, error) {
	if host := r.Getenv(EnvHost); host != "" {
		return host, nil
	}
	name, _, err := r.ProfileName(flag)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", ErrNoProfile
	}
	cfg, err := LoadConfig(r.ConfigDir)
	if err != nil {
		return "", err
	}
	pc, ok := cfg.Profiles[name]
	if !ok {
		return "", fmt.Errorf("profile %q does not exist", name)
	}
	return pc.Host, nil
}
