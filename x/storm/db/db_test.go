package db

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stevegt/grokker/x/storm/db/kv"
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

func TestInitializeBuckets(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmpDir, "buckets.db"))
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Close()

	// Verify required buckets exist and are accessible
	err = mgr.store.View(func(tx kv.ReadTx) error {
		requiredBuckets := []string{
			"projects",
			"files",
			"embeddings",
			"hnsw_metadata",
			"config",
		}
		for i := 0; i < len(requiredBuckets); i++ {
			bucket := requiredBuckets[i]
			// ForEach will fail if bucket doesn't exist; nil is acceptable for empty bucket
			if err := tx.ForEach(bucket, func(k, v []byte) error {
				return nil
			}); err != nil {
				return fmt.Errorf("bucket %s failed: %w", bucket, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProjectRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmpDir, "project.db"))
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Close()

	// Create test project
	createdAt := time.Now().UTC().Truncate(time.Second)
	project := &Project{
		ID:                    "test-project",
		BaseDir:               "/test/dir",
		CurrentDiscussionFile: "discussion.md",
		AuthorizedFiles:       []string{"file1.txt", "file2.txt"},
		CreatedAt:             createdAt,
	}

	// Save and reload
	if err := mgr.SaveProject(project); err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}

	loaded, err := mgr.LoadProject("test-project")
	if err != nil {
		t.Fatalf("LoadProject failed: %v", err)
	}

	// Verify fields
	if loaded.ID != project.ID {
		t.Errorf("ID mismatch: expected %s, got %s", project.ID, loaded.ID)
	}
	if len(loaded.AuthorizedFiles) != len(project.AuthorizedFiles) {
		t.Errorf("AuthorizedFiles count mismatch: expected %d, got %d",
			len(project.AuthorizedFiles), len(loaded.AuthorizedFiles))
	}
	if !loaded.CreatedAt.Equal(project.CreatedAt) {
		t.Errorf("CreatedAt mismatch: expected %v, got %v",
			project.CreatedAt, loaded.CreatedAt)
	}
}

func TestConcurrentProjectAccess(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmpDir, "concurrent.db"))
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Close()

	project := &Project{
		ID:      "concurrent-test",
		BaseDir: "/test/dir",
	}
	if err := mgr.SaveProject(project); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := mgr.LoadProject("concurrent-test")
			if err != nil {
				t.Errorf("Concurrent load failed: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestLargeProject(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmpDir, "large.db"))
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Close()

	project := &Project{
		ID:      "large-project",
		BaseDir: "/test/dir",
	}
	for i := 0; i < 10000; i++ {
		project.AuthorizedFiles = append(project.AuthorizedFiles,
			fmt.Sprintf("file-%d.txt", i))
	}

	if err := mgr.SaveProject(project); err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}

	loaded, err := mgr.LoadProject("large-project")
	if err != nil {
		t.Fatalf("LoadProject failed: %v", err)
	}
	if len(loaded.AuthorizedFiles) != 10000 {
		t.Errorf("Expected 10000 files, got %d", len(loaded.AuthorizedFiles))
	}
}

func TestSpecialCharacterKeys(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmpDir, "special.db"))
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Close()

	project := &Project{
		ID:                    "key/with/slashes",
		BaseDir:               "dir/with/special#chars",
		CurrentDiscussionFile: "file with spaces.md",
	}

	if err := mgr.SaveProject(project); err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}

	loaded, err := mgr.LoadProject("key/with/slashes")
	if err != nil {
		t.Fatalf("LoadProject failed: %v", err)
	}
	if loaded.BaseDir != project.BaseDir {
		t.Errorf("BaseDir mismatch: expected %s, got %s",
			project.BaseDir, loaded.BaseDir)
	}
}

func TestDeleteNonexistentProject(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmpDir, "delete.db"))
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Close()

	err = mgr.DeleteProject("nonexistent-id")
	if err == nil {
		t.Fatal("Expected error when deleting nonexistent project")
	}
}

func TestListProjectIDs(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmpDir, "list.db"))
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Close()

	// Create test projects
	projects := []string{"proj1", "proj2", "proj3"}
	for i := 0; i < len(projects); i++ {
		if err := mgr.SaveProject(&Project{
			ID:      projects[i],
			BaseDir: "/test/dir",
		}); err != nil {
			t.Fatal(err)
		}
	}

	ids, err := mgr.ListProjectIDs()
	if err != nil {
		t.Fatalf("ListProjectIDs failed: %v", err)
	}

	// Verify we got all IDs
	if len(ids) != len(projects) {
		t.Fatalf("Expected %d projects, got %d", len(projects), len(ids))
	}
	for i := 0; i < len(projects); i++ {
		id := projects[i]
		found := false
		for j := 0; j < len(ids); j++ {
			if ids[j] == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Project ID %s not found in list", id)
		}
	}
}