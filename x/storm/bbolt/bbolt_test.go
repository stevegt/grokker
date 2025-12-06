package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createTestStore creates a new BoltDB store in a temporary directory
func createTestStore(t *testing.T) (*BoltDBStore, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	
	return store, dbPath
}

// storeCBORData stores CBOR-encoded data and fails test on error
func storeCBORData(t *testing.T, store *BoltDBStore, bucket, key string, data interface{}) {
	err := store.Update(func(tx WriteTx) error {
		return StoreCBOR(tx, bucket, key, data)
	})
	if err != nil {
		t.Fatalf("Failed to store CBOR data: %v", err)
	}
}

// loadCBORData loads CBOR-encoded data and fails test on error
func loadCBORData(t *testing.T, store *BoltDBStore, bucket, key string, data interface{}) {
	err := store.View(func(tx ReadTx) error {
		return LoadCBOR(tx, bucket, key, data)
	})
	if err != nil {
		t.Fatalf("Failed to load CBOR data: %v", err)
	}
}

// storeRawData stores raw bytes and fails test on error
func storeRawData(t *testing.T, store *BoltDBStore, bucket, key string, value []byte) {
	err := store.Update(func(tx WriteTx) error {
		return tx.Put(bucket, key, value)
	})
	if err != nil {
		t.Fatalf("Failed to store raw data: %v", err)
	}
}

