package topology

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
)

type Edge struct {
	ID         string            `json:"id"`
	Source     entities.EntityID `json:"source"`
	Target     entities.EntityID `json:"target"`
	Relation   RelationType      `json:"relation"`
	Confidence float64           `json:"confidence"`
	Provenance []string          `json:"provenance,omitempty"`
	FirstSeen  time.Time         `json:"first_seen"`
	LastSeen   time.Time         `json:"last_seen"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func NewEdge(source, target entities.EntityID, relation RelationType, firstSeen time.Time) Edge {
	return Edge{
		ID:         ComputeEdgeID(source, target, relation),
		Source:     source,
		Target:     target,
		Relation:   relation,
		Confidence: 1.0,
		FirstSeen:  firstSeen,
		LastSeen:   firstSeen,
	}
}

func ComputeEdgeID(source, target entities.EntityID, relation RelationType) string {
	return ComputeEdgeIDWithDiscriminator(source, target, relation, "")
}

// ComputeEdgeIDWithDiscriminator adds a stable source-specific identity to an
// edge while preserving its public source and target endpoints. An empty
// discriminator retains the historical edge ID.
func ComputeEdgeIDWithDiscriminator(source, target entities.EntityID, relation RelationType, discriminator string) string {
	parts := []string{string(source), string(target), string(relation)}
	if discriminator != "" {
		parts = append(parts, discriminator)
	}
	payload := strings.Join(parts, "/")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
