package vault_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bjaus/vault"
)

func TestNew_defaultsToMemoryStore(t *testing.T) {
	t.Parallel()

	v := vault.New()
	ctx := context.Background()

	_, err := v.Get(ctx, "missing")
	require.ErrorIs(t, err, vault.ErrNotFound)

	require.NoError(t, v.Set(ctx, vault.Entry{Key: "k", Value: "v"}))

	got, err := v.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got.Value)
}

func TestGet_cacheHit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := vault.NewMemory()
	require.NoError(t, store.Set(ctx, vault.Entry{
		Key:       "hit",
		Value:     "cached",
		CreatedAt: time.Now(),
		Source:    "test",
	}))

	calls := 0
	src := vault.SourceFunc(func(_ context.Context) ([]vault.Entry, error) {
		calls++
		return nil, nil
	})

	v := vault.New(vault.WithStore(store), vault.WithSource(src))

	got, err := v.Get(ctx, "hit")
	require.NoError(t, err)
	assert.Equal(t, "cached", got.Value)
	assert.Equal(t, 0, calls, "source should not be called on cache hit")
}

func TestGet_cacheMiss_withSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	src := vault.SourceFunc(func(_ context.Context) ([]vault.Entry, error) {
		return []vault.Entry{
			{Key: "fetched", Value: "from-source", Source: "mock"},
		}, nil
	})

	v := vault.New(vault.WithSource(src))

	got, err := v.Get(ctx, "fetched")
	require.NoError(t, err)
	assert.Equal(t, "from-source", got.Value)
	assert.Equal(t, "mock", got.Source)
}

func TestGet_cacheMiss_noSource(t *testing.T) {
	t.Parallel()

	v := vault.New()
	_, err := v.Get(context.Background(), "nope")
	require.ErrorIs(t, err, vault.ErrNotFound)
}

func TestGet_expired_autoRefresh(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := vault.NewMemory()

	// Seed with an old entry.
	require.NoError(t, store.Set(ctx, vault.Entry{
		Key:       "stale",
		Value:     "old",
		CreatedAt: time.Now().Add(-time.Hour),
		Source:    "seed",
	}))

	src := vault.SourceFunc(func(_ context.Context) ([]vault.Entry, error) {
		return []vault.Entry{
			{Key: "stale", Value: "fresh", Source: "mock"},
		}, nil
	})

	v := vault.New(
		vault.WithStore(store),
		vault.WithSource(src),
		vault.WithTTL(time.Millisecond),
	)

	got, err := v.Get(ctx, "stale")
	require.NoError(t, err)
	assert.Equal(t, "fresh", got.Value)
}

func TestGet_storeError_propagated(t *testing.T) {
	t.Parallel()

	errBroken := errors.New("store is broken")
	store := &failStore{err: errBroken}

	v := vault.New(vault.WithStore(store))

	_, err := v.Get(context.Background(), "any")
	require.ErrorIs(t, err, errBroken)
}

func TestSet_populatesDefaults(t *testing.T) {
	t.Parallel()

	v := vault.New()
	ctx := context.Background()

	require.NoError(t, v.Set(ctx, vault.Entry{Key: "k", Value: "v"}))

	got, err := v.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "manual", got.Source)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestSet_preservesExplicitValues(t *testing.T) {
	t.Parallel()

	v := vault.New()
	ctx := context.Background()
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, v.Set(ctx, vault.Entry{
		Key:       "k",
		Value:     "v",
		CreatedAt: ts,
		Source:    "custom",
	}))

	got, err := v.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "custom", got.Source)
	assert.Equal(t, ts, got.CreatedAt)
}

func TestDelete(t *testing.T) {
	t.Parallel()

	v := vault.New()
	ctx := context.Background()

	require.NoError(t, v.Set(ctx, vault.Entry{Key: "k", Value: "v"}))
	require.NoError(t, v.Delete(ctx, "k"))

	_, err := v.Get(ctx, "k")
	require.ErrorIs(t, err, vault.ErrNotFound)
}

