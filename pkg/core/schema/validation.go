package schema

import (
	"errors"
	"fmt"
)

func ValidateEnvelope(envelope RawEnvelope) error {
	if envelope.ID == "" {
		return errors.New("id is required")
	}
	if envelope.Source == "" {
		return errors.New("source is required")
	}
	if envelope.SignalType == "" {
		return errors.New("signal_type is required")
	}
	if !isValidSignalType(envelope.SignalType) {
		return fmt.Errorf("signal_type %q is invalid", envelope.SignalType)
	}
	if envelope.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	if len(envelope.Payload) == 0 {
		return errors.New("payload is required")
	}
	if err := ValidateSchemaVersion(envelope.Version); err != nil {
		return fmt.Errorf("version: %w", err)
	}

	return nil
}

func isValidSignalType(signalType SignalType) bool {
	switch signalType {
	case SignalMetric, SignalTrace:
		return true
	default:
		return false
	}
}

func ValidateSchemaVersion(version SchemaVersion) error {
	if version.Major < 1 {
		return errors.New("major version must be >= 1")
	}
	if version.Minor < 0 {
		return errors.New("minor version must be >= 0")
	}

	return nil
}
