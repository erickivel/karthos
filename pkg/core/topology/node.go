package topology

import "github.com/erickivel/oksg/pkg/core/entities"

// Node is the generic graph projection of a canonical entity. Construction is
// explicit at each boundary so topology does not duplicate entity wrappers.
type Node struct {
	ID     entities.EntityID   `json:"id"`
	Kind   entities.EntityKind `json:"kind"`
	Labels map[string]string   `json:"labels,omitempty"`
}
