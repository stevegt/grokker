package kv

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