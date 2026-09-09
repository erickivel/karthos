package storage

import "time"

// StorageBackend identifies which storage technology to use.
type StorageBackend string

const (
	// BackendMemory uses in-memory stores. Suitable for testing and development.
	BackendMemory StorageBackend = "memory"

	// BackendExternal uses PostgreSQL for graph/metadata and VictoriaMetrics for metrics.
	BackendExternal StorageBackend = "external"
)

// StorageConfig holds all storage-related configuration needed by the factory.
type StorageConfig struct {
	Backend     StorageBackend
	PostgresDSN string        // required when Backend == BackendExternal
	VMURL       string        // required when Backend == BackendExternal
	NodeTTL     time.Duration // age out topology nodes; 0 disables
}
