package kv

import (
	"fmt"

	"github.com/stevegt/grokker/x/storm/db/kv/bbolt"
)

// ReadTx defines read-only transaction operations
type ReadTx interface {
	Get(bucket, key string) []byte
	ForEach(bucket string, fn func(k, v []byte) error) error
}

// WriteTx defines read-write transaction operations
type WriteTx interface {
	ReadTx
	Put(bucket, key string, value []byte) error
	Delete(bucket, key string) error
	CreateBucketIfNotExists(bucket string) error
}

// KVStore defines the key-value store abstraction
type KVStore interface {
	View(fn func(ReadTx) error) error
	Update(fn func(WriteTx) error) error
	Close() error
}

// BackendType specifies which backend implementation to use
type BackendType string

const (
	BoltDB BackendType = "bbolt"
)

// NewStore creates a KVStore instance for the specified backend
func NewStore(dbPath string, backend BackendType) (KVStore, error) {
	switch backend {
	case BoltDB:
		return newBoltDBStore(dbPath)
	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}

// NewStoreDefault creates a KVStore with BoltDB backend
func NewStoreDefault(dbPath string) (KVStore, error) {
	return NewStore(dbPath, BoltDB)
}

// newBoltDBStore wraps bbolt.BoltDBStore to implement KVStore interface
func newBoltDBStore(dbPath string) (KVStore, error) {
	return bbolt.NewBoltDBStore(dbPath)
}
