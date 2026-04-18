package mapdb

import (
	"testing"

	"github.com/alash3al/stash/internal/store/storetest"
)

func TestStoreSuite(t *testing.T) {
	s, err := New(Config{VectorDim: 3})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	storetest.RunSuite(t, s)
}
