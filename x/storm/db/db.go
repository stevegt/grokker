package db

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/stevegt/grokker/x/storm/db/bbolt"
	"github.com/stevegt/grokker/x/storm/db/kv"
)

// MarshalCBOR marshals data to CBOR canonical form
func MarshalCBOR(v interface{}) ([]byte, error) {
	encOptions := cbor.EncOptions{Sort: cbor.SortCanonical}
	encoder, err := encOptions.EncMode()
	if err != nil {
		return nil, fmt.Errorf("failed to create CBOR encoder: %w", err)
	}
	return encoder.Marshal(v)
}

// UnmarshalCBOR unmarshals CBOR data
func UnmarshalCBOR(data []byte, v interface{}) error {
	decOptions := cbor.DecOptions{}
	decoder, err := decOptions.DecMode()
	if err != nil {
		return fmt.Errorf("failed to create CBOR decoder: %w", err)
	}
	return decoder.Unmarshal(data, v)
}

// BackendType specifies which backend implementation to use
type BackendType string

const (
	BoltDB BackendType = "bbolt"
)

// NewStore creates a KVStore instance for the specified backend
func NewStore(dbPath string, backend BackendType) (kv.KVStore, error) {
	switch backend {
	case BoltDB:
		return bbolt.NewBoltDBStore(dbPath)
	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}

// NewStoreDefault creates a KVStore with BoltDB backend
func NewStoreDefault(dbPath string) (kv.KVStore, error) {
	return NewStore(dbPath, BoltDB)
}

// Manager provides database operations for Storm
type Manager struct {
	store kv.KVStore
}

// NewManager creates a new database manager
func NewManager(dbPath string) (*Manager, error) {
	store, err := NewStoreDefault(dbPath)
	if err != nil {
		return nil, err
	}
	return &Manager{store: store}, nil
}

// Close closes the database
func (m *Manager) Close() error {
	return m.store.Close()
}