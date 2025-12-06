package main

// ReadTx represents a read-only transaction against a KV store
type ReadTx interface {
        // Get retrieves a value from a bucket by key
        // Returns nil if the key does not exist
        Get(bucket, key string) []byte

        // ForEach iterates over all key-value pairs in a bucket
        // fn should not hold references to keys or values beyond the callback
        ForEach(bucket string, fn func(k, v []byte) error) error
}

// WriteTx represents a read-write transaction against a KV store
type WriteTx interface {
        ReadTx

        // Put stores a value in a bucket under a key
        // Creates the bucket if it doesn't exist
        Put(bucket, key string, value []byte) error

        // Delete removes a value from a bucket by key
        // Returns nil if the key doesn't exist
        Delete(bucket, key string) error

        // CreateBucketIfNotExists ensures a bucket exists
        CreateBucketIfNotExists(bucket string) error
}

// KVStore defines the interface for persistent key-value storage
// All interactions with the store must go through transactions to ensure consistency
type KVStore interface {
        // View executes a read-only transaction
        // The transaction callback receives a ReadTx for reading data
        // View callbacks should not attempt write operations
        View(fn func(ReadTx) error) error

        // Update executes a read-write transaction
        // The transaction callback receives a WriteTx for reading and writing data
        // All operations within Update are atomic and consistent
        Update(fn func(WriteTx) error) error

        // Close closes the KV store and releases resources
        // No operations should be performed after Close() is called
        Close() error
}
