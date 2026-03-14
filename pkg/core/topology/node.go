package topology

import "github.com/erickivel/oksg/pkg/core/entities"

type Node struct {
	ID     entities.EntityID   `json:"id"`
	Kind   entities.EntityKind `json:"kind"`
	Labels map[string]string   `json:"labels,omitempty"`
}

func NewNode(entity entities.Entity) Node {
	return Node{
		ID:     entity.ID(),
		Kind:   entity.Kind(),
		Labels: entity.Labels(),
	}
}
