# Stash Deployment Guide

Complete guide for building, testing, and deploying Stash.

---

## Quick Start

### Local Development

```bash
# Start PostgreSQL + pgAdmin
docker-compose up -d

# Build CLI
go build -o stash ./cmd/cli

# Run tests
go test ./...

# Use CLI
./stash events create "Test event" --namespace=dev
./stash facts query --namespace=dev
```

### Docker Build

```bash
# Build locally
docker build -t stash:latest .

# Run with PostgreSQL
docker run --rm \
  --network stash-network \
  -e STASH_STORE_DRIVER=postgres \
  -e STASH_STORE_POSTGRES_DSN="postgresql://stash:stash_dev_password@postgres:5432/stash" \
  stash:latest facts query --namespace=test
```

---

## CI/CD Pipeline

### GitHub Actions Workflows

Two workflows are configured:

#### 1. Tests (`test.yml`)
**Trigger:** Push to main/master/develop, Pull Requests

**Jobs:**
- Unit tests with race detection
- Code format check
- Go vet analysis
- Build verification
- Coverage upload to Codecov

**Configuration:**
- Go 1.22
- PostgreSQL 16 with pgvector
- Runs on: ubuntu-latest

```bash
# Manually trigger (if needed)
gh workflow run test.yml
```

#### 2. Release (`release.yml`)
**Trigger:** GitHub Release published

**Jobs:**
1. **Build Binaries** (Matrix)
   - Linux x86_64 & ARM64
   - macOS x86_64 & ARM64
   - Windows x86_64
   - All uploaded to release page

2. **Build Docker** (Multi-arch)
   - Builds for linux/amd64 and linux/arm64
   - Pushes to ghcr.io
   - Tags: semantic version + latest

---

## Multi-Platform Building

### Using Docker Buildx

```bash
# Enable buildx (usually auto-enabled)
docker buildx ls

# Build for multiple platforms locally
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t stash:latest \
  --load \
  .

# Build and push to registry
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ghcr.io/alash3al/stash:latest \
  --push \
  .
```

### Cross-Compilation (Go)

Build for specific OS/Architecture:

```bash
# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o stash-linux-arm64 ./cmd/cli

# macOS ARM64 (from Linux)
GOOS=darwin GOARCH=arm64 go build -o stash-darwin-arm64 ./cmd/cli

# Windows x86_64
GOOS=windows GOARCH=amd64 go build -o stash-windows-amd64.exe ./cmd/cli
```

All binaries are fully static (CGO_ENABLED=0) and ready to run anywhere.

---

## Container Registry

### GitHub Container Registry (GHCR)

Automated on release:

```bash
# Login (if needed for private images)
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Pull image
docker pull ghcr.io/alash3al/stash:latest

# Run
docker run ghcr.io/alash3al/stash:latest facts query
```

### Alternative Registries

Push to Docker Hub:

```bash
docker tag stash:latest myuser/stash:latest
docker push myuser/stash:latest
```

---

## Production Deployment

### Docker Compose (Single Host)

```yaml
# docker-compose.prod.yml
version: '3.8'

services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_DB: stash
      POSTGRES_USER: stash
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    restart: always

  stash:
    image: ghcr.io/alash3al/stash:latest
    environment:
      STASH_STORE_DRIVER: postgres
      STASH_STORE_POSTGRES_DSN: postgresql://stash:${DB_PASSWORD}@postgres:5432/stash
      STASH_EMBEDDER_DRIVER: openai
      STASH_EMBEDDER_MODEL: text-embedding-3-small
      STASH_REASONER_DRIVER: openai
      STASH_REASONER_MODEL: gpt-4o-mini
      STASH_OPENAI_API_KEY: ${OPENAI_API_KEY}
    depends_on:
      postgres:
        condition: service_healthy
    restart: always

volumes:
  postgres_data:
```

Deploy:

```bash
# Set environment
export DB_PASSWORD=secure_password_here
export OPENAI_API_KEY=sk-...

# Deploy
docker-compose -f docker-compose.prod.yml up -d

# View logs
docker-compose -f docker-compose.prod.yml logs -f stash
```

### Kubernetes

Example deployment manifest:

```yaml
# k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: stash
spec:
  replicas: 3
  selector:
    matchLabels:
      app: stash
  template:
    metadata:
      labels:
        app: stash
    spec:
      containers:
      - name: stash
        image: ghcr.io/alash3al/stash:latest
        imagePullPolicy: Always
        env:
        - name: STASH_STORE_DRIVER
          value: postgres
        - name: STASH_STORE_POSTGRES_DSN
          valueFrom:
            secretKeyRef:
              name: stash-db
              key: dsn
        - name: STASH_OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: stash-secrets
              key: openai-key
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          exec:
            command:
            - /stash
            - env
          initialDelaySeconds: 10
          periodSeconds: 30
```

Deploy:

```bash
# Create secrets
kubectl create secret generic stash-db --from-literal=dsn="postgresql://..."
kubectl create secret generic stash-secrets --from-literal=openai-key="sk-..."

# Deploy
kubectl apply -f k8s/

# Check status
kubectl get pods -l app=stash
kubectl logs -l app=stash -f
```

---

## Environment Variables

### Storage

```bash
# In-memory (default, for testing)
STASH_STORE_DRIVER=mapdb

# PostgreSQL (production)
STASH_STORE_DRIVER=postgres
STASH_STORE_POSTGRES_DSN="postgresql://user:pass@localhost:5432/stash"
```

### Embeddings

```bash
# OpenAI (recommended)
STASH_EMBEDDER_DRIVER=openai
STASH_EMBEDDER_MODEL=text-embedding-3-small
STASH_OPENAI_API_KEY=sk-...

# Alternatives (OpenAI-compatible)
# STASH_EMBEDDER_DRIVER=openai
# STASH_EMBEDDER_MODEL=gpt-4-turbo
# STASH_OPENAI_API_KEY=sk-...
```

### Reasoning (LLM)

```bash
# OpenAI
STASH_REASONER_DRIVER=openai
STASH_REASONER_MODEL=gpt-4o-mini
STASH_OPENAI_API_KEY=sk-...

# OpenRouter (compatible)
STASH_REASONER_DRIVER=openai
STASH_REASONER_MODEL=openrouter/google/gemma-4-27b
STASH_OPENAI_API_KEY=sk-or-...
# Optional: Set base URL
# STASH_OPENAI_BASE_URL=https://openrouter.ai/api/v1
```

---

## CI/CD Checks

### Local Pre-Commit Checks

```bash
#!/bin/bash

set -e

echo "Running pre-commit checks..."

# Format
go fmt ./...

# Vet
go vet ./...

# Tests
go test ./...

# Build
go build -o /tmp/stash ./cmd/cli

echo "✅ All checks passed!"
```

Save as `.git/hooks/pre-commit` and `chmod +x`.

### Verify Docker Build Works

```bash
docker build -t stash:test .
docker run --rm stash:test --help
```

---

## Testing Strategies

### Unit Tests

```bash
# Run all tests
go test ./...

# Verbose
go test ./... -v

# With coverage
go test ./... -cover

# Race detection
go test ./... -race

# Specific package
go test ./internal/memory -v
```

### Integration Tests

Requires PostgreSQL running:

```bash
docker-compose up -d postgres

go test ./internal/store/postgres -v
```

### User-Level Tests

```bash
# Build CLI
go build -o stash ./cmd/cli

# Run user-level tests
./test-phase3-task0017.sh
./test-phase3-task0016.sh
```

---

## Performance Optimization

### Build Optimization

```bash
# Smaller binary (strip debug info)
go build -ldflags="-s -w" -o stash ./cmd/cli

# Static binary (for Docker)
CGO_ENABLED=0 go build -o stash ./cmd/cli
```

### Docker Layer Caching

Dockerfile strategy:
1. Copy `go.mod`, `go.sum` first (changes rarely)
2. Run `go mod download` (cached layer)
3. Copy source code (changes frequently)
4. Build binary

This avoids re-downloading dependencies on every code change.

### PostgreSQL Performance

```sql
-- Enable vector similarity search optimization
CREATE EXTENSION IF NOT EXISTS vector;

-- Create index for fast vector search
CREATE INDEX idx_vectors ON records USING ivfflat (
  (vectors -> 'text-embedding-3-small')
);

-- Analyze query performance
EXPLAIN ANALYZE
SELECT * FROM records
WHERE namespace = 'test'
ORDER BY similarity(vectors -> 'text-embedding-3-small', '[...]') DESC
LIMIT 10;
```

---

## Troubleshooting

### Docker Build Fails

```bash
# Check Docker daemon
docker ps

# Rebuild without cache
docker build --no-cache -t stash:latest .

# Inspect image
docker inspect stash:latest
```

### PostgreSQL Connection Issues

```bash
# Test connection
docker-compose exec postgres psql -U stash -d stash -c "SELECT 1"

# View logs
docker-compose logs postgres

# Restart
docker-compose down postgres
docker-compose up -d postgres
```

### Tests Fail

```bash
# Clear cache
go clean -cache

# Rebuild
go build ./...

# Run subset of tests
go test ./internal/memory -v -run TestRecallFactsRanked
```

---

## Release Process

### Creating a Release

1. **Push changes to main**
   ```bash
   git push origin main
   ```

2. **Create release tag**
   ```bash
   git tag -a v0.3.0 -m "Phase 3 complete"
   git push origin v0.3.0
   ```

3. **Create GitHub Release**
   - Go to https://github.com/alash3al/stash/releases
   - Click "Create release"
   - Select tag `v0.3.0`
   - Write release notes
   - Publish release

4. **Workflows trigger automatically**
   - `build-binaries` builds for all platforms
   - Uploads to release page
   - `build-docker` builds and pushes images

---

## Monitoring & Logging

### Docker Logs

```bash
# View logs
docker-compose logs stash

# Follow logs
docker-compose logs -f stash

# Last 100 lines
docker-compose logs --tail=100 stash
```

### Health Checks

```bash
# Check CLI works
docker run --rm stash:latest --help

# Health check endpoint (if implemented)
curl http://localhost:8080/health
```

---

## Security Considerations

### Distroless Image

- No shell ✅
- No package manager ✅
- No unnecessary tools ✅
- Minimal attack surface ✅

### Static Binary

- No C dependencies ✅
- Fully portable ✅
- Easy to verify ✅

### Environment Variables

- Secrets in env, not hardcoded ✅
- API keys external ✅
- Database passwords separate ✅

### Non-Root User

```dockerfile
USER nonroot:nonroot
```

Runs as unprivileged user by default.

---

## Documentation

See also:
- `README.md` — Project overview
- `CLI-REFERENCE.md` — Command reference
- `TESTING.md` — Testing guide
- `STATUS.md` — Project status
- `AGENTS.md` — Development rules

---

## Support

For issues:
1. Check `TROUBLESHOOTING.md` (if exists)
2. Review `TESTING.md` for test commands
3. Search GitHub issues
4. Create new issue with:
   - Go version: `go version`
   - Docker version: `docker --version`
   - Error message
   - Steps to reproduce
