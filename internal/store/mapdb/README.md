# mapdb

An in-memory implementation of the `store.Store` interface.

## Overview

`mapdb` provides a fully-featured in-memory store with:

- **Vector search** using cosine similarity
- **Predicate filtering** on all record fields including nested metadata
- **Transactional isolation** with snapshot isolation
- **Thread-safe operations** using reader/writer locks
- **Soft delete semantics** matching PostgreSQL implementation

## Usage

```go
import "github.com/alash3al/stash/internal/store/mapdb"

cfg := mapdb.Config{
    VectorDim: 1536,  // Must match embedder dimensions
}

store, err := mapdb.New(cfg)
if err != nil {
    // handle error
}
defer store.Close()
```

## Configuration

| Field | Description | Default |
|-------|-------------|---------|
| `VectorDim` | Dimension of all vectors stored in this store | **Required** |
| `MaxResultSize` | Hard cap on `Limit` in `List` and `Search` | 10000 |

## Implementation Details

### Data Structures

- **Records**: `map[string]*Record` for O(1) lookups by ID
- **Vectors**: `map[string][]*vectorEntry` for O(n) similarity search
- **Deleted**: `map[string]bool` for soft delete tracking

### Search Algorithms

- **Vector search**: Brute-force cosine similarity with O(n) time complexity
- **Text search**: Basic substring matching on record content
- **Predicate evaluation**: Recursive tree walker with dotted path resolution

### Transaction Model

- **Snapshot isolation**: Each transaction sees a consistent snapshot
- **Copy-on-write**: Records are copied on transaction start
- **Atomic commit**: All changes applied together on success

## Performance Characteristics

- **Memory**: Stores all records and vectors in memory
- **Search**: Linear scan for vector similarity (suitable for <10K records)
- **Concurrency**: Reader/writer locks for thread safety
- **Transactions**: Full snapshot isolation with MVCC semantics

## Testing

The implementation passes the full `storetest` validation suite:

```go
go test ./internal/store/mapdb/...
```

## Limitations

1. **Vector search**: No approximate nearest neighbor indexing
2. **Persistence**: Data lost on process termination  
3. **Memory**: No automatic eviction or size limits
4. **Distribution**: Single-process, no replication

## When to Use

- **Testing**: Fast, deterministic store for unit tests
- **Development**: Local development without PostgreSQL
- **Embedded**: Standalone applications without external dependencies
- **Prototyping**: Rapid iteration before database deployment