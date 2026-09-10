package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

// ErrNotFound is returned when a profile has no stored secret.
var ErrNotFound = errors.New("no stored secret")

// keyringService is the service name secrets are filed under in
// the operating system's keyring.
const keyringService = "transpareo"

// Store keeps one secret per profile.
type Store interface {
	// Name says which kind of store this is: keyring, file or
	// memory. It is recorded in the profile so later runs read
	// from the same place.
	Name() string
	Get(profile string) (string, error)
	Set(profile, secret string) error
	Delete(profile string) error
}

// KeyringStore uses the macOS Keychain, the Windows Credential
// Manager or the Linux Secret Service, none of which needs a
// native build step.
type KeyringStore struct{}

func (KeyringStore) Name() string { return "keyring" }

func (KeyringStore) Get(profile string) (string, error) {
	secret, err := keyring.Get(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return secret, err
}

func (KeyringStore) Set(profile, secret string) error {
	return keyring.Set(keyringService, profile, secret)
}

func (KeyringStore) Delete(profile string) error {
	err := keyring.Delete(keyringService, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Available reports whether the keyring answers at all, by
// asking for an entry that does not exist.
func (KeyringStore) Available() bool {
	_, err := keyring.Get(keyringService, "availability-probe")
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}

// FileStore keeps secrets in credentials.json, readable by its
// owner only. It is the fallback on hosts without a keyring:
// servers without a desktop session, containers, pipelines.
type FileStore struct {
	Path string
}

// NewFileStore returns the store for credentials.json in dir.
func NewFileStore(dir string) *FileStore {
	return &FileStore{Path: filepath.Join(dir, credentialsFile)}
}

type credentialsDoc struct {
	Profiles map[string]credentialEntry `json:"profiles"`
}

type credentialEntry struct {
	ClientSecret string `json:"clientSecret"`
}

func (*FileStore) Name() string { return "file" }

func (s *FileStore) load() (*credentialsDoc, error) {
	doc := &credentialsDoc{Profiles: map[string]credentialEntry{}}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	if doc.Profiles == nil {
		doc.Profiles = map[string]credentialEntry{}
	}
	return doc, nil
}

func (s *FileStore) Get(profile string) (string, error) {
	doc, err := s.load()
	if err != nil {
		return "", err
	}
	entry, ok := doc.Profiles[profile]
	if !ok {
		return "", ErrNotFound
	}
	return entry.ClientSecret, nil
}

func (s *FileStore) Set(profile, secret string) error {
	doc, err := s.load()
	if err != nil {
		return err
	}
	doc.Profiles[profile] = credentialEntry{ClientSecret: secret}
	return writePrivate(s.Path, doc)
}

func (s *FileStore) Delete(profile string) error {
	doc, err := s.load()
	if err != nil {
		return err
	}
	delete(doc.Profiles, profile)
	return writePrivate(s.Path, doc)
}

// MemoryStore keeps secrets for the length of the process. Tests
// use it in place of the keyring.
type MemoryStore struct {
	mu      sync.Mutex
	secrets map[string]string
}

func (*MemoryStore) Name() string { return "memory" }

func (m *MemoryStore) Get(profile string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	secret, ok := m.secrets[profile]
	if !ok {
		return "", ErrNotFound
	}
	return secret, nil
}

func (m *MemoryStore) Set(profile, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.secrets == nil {
		m.secrets = map[string]string{}
	}
	m.secrets[profile] = secret
	return nil
}

func (m *MemoryStore) Delete(profile string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.secrets, profile)
	return nil
}

// Stores chooses the credential store: the keyring when it
// answers, else the file beside the configuration.
type Stores struct {
	Dir     string
	Keyring Store
	File    Store

	// KeyringAvailable is consulted before the keyring is used;
	// nil means the keyring is not used.
	KeyringAvailable func() bool
}

// NewStores returns the stores for the configuration directory.
func NewStores(dir string) *Stores {
	kr := KeyringStore{}
	return &Stores{
		Dir:              dir,
		Keyring:          kr,
		File:             NewFileStore(dir),
		KeyringAvailable: kr.Available,
	}
}

// ForNewSecret returns the store a new secret goes to.
func (s *Stores) ForNewSecret() Store {
	if s.Keyring != nil && s.KeyringAvailable != nil && s.KeyringAvailable() {
		return s.Keyring
	}
	return s.File
}

// tokenSuffix marks the entry that holds a profile's current
// token, filed in the same store as its secret because a bearer
// token opens the same doors for an hour.
const tokenSuffix = "/token"

// StoredToken is a profile's current token as the store keeps it.
type StoredToken struct {
	AccessToken string    `json:"accessToken"`
	Scope       []string  `json:"scope,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// TokenStore reads and writes the token of one profile.
type TokenStore struct {
	store   Store
	profile string
}

// TokenStore returns the token store of a profile whose secret
// is in the named store.
func (s *Stores) TokenStore(profile, storeName string) *TokenStore {
	return &TokenStore{store: s.ByName(storeName), profile: profile}
}

func (t *TokenStore) key() string { return t.profile + tokenSuffix }

// Load answers the stored token, or nil when there is none.
func (t *TokenStore) Load() (*StoredToken, error) {
	data, err := t.store.Get(t.key())
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the token of profile %q: %w",
			t.profile, err)
	}
	var token StoredToken
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return nil, fmt.Errorf("the stored token of profile %q is "+
			"unreadable; run transpareo auth logout and login again", t.profile)
	}
	return &token, nil
}

func (t *TokenStore) Save(token *StoredToken) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	if err := t.store.Set(t.key(), string(data)); err != nil {
		return fmt.Errorf("storing the token of profile %q: %w", t.profile,
			err)
	}
	return nil
}

func (t *TokenStore) Clear() error {
	if err := t.store.Delete(t.key()); err != nil {
		return fmt.Errorf("removing the token of profile %q: %w", t.profile,
			err)
	}
	return nil
}

// ByName returns the store a profile recorded, or the file store
// for a profile written by hand without one.
func (s *Stores) ByName(name string) Store {
	switch name {
	case "keyring":
		if s.Keyring != nil {
			return s.Keyring
		}
	case "memory":
		if s.Keyring != nil && s.Keyring.Name() == "memory" {
			return s.Keyring
		}
	}
	return s.File
}
