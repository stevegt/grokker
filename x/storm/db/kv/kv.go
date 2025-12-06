package kv

import (
	"github.com/stevegt/grokker/x/storm/db/kv/bbolt"
)

// ReadTx defines read operations in a transaction
type ReadTx interface {
	Get(bucket, key string) []byte
	ForEach(bucket string, fn func(k, v []byte) error) error
}

// WriteTx defines read and write operations in a transaction
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

// NewBoltDBStore creates a new BoltDB-backed KV store
func NewBoltDBStore(dbPath string) (KVStore, error) {
	return bbolt.NewBoltDBStore(dbPath)
}
