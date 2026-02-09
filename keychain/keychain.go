// Package keychain implements a [vault.Store] backed by the operating
// system keychain via go-keyring. On macOS this uses Keychain, on Linux
// the Secret Service API, and on Windows the Credential Manager.
//
// An index entry is maintained alongside stored values so that [Store.List]
// works across all platforms. The index is stored under a reserved key
// within the same keyring service.
package keychain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/bjaus/vault"
	"github.com/zalando/go-keyring"
)

const (
	defaultService = "vault"
	indexKey        = "__vault_index__"
)

// Store is a [vault.Store] backed by the system keychain. It implements
// [vault.Namespaced] — calling [Store.WithNamespace] returns a store
// scoped to a different keyring service name.
type Store struct {
	service string
	mu      sync.Mutex // serializes index updates
}

// Option configures a keychain [Store].
type Option func(*Store)

// WithService overrides the default keyring service name ("vault").
func WithService(name string) Option {
	return func(s *Store) { s.service = name }
}

// New creates a keychain-backed store.
func New(opts ...Option) *Store {
	s := &Store{service: defaultService}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithNamespace returns a [vault.Store] scoped to the given namespace.
// The namespace is appended to the service name (e.g. "vault/prod").
func (s *Store) WithNamespace(ns string) vault.Store {
	return &Store{service: s.service + "/" + ns}
}

// Get retrieves an entry by key from the keychain.
func (s *Store) Get(_ context.Context, key string) (vault.Entry, error) {
	data, err := keyring.Get(s.service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return vault.Entry{}, vault.ErrNotFound
		}
		return vault.Entry{}, fmt.Errorf("keychain: get %q: %w", key, err)
	}

	var entry vault.Entry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return vault.Entry{}, fmt.Errorf("keychain: unmarshal %q: %w", key, err)
	}

	return entry, nil
}

// Set stores an entry in the keychain and updates the key index.
func (s *Store) Set(_ context.Context, entry vault.Entry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("keychain: marshal %q: %w", entry.Key, err)
	}

	if err := keyring.Set(s.service, entry.Key, string(data)); err != nil {
		return fmt.Errorf("keychain: set %q: %w", entry.Key, err)
	}

	return s.addToIndex(entry.Key)
}

// Delete removes an entry from the keychain and updates the key index.
func (s *Store) Delete(_ context.Context, key string) error {
	if err := keyring.Delete(s.service, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keychain: delete %q: %w", key, err)
	}
	return s.removeFromIndex(key)
}

// List returns all entries stored in the keychain by reading the key
// index and fetching each entry individually.
func (s *Store) List(ctx context.Context) ([]vault.Entry, error) {
	keys := s.readIndex()
	entries := make([]vault.Entry, 0, len(keys))

	for _, key := range keys {
		e, err := s.Get(ctx, key)
		if errors.Is(err, vault.ErrNotFound) {
			continue // index is stale, skip
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	return entries, nil
}

func (s *Store) addToIndex(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := s.readIndex()
	for _, k := range keys {
		if k == key {
			return nil
		}
	}

	return s.writeIndex(append(keys, key))
}

func (s *Store) removeFromIndex(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	keys := s.readIndex()
	filtered := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != key {
			filtered = append(filtered, k)
		}
	}

	return s.writeIndex(filtered)
}

func (s *Store) readIndex() []string {
	data, err := keyring.Get(s.service, indexKey)
	if err != nil {
		return nil
	}

	var keys []string
	_ = json.Unmarshal([]byte(data), &keys) //nolint:errcheck // best-effort index read
	return keys
}

func (s *Store) writeIndex(keys []string) error {
	data, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("keychain: index marshal: %w", err)
	}

	if err := keyring.Set(s.service, indexKey, string(data)); err != nil {
		return fmt.Errorf("keychain: index write: %w", err)
	}

	return nil
}
