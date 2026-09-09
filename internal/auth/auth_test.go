package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func newResolver(t *testing.T, values map[string]string) (*Resolver,
	*MemoryStore) {
	t.Helper()
	dir := t.TempDir()
	mem := &MemoryStore{}
	stores := &Stores{Dir: dir, Keyring: mem, File: NewFileStore(dir),
		KeyringAvailable: func() bool { return true }}
	return &Resolver{Getenv: env(values), ConfigDir: dir, Stores: stores}, mem
}

func TestSaveAndResolveProfile(t *testing.T) {
	r, mem := newResolver(t, nil)
	err := r.Save(&Profile{Name: "acme", Host: "acme.example.com",
		ClientID: "id", ClientSecret: "s3cret", Scope: []string{"dpp_read"}})
	if err != nil {
		t.Fatal(err)
	}
	if secret, _ := mem.Get("acme"); secret != "s3cret" {
		t.Errorf("secret in memory store = %q", secret)
	}
	data, _ := os.ReadFile(filepath.Join(r.ConfigDir, configFile))
	if string(data) == "" || strings.Contains(string(data), "s3cret") {
		t.Errorf("config.json = %s", data)
	}
	p, err := r.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "acme" || p.Host != "acme.example.com" ||
		p.ClientSecret != "s3cret" ||
		p.Store != "memory" || len(p.Scope) != 1 {
		t.Errorf("profile = %+v", p)
	}
}

func TestFirstProfileBecomesDefault(t *testing.T) {
	r, _ := newResolver(t, nil)
	r.Save(&Profile{Name: "one", Host: "one.example.com", ClientID: "a",
		ClientSecret: "x"})
	r.Save(&Profile{Name: "two", Host: "two.example.com", ClientID: "b",
		ClientSecret: "y"})
	p, err := r.Resolve("")
	if err != nil || p.Name != "one" {
		t.Fatalf("default = %+v, err = %v", p, err)
	}
	if err := r.SetDefault("two"); err != nil {
		t.Fatal(err)
	}
	if p, _ := r.Resolve(""); p.Name != "two" {
		t.Errorf("default after SetDefault = %q", p.Name)
	}
	if err := r.SetDefault("nope"); err == nil {
		t.Error("an unknown default must be refused")
	}
}

func TestProfileOptionAndEnvironmentSelect(t *testing.T) {
	values := map[string]string{}
	r, _ := newResolver(t, values)
	r.Save(&Profile{Name: "one", Host: "one.example.com", ClientID: "a",
		ClientSecret: "x"})
	r.Save(&Profile{Name: "two", Host: "two.example.com", ClientID: "b",
		ClientSecret: "y"})
	if p, _ := r.Resolve("two"); p.Host != "two.example.com" {
		t.Errorf("--profile two gave %+v", p)
	}
	values[EnvProfile] = "two"
	if p, _ := r.Resolve(""); p.Host != "two.example.com" {
		t.Errorf("TRANSPAREO_PROFILE gave %+v", p)
	}
	if _, err := r.Resolve("three"); err == nil {
		t.Error("an unknown --profile must fail")
	}
}

func TestProjectConfigNamesProfile(t *testing.T) {
	r, _ := newResolver(t, nil)
	r.Save(&Profile{Name: "one", Host: "one.example.com", ClientID: "a",
		ClientSecret: "x"})
	r.Save(&Profile{Name: "two", Host: "two.example.com", ClientID: "b",
		ClientSecret: "y"})
	project := t.TempDir()
	os.MkdirAll(filepath.Join(project, projectDir), 0o755)
	os.WriteFile(filepath.Join(project, projectDir, configFile),
		[]byte(`{"profile": "two"}`), 0o644)
	nested := filepath.Join(project, "a", "b")
	os.MkdirAll(nested, 0o755)
	r.WorkDir = nested
	p, err := r.Resolve("")
	if err != nil || p.Name != "two" {
		t.Errorf("project profile = %+v, err = %v", p, err)
	}
	name, source, _ := r.ProfileName("")
	if name != "two" || source != "project" {
		t.Errorf("name = %q from %q", name, source)
	}
}

func TestEnvironmentOverridesAndSuffices(t *testing.T) {
	values := map[string]string{
		EnvHost: "env.example.com", EnvClientID: "envid",
		EnvClientSecret: "envsecret",
	}
	r, _ := newResolver(t, values)
	p, err := r.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "" || p.Host != "env.example.com" || p.ClientID != "envid" ||
		p.ClientSecret != "envsecret" || p.Store != "environment" {
		t.Errorf("profile = %+v", p)
	}

	r.Save(&Profile{Name: "one", Host: "one.example.com", ClientID: "a",
		ClientSecret: "x"})
	delete(values, EnvClientID)
	delete(values, EnvClientSecret)
	p, _ = r.Resolve("")
	if p.Name != "one" || p.Host != "env.example.com" || p.ClientSecret != "x" {
		t.Errorf("host override = %+v", p)
	}

	values[EnvToken] = "tok"
	p, _ = r.Resolve("")
	if p.Token != "tok" || p.Store != "environment" {
		t.Errorf("token = %+v", p)
	}
}

func TestTokenAloneNeedsAHost(t *testing.T) {
	r, _ := newResolver(t, map[string]string{EnvToken: "tok"})
	if _, err := r.Resolve(""); !errors.Is(err, ErrNoProfile) {
		t.Errorf("err = %v", err)
	}
	r, _ = newResolver(t, map[string]string{EnvToken: "tok",
		EnvHost: "h.example.com"})
	if p, err := r.Resolve(""); err != nil || p.Token != "tok" {
		t.Errorf("profile = %+v, err = %v", p, err)
	}
}

