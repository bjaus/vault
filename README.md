# vault

[![Go Reference](https://pkg.go.dev/badge/github.com/bjaus/vault.svg)](https://pkg.go.dev/github.com/bjaus/vault)
[![Go Report Card](https://goreportcard.com/badge/github.com/bjaus/vault)](https://goreportcard.com/report/github.com/bjaus/vault)

A pluggable configuration and secret store for Go.

Vault decouples where configuration comes from (sources) and where it lives locally (stores). Sources are read-only providers that fetch entries from external systems. Stores are read-write backends that persist entries locally for fast access.

## Install

```bash
go get github.com/bjaus/vault
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/bjaus/vault"
    "github.com/bjaus/vault/keychain"
)

func main() {
    v := vault.New(
        vault.WithStore(keychain.New()),
        vault.WithNamespace("prod"),
    )

    ctx := context.Background()

    // Store a secret manually.
    if err := v.Set(ctx, vault.Entry{Key: "api-token", Value: "sk-abc123"}); err != nil {
        log.Fatal(err)
    }

    // Retrieve it.
    entry, err := v.Get(ctx, "api-token")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(entry.Value)
}
```

## Core Interfaces

Two pluggable interfaces define the contract:

```go
// Store persists entries locally (read-write).
type Store interface {
    Get(ctx context.Context, key string) (Entry, error)
    Set(ctx context.Context, entry Entry) error
    Delete(ctx context.Context, key string) error
    List(ctx context.Context) ([]Entry, error)
}

// Source fetches entries from an external system (read-only).
type Source interface {
    Fetch(ctx context.Context) ([]Entry, error)
}
```

`SourceFunc` adapts a plain function into a `Source`.

## Sources and Auto-Refresh

When configured with sources, the vault auto-refreshes on the first cache miss. With a TTL configured, it re-refreshes after entries expire.

```go
src := vault.SourceFunc(func(ctx context.Context) ([]vault.Entry, error) {
    // Fetch from SSM, Secrets Manager, a file, an API, etc.
    return []vault.Entry{
        {Key: "db-password", Value: "secret", Source: "ssm"},
    }, nil
})

v := vault.New(
    vault.WithStore(keychain.New()),
    vault.WithSource(src),
    vault.WithTTL(7 * 24 * time.Hour),
)

// First call: store miss -> fetches from source -> caches -> returns.
entry, err := v.Get(ctx, "db-password")
```

Explicit `v.Refresh(ctx)` is always available regardless of TTL.

## Namespace Support

Store implementations that support scoping implement `Namespaced`:

```go
type Namespaced interface {
    WithNamespace(namespace string) Store
}
```

Both the built-in `Memory` store and `keychain.Store` implement this. When a vault is configured with a namespace, operations are scoped automatically:

```go
prod := vault.New(vault.WithStore(keychain.New()), vault.WithNamespace("prod"))
qa   := vault.New(vault.WithStore(keychain.New()), vault.WithNamespace("qa"))

// These don't collide — they're stored under different keyring services.
prod.Set(ctx, vault.Entry{Key: "db-host", Value: "prod.db.internal"})
qa.Set(ctx, vault.Entry{Key: "db-host", Value: "qa.db.internal"})
```

## Store Implementations

| Store | Package | Description |
|-------|---------|-------------|
| Memory | `vault` | In-memory, safe for concurrent use. Default when no store is provided. |
| Keychain | `vault/keychain` | OS keychain via [go-keyring](https://github.com/zalando/go-keyring). macOS Keychain, Linux Secret Service, Windows Credential Manager. |

## License

MIT
