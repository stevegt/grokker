package db

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
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

// Manager provides database operations for Storm
type Manager struct {
	store kv.KVStore
}

// NewManager creates a new database manager
func NewManager(dbPath string) (*Manager, error) {
	store, err := kv.NewStoreDefault(dbPath)
	if err != nil {
		return nil, err
	}
	return &Manager{store: store}, nil
}

// Close closes the database
func (m *Manager) Close() error {
	return m.store.Close()
}
