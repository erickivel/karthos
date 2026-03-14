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