func TestNothingConfigured(t *testing.T) {
	r, _ := newResolver(t, nil)
	if _, err := r.Resolve(""); !errors.Is(err, ErrNoProfile) {
		t.Errorf("err = %v", err)
	}
}

func TestMissingSecretIsReported(t *testing.T) {
	r, mem := newResolver(t, nil)
	r.Save(&Profile{Name: "one", Host: "one.example.com", ClientID: "a",
		ClientSecret: "x"})
	mem.Delete("one")
	_, err := r.Resolve("")
	if err == nil || !strings.Contains(err.Error(), "no stored secret") {
		t.Errorf("err = %v", err)
	}
}

func TestDeleteProfile(t *testing.T) {
	r, mem := newResolver(t, nil)
	r.Save(&Profile{Name: "b", Host: "b.example.com", ClientID: "a",
		ClientSecret: "x"})
	r.Save(&Profile{Name: "a", Host: "a.example.com", ClientID: "a",
		ClientSecret: "y"})
	if err := r.Delete("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get("b"); !errors.Is(err, ErrNotFound) {
		t.Error("the secret must be gone")
	}
	cfg, _ := LoadConfig(r.ConfigDir)
	if cfg.DefaultProfile != "a" || len(cfg.Profiles) != 1 {
		t.Errorf("config = %+v", cfg)
	}
	if err := r.Delete("b"); err == nil {
		t.Error("deleting twice must fail")
	}
}

func TestFileStoreFallback(t *testing.T) {
	dir := t.TempDir()
	stores := &Stores{Dir: dir, Keyring: KeyringStore{},
		File:             NewFileStore(dir),
		KeyringAvailable: func() bool { return false }}
	r := &Resolver{Getenv: env(nil), ConfigDir: dir, Stores: stores}
	if err := r.Save(&Profile{Name: "one", Host: "h", ClientID: "a",
		ClientSecret: "x"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, credentialsFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credentials.json mode = %o", perm)
	}
	p, err := r.Resolve("")
	if err != nil || p.ClientSecret != "x" || p.Store != "file" {
		t.Errorf("profile = %+v, err = %v", p, err)
	}
	if err := r.Delete("one"); err != nil {
		t.Fatal(err)
	}
	if _, err := stores.File.Get("one"); !errors.Is(err, ErrNotFound) {
		t.Error("the file entry must be gone")
	}
}

func TestStoresByName(t *testing.T) {
	dir := t.TempDir()
	mem := &MemoryStore{}
	s := &Stores{Dir: dir, Keyring: mem, File: NewFileStore(dir)}
	if s.ByName("memory") != mem || s.ByName("file").Name() != "file" ||
		s.ByName("").Name() != "file" {
		t.Error("store lookup by name is wrong")
	}
	s = &Stores{Dir: dir, Keyring: KeyringStore{}, File: NewFileStore(dir)}
	if s.ByName("keyring").Name() != "keyring" {
		t.Error("keyring lookup is wrong")
	}
}

func TestConfigDir(t *testing.T) {
	dir, _ := ConfigDir(env(map[string]string{"TRANSPAREO_CONFIG_DIR": "/x"}))
	if dir != "/x" {
		t.Errorf("dir = %q", dir)
	}
	dir, _ = ConfigDir(env(map[string]string{"XDG_CONFIG_HOME": "/xdg"}))
	if dir != filepath.Join("/xdg", "transpareo") {
		t.Errorf("dir = %q", dir)
	}
	dir, _ = ConfigDir(env(nil))
	if filepath.Base(dir) != "transpareo" ||
		filepath.Base(filepath.Dir(dir)) != ".config" {
		t.Errorf("dir = %q", dir)
	}
}

func TestValidateProfileName(t *testing.T) {
	for _, ok := range []string{"acme", "acme-prod", "a.b_c", "A1"} {
		if err := ValidateProfileName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".hidden", "a/b", "with space", "-x"} {
		if err := ValidateProfileName(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestParseScope(t *testing.T) {
	got := ParseScope("dpp_read, dpp_write product_access")
	if len(got) != 3 || got[2] != "product_access" {
		t.Errorf("scope = %v", got)
	}
	if ParseScope("") != nil {
		t.Error("empty scope must be nil")
	}
}

func TestBrokenConfigIsReported(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, configFile), []byte("{oops"), 0o600)
	if _, err := LoadConfig(dir); err == nil {
		t.Error("invalid JSON must be reported")
	}
}

func TestHostWithoutCredentials(t *testing.T) {
	values := map[string]string{}
	r, mem := newResolver(t, values)
	if _, err := r.Host(""); !errors.Is(err, ErrNoProfile) {
		t.Errorf("err = %v", err)
	}
	r.Save(&Profile{Name: "one", Host: "one.example.com", ClientID: "a",
		ClientSecret: "x"})
	mem.Delete("one")
	if host, err := r.Host(""); err != nil || host != "one.example.com" {
		t.Errorf("host = %q, err = %v", host, err)
	}
	values[EnvHost] = "env.example.com"
	if host, _ := r.Host(""); host != "env.example.com" {
		t.Errorf("host = %q", host)
	}
	delete(values, EnvHost)
	if _, err := r.Host("nope"); err == nil {
		t.Error("unknown profile must fail")
	}
}
