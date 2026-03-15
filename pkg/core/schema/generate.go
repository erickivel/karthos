package schema

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/invopop/jsonschema"
)

//go:generate go run ../../../cmd/schema-gen

func GenerateSchemas(root string) error {
	schemasDir := filepath.Join(root, "schemas")
	if err := os.MkdirAll(schemasDir, 0o750); err != nil {
		return err
	}

	reflector := jsonschema.Reflector{}
	envelopeSchema := reflector.Reflect(&RawEnvelope{})

	data, err := json.MarshalIndent(envelopeSchema, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(schemasDir, "raw-envelope.schema.json"), data, 0o600)
}
