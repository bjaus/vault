package vault

import (
	"context"
	"sync"
)

type memoryState struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// Memory is an in-memory [Store]. It is safe for concurrent use and
// implements [Namespaced]. Useful for testing and as the default store.
type Memory struct {
	state  *memoryState
	prefix string
}

// NewMemory creates an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{
		state: &memoryState{
			entries: make(map[string]Entry),
		},
	}
}

// WithNamespace returns a [Store] scoped to the given namespace. The
// returned store shares the same backing data as the original.
func (m *Memory) WithNamespace(ns string) Store {
	return &Memory{
		state:  m.state,
		prefix: ns + "/",
	}
}

// Get retrieves an entry by key.
func (m *Memory) Get(_ context.Context, key string) (Entry, error) {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	e, ok := m.state.entries[m.prefix+key]
	if !ok {
		return Entry{}, ErrNotFound
	}
	return e, nil
}

// Set stores an entry.
func (m *Memory) Set(_ context.Context, entry Entry) error {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()

	m.state.entries[m.prefix+entry.Key] = entry
	return nil
}

// Delete removes an entry by key.
func (m *Memory) Delete(_ context.Context, key string) error {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()

	delete(m.state.entries, m.prefix+key)
	return nil
}

// List returns all entries in the store (within the current namespace).
func (m *Memory) List(_ context.Context) ([]Entry, error) {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()

	entries := make([]Entry, 0, len(m.state.entries))
	for k, e := range m.state.entries {
		if m.prefix == "" || len(k) > len(m.prefix) && k[:len(m.prefix)] == m.prefix {
			entries = append(entries, e)
		}
	}

	return entries, nil
}
