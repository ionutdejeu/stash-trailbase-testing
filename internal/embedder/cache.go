package embedder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"
	"time"
)

// Cached stores embeddings in a generic store using a special namespace.
// Composes with store, doesn't extend it.
type Cached struct {
	embedder  Embedder
	getRecord func(ctx context.Context, id string) (map[string][]float32, error)
	putRecord func(ctx context.Context, id string, text string, vector []float32, model string) error
	
	// Request deduplication to prevent duplicate API calls
	inflight sync.Map // map[string]*call
}

// call represents an in-flight embedding request.
type call struct {
	wg  sync.WaitGroup
	vec []float32
	err error
}

// NewCached creates a cached embedder using store operations.
func NewCached(
	e Embedder,
	getRecord func(ctx context.Context, id string) (map[string][]float32, error),
	putRecord func(ctx context.Context, id string, text string, vector []float32, model string) error,
) *Cached {
	return &Cached{
		embedder:  e,
		getRecord: getRecord,
		putRecord: putRecord,
	}
}

// Embed returns cached embedding or calls the underlying embedder.
// Deduplicates concurrent requests for the same text.
func (c *Cached) Embed(ctx context.Context, text string) ([]float32, error) {
	hash := cacheKey(text)

	// Try cache first
	vectors, err := c.getRecord(ctx, hash)
	if err == nil && vectors != nil {
		if vec, ok := vectors[c.embedder.Model()]; ok && len(vec) > 0 {
			return vec, nil
		}
	}

	// Cache miss — check if someone else is already embedding this
	callVal, loaded := c.inflight.LoadOrStore(hash, &call{})
	callInfo := callVal.(*call)
	
	if loaded {
		// Someone else is already embedding this text, wait for them
		callInfo.wg.Wait()
		return callInfo.vec, callInfo.err
	}

	// We're the first — do the embedding
	callInfo.wg.Add(1)
	defer func() {
		callInfo.wg.Done()
		c.inflight.Delete(hash)
	}()

	// Call underlying embedder
	vec, err := c.embedder.Embed(ctx, text)
	if err != nil {
		callInfo.err = err
		return nil, err
	}

	callInfo.vec = vec

	// Store in cache synchronously with timeout (don't block caller indefinitely)
	cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := c.putRecord(cacheCtx, hash, text, vec, c.embedder.Model()); err != nil {
		// Log but don't fail the request
		log.Printf("embedder: cache write failed for hash %s: %v", hash[:8], err)
	}

	return vec, nil
}

// Model returns the underlying embedder's model.
func (c *Cached) Model() string {
	return c.embedder.Model()
}

// Dims returns the underlying embedder's dimensions.
func (c *Cached) Dims() int {
	return c.embedder.Dims()
}

func cacheKey(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