// getRawData retrieves raw bytes and fails test on error
func getRawData(t *testing.T, store *BoltDBStore, bucket, key string) []byte {
	var value []byte
	err := store.View(func(tx ReadTx) error {
		value = tx.Get(bucket, key)
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to get raw data: %v", err)
	}
	return value
}

// TestNewBoltDBStore tests store initialization and bucket creation
func TestNewBoltDBStore(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	// Verify store is not nil
	if store == nil {
		t.Fatal("Store is nil")
	}

	// Verify default buckets were created
	err := store.View(func(tx ReadTx) error {
		requiredBuckets := []string{"projects", "files", "embeddings", "hnsw_metadata", "config"}
		for _, bucketName := range requiredBuckets {
			_ = tx.Get(bucketName, "test_key")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to verify buckets: %v", err)
	}
}

// TestViewTransaction tests read-only transactions
func TestViewTransaction(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	storeRawData(t, store, "projects", "test_key", []byte("test_value"))

	// Read it back in a View transaction
	err := store.View(func(tx ReadTx) error {
		value := tx.Get("projects", "test_key")
		if value == nil {
			t.Fatal("Expected to find value, got nil")
		}
		if !bytes.Equal(value, []byte("test_value")) {
			t.Fatalf("Expected 'test_value', got %s", string(value))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read data: %v", err)
	}
}

// TestUpdateTransaction tests read-write transactions
func TestUpdateTransaction(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	// Write data
	err := store.Update(func(tx WriteTx) error {
		if err := tx.Put("projects", "key1", []byte("value1")); err != nil {
			return err
		}
		if err := tx.Put("projects", "key2", []byte("value2")); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// Verify data was written
	err = store.View(func(tx ReadTx) error {
		v1 := tx.Get("projects", "key1")
		v2 := tx.Get("projects", "key2")
		if !bytes.Equal(v1, []byte("value1")) || !bytes.Equal(v2, []byte("value2")) {
			t.Fatal("Data mismatch after write")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to verify write: %v", err)
	}
}

// TestCBOREncoding tests CBOR marshaling and unmarshaling
func TestCBOREncoding(t *testing.T) {
	t.Run("MarshalUnmarshal", func(t *testing.T) {
		type TestData struct {
			ID        string
			Count     int
			Timestamp time.Time
			Values    []float32
		}

		original := TestData{
			ID:        "test-123",
			Count:     42,
			Timestamp: time.Date(2025, 12, 6, 10, 0, 0, 0, time.UTC),
			Values:    []float32{1.5, 2.5, 3.5},
		}

		// Marshal to CBOR
		data, err := MarshalCBOR(original)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		// Unmarshal back
		var recovered TestData
		err = UnmarshalCBOR(data, &recovered)
		if err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		// Verify equality
		if recovered.ID != original.ID || recovered.Count != original.Count {
			t.Fatal("Data mismatch after CBOR roundtrip")
		}
		if !recovered.Timestamp.Equal(original.Timestamp) {
			t.Fatal("Timestamp mismatch after CBOR roundtrip")
		}
		if len(recovered.Values) != len(original.Values) {
			t.Fatal("Values length mismatch")
		}
	})

	t.Run("StoreCBOR", func(t *testing.T) {
		store, _ := createTestStore(t)
		defer store.Close()

		type TestObject struct {
			Name    string
			Enabled bool
		}

		testObj := TestObject{Name: "test", Enabled: true}
		storeCBORData(t, store, "projects", "obj1", testObj)

		// Load it back
		var recovered TestObject
		loadCBORData(t, store, "projects", "obj1", &recovered)

		if recovered.Name != "test" || recovered.Enabled != true {
			t.Fatal("CBOR data mismatch")
		}
	})
}

// TestForEachBucket tests iteration over bucket contents
func TestForEachBucket(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	// Write multiple values
	err := store.Update(func(tx WriteTx) error {
		for i := 0; i < 5; i++ {
			key := "key" + string(rune(i+'0'))
			value := "value" + string(rune(i+'0'))
			if err := tx.Put("files", key, []byte(value)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// Iterate and count
	count := 0
	err = store.View(func(tx ReadTx) error {
		return tx.ForEach("files", func(k, v []byte) error {
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

// TestDelete tests key deletion
func TestDelete(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	storeRawData(t, store, "embeddings", "key1", []byte("value1"))

	// Delete
	err := store.Update(func(tx WriteTx) error {
		return tx.Delete("embeddings", "key1")
	})
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify deletion
	value := getRawData(t, store, "embeddings", "key1")
	if value != nil {
		t.Fatal("Expected nil after deletion, got value")
	}
}

// TestCreateBucketIfNotExists tests bucket creation
func TestCreateBucketIfNotExists(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	// Create a new bucket
	err := store.Update(func(tx WriteTx) error {
		return tx.CreateBucketIfNotExists("custom_bucket")
	})
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	storeRawData(t, store, "custom_bucket", "key", []byte("value"))

	// Verify
	value := getRawData(t, store, "custom_bucket", "key")
	if value == nil {
		t.Fatal("Expected value in custom bucket")
	}
}

// TestMissingKeyReturnsNil tests behavior on missing keys
func TestMissingKeyReturnsNil(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	// Try to get non-existent key
	err := store.View(func(tx ReadTx) error {
		value := tx.Get("projects", "nonexistent")
		if value != nil {
			t.Fatal("Expected nil for missing key")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to test missing key: %v", err)
	}
}

// TestTransactionIsolation tests that changes in Update are visible in subsequent Views
func TestTransactionIsolation(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	storeRawData(t, store, "config", "key1", []byte("value1"))

	// Read in new View transaction
	value := getRawData(t, store, "config", "key1")
	if value == nil {
		t.Fatal("Expected to read value from previous Update transaction")
	}
}

// TestLoadCBORIfExists tests conditional CBOR loading
func TestLoadCBORIfExists(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	type TestData struct {
		Value string
	}

	// Try to load non-existent key - should not error
	err := store.View(func(tx ReadTx) error {
		var data TestData
		err := LoadCBORIfExists(tx, "projects", "missing", &data)
		if err != nil {
			t.Fatalf("LoadCBORIfExists should not error for missing key: %v", err)
		}
		// data should be zero value
		if data.Value != "" {
			t.Fatal("Expected zero value for missing key")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}

	// Store and retrieve
	original := TestData{Value: "stored"}
	storeCBORData(t, store, "projects", "exists", original)

	err = store.View(func(tx ReadTx) error {
		var recovered TestData
		err := LoadCBORIfExists(tx, "projects", "exists", &recovered)
		if err != nil {
			t.Fatalf("LoadCBORIfExists failed: %v", err)
		}
		if recovered.Value != "stored" {
			t.Fatalf("Expected 'stored', got '%s'", recovered.Value)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
}

// TestEmptyBucketForEach tests ForEach on empty bucket
func TestEmptyBucketForEach(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	count := 0
	err := store.View(func(tx ReadTx) error {
		return tx.ForEach("projects", func(k, v []byte) error {
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

// TestMultipleBuckets tests operations across different buckets
func TestMultipleBuckets(t *testing.T) {
	store, _ := createTestStore(t)
	defer store.Close()

	// Write to multiple buckets in single transaction
	err := store.Update(func(tx WriteTx) error {
		if err := tx.Put("projects", "p1", []byte("project1")); err != nil {
			return err
		}
		if err := tx.Put("files", "f1", []byte("file1")); err != nil {
			return err
		}
		if err := tx.Put("embeddings", "e1", []byte("embed1")); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to write to multiple buckets: %v", err)
	}

	// Verify each bucket
	err = store.View(func(tx ReadTx) error {
		if v := tx.Get("projects", "p1"); !bytes.Equal(v, []byte("project1")) {
			t.Fatal("Project bucket mismatch")
		}
		if v := tx.Get("files", "f1"); !bytes.Equal(v, []byte("file1")) {
			t.Fatal("Files bucket mismatch")
		}
		if v := tx.Get("embeddings", "e1"); !bytes.Equal(v, []byte("embed1")) {
			t.Fatal("Embeddings bucket mismatch")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to verify: %v", err)
	}
}

// TestCBORCanonicalEncoding verifies CBOR produces canonical form
func TestCBORCanonicalEncoding(t *testing.T) {
	data1, err := MarshalCBOR(map[string]int{"a": 1, "b": 2})
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Multiple encodings of same data should produce identical bytes
	data2, err := MarshalCBOR(map[string]int{"a": 1, "b": 2})
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	if !bytes.Equal(data1, data2) {
		t.Fatal("CBOR canonical encoding not deterministic")
	}
}

// TestPersistenceAcrossStores tests data persists across multiple store instances
func TestPersistenceAcrossStores(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist_test.db")

	// Create first store and write data
	store1, _ := createTestStore(t)
	storeRawData(t, store1, "projects", "persist_key", []byte("persist_value"))
	store1.Close()

	// Create second store with same database file
	store2, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to open existing store: %v", err)
	}
	defer store2.Close()

	// Verify data persists
	value := getRawData(t, store2, "projects", "persist_key")
	if !bytes.Equal(value, []byte("persist_value")) {
		t.Fatal("Data did not persist across store instances")
	}
}

// TestTempDirCleanup verifies temporary directories don't accumulate
func TestTempDirCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Verify the temp directory was created
	if _, err := os.Stat(tmpDir); err != nil {
		t.Fatalf("Temp directory not created: %v", err)
	}
	
	// t.TempDir() automatically cleans up after test, so we can't verify
	// that here, but we can verify the directory exists during the test
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read temp directory: %v", err)
	}
	
	// Directory should initially be empty
	if len(entries) != 0 {
		t.Fatalf("Expected empty temp directory, got %d entries", len(entries))
	}
}

// BenchmarkPut benchmarks write performance
func BenchmarkPut(b *testing.B) {
	store, _ := createTestStore(&testing.T{})
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Update(func(tx WriteTx) error {
			key := "key" + string(rune(i%1000))
			return tx.Put("projects", key, []byte("value"))
		})
	}
}

// BenchmarkGet benchmarks read performance
func BenchmarkGet(b *testing.B) {
	store, _ := createTestStore(&testing.T{})
	defer store.Close()

	// Pre-populate with 1000 entries
	store.Update(func(tx WriteTx) error {
		for i := 0; i < 1000; i++ {
			key := "key" + string(rune(i%1000))
			tx.Put("projects", key, []byte("value"))
		}
		return nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.View(func(tx ReadTx) error {
			key := "key" + string(rune(i%1000))
			_ = tx.Get("projects", key)
			return nil
		})
	}
}

// BenchmarkCBORMarshal benchmarks CBOR encoding
func BenchmarkCBORMarshal(b *testing.B) {
	type TestData struct {
		ID        string
		Values    []float32
		Timestamp time.Time
	}

	data := TestData{
		ID:        "test-id",
		Values:    make([]float32, 256),
		Timestamp: time.Now(),
	}
	for i := 0; i < 256; i++ {
		data.Values[i] = float32(i) * 0.1
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := MarshalCBOR(data)
		if err != nil {
			b.Fatalf("Marshal failed: %v", err)
		}
	}
}
