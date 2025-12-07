package kv

import (
	"fmt"

	"github.com/stevegt/grokker/x/storm/db/bbolt"
)

// ReadTx defines read-only transaction operations
type ReadTx interface {
	Get(bucket, key string) ([]byte, bool)
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

// boltDBWrapper adapts bbolt.BoltDBStore to implement KVStore interface
type boltDBWrapper struct {
	store *bbolt.BoltDBStore
}

// View executes a read-only transaction
func (w *boltDBWrapper) View(fn func(ReadTx) error) error {
	return w.store.View(func(tx *bbolt.BoltReadTx) error {
		return fn(&kvReadTxAdapter{tx: tx})
	})
}

// Update executes a read-write transaction
func (w *boltDBWrapper) Update(fn func(WriteTx) error) error {
	return w.store.Update(func(tx *bbolt.BoltWriteTx) error {
		return fn(&kvWriteTxAdapter{tx: tx})
	})
}

// Close closes the store
func (w *boltDBWrapper) Close() error {
	return w.store.Close()
}

// kvReadTxAdapter adapts bbolt.BoltReadTx to kv.ReadTx interface
type kvReadTxAdapter struct {
	tx *bbolt.BoltReadTx
}

func (a *kvReadTxAdapter) Get(bucket, key string) ([]byte, bool) {
	return a.tx.Get(bucket, key)
}

func (a *kvReadTxAdapter) ForEach(bucket string, fn func(k, v []byte) error) error {
	return a.tx.ForEach(bucket, fn)
}

// kvWriteTxAdapter adapts bbolt.BoltWriteTx to kv.WriteTx interface
type kvWriteTxAdapter struct {
	tx *bbolt.BoltWriteTx
}

func (a *kvWriteTxAdapter) Get(bucket, key string) ([]byte, bool) {
	return a.tx.Get(bucket, key)
}

func (a *kvWriteTxAdapter) ForEach(bucket string, fn func(k, v []byte) error) error {
	return a.tx.ForEach(bucket, fn)
}

func (a *kvWriteTxAdapter) Put(bucket, key string, value []byte) error {
	return a.tx.Put(bucket, key, value)
}

func (a *kvWriteTxAdapter) Delete(bucket, key string) error {
	return a.tx.Delete(bucket, key)
}

func (a *kvWriteTxAdapter) CreateBucketIfNotExists(bucket string) error {
	return a.tx.CreateBucketIfNotExists(bucket)
}

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

// newBoltDBStore creates a wrapped BoltDB store implementing KVStore
func newBoltDBStore(dbPath string) (KVStore, error) {
	store, err := bbolt.NewBoltDBStore(dbPath)
	if err != nil {
		return nil, err
	}
	return &boltDBWrapper{store: store}, nil
}