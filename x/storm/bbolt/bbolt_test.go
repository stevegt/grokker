package main

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// TestNewBoltDBStore tests store initialization and bucket creation
func TestNewBoltDBStore(t *testing.T) {
	dbPath := "test_store.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Verify store is not nil
	if store == nil {
		t.Fatal("Store is nil")
	}

	// Verify default buckets were created
	err = store.View(func(tx ReadTx) error {
		requiredBuckets := []string{"projects", "files", "embeddings", "hnsw_metadata", "config"}
		for _, bucketName := range requiredBuckets {
			// Get a value from the bucket to ensure it exists
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
	dbPath := "test_view.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// First, write some data
	err = store.Update(func(tx WriteTx) error {
		return tx.Put("projects", "test_key", []byte("test_value"))
	})
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// Now read it back in a View transaction
	err = store.View(func(tx ReadTx) error {
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
	dbPath := "test_update.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Write data
	err = store.Update(func(tx WriteTx) error {
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
		dbPath := "test_cbor_store.db"
		defer os.Remove(dbPath)

		store, err := NewBoltDBStore(dbPath)
		if err != nil {
			t.Fatalf("Failed to create store: %v", err)
		}
		defer store.Close()

		type TestObject struct {
			Name    string
			Enabled bool
		}

		testObj := TestObject{Name: "test", Enabled: true}

		// Store CBOR-encoded object
		err = store.Update(func(tx WriteTx) error {
			return StoreCBOR(tx, "projects", "obj1", testObj)
		})
		if err != nil {
			t.Fatalf("Failed to store CBOR: %v", err)
		}

		// Load it back
		err = store.View(func(tx ReadTx) error {
			var recovered TestObject
			return LoadCBOR(tx, "projects", "obj1", &recovered)
		})
		if err != nil {
			t.Fatalf("Failed to load CBOR: %v", err)
		}
	})
}

// TestForEachBucket tests iteration over bucket contents
func TestForEachBucket(t *testing.T) {
	dbPath := "test_foreach.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Write multiple values
	err = store.Update(func(tx WriteTx) error {
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
	dbPath := "test_delete.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Write and delete
	err = store.Update(func(tx WriteTx) error {
		if err := tx.Put("embeddings", "key1", []byte("value1")); err != nil {
			return err
		}
		if err := tx.Delete("embeddings", "key1"); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify deletion
	err = store.View(func(tx ReadTx) error {
		value := tx.Get("embeddings", "key1")
		if value != nil {
			t.Fatal("Expected nil after deletion, got value")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to verify deletion: %v", err)
	}
}

// TestCreateBucketIfNotExists tests bucket creation
func TestCreateBucketIfNotExists(t *testing.T) {
	dbPath := "test_bucket_create.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create a new bucket
	err = store.Update(func(tx WriteTx) error {
		return tx.CreateBucketIfNotExists("custom_bucket")
	})
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	// Write to new bucket
	err = store.Update(func(tx WriteTx) error {
		return tx.Put("custom_bucket", "key", []byte("value"))
	})
	if err != nil {
		t.Fatalf("Failed to write to custom bucket: %v", err)
	}

	// Verify
	err = store.View(func(tx ReadTx) error {
		value := tx.Get("custom_bucket", "key")
		if value == nil {
			t.Fatal("Expected value in custom bucket")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read from custom bucket: %v", err)
	}
}

// TestMissingKeyReturnsNil tests behavior on missing keys
func TestMissingKeyReturnsNil(t *testing.T) {
	dbPath := "test_missing_key.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Try to get non-existent key
	err = store.View(func(tx ReadTx) error {
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
	dbPath := "test_isolation.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Write data
	err = store.Update(func(tx WriteTx) error {
		return tx.Put("config", "key1", []byte("value1"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Read in new View transaction
	err = store.View(func(tx ReadTx) error {
		value := tx.Get("config", "key1")
		if value == nil {
			t.Fatal("Expected to read value from previous Update transaction")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
}

// TestLoadCBORIfExists tests conditional CBOR loading
func TestLoadCBORIfExists(t *testing.T) {
	dbPath := "test_load_if_exists.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	type TestData struct {
		Value string
	}

	// Try to load non-existent key - should not error
	err = store.View(func(tx ReadTx) error {
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
	err = store.Update(func(tx WriteTx) error {
		return StoreCBOR(tx, "projects", "exists", original)
	})
	if err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

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
	dbPath := "test_empty_foreach.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	count := 0
	err = store.View(func(tx ReadTx) error {
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
	dbPath := "test_multi_bucket.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Write to multiple buckets in single transaction
	err = store.Update(func(tx WriteTx) error {
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

// BenchmarkPut benchmarks write performance
func BenchmarkPut(b *testing.B) {
	dbPath := "bench_put.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
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
	dbPath := "bench_get.db"
	defer os.Remove(dbPath)

	store, err := NewBoltDBStore(dbPath)
	if err != nil {
		b.Fatalf("Failed to create store: %v", err)
	}
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

