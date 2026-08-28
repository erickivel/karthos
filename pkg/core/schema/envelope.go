package schema

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type SignalType string

const (
	SignalMetric SignalType = "metric"
	SignalTrace  SignalType = "trace"
)

type RawEnvelope struct {
	ID         string            `json:"id"`
	Version    SchemaVersion     `json:"version"`
	Source     string            `json:"source"`
	SignalType SignalType        `json:"signal_type" jsonschema:"enum=metric,enum=trace"`
	Timestamp  time.Time         `json:"timestamp"`
	Resource   map[string]string `json:"resource,omitempty"`
	Payload    json.RawMessage   `json:"payload"`
}

func NewRawEnvelope(source string, signalType SignalType, resource map[string]string, payload json.RawMessage, timestamp time.Time) (RawEnvelope, error) {
	envelope := RawEnvelope{
		ID:         uuid.NewString(),
		Version:    CurrentVersion,
		Source:     source,
		SignalType: signalType,
		Timestamp:  timestamp,
		Resource:   cloneStringMap(resource),
		Payload:    payload,
	}

	if err := ValidateEnvelope(envelope); err != nil {
		return RawEnvelope{}, err
	}

	return envelope, nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}

	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}

	return output
}
