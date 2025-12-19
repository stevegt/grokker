package bbolt

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stevegt/grokker/x/storm/db/kv"
)

func createTestStore(t *testing.T) kv.KVStore {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	return store
}

func TestNewBoltDBStore(t *testing.T) {
	store := createTestStore(t)
	if store == nil {
		t.Fatal("Store is nil")
	}
	store.Close()
}

func TestViewTransaction(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.Update(func(tx kv.WriteTx) error {
		return tx.Put("projects", "key1", []byte("value1"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	err = store.View(func(tx kv.ReadTx) error {
		value, ok := tx.Get("projects", "key1")
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

	err := store.Update(func(tx kv.WriteTx) error {
		if err := tx.Put("files", "f1", []byte("data1")); err != nil {
			return err
		}
		return tx.Put("files", "f2", []byte("data2"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	err = store.View(func(tx kv.ReadTx) error {
		val1, ok1 := tx.Get("files", "f1")
		if !ok1 {
			t.Fatal("f1 should exist")
		}
		if !bytes.Equal(val1, []byte("data1")) {
			t.Fatal("f1 mismatch")
		}

		val2, ok2 := tx.Get("files", "f2")
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

	err := store.Update(func(tx kv.WriteTx) error {
		for i := 0; i < 5; i++ {
			key := string(rune('a' + i))
			if err := tx.Put("embeddings", key, []byte("val")); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	count := 0
	err = store.View(func(tx kv.ReadTx) error {
		return tx.ForEach("embeddings", func(k, v []byte) error {
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

	err := store.Update(func(tx kv.WriteTx) error {
		return tx.Put("config", "key1", []byte("value"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	err = store.Update(func(tx kv.WriteTx) error {
		return tx.Delete("config", "key1")
	})
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	err = store.View(func(tx kv.ReadTx) error {
		_, ok := tx.Get("config", "key1")
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

	err := store.View(func(tx kv.ReadTx) error {
		_, ok := tx.Get("projects", "nonexistent")
		if ok {
			t.Fatal("Expected key to not exist")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
}

func TestGetNonexistentBucket(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.View(func(tx kv.ReadTx) error {
		_, ok := tx.Get("nonexistent_bucket", "key")
		if ok {
			t.Fatal("Expected key in nonexistent bucket to not exist")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
}

func TestEmptyValue(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.Update(func(tx kv.WriteTx) error {
		return tx.Put("projects", "empty_key", []byte{})
	})
	if err != nil {
		t.Fatalf("Failed to write empty value: %v", err)
	}

	err = store.View(func(tx kv.ReadTx) error {
		value, ok := tx.Get("projects", "empty_key")
		if !ok {
			t.Fatal("Empty value key should exist")
		}
		if len(value) != 0 {
			t.Fatalf("Expected empty value, got %d bytes", len(value))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read empty value: %v", err)
	}
}

func TestLargeValue(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	largeValue := make([]byte, 1024*1024) // 1MB
	for i := 0; i < len(largeValue); i++ {
		largeValue[i] = byte(i % 256)
	}

	err := store.Update(func(tx kv.WriteTx) error {
		return tx.Put("files", "large_key", largeValue)
	})
	if err != nil {
		t.Fatalf("Failed to write large value: %v", err)
	}

	err = store.View(func(tx kv.ReadTx) error {
		value, ok := tx.Get("files", "large_key")
		if !ok {
			t.Fatal("Large value key should exist")
		}
		if !bytes.Equal(value, largeValue) {
			t.Fatal("Large value mismatch")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read large value: %v", err)
	}
}

func TestSpecialCharactersInKey(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	specialKeys := []string{
		"key:with:colons",
		"key/with/slashes",
		"key.with.dots",
		"key-with-dashes",
		"key_with_underscores",
		"key@with#symbols",
	}

	for i := 0; i < len(specialKeys); i++ {
		key := specialKeys[i]
		err := store.Update(func(tx kv.WriteTx) error {
			return tx.Put("projects", key, []byte("value"))
		})
		if err != nil {
			t.Fatalf("Failed to write key with special chars: %v", err)
		}
	}

	err := store.View(func(tx kv.ReadTx) error {
		for i := 0; i < len(specialKeys); i++ {
			key := specialKeys[i]
			_, ok := tx.Get("projects", key)
			if !ok {
				t.Fatalf("Key %s should exist", key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read special character keys: %v", err)
	}
}

func TestMemoryCopySafety(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	originalValue := []byte("original_value")
	err := store.Update(func(tx kv.WriteTx) error {
		return tx.Put("projects", "copy_test", originalValue)
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	err = store.View(func(tx kv.ReadTx) error {
		value1, ok := tx.Get("projects", "copy_test")
		if !ok {
			t.Fatal("Key should exist")
		}

		value2, ok := tx.Get("projects", "copy_test")
		if !ok {
			t.Fatal("Key should exist")
		}

		// Verify they are different byte slices (copies)
		if &value1[0] == &value2[0] {
			t.Fatal("Values should be different byte slices (copies)")
		}

		// But have the same content
		if !bytes.Equal(value1, value2) {
			t.Fatal("Values should have equal content")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
}

func TestForEachEmptyBucket(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	count := 0
	err := store.View(func(tx kv.ReadTx) error {
		return tx.ForEach("empty_bucket", func(k, v []byte) error {
			count++
			return nil
		})
	})
	if err != nil {
		t.Fatalf("Failed to iterate empty bucket: %v", err)
	}

	if count != 0 {
		t.Fatalf("Expected 0 items in empty bucket, got %d", count)
	}
}

func TestCreateBucketIfNotExists(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.Update(func(tx kv.WriteTx) error {
		if err := tx.CreateBucketIfNotExists("new_bucket"); err != nil {
			return err
		}
		return tx.Put("new_bucket", "key", []byte("value"))
	})
	if err != nil {
		t.Fatalf("Failed to create bucket and put value: %v", err)
	}

	err = store.View(func(tx kv.ReadTx) error {
		value, ok := tx.Get("new_bucket", "key")
		if !ok {
			t.Fatal("Value in new bucket should exist")
		}
		if !bytes.Equal(value, []byte("value")) {
			t.Fatal("Value mismatch")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read from new bucket: %v", err)
	}
}

func TestDeleteNonexistentKey(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.Update(func(tx kv.WriteTx) error {
		// Deleting a key that doesn't exist should not error
		return tx.Delete("projects", "nonexistent")
	})
	if err != nil {
		t.Fatalf("Failed to delete nonexistent key: %v", err)
	}
}

func TestDeleteFromNonexistentBucket(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	err := store.Update(func(tx kv.WriteTx) error {
		// Deleting from a bucket that doesn't exist should not error
		return tx.Delete("nonexistent_bucket", "key")
	})
	if err != nil {
		t.Fatalf("Failed to delete from nonexistent bucket: %v", err)
	}
}

func TestPersistenceAcrossInstances(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")

	store1, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store1: %v", err)
	}
	err = store1.Update(func(tx kv.WriteTx) error {
		return tx.Put("projects", "persist_key", []byte("persist_val"))
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

	err = store2.View(func(tx kv.ReadTx) error {
		val, ok := tx.Get("projects", "persist_key")
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

func TestConcurrentReads(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Write initial data
	err := store.Update(func(tx kv.WriteTx) error {
		return tx.Put("projects", "shared_key", []byte("shared_value"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Concurrent reads
	numGoroutines := 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := store.View(func(tx kv.ReadTx) error {
				value, ok := tx.Get("projects", "shared_key")
				if !ok {
					return fmt.Errorf("Key should exist")
				}
				if !bytes.Equal(value, []byte("shared_value")) {
					return fmt.Errorf("Value mismatch")
				}
				return nil
			})
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Fatalf("Concurrent read error: %v", err)
		}
	}
}

func TestConcurrentWrites(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	numGoroutines := 5
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key_%d", id)
			value := []byte(fmt.Sprintf("value_%d", id))
			err := store.Update(func(tx kv.WriteTx) error {
				return tx.Put("projects", key, value)
			})
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Fatalf("Concurrent write error: %v", err)
		}
	}

	// Verify all writes succeeded
	err := store.View(func(tx kv.ReadTx) error {
		for i := 0; i < numGoroutines; i++ {
			key := fmt.Sprintf("key_%d", i)
			expectedValue := []byte(fmt.Sprintf("value_%d", i))
			value, ok := tx.Get("projects", key)
			if !ok {
				return fmt.Errorf("Key %s should exist", key)
			}
			if !bytes.Equal(value, expectedValue) {
				return fmt.Errorf("Value mismatch for key %s", key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to verify concurrent writes: %v", err)
	}
}

func TestTransactionError(t *testing.T) {
	store := createTestStore(t)
	defer store.Close()

	// Write should fail when callback returns error
	err := store.Update(func(tx kv.WriteTx) error {
		if err := tx.Put("projects", "key1", []byte("value1")); err != nil {
			return err
		}
		return fmt.Errorf("intentional error")
	})
	if err == nil {
		t.Fatal("Expected error from transaction")
	}

	// Key should not exist since transaction was rolled back
	err = store.View(func(tx kv.ReadTx) error {
		_, ok := tx.Get("projects", "key1")
		if ok {
			t.Fatal("Key should not exist after failed transaction")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to verify rollback: %v", err)
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
		store.Update(func(tx kv.WriteTx) error {
			return tx.Put("projects", "key", []byte("value"))
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

	store.Update(func(tx kv.WriteTx) error {
		return tx.Put("projects", "key", []byte("value"))
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.View(func(tx kv.ReadTx) error {
			_, _ = tx.Get("projects", "key")
			return nil
		})
	}
}
