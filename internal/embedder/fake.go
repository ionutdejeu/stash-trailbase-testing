package embedder

import (
	"context"
	"hash/fnv"
)

// Fake returns deterministic vectors for testing.
// Same input always produces the same output.
// No external calls. No API key required.
// NOT suitable for semantic correctness testing — only plumbing tests.
type Fake struct{}

// NewFake creates a Fake embedder.
// No configuration required — no model, no dims, no API key.
func NewFake() *Fake {
	return &Fake{}
}

// Model returns "fake".
func (f *Fake) Model() string {
	return "fake"
}

// Dims returns 8.
func (f *Fake) Dims() int {
	return 8
}

// Embed returns a deterministic 8-dimensional vector based on fnv32a hash of input.
// Same text always produces the same output.
func (f *Fake) Embed(_ context.Context, text string) ([]float32, error) {
	h := fnv.New32a()
	h.Write([]byte(text))
	sum := h.Sum32()

	vec := make([]float32, 8)
	for i := range vec {
		vec[i] = float32((sum>>uint(i*4))&0xFF) / 127.5 - 1.0
	}
	return vec, nil
}