package schema

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewRawEnvelopePopulatesCoreFields(t *testing.T) {
	payload := json.RawMessage(`{"value":42}`)
	resource := map[string]string{"service.name": "checkout"}

	envelope, err := NewRawEnvelope("prometheus", SignalMetric, resource, payload, time.Unix(1700000000, 0))
	require.NoError(t, err)
	require.NotEmpty(t, envelope.ID)
	require.Equal(t, "prometheus", envelope.Source)
	require.Equal(t, SignalMetric, envelope.SignalType)
}

func TestValidateEnvelopeRejectsMissingRequiredFields(t *testing.T) {
	envelope := RawEnvelope{}

	require.Error(t, ValidateEnvelope(envelope))
}

func TestValidateEnvelopeRejectsUnknownSignalType(t *testing.T) {
	envelope := RawEnvelope{
		ID:         "evt-1",
		Version:    CurrentVersion,
		Source:     "prometheus",
		SignalType: SignalType("unknown"),
		Timestamp:  time.Unix(1700000000, 0),
		Payload:    json.RawMessage(`{"value":1}`),
		ReceivedAt: time.Unix(1700000001, 0),
	}

	require.Error(t, ValidateEnvelope(envelope))
}
