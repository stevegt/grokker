package kv

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestNewStoreDefaultCreatesStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStoreDefault(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	if store == nil {
		t.Fatal("Store is nil")
	}
	defer store.Close()
}

func TestNewStoreInvalidBackend(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := NewStore(filepath.Join(tmpDir, "test.db"), BackendType("unknown"))
	if err == nil {
		t.Fatal("Expected error for unknown backend")
	}
}

func TestNewStoreBoltDBBackend(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "test.db"), BoltDB)
	if err != nil {
		t.Fatalf("Failed to create BoltDB store: %v", err)
	}
	if store == nil {
		t.Fatal("Store is nil")
	}
	defer store.Close()
}

func TestKVStoreContractRead(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStoreDefault(filepath.Join(tmpDir, "contract.db"))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Write via adapter
	err = store.Update(func(tx WriteTx) error {
		return tx.Put("projects", "test_key", []byte("test_value"))
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Read via adapter
	err = store.View(func(tx ReadTx) error {
		value, ok := tx.Get("projects", "test_key")
		if !ok {
			t.Fatal("Expected key to exist")
		}
		if !bytes.Equal(value, []byte("test_value")) {
			t.Fatalf("Expected test_value, got %s", string(value))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
}

func TestKVStoreContractIteration(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStoreDefault(filepath.Join(tmpDir, "iter.db"))
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Write multiple values
	err = store.Update(func(tx WriteTx) error {
		for i := 0; i < 3; i++ {
			key := string(rune('a' + i))
			if err := tx.Put("files", key, []byte(key+"_value")); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Iterate and verify
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

	if count != 3 {
		t.Fatalf("Expected 3 items, got %d", count)
	}
}