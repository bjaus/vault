package vault_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bjaus/vault"
)

func TestMemory_GetSetDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := vault.NewMemory()

	require.NoError(t, m.Set(ctx, vault.Entry{Key: "k", Value: "v", CreatedAt: time.Now()}))

	got, err := m.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got.Value)

	require.NoError(t, m.Delete(ctx, "k"))

	_, err = m.Get(ctx, "k")
	require.ErrorIs(t, err, vault.ErrNotFound)
}

func TestMemory_NotFound(t *testing.T) {
	t.Parallel()

	m := vault.NewMemory()
	_, err := m.Get(context.Background(), "missing")
	require.ErrorIs(t, err, vault.ErrNotFound)
}

func TestMemory_List(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := vault.NewMemory()

	require.NoError(t, m.Set(ctx, vault.Entry{Key: "a", Value: "1"}))
	require.NoError(t, m.Set(ctx, vault.Entry{Key: "b", Value: "2"}))
	require.NoError(t, m.Set(ctx, vault.Entry{Key: "c", Value: "3"}))

	entries, err := m.List(ctx)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestMemory_List_empty(t *testing.T) {
	t.Parallel()

	m := vault.NewMemory()
	entries, err := m.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestMemory_Namespace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := vault.NewMemory()

	prod := m.WithNamespace("prod")
	qa := m.WithNamespace("qa")

	require.NoError(t, prod.Set(ctx, vault.Entry{Key: "host", Value: "prod.db"}))
	require.NoError(t, qa.Set(ctx, vault.Entry{Key: "host", Value: "qa.db"}))

	got, err := prod.Get(ctx, "host")
	require.NoError(t, err)
	assert.Equal(t, "prod.db", got.Value)

	got, err = qa.Get(ctx, "host")
	require.NoError(t, err)
	assert.Equal(t, "qa.db", got.Value)
}

func TestMemory_NamespaceIsolation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := vault.NewMemory()

	scoped := m.WithNamespace("ns")
	require.NoError(t, scoped.Set(ctx, vault.Entry{Key: "k", Value: "scoped"}))

	// The unscoped store should not see the namespaced key directly.
	_, err := m.Get(ctx, "k")
	require.ErrorIs(t, err, vault.ErrNotFound)
}

func TestMemory_NamespaceList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := vault.NewMemory()

	prod := m.WithNamespace("prod")
	qa := m.WithNamespace("qa")

	require.NoError(t, prod.Set(ctx, vault.Entry{Key: "a", Value: "1"}))
	require.NoError(t, prod.Set(ctx, vault.Entry{Key: "b", Value: "2"}))
	require.NoError(t, qa.Set(ctx, vault.Entry{Key: "c", Value: "3"}))

	prodEntries, err := prod.List(ctx)
	require.NoError(t, err)
	assert.Len(t, prodEntries, 2)

	qaEntries, err := qa.List(ctx)
	require.NoError(t, err)
	assert.Len(t, qaEntries, 1)
}

func TestMemory_SharedBacking(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := vault.NewMemory()
	scoped := m.WithNamespace("ns")

	// Set via scoped view.
	require.NoError(t, scoped.Set(ctx, vault.Entry{Key: "k", Value: "v"}))

	// Visible via scoped view.
	got, err := scoped.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got.Value)

	// Delete via scoped view.
	require.NoError(t, scoped.Delete(ctx, "k"))

	_, err = scoped.Get(ctx, "k")
	require.ErrorIs(t, err, vault.ErrNotFound)
}

func TestMemory_ImplementsNamespaced(t *testing.T) {
	t.Parallel()

	var store vault.Store = vault.NewMemory()

	_, ok := store.(vault.Namespaced)
	assert.True(t, ok, "Memory should implement Namespaced")
}
