# Task: Build Unified Configuration & Bootstrap System

**Status:** Ready for Execution  
**Date:** 2026-04-18

## 1. Context

**Goal:** Create a production-ready configuration system with centralized bootstrap for all Stash entry points (CLI, server, future agents).

**Why:** Current configuration is scattered and entry points duplicate initialization logic. We need:
1. Consistent configuration loading with `STASH_` prefix convention
2. Centralized system bootstrap that handles driver selection, validation, and cleanup
3. Fail-fast startup with clear error messages
4. Simple entry points that just call `bootstrap.MustNew()`

## 2. Boundaries

**In Scope:**
- `internal/config`: Pure configuration loading from `.env` files + environment variables
- `internal/bootstrap`: System initialization, driver selection, cross-component validation
- `.env.example`: Example configuration with all required env vars
- `STASHCONFIG` env var support for config file location
- Required-only configuration (no defaults in code)
- Panic-on-failure bootstrap for production entry points

**Non-Goals:**
- Driver-specific configuration beyond DSN/connection strings
- Config file formats beyond `.env` (YAML/TOML/JSON)
- Secret management integration (AWS Secrets Manager, etc.)
- Background config reloading
- Configuration UI or generation tools

**Constraints:**
- `internal/config` has zero imports from other Stash packages
- All application env vars prefixed with `STASH_`
- Bootstrap owns `STASHCONFIG` resolution and system initialization
- Everything required, no defaults in struct tags
- `bootstrap.MustNew()` panics on failure (for production)

## 3. Approach & Review

**Proposed Approach:**
1. Add dependencies: `caarlos0/env/v11`, `joho/godotenv`
2. Create `internal/config` with `Config` struct and `NewFromFile(filename string)`
3. Create `internal/bootstrap` with `MustNew()` (panic) and `New()` (error) variants
4. Bootstrap handles: config file resolution → config loading → driver selection → validation → initialization
5. Create `.env.example` with suggested values
6. Update test script to use new system
7. Create `cmd/cli` skeleton showing bootstrap usage

**Self-Critique:**
- `bootstrap` violates "lower layers know nothing about higher layers" but is an orchestrator, not a layer
- Required-only config is verbose but forces explicit configuration
- Panic-on-failure is aggressive but correct for startup failures
- Three dependencies added for config system

**Decision:** Proceed with `config` + `bootstrap` approach. The benefits (consistent startup, proper cleanup, simple entry points) outweigh architectural purity concerns.

**Explicit Assumptions:**
1. Users will copy `.env.example` to `.env` and fill in values
2. Embedder dimension validation will be added in follow-up task
3. Only Postgres store and OpenAI embedder drivers exist for now
4. `STASHCONFIG` env var follows `KUBECONFIG` pattern (no underscore)

## 4. Configuration Schema (FINAL)

### Env Var Rules
- All application config: `STASH_` prefix (e.g., `STASH_STORE_DRIVER`)
- Bootstrap config: `STASHCONFIG` (no underscore, like `KUBECONFIG`)
- No support for unprefixed variants
- Everything required, no defaults

### Required Env Vars
```
# Store Configuration
STASH_STORE_DRIVER=postgres
STASH_STORE_DSN=postgres://user:pass@localhost:5432/stash?sslmode=disable
STASH_VECTOR_DIM=1536
STASH_MAX_RESULT_SIZE=10000

# Embedder Configuration
STASH_EMBEDDER_DRIVER=openai
STASH_OPENAI_API_KEY=your-api-key-here
STASH_OPENAI_BASE_URL=https://api.openai.com/v1
STASH_EMBEDDING_MODEL=text-embedding-3-small

# Memory Configuration  
STASH_FRAME_TTL=1h

# Server Configuration (for future use)
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
STASH_LOG_FORMAT=text
```

### Config Struct (`internal/config/config.go`)
```go
type Config struct {
    // Store
    StoreDriver   string `env:"STASH_STORE_DRIVER,required"`
    StoreDSN      string `env:"STASH_STORE_DSN,required"`
    VectorDim     int    `env:"STASH_VECTOR_DIM,required"`
    MaxResultSize int    `env:"STASH_MAX_RESULT_SIZE,required"`
    
    // Embedder
    EmbedderDriver string `env:"STASH_EMBEDDER_DRIVER,required"`
    OpenAIAPIKey   string `env:"STASH_OPENAI_API_KEY,required"`
    OpenAIBaseURL  string `env:"STASH_OPENAI_BASE_URL,required"`
    EmbeddingModel string `env:"STASH_EMBEDDING_MODEL,required"`
    
    // Memory
    ContextTTL     time.Duration `env:"STASH_CONTEXT_TTL,required"`
    
    // Server (future)
    HTTPAddr       string `env:"STASH_HTTP_ADDR,required"`
    LogLevel       string `env:"STASH_LOG_LEVEL,required"`
    LogFormat      string `env:"STASH_LOG_FORMAT,required"`
}
```