func TestList(t *testing.T) {
	t.Parallel()

	v := vault.New()
	ctx := context.Background()

	require.NoError(t, v.Set(ctx, vault.Entry{Key: "a", Value: "1"}))
	require.NoError(t, v.Set(ctx, vault.Entry{Key: "b", Value: "2"}))

	entries, err := v.List(ctx)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestRefresh(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	calls := 0
	src := vault.SourceFunc(func(_ context.Context) ([]vault.Entry, error) {
		calls++
		return []vault.Entry{
			{Key: "x", Value: "refreshed", Source: "src"},
		}, nil
	})

	v := vault.New(vault.WithSource(src))

	require.NoError(t, v.Refresh(ctx))
	assert.Equal(t, 1, calls)

	got, err := v.Get(ctx, "x")
	require.NoError(t, err)
	assert.Equal(t, "refreshed", got.Value)
}

func TestRefresh_sourceError(t *testing.T) {
	t.Parallel()

	errFetch := errors.New("network down")
	src := vault.SourceFunc(func(_ context.Context) ([]vault.Entry, error) {
		return nil, errFetch
	})

	v := vault.New(vault.WithSource(src))

	err := v.Refresh(context.Background())
	require.ErrorIs(t, err, errFetch)
}

func TestAutoRefresh_onlyOnceWithoutTTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	calls := 0
	src := vault.SourceFunc(func(_ context.Context) ([]vault.Entry, error) {
		calls++
		return []vault.Entry{
			{Key: "exists", Value: "yes", Source: "src"},
		}, nil
	})

	v := vault.New(vault.WithSource(src))

	// First Get triggers auto-refresh.
	_, err := v.Get(ctx, "exists")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)

	// Second Get for a missing key should NOT trigger refresh (no TTL, already refreshed).
	_, err = v.Get(ctx, "missing")
	require.ErrorIs(t, err, vault.ErrNotFound)
	assert.Equal(t, 1, calls, "should not refresh again without TTL")
}

func TestAutoRefresh_retriggersAfterTTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	calls := 0
	src := vault.SourceFunc(func(_ context.Context) ([]vault.Entry, error) {
		calls++
		return []vault.Entry{
			{Key: "k", Value: "v", Source: "src"},
		}, nil
	})

	v := vault.New(
		vault.WithSource(src),
		vault.WithTTL(time.Millisecond),
	)

	_, err := v.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)

	time.Sleep(5 * time.Millisecond)

	_, err = v.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, 2, calls, "should refresh again after TTL expires")
}

func TestNamespace_scopesStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := vault.NewMemory()

	prod := vault.New(vault.WithStore(store), vault.WithNamespace("prod"))
	qa := vault.New(vault.WithStore(store), vault.WithNamespace("qa"))

	require.NoError(t, prod.Set(ctx, vault.Entry{Key: "db", Value: "prod-host"}))
	require.NoError(t, qa.Set(ctx, vault.Entry{Key: "db", Value: "qa-host"}))

	got, err := prod.Get(ctx, "db")
	require.NoError(t, err)
	assert.Equal(t, "prod-host", got.Value)

	got, err = qa.Get(ctx, "db")
	require.NoError(t, err)
	assert.Equal(t, "qa-host", got.Value)
}

func TestSourceFunc(t *testing.T) {
	t.Parallel()

	fn := vault.SourceFunc(func(_ context.Context) ([]vault.Entry, error) {
		return []vault.Entry{{Key: "k", Value: "v"}}, nil
	})

	entries, err := fn.Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "k", entries[0].Key)
}

// failStore is a Store that always returns an error on Get.
type failStore struct {
	err error
}

func (f *failStore) Get(_ context.Context, _ string) (vault.Entry, error) {
	return vault.Entry{}, f.err
}

func (f *failStore) Set(_ context.Context, _ vault.Entry) error { return nil }

func (f *failStore) Delete(_ context.Context, _ string) error { return nil }

func (f *failStore) List(_ context.Context) ([]vault.Entry, error) { return nil, nil }
