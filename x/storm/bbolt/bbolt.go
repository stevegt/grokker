package main

import (
        "fmt"

        "github.com/fxamacker/cbor/v2"
        "go.etcd.io/bbolt"
)

// BoltDBStore implements KVStore interface using BoltDB for persistence
type BoltDBStore struct {
        db *bbolt.DB
}

// NewBoltDBStore creates a new BoltDB-backed KV store
func NewBoltDBStore(dbPath string) (*BoltDBStore, error) {
        db, err := bbolt.Open(dbPath, 0600, nil)
        if err != nil {
                return nil, fmt.Errorf("failed to open BoltDB: %w", err)
        }

        store := &BoltDBStore{db: db}

        // Initialize default buckets
        err = store.Update(func(tx WriteTx) error {
                // Create essential buckets if they don't exist
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
func (b *BoltDBStore) View(fn func(ReadTx) error) error {
        return b.db.View(func(tx *bbolt.Tx) error {
                return fn(&boltReadTx{tx: tx})
        })
}

// Update executes a read-write transaction
func (b *BoltDBStore) Update(fn func(WriteTx) error) error {
        return b.db.Update(func(tx *bbolt.Tx) error {
                return fn(&boltWriteTx{tx: tx})
        })
}

// Close closes the database connection
func (b *BoltDBStore) Close() error {
        return b.db.Close()
}

// BoltDB transaction adapters

type boltReadTx struct {
        tx *bbolt.Tx
}

func (b *boltReadTx) Get(bucket, key string) []byte {
        buck := b.tx.Bucket([]byte(bucket))
        if buck == nil {
                return nil
        }
        value := buck.Get([]byte(key))
        if value == nil {
                return nil
        }
        // Return a copy since BoltDB value pointers are only valid within transaction
        result := make([]byte, len(value))
        copy(result, value)
        return result
}

func (b *boltReadTx) ForEach(bucket string, fn func(k, v []byte) error) error {
        buck := b.tx.Bucket([]byte(bucket))
        if buck == nil {
                return nil // Bucket doesn't exist, skip
        }
        return buck.ForEach(func(k, v []byte) error {
                // Pass copies to callback since BoltDB pointers expire after transaction
                kCopy := make([]byte, len(k))
                copy(kCopy, k)
                vCopy := make([]byte, len(v))
                copy(vCopy, v)
                return fn(kCopy, vCopy)
        })
}

type boltWriteTx struct {
        tx *bbolt.Tx
}

func (b *boltWriteTx) Get(bucket, key string) []byte {
        buck := b.tx.Bucket([]byte(bucket))
        if buck == nil {
                return nil
        }
        value := buck.Get([]byte(key))
        if value == nil {
                return nil
        }
        result := make([]byte, len(value))
        copy(result, value)
        return result
}

func (b *boltWriteTx) ForEach(bucket string, fn func(k, v []byte) error) error {
        buck := b.tx.Bucket([]byte(bucket))
        if buck == nil {
                return nil
        }
        return buck.ForEach(func(k, v []byte) error {
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
                return nil // Bucket doesn't exist, nothing to delete
        }
        return buck.Delete([]byte(key))
}

func (b *boltWriteTx) CreateBucketIfNotExists(bucket string) error {
        _, err := b.tx.CreateBucketIfNotExists([]byte(bucket))
        return err
}

// CBOR encoding/decoding helpers for working with KVStore

// MarshalCBOR encodes a value to CBOR bytes using canonical encoding
func MarshalCBOR(v interface{}) ([]byte, error) {
        encOptions := cbor.EncOptions{Sort: cbor.SortCanonical}
        encoder, err := encOptions.EncMode()
        if err != nil {
                return nil, fmt.Errorf("failed to create CBOR encoder: %w", err)
        }
        return encoder.Marshal(v)
}

// UnmarshalCBOR decodes CBOR bytes into a value
func UnmarshalCBOR(data []byte, v interface{}) error {
        decOptions := cbor.DecOptions{}
        decoder, err := decOptions.DecMode()
        if err != nil {
                return fmt.Errorf("failed to create CBOR decoder: %w", err)
        }
        return decoder.Unmarshal(data, v)
}

// StoreCBOR is a convenience helper to CBOR-encode and store a value
func StoreCBOR(tx WriteTx, bucket, key string, value interface{}) error {
        data, err := MarshalCBOR(value)
        if err != nil {
                return err
        }
        return tx.Put(bucket, key, data)
}

// LoadCBOR is a convenience helper to retrieve and CBOR-decode a value
func LoadCBOR(tx ReadTx, bucket, key string, value interface{}) error {
        data := tx.Get(bucket, key)
        if data == nil {
                return fmt.Errorf("key %s not found in bucket %s", key, bucket)
        }
        return UnmarshalCBOR(data, value)
}

// LoadCBORIfExists is like LoadCBOR but returns nil if key doesn't exist
func LoadCBORIfExists(tx ReadTx, bucket, key string, value interface{}) error {
        data := tx.Get(bucket, key)
        if data == nil {
                return nil // Key doesn't exist, that's fine
        }
        return UnmarshalCBOR(data, value)
}
