package db

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
)

func TestMarshalCBOR(t *testing.T) {
	type TestData struct {
		ID    string
		Count int
		Time  time.Time
	}

	data := TestData{
		ID:    "test-123",
		Count: 42,
		Time:  time.Date(2025, 12, 6, 10, 0, 0, 0, time.UTC),
	}

	encoded, err := MarshalCBOR(data)
	if err != nil {
		t.Fatalf("MarshalCBOR failed: %v", err)
	}

	if len(encoded) == 0 {
		t.Fatal("Encoded data is empty")
	}
}

func TestUnmarshalCBOR(t *testing.T) {
	type TestData struct {
		ID    string
		Count int
		Time  time.Time
	}

	original := TestData{
		ID:    "test-123",
		Count: 42,
		Time:  time.Date(2025, 12, 6, 10, 0, 0, 0, time.UTC),
	}

	encoded, err := MarshalCBOR(original)
	if err != nil {
		t.Fatalf("MarshalCBOR failed: %v", err)
	}

	var decoded TestData
	err = UnmarshalCBOR(encoded, &decoded)
	if err != nil {
		t.Fatalf("UnmarshalCBOR failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: expected %s, got %s", original.ID, decoded.ID)
	}

	if decoded.Count != original.Count {
		t.Errorf("Count mismatch: expected %d, got %d", original.Count, decoded.Count)
	}

	if !decoded.Time.Equal(original.Time) {
		t.Errorf("Time mismatch: expected %v, got %v", original.Time, decoded.Time)
	}
}

func TestCBORRoundtrip(t *testing.T) {
	type ComplexData struct {
		ID       string
		Values   []int
		Metadata map[string]interface{}
	}

	original := ComplexData{
		ID:     "complex-1",
		Values: []int{1, 2, 3, 4, 5},
		Metadata: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
			"key3": 3.14,
		},
	}

	encoded, err := MarshalCBOR(original)
	if err != nil {
		t.Fatalf("MarshalCBOR failed: %v", err)
	}

	var decoded ComplexData
	err = UnmarshalCBOR(encoded, &decoded)
	if err != nil {
		t.Fatalf("UnmarshalCBOR failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: expected %s, got %s", original.ID, decoded.ID)
	}

	if len(decoded.Values) != len(original.Values) {
		t.Errorf("Values length mismatch: expected %d, got %d", len(original.Values), len(decoded.Values))
	}

	for i := 0; i < len(decoded.Values); i++ {
		if decoded.Values[i] != original.Values[i] {
			t.Errorf("Values[%d] mismatch: expected %d, got %d", i, original.Values[i], decoded.Values[i])
		}
	}
}

func TestCBORCanonical(t *testing.T) {
	type Data struct {
		A string
		B int
		C float64
	}

	data := Data{A: "test", B: 123, C: 45.67}

	encoded1, err1 := MarshalCBOR(data)
	encoded2, err2 := MarshalCBOR(data)

	if err1 != nil || err2 != nil {
		t.Fatalf("MarshalCBOR failed: %v, %v", err1, err2)
	}

	if !bytes.Equal(encoded1, encoded2) {
		t.Fatal("Canonical CBOR encoding not deterministic")
	}
}

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	if mgr == nil {
		t.Fatal("Manager is nil")
	}
	mgr.Close()
}

func TestNewStoreFactory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "factory.db")

	store, err := NewStore(dbPath, BoltDB)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	if store == nil {
		t.Fatal("Store is nil")
	}
	defer store.Close()

	// Verify store works
	err = store.Update(func(tx kv.WriteTx) error {
		return tx.Put("projects", "test", []byte("data"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
}

func TestNewStoreInvalidBackend(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := NewStore(filepath.Join(tmpDir, "test.db"), BackendType("invalid"))
	if err == nil {
		t.Fatal("Expected error for invalid backend")
	}
}