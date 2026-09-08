package persistence

import (
	"context"
	"sync"

	"github.com/erickivel/oksg/pkg/discovery/profile"
)

// InMemoryMetadataStore is a thread-safe in-memory implementation of MetadataStore.
type InMemoryMetadataStore struct {
	mu      sync.RWMutex
	profile *profile.ClusterProfile
}

// NewInMemoryMetadataStore returns a new empty InMemoryMetadataStore.
func NewInMemoryMetadataStore() *InMemoryMetadataStore {
	return &InMemoryMetadataStore{}
}

// PutDiscovery stores the given cluster profile, replacing any previous one.
func (s *InMemoryMetadataStore) PutDiscovery(_ context.Context, snapshot profile.ClusterProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := cloneClusterProfile(snapshot)
	s.profile = &cloned
	return nil
}

// GetDiscovery returns the most recently stored cluster profile.
// Returns nil, nil if no profile has been stored.
func (s *InMemoryMetadataStore) GetDiscovery(_ context.Context) (*profile.ClusterProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.profile == nil {
		return nil, nil
	}

	cp := cloneClusterProfile(*s.profile)
	return &cp, nil
}
