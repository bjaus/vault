package keychain_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/bjaus/vault"
	"github.com/bjaus/vault/keychain"
)

func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

func TestStore_GetSet(t *testing.T) {
	s := keychain.New(keychain.WithService("test-getset"))
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, vault.Entry{Key: "secret", Value: "hunter2"}))

	got, err := s.Get(ctx, "secret")
	require.NoError(t, err)
	assert.Equal(t, "secret", got.Key)
	assert.Equal(t, "hunter2", got.Value)
}

func TestStore_NotFound(t *testing.T) {
	s := keychain.New(keychain.WithService("test-notfound"))

	_, err := s.Get(context.Background(), "missing")
	require.ErrorIs(t, err, vault.ErrNotFound)
}

func TestStore_Delete(t *testing.T) {
	s := keychain.New(keychain.WithService("test-delete"))
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, vault.Entry{Key: "k", Value: "v"}))
	require.NoError(t, s.Delete(ctx, "k"))

	_, err := s.Get(ctx, "k")
	require.ErrorIs(t, err, vault.ErrNotFound)
}

func TestStore_Delete_nonexistent(t *testing.T) {
	s := keychain.New(keychain.WithService("test-delete-noop"))

	// Should not error when deleting a key that doesn't exist.
	require.NoError(t, s.Delete(context.Background(), "nope"))
}

func TestStore_List(t *testing.T) {
	s := keychain.New(keychain.WithService("test-list"))
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, vault.Entry{Key: "a", Value: "1"}))
	require.NoError(t, s.Set(ctx, vault.Entry{Key: "b", Value: "2"}))
	require.NoError(t, s.Set(ctx, vault.Entry{Key: "c", Value: "3"}))

	entries, err := s.List(ctx)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestStore_List_empty(t *testing.T) {
	s := keychain.New(keychain.WithService("test-list-empty"))

	entries, err := s.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestStore_SetUpdatesIndex(t *testing.T) {
	s := keychain.New(keychain.WithService("test-index-update"))
	ctx := context.Background()

	// Set the same key twice — index should not duplicate.
	require.NoError(t, s.Set(ctx, vault.Entry{Key: "k", Value: "v1"}))
	require.NoError(t, s.Set(ctx, vault.Entry{Key: "k", Value: "v2"}))

	entries, err := s.List(ctx)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "v2", entries[0].Value)
}

func TestStore_DeleteUpdatesIndex(t *testing.T) {
	s := keychain.New(keychain.WithService("test-index-delete"))
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, vault.Entry{Key: "a", Value: "1"}))
	require.NoError(t, s.Set(ctx, vault.Entry{Key: "b", Value: "2"}))
	require.NoError(t, s.Delete(ctx, "a"))

	entries, err := s.List(ctx)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "b", entries[0].Key)
}

func TestStore_Namespace(t *testing.T) {
	s := keychain.New(keychain.WithService("test-ns"))
	ctx := context.Background()

	prod := s.WithNamespace("prod")
	qa := s.WithNamespace("qa")

	require.NoError(t, prod.Set(ctx, vault.Entry{Key: "db", Value: "prod-host"}))
	require.NoError(t, qa.Set(ctx, vault.Entry{Key: "db", Value: "qa-host"}))

	got, err := prod.Get(ctx, "db")
	require.NoError(t, err)
	assert.Equal(t, "prod-host", got.Value)

	got, err = qa.Get(ctx, "db")
	require.NoError(t, err)
	assert.Equal(t, "qa-host", got.Value)
}

func TestStore_ImplementsInterfaces(t *testing.T) {
	var store vault.Store = keychain.New()

	_, ok := store.(vault.Namespaced)
	assert.True(t, ok, "keychain.Store should implement vault.Namespaced")
}
