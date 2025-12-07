package bbolt

import (
	"bytes"
	"path/filepath"
	"testing"
)

func createTestStore(t *testing.T) *BoltDBStore {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	// Type assertion to get concrete type for testing
	return store.(*BoltDBStore)
}

func TestNewBoltDBStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewBoltDBStore(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	if store == nil {
		t.Fatal("Store is nil")
	}
	store.Close()
}

func TestViewTransaction(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.Update(func(tx interface{}) error {
		writeTx := tx.(*boltWriteTx)
		return writeTx.Put("projects", "key1", []byte("value1"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	err = store.View(func(tx interface{}) error {
		readTx := tx.(*boltReadTx)
		value, ok := readTx.Get("projects", "key1")
		if !ok {
			t.Fatal("Key should exist")
		}
		if !bytes.Equal(value, []byte("value1")) {
			t.Fatal("Value mismatch")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
}

func TestUpdateTransaction(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.Update(func(tx interface{}) error {
		writeTx := tx.(*boltWriteTx)
		if err := writeTx.Put("files", "f1", []byte("data1")); err != nil {
			return err
		}
		return writeTx.Put("files", "f2", []byte("data2"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	err = store.View(func(tx interface{}) error {
		readTx := tx.(*boltReadTx)
		val1, ok1 := readTx.Get("files", "f1")
		if !ok1 {
			t.Fatal("f1 should exist")
		}
		if !bytes.Equal(val1, []byte("data1")) {
			t.Fatal("f1 mismatch")
		}

		val2, ok2 := readTx.Get("files", "f2")
		if !ok2 {
			t.Fatal("f2 should exist")
		}
		if !bytes.Equal(val2, []byte("data2")) {
			t.Fatal("f2 mismatch")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to verify: %v", err)
	}
}

func TestForEachBucket(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.Update(func(tx interface{}) error {
		writeTx := tx.(*boltWriteTx)
		for i := 0; i < 5; i++ {
			key := string(rune('a' + i))
			if err := writeTx.Put("embeddings", key, []byte("val")); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	count := 0
	err = store.View(func(tx interface{}) error {
		readTx := tx.(*boltReadTx)
		return readTx.ForEach("embeddings", func(k, v []byte) error {
			count++
			return nil
		})
	})
	if err != nil {
		t.Fatalf("Failed to iterate: %v", err)
	}

	if count != 5 {
		t.Fatalf("Expected 5 items, got %d", count)
	}
}

func TestDelete(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.Update(func(tx interface{}) error {
		writeTx := tx.(*boltWriteTx)
		return writeTx.Put("config", "key1", []byte("value"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	err = store.Update(func(tx interface{}) error {
		writeTx := tx.(*boltWriteTx)
		return writeTx.Delete("config", "key1")
	})
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	err = store.View(func(tx interface{}) error {
		readTx := tx.(*boltReadTx)
		_, ok := readTx.Get("config", "key1")
		if ok {
			t.Fatal("Expected key to not exist after deletion")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
}

func TestGetNonexistentKey(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.View(func(tx interface{}) error {
		readTx := tx.(*boltReadTx)
		_, ok := readTx.Get("projects", "nonexistent")
		if ok {
			t.Fatal("Expected key to not exist")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
}

func TestPersistenceAcrossInstances(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")

	store1, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store1: %v", err)
	}
	err = store1.Update(func(tx interface{}) error {
		writeTx := tx.(*boltWriteTx)
		return writeTx.Put("projects", "persist_key", []byte("persist_val"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	store1.Close()

	store2, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store2: %v", err)
	}
	defer store2.Close()

	err = store2.View(func(tx interface{}) error {
		readTx := tx.(*boltReadTx)
		val, ok := readTx.Get("projects", "persist_key")
		if !ok {
			t.Fatal("Data should persist")
		}
		if !bytes.Equal(val, []byte("persist_val")) {
			t.Fatal("Data mismatch after persistence")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to verify: %v", err)
	}
}

func BenchmarkPut(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := NewBoltDBStore(filepath.Join(tmpDir, "bench.db"))
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Update(func(tx interface{}) error {
			writeTx := tx.(*boltWriteTx)
			return writeTx.Put("projects", "key", []byte("value"))
		})
	}
}

func BenchmarkGet(b *testing.B) {
	tmpDir := b.TempDir()
	store, err := NewBoltDBStore(filepath.Join(tmpDir, "bench.db"))
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	store.Update(func(tx interface{}) error {
		writeTx := tx.(*boltWriteTx)
		return writeTx.Put("projects", "key", []byte("value"))
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.View(func(tx interface{}) error {
			readTx := tx.(*boltReadTx)
			_, _ = readTx.Get("projects", "key")
			return nil
		})
	}
}