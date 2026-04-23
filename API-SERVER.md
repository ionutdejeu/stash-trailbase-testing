# Stash HTTP API Server

The Stash semantic memory system is now available as an HTTP API via the `stash server` CLI command.

## Starting the Server

```bash
stash server --port 8080 --host 0.0.0.0
```

Default: `http://localhost:8080`

## API Endpoints

### Health Check
```
GET /health
```
Returns `{"status": "ok"}` if the server is running.

### 1. Remember Events
```
POST /api/v1/facts?namespace=<namespace>
Content-Type: application/json

{
  "content": "Alice works at TechCorp",
  "confidence": 0.95,
  "metadata": {
    "source": "chat",
    "user_id": "123"
  }
}
```

**Response (201):**
```json
{
  "id": "574e3c67-0ae2-4cfd-8b31-d0cd688d3bd8",
  "message": "Event remembered successfully"
}
```

### 2. Recall Facts
```
GET /api/v1/facts?query=<query>&namespace=<namespace>&limit=10&ranked=true

Query parameters:
- query (required): Search query
- namespace (optional): Filter by namespace
- limit (optional, default: 10): Max results
- ranked (optional, default: false): Use confidence-ranked retrieval
```

**Response (200):**
```json
{
  "query": "TechCorp",
  "namespace": "default",
  "ranked": true,
  "limit": 10,
  "facts": [
    {
      "id": "fact-uuid",
      "content": "Alice works at TechCorp",
      "type": "state",
      "confidence": 0.95,
      "observation_count": 1,
      "source": "event",
      "score": 0.89,
      "valid_from": "2026-04-24T02:03:22Z"
    }
  ]
}
```

### 3. Extract Relationships
```
POST /api/v1/facts/relationships/extract
Content-Type: application/json

{
  "facts": [
    "Alice works at TechCorp",
    "Bob manages Alice"
  ]
}
```

**Response (200):**
```json
{
  "relationships": [
    {
      "subject": "Alice",
      "relation": "works_at",
      "object": "TechCorp"
    },
    {
      "subject": "Bob",
      "relation": "manages",
      "object": "Alice"
    }
  ]
}
```

### 4. Consolidate Relationships
```
POST /api/v1/facts/relationships/consolidate
Content-Type: application/json

{
  "namespace": "default",
  "limit": 100
}
```

**Response (200):**
```json
{
  "message": "Relationships consolidated successfully",
  "count": 5
}
```

## Configuration

The server respects all Stash environment variables:

```bash
# Database
STASH_STORE_TYPE=postgres  # or 'memory' for in-memory
STASH_POSTGRES_URL=postgres://user:pass@localhost/stash

# LLM Reasoning
STASH_REASONER_TYPE=openai  # or 'fake' for testing
STASH_OPENAI_API_KEY=sk-...
STASH_OPENAI_MODEL=gpt-4

# Embedding
STASH_EMBEDDER_TYPE=openai  # or 'fake' for testing
```

## Docker

Run the server in Docker:

```bash
docker build -t stash .
docker run -p 8080:8080 \
  -e STASH_STORE_TYPE=memory \
  -e STASH_REASONER_TYPE=fake \
  -e STASH_EMBEDDER_TYPE=fake \
  stash server --host 0.0.0.0
```

With docker-compose:

```bash
docker-compose up -d postgres
docker run -p 8080:8080 \
  --network stash-network \
  -e STASH_STORE_TYPE=postgres \
  -e STASH_POSTGRES_URL=postgres://stash:stash_dev_password@postgres/stash \
  stash server
```

## Error Handling

All errors return JSON with an `error` field:

```json
{
  "error": "query is required"
}
```

HTTP status codes:
- `200 OK` — Success
- `201 Created` — Resource created
- `400 Bad Request` — Invalid input
- `500 Internal Server Error` — Server error

## Integration for Agents

### Python Agent Example

```python
import requests
import json

BASE_URL = "http://localhost:8080"

# Remember a fact
response = requests.post(
    f"{BASE_URL}/api/v1/facts",
    json={
        "content": "The user's name is Alice",
        "confidence": 0.95
    },
    params={"namespace": "user-context"}
)
fact_id = response.json()["id"]

# Recall facts
response = requests.get(
    f"{BASE_URL}/api/v1/facts",
    params={
        "query": "user name",
        "namespace": "user-context",
        "ranked": True
    }
)
facts = response.json()["facts"]

# Extract relationships
response = requests.post(
    f"{BASE_URL}/api/v1/facts/relationships/extract",
    json={
        "facts": ["Alice works at TechCorp", "Bob manages Alice"]
    }
)
relationships = response.json()["relationships"]
```

### JavaScript/Node.js Example

```javascript
const axios = require('axios');

const BASE_URL = "http://localhost:8080";

async function rememberFact(content, confidence = 0.9) {
  const response = await axios.post(
    `${BASE_URL}/api/v1/facts?namespace=default`,
    { content, confidence }
  );
  return response.data.id;
}

async function recallFacts(query, ranked = true) {
  const response = await axios.get(`${BASE_URL}/api/v1/facts`, {
    params: { query, ranked, limit: 10 }
  });
  return response.data.facts;
}

async function extractRelationships(facts) {
  const response = await axios.post(
    `${BASE_URL}/api/v1/facts/relationships/extract`,
    { facts }
  );
  return response.data.relationships;
}
```

## Performance Notes

- Embeddings are generated asynchronously during `Remember` operations (~200-800ms per fact)
- LLM-based relationship extraction is called per-fact for the `extract` endpoint
- Confidence-ranked retrieval combines semantic similarity and confidence scores
- All operations support optional namespacing for multi-tenant use
