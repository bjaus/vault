// Package vault is a pluggable configuration and secret store.
//
// Vault decouples where configuration comes from ([Source]) and where it
// lives locally ([Store]). Sources are read-only providers that fetch
// entries from external systems. Stores are read-write backends that
// persist entries locally for fast access.
//
// # Core Interfaces
//
// Two pluggable interfaces define the contract:
//
//   - [Store] — persists entries locally (read-write)
//   - [Source] — fetches entries from an external system (read-only)
//
// [SourceFunc] adapts a plain function into a [Source].
//
// # Namespace Support
//
// Store implementations that support scoping implement [Namespaced].
// When a [Vault] is configured with a namespace and its store implements
// [Namespaced], the vault automatically scopes all operations.
//
// # Resolution Flow
//
// When [Vault.Get] is called:
//
//  1. Check the store for the key
//  2. If found and not expired (when TTL is configured), return it
//  3. If missing or expired, auto-refresh from all sources (at most once per TTL period)
//  4. Check the store again
//
// Explicit [Vault.Refresh] is always available regardless of TTL.
//
// # Usage
//
//	v := vault.New(
//	    vault.WithStore(keychain.New()),
//	    vault.WithSource(mySSMSource),
//	    vault.WithNamespace("prod"),
//	    vault.WithTTL(7 * 24 * time.Hour),
//	)
//
//	entry, err := v.Get(ctx, "db-password")
package vault

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrNotFound is returned when an entry does not exist in the store.
var ErrNotFound = errors.New("vault: not found")

// Entry is a configuration or secret value.
type Entry struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
}

// Store persists entries locally. Implementations must be safe for
// concurrent use.
type Store interface {
	Get(ctx context.Context, key string) (Entry, error)
	Set(ctx context.Context, entry Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context) ([]Entry, error)
}

// Namespaced is an optional interface for stores that support scoping.
// [Store] implementations that support namespacing implement this
// interface. When implemented, [Namespaced.WithNamespace] returns a
// [Store] whose operations are scoped to the given namespace. The
// returned store shares the same backing data as the original.
type Namespaced interface {
	WithNamespace(namespace string) Store
}

// Source fetches entries from an external system. Implementations
// are read-only providers — they produce entries but do not store them.
type Source interface {
	Fetch(ctx context.Context) ([]Entry, error)
}

// SourceFunc adapts a plain function into a [Source].
type SourceFunc func(ctx context.Context) ([]Entry, error)

// Fetch calls the underlying function.
func (f SourceFunc) Fetch(ctx context.Context) ([]Entry, error) { return f(ctx) }

// Vault is a [Store] that resolves entries from external [Source]
// providers and caches them in a local [Store]. Use [New] to create one.
type Vault interface {
	Store
	Refresh(ctx context.Context) error
}

// New creates a [Vault] with the given options.
// If no store is provided, an in-memory store is used.
func New(opts ...Option) Vault {
	cfg := &config{
		store: NewMemory(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	store := cfg.store
	if cfg.namespace != "" {
		if ns, ok := store.(Namespaced); ok {
			store = ns.WithNamespace(cfg.namespace)
		}
	}

	return &vault{
		store:   store,
		sources: cfg.sources,
		ttl:     cfg.ttl,
	}
}

type vault struct {
	store   Store
	sources []Source
	ttl     time.Duration

	mu          sync.Mutex
	lastRefresh time.Time
}

// Get retrieves an entry by key. If the entry is missing or expired and
// sources are configured, an automatic refresh is attempted at most once
// per TTL period.
func (v *vault) Get(ctx context.Context, key string) (Entry, error) {
	e, err := v.store.Get(ctx, key)
	if err == nil && !v.expired(e) {
		return e, nil
	}

	miss := errors.Is(err, ErrNotFound) || (err == nil && v.expired(e))
	if !miss {
		return Entry{}, err
	}

	if !v.shouldAutoRefresh() {
		return Entry{}, ErrNotFound
	}

	if rerr := v.Refresh(ctx); rerr != nil {
		return Entry{}, rerr
	}

	return v.store.Get(ctx, key)
}

// Set stores an entry directly. If [Entry.CreatedAt] is zero it is set
// to the current time. If [Entry.Source] is empty it defaults to "manual".
func (v *vault) Set(ctx context.Context, entry Entry) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.Source == "" {
		entry.Source = "manual"
	}
	return v.store.Set(ctx, entry)
}

// Delete removes an entry by key.
func (v *vault) Delete(ctx context.Context, key string) error {
	return v.store.Delete(ctx, key)
}

// List returns all entries in the store.
func (v *vault) List(ctx context.Context) ([]Entry, error) {
	return v.store.List(ctx)
}

// Refresh fetches entries from all configured sources and writes them
// to the store. This always executes regardless of TTL.
func (v *vault) Refresh(ctx context.Context) error {
	now := time.Now()

	for _, src := range v.sources {
		entries, err := src.Fetch(ctx)
		if err != nil {
			return fmt.Errorf("vault: refresh: %w", err)
		}

		for _, e := range entries {
			e.CreatedAt = now
			if serr := v.store.Set(ctx, e); serr != nil {
				return fmt.Errorf("vault: refresh: set %q: %w", e.Key, serr)
			}
		}
	}

	v.mu.Lock()
	v.lastRefresh = now
	v.mu.Unlock()

	return nil
}

func (v *vault) shouldAutoRefresh() bool {
	if len(v.sources) == 0 {
		return false
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.lastRefresh.IsZero() {
		return true
	}

	if v.ttl > 0 {
		return time.Since(v.lastRefresh) > v.ttl
	}

	return false
}

func (v *vault) expired(e Entry) bool {
	if v.ttl <= 0 {
		return false
	}
	return time.Since(e.CreatedAt) > v.ttl
}