## 5. Component Specifications

### `internal/config`
```go
// NewFromFile loads configuration from .env file and environment variables.
// filename must be non-empty. Returns error if file has syntax errors or
// required env vars are missing.
func NewFromFile(filename string) (*Config, error)

// Note: No NewFromEnv() - use NewFromFile("/dev/null") for env-only if needed
```

### `internal/bootstrap`
```go
type Context struct {
    Config  *config.Config
    Store   store.Store
    Embedder embedder.Embedder
    Memory  *memory.Memory
}

// MustNew initializes the entire Stash system.
// Panics on any failure. Use for production entry points.
func MustNew(ctx context.Context) *Context

// New initializes the entire Stash system.
// Returns error if any component fails to initialize.
func New(ctx context.Context) (*Context, error)

// Close releases all resources in reverse initialization order.
func (c *Context) Close() error

// configFile() internal: STASHCONFIG → .env resolution
```

### Bootstrap Initialization Flow
1. Determine config file: `STASHCONFIG` env var → `.env` default
2. Load config: `config.NewFromFile()`
3. Build store: switch on `StoreDriver` (`postgres`, `mapdb`)
4. Build embedder: switch on `EmbedderDriver` (`openai`, `fake`)
5. Validate: `embedder.Dims() == config.VectorDim`
6. Build memory: `memory.New(store, embedder)`
7. Return `Context` with all components

## 6. Next-Step Handoff

**Implementation Notes:**
- `internal/config` must not import other Stash packages
- `internal/bootstrap` imports everything - it's the orchestrator
- Error messages should clearly indicate which component failed
- `.env.example` should have real example values (not placeholders)
- Use `time.Duration` for `ContextTTL` (parsed by `caarlos0/env`)

**Files / Areas Likely Affected:**
- New: `internal/config/` (config.go, config_test.go)
- New: `internal/bootstrap/` (bootstrap.go, bootstrap_test.go)
- New: `.env.example`
- New: `cmd/cli/` (skeleton showing bootstrap usage)
- Update: `go.mod` (add `caarlos0/env/v11`, `joho/godotenv`)
- Update: `scripts/test_memory.sh` (use new env var names)
- Update: `docs/` (document env vars and bootstrap usage)

**Risks / Watchouts:**
1. **Dimension mismatch**: `STASH_VECTOR_DIM` wrong → bootstrap fails (good!)
2. **Required verbosity**: 10+ env vars to set, but explicit
3. **Bootstrap knows all**: Violates layer purity but necessary for orchestration
4. **Panic in production**: `MustNew()` panics, but startup failures are unrecoverable

**Verification Plan:**
1. Unit tests: Config loads/fails appropriately
2. Unit tests: Bootstrap initializes/fails appropriately
3. Integration: Test script works with new system
4. Documentation: `.env.example` is valid
5. Build: All packages compile cleanly

**Acceptance Criteria:**
1. `bootstrap.MustNew(context.Background())` panics with clear error if `STASH_*` env vars missing
2. `bootstrap.New(context.Background())` returns error if initialization fails
3. `STASHCONFIG=.env.test` loads from specified file
4. `.env.example` contains all required vars with realistic example values
5. `scripts/test_memory.sh` works with new `STASH_` prefixed env vars
6. `go build ./internal/config/... ./internal/bootstrap/...` succeeds
7. `internal/config` has zero imports from other Stash packages

## 7. Execution Steps

- [ ] Add dependencies to `go.mod`: `caarlos0/env/v11`, `joho/godotenv`
- [ ] Create `internal/config/config.go` with `Config` struct and `NewFromFile()`
- [ ] Create `internal/config/config_test.go` with comprehensive tests
- [ ] Create `internal/bootstrap/bootstrap.go` with `MustNew()`, `New()`, `Close()`
- [ ] Create `internal/bootstrap/bootstrap_test.go` with initialization tests
- [ ] Create `.env.example` with all required vars and realistic examples
- [ ] Create `cmd/cli/main.go` skeleton using `bootstrap.MustNew()`
- [ ] Update `scripts/test_memory.sh` to use new env var names
- [ ] Update relevant READMEs with configuration documentation

## 8. Progress Notes

- [Date/Time] — 

## 9. Outcome

**Final Result:** A production-ready configuration and bootstrap system that allows any Stash entry point to initialize with a single call to `bootstrap.MustNew()`, with proper error handling, cleanup, and consistent behavior across all entry points.