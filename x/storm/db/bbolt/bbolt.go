package bbolt

import (
	"fmt"

	"github.com/stevegt/grokker/x/storm/db/kv"
	"go.etcd.io/bbolt"
)

// BoltDBStore wraps bbolt.DB and implements kv.KVStore interface
type BoltDBStore struct {
	db *bbolt.DB
}

// NewBoltDBStore creates and initializes a BoltDB store implementing kv.KVStore
func NewBoltDBStore(dbPath string) (kv.KVStore, error) {
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open BoltDB: %w", err)
	}

	store := &BoltDBStore{db: db}

	// Initialize default buckets
	err = store.Update(func(tx kv.WriteTx) error {
		requiredBuckets := []string{
			"projects",
			"files",
			"embeddings",
			"hnsw_metadata",
			"config",
		}
		for _, bucketName := range requiredBuckets {
			if err := tx.CreateBucketIfNotExists(bucketName); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucketName, err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// View executes a read-only transaction
func (b *BoltDBStore) View(fn func(kv.ReadTx) error) error {
	return b.db.View(func(tx *bbolt.Tx) error {
		return fn(&boltReadTx{tx: tx})
	})
}

// Update executes a read-write transaction
func (b *BoltDBStore) Update(fn func(kv.WriteTx) error) error {
	return b.db.Update(func(tx *bbolt.Tx) error {
		return fn(&boltWriteTx{tx: tx})
	})
}

// Close closes the database
func (b *BoltDBStore) Close() error {
	return b.db.Close()
}

// boltReadTx implements kv.ReadTx interface
type boltReadTx struct {
	tx *bbolt.Tx
}

// Get retrieves a value from the bucket. Returns (value, true) if key exists, (nil, false) otherwise.
// The returned byte slice is a copy and remains valid after the transaction ends.
func (b *boltReadTx) Get(bucket, key string) ([]byte, bool) {
	buck := b.tx.Bucket([]byte(bucket))
	if buck == nil {
		return nil, false
	}
	value := buck.Get([]byte(key))
	if value == nil {
		return nil, false
	}
	// Copy required: BoltDB's memory-mapped values are only valid during the transaction
	result := make([]byte, len(value))
	copy(result, value)
	return result, true
}

// ForEach iterates over all key-value pairs in the bucket.
// Keys and values are copied to remain valid after the transaction.
func (b *boltReadTx) ForEach(bucket string, fn func(k, v []byte) error) error {
	buck := b.tx.Bucket([]byte(bucket))
	if buck == nil {
		return nil
	}
	return buck.ForEach(func(k, v []byte) error {
		// Copy required: BoltDB's memory-mapped data is only valid during iteration
		kCopy := make([]byte, len(k))
		copy(kCopy, k)
		vCopy := make([]byte, len(v))
		copy(vCopy, v)
		return fn(kCopy, vCopy)
	})
}

// boltWriteTx implements kv.WriteTx interface
type boltWriteTx struct {
	tx *bbolt.Tx
}

// Get retrieves a value from the bucket. Returns (value, true) if key exists, (nil, false) otherwise.
// The returned byte slice is a copy and remains valid after the transaction ends.
func (b *boltWriteTx) Get(bucket, key string) ([]byte, bool) {
	buck := b.tx.Bucket([]byte(bucket))
	if buck == nil {
		return nil, false
	}
	value := buck.Get([]byte(key))
	if value == nil {
		return nil, false
	}
	// Copy required: BoltDB's memory-mapped values are only valid during the transaction
	result := make([]byte, len(value))
	copy(result, value)
	return result, true
}

// ForEach iterates over all key-value pairs in the bucket.
// Keys and values are copied to remain valid after the transaction.
func (b *boltWriteTx) ForEach(bucket string, fn func(k, v []byte) error) error {
	buck := b.tx.Bucket([]byte(bucket))
	if buck == nil {
		return nil
	}
	return buck.ForEach(func(k, v []byte) error {
		// Copy required: BoltDB's memory-mapped data is only valid during iteration
		kCopy := make([]byte, len(k))
		copy(kCopy, k)
		vCopy := make([]byte, len(v))
		copy(vCopy, v)
		return fn(kCopy, vCopy)
	})
}

func (b *boltWriteTx) Put(bucket, key string, value []byte) error {
	buck, err := b.tx.CreateBucketIfNotExists([]byte(bucket))
	if err != nil {
		return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
	}
	return buck.Put([]byte(key), value)
}

func (b *boltWriteTx) Delete(bucket, key string) error {
	buck := b.tx.Bucket([]byte(bucket))
	if buck == nil {
		return nil
	}
	return buck.Delete([]byte(key))
}

func (b *boltWriteTx) CreateBucketIfNotExists(bucket string) error {
	_, err := b.tx.CreateBucketIfNotExists([]byte(bucket))
	return err
}