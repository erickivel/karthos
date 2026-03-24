package persistence

import (
	"context"
	"sync"

	"github.com/erickivel/oksg/pkg/core/schema"
)

// InMemoryRawStore is a thread-safe in-memory implementation of RawStore.
type InMemoryRawStore struct {
	mu        sync.RWMutex
	envelopes []schema.RawEnvelope
}

// NewInMemoryRawStore returns a new empty InMemoryRawStore.
func NewInMemoryRawStore() *InMemoryRawStore {
	return &InMemoryRawStore{}
}

// WriteRaw appends envelopes to the store.
func (s *InMemoryRawStore) WriteRaw(_ context.Context, envelopes []schema.RawEnvelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.envelopes = append(s.envelopes, envelopes...)
	return nil
}

// QueryRaw filters envelopes by SignalType (if non-empty), Source (if non-empty),
// and time range (if non-zero). Returns a copy of the matching envelopes.
func (s *InMemoryRawStore) QueryRaw(_ context.Context, q RawQuery) ([]schema.RawEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []schema.RawEnvelope
	for _, env := range s.envelopes {
		if q.SignalType != "" && env.SignalType != q.SignalType {
			continue
		}
		if q.Source != "" && env.Source != q.Source {
			continue
		}
		if !q.From.IsZero() && env.Timestamp.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && env.Timestamp.After(q.To) {
			continue
		}
		result = append(result, env)
	}
	return result, nil
}
