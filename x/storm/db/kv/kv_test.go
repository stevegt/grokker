package kv

import (
	"testing"
)

func TestNewStoreDefaultCreatesStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStoreDefault(tmpDir + "/test.db")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	if store == nil {
		t.Fatal("Store is nil")
	}
	store.Close()
}

func TestNewStoreInvalidBackend(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := NewStore(tmpDir+"/test.db", BackendType("unknown"))
	if err == nil {
		t.Fatal("Expected error for unknown backend")
	}
}

func TestNewStoreBoltDBBackend(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir+"/test.db", BoltDB)
	if err != nil {
		t.Fatalf("Failed to create BoltDB store: %v", err)
	}
	if store == nil {
		t.Fatal("Store is nil")
	}
	store.Close()
}
