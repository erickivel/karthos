package entities

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func ComputeEntityID(kind EntityKind, parts ...string) EntityID {
	builder := strings.Builder{}
	builder.WriteString(string(kind))

	for _, part := range parts {
		builder.WriteString("/")
		builder.WriteString(strings.TrimSpace(part))
	}

	sum := sha256.Sum256([]byte(builder.String()))
	return EntityID(hex.EncodeToString(sum[:]))
}

// IsCanonicalID reports whether id is an OKSG canonical SHA-256 entity ID.
// Public query boundaries use this to reject arbitrary label values before
// they reach a storage adapter.
func IsCanonicalID(id EntityID) bool {
	if len(id) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(string(id))
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	return string(id) == strings.ToLower(string(id))
}
