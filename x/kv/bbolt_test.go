package kv

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/stevegt/goadapt"
)

var tmpDir string

func TestMain(m *testing.M) {
	// Create temporary directory
	var err error
	tmpDir, err = ioutil.TempDir("", "grokker")
	Ck(err)

	// Run tests
	exitCode := m.Run()

	// Remove temporary directory
	err = os.RemoveAll(tmpDir)
	Ck(err)

	// Exit with test status code
	os.Exit(exitCode)
}

func newDb(t *testing.T) (db *Db) {
	fn := filepath.Join(tmpDir, "test.db")
	db, err := Open(fn)
	Tassert(t, err == nil)
	Tassert(t, db != nil)
	return
}

// As a caller, I want to be able to create a new database.
func TestOpen(t *testing.T) {
	db := newDb(t)
	defer db.Close()
}

// As a caller, I want to be able to put a chunk of data and get back
// the hash, then retrieve the data using the hash. If the bucket
// doesn't exist, it should be created.
func TestPut(t *testing.T) {
	db := newDb(t)
	defer db.Close()

	// start a transaction
	tx, err := db.Begin(true)
	Tassert(t, err == nil)

	Pl("putting data")

	// put some data
	err = tx.Put("bucket1", "key1", []byte("Hello, world!"))
	Tassert(t, err == nil)

	Pl("after put")

	// commit the transaction
	err = tx.Commit()
	Tassert(t, err == nil)

	// start a new transaction
	tx, err = db.Begin(false)
	Tassert(t, err == nil)

	// get the data back
	data, err := tx.Get("bucket1", "key1")
	Tassert(t, err == nil)
	Tassert(t, string(data) == "Hello, world!")

	// close the transaction
	err = tx.Rollback()
	Tassert(t, err == nil)
}

// As a caller I want to be able to get a key from a non-existent
// bucket, and get back a nil value.
func TestGetNonExistentBucket(t *testing.T) {
	db := newDb(t)
	defer db.Close()

	// start a transaction
	tx, err := db.Begin(false)
	Tassert(t, err == nil)

	// get the data back
	data, err := tx.Get("bucket99", "key99")
	Tassert(t, err == nil)
	Tassert(t, data == nil)

	// close the transaction
	err = tx.Rollback()
	Tassert(t, err == nil)
}

// As a caller, I want to be able to delete a key.  If the bucket
// or key doesn't exist, it should be a no-op.
func TestDelete(t *testing.T) {
	db := newDb(t)
	defer db.Close()

	// start a transaction
	tx, err := db.Begin(true)
	Tassert(t, err == nil)

	// put some data
	err = tx.Put("bucket1", "key1", []byte("Hello, world!"))
	Tassert(t, err == nil)

	// delete the data
	err = tx.Delete("bucket1", "key1")
	Tassert(t, err == nil)

	// commit the transaction
	err = tx.Commit()
	Tassert(t, err == nil)

	// start a new transaction
	tx, err = db.Begin(true)
	Tassert(t, err == nil)

	// get the data back
	data, err := tx.Get("bucket1", "key1")
	Tassert(t, err == nil)
	Tassert(t, data == nil)

	// delete the data again
	err = tx.Delete("bucket1", "key1")
	Tassert(t, err == nil)

	// delete data in a non-existent bucket
	err = tx.Delete("bucket99", "key99")
	Tassert(t, err == nil)

	// close the transaction
	err = tx.Rollback()
	Tassert(t, err == nil)
}

// As a caller, I want to be able to list all keys in a bucket.
func TestList(t *testing.T) {
	db := newDb(t)
	defer db.Close()

	// start a transaction
	tx, err := db.Begin(true)
	Tassert(t, err == nil)

	// put some data
	err = tx.Put("bucket1", "key1", []byte("Hello, world!"))
	Tassert(t, err == nil)
	err = tx.Put("bucket1", "key2", []byte("Hello, world!"))
	Tassert(t, err == nil)
	err = tx.Put("bucket1", "key3", []byte("Hello, world!"))
	Tassert(t, err == nil)
	err = tx.Put("bucket1", "key4", []byte("Hello, world!"))
	Tassert(t, err == nil)

	// commit the transaction
	err = tx.Commit()
	Tassert(t, err == nil)

	// start a new transaction
	tx, err = db.Begin(false)
	Tassert(t, err == nil)

	// list the keys
	keys, err := tx.List("bucket1")
	Tassert(t, err == nil)
	i := 1
	for key := range keys {
		Tassert(t, key == Spf("key%d", i))
		i++
	}

	// close the transaction
	err = tx.Rollback()
	Tassert(t, err == nil)
}

// As a caller, when I list keys in a non-existent bucket, I should
// get back an empty list.
func TestListNonExistentBucket(t *testing.T) {
	db := newDb(t)
	defer db.Close()

	// start a transaction
	tx, err := db.Begin(false)
	Tassert(t, err == nil)

	// list the keys
	keys, err := tx.List("bucket99")
	Tassert(t, err == nil)
	i := 1
	for key := range keys {
		Tassert(t, key == Spf("key%d", i))
		i++
	}

	// close the transaction
	err = tx.Rollback()
	Tassert(t, err == nil)
}
