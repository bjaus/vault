package vault

import "time"

// Option configures a [Vault] created by [New].
type Option func(*config)

type config struct {
	store     Store
	sources   []Source
	namespace string
	ttl       time.Duration
}

// WithStore sets the backing store for the vault.
// If not provided, an in-memory store is used.
func WithStore(s Store) Option {
	return func(c *config) { c.store = s }
}

// WithSource adds a source to the vault. Sources are consulted in the
// order they are added during [Vault.Refresh].
func WithSource(s Source) Option {
	return func(c *config) { c.sources = append(c.sources, s) }
}

// WithNamespace scopes the vault to a namespace. If the configured store
// implements [Namespaced], all operations are scoped automatically. If
// the store does not implement [Namespaced], this option has no effect.
func WithNamespace(ns string) Option {
	return func(c *config) { c.namespace = ns }
}

// WithTTL sets the time-to-live for cached entries. When set, entries
// older than the TTL are considered expired and trigger an automatic
// refresh from sources on the next [Vault.Get]. A zero TTL means
// entries never expire automatically.
func WithTTL(d time.Duration) Option {
	return func(c *config) { c.ttl = d }
}
