package kv

import (
	"time"

	. "github.com/stevegt/goadapt"
	bolt "go.etcd.io/bbolt"
)

// Db is a generic key-value database that supports transactions and
// buckets.  Keys and bucket names are strings.  Values are byte
// arrays. This struct is an adapter for bolt.
type Db struct {
	bdb *bolt.DB
}

// Open opens a database, creating it if it doesn't exist.
func Open(path string) (db *Db, err error) {
	defer Return(&err)
	db = &Db{}
	opts := &bolt.Options{Timeout: 10 * time.Second}
	db.bdb, err = bolt.Open(path, 0600, opts)
	Ck(err)
	return
}

// Close closes the db.
func (db *Db) Close() (err error) {
	defer Return(&err)
	err = db.bdb.Close()
	Ck(err)
	return
}

// Begin starts a tansaction
func (db *Db) Begin(writable bool) (tx *Tx, err error) {
	defer Return(&err)
	btx, err := db.bdb.Begin(writable)
	Ck(err)
	tx = &Tx{btx}
	return
}

// Tx is a generic transaction.  This struct is an adapter for bolt.
type Tx struct {
	btx *bolt.Tx
}

// Rollback rolls back a transaction
func (tx *Tx) Rollback() (err error) {
	defer Return(&err)
	err = tx.btx.Rollback()
	Ck(err)
	return
}

// Commit commits a transaction
func (tx *Tx) Commit() (err error) {
	defer Return(&err)
	err = tx.btx.Commit()
	Ck(err)
	return
}

// Put adds or replaces a record in the given bucket.
func (tx *Tx) Put(bucket string, key string, value []byte) (err error) {
	defer Return(&err)
	Pl("put", bucket, key, value)
	b := tx.btx.Bucket([]byte(bucket))
	Pf("bucket %s %v\n", bucket, b)
	if b == nil {
		Pf("creating bucket %s\n", bucket)
		b, err = tx.MakeBucket(bucket)
	}
	Pf("put %s %s %v\n", bucket, key, value)
	err = b.Put([]byte(key), value)
	Ck(err)
	return
}

// Get retrieves a record from the given bucket. Returns a nil value
// if the key or bucket does not exist or if the key is a nested bucket.
func (tx *Tx) Get(bucket string, key string) (value []byte, err error) {
	defer Return(&err)
	b := tx.btx.Bucket([]byte(bucket))
	if b == nil {
		return
	}
	value = b.Get([]byte(key))
	return
}

// Delete removes a record from the given bucket.
func (tx *Tx) Delete(bucket string, key string) (err error) {
	defer Return(&err)
	b := tx.btx.Bucket([]byte(bucket))
	if b == nil {
		return
	}
	err = b.Delete([]byte(key))
	Ck(err)
	return
}

// List generates a list of all keys in the given bucket.
func (tx *Tx) List(bucket string) (keys chan string, err error) {
	defer Return(&err)
	keys = make(chan string)
	b := tx.btx.Bucket([]byte(bucket))
	if b == nil {
		close(keys)
		return
	}
	go func() {
		defer close(keys)
		b.ForEach(func(k, v []byte) error {
			keys <- string(k)
			return nil
		})
	}()
	return
}

// MakeBucket creates a new bucket.
func (tx *Tx) MakeBucket(bucket string) (b *bolt.Bucket, err error) {
	defer Return(&err)
	b, err = tx.btx.CreateBucketIfNotExists([]byte(bucket))
	Ck(err)
	return
}
