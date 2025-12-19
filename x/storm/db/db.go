package db

import (
	"fmt"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/stevegt/grokker/x/storm/db/bbolt"
	"github.com/stevegt/grokker/x/storm/db/kv"
)

// MarshalCBOR marshals data to CBOR canonical form
func MarshalCBOR(v interface{}) ([]byte, error) {
	encOptions := cbor.EncOptions{Sort: cbor.SortCanonical}
	encoder, err := encOptions.EncMode()
	if err != nil {
		return nil, fmt.Errorf("failed to create CBOR encoder: %w", err)
	}
	return encoder.Marshal(v)
}

// UnmarshalCBOR unmarshals CBOR data
func UnmarshalCBOR(data []byte, v interface{}) error {
	decOptions := cbor.DecOptions{}
	decoder, err := decOptions.DecMode()
	if err != nil {
		return fmt.Errorf("failed to create CBOR decoder: %w", err)
	}
	return decoder.Unmarshal(data, v)
}

// BackendType specifies which backend implementation to use
type BackendType string

const (
	BoltDB BackendType = "bbolt"
)

// NewStore creates a KVStore instance for the specified backend
func NewStore(dbPath string, backend BackendType) (kv.KVStore, error) {
	switch backend {
	case BoltDB:
		return bbolt.NewBoltDBStore(dbPath)
	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}

// NewStoreDefault creates a KVStore with BoltDB backend
func NewStoreDefault(dbPath string) (kv.KVStore, error) {
	return NewStore(dbPath, BoltDB)
}

// initializeBuckets creates required application-level buckets
func initializeBuckets(store kv.KVStore) error {
	return store.Update(func(tx kv.WriteTx) error {
		requiredBuckets := []string{
			"projects",
			"files",
			"embeddings",
			"hnsw_metadata",
			"config",
		}
		for i := 0; i < len(requiredBuckets); i++ {
			bucketName := requiredBuckets[i]
			if err := tx.CreateBucketIfNotExists(bucketName); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucketName, err)
			}
		}
		return nil
	})
}

// Manager provides database operations for Storm
type Manager struct {
	store kv.KVStore
}

// NewManager creates a new database manager and initializes required buckets
func NewManager(dbPath string) (*Manager, error) {
	store, err := NewStoreDefault(dbPath)
	if err != nil {
		return nil, err
	}

	// TODO we need some sort of versioning/migration system here

	// Initialize application-level buckets
	if err := initializeBuckets(store); err != nil {
		store.Close()
		return nil, err
	}

	return &Manager{store: store}, nil
}

// Close closes the database
func (m *Manager) Close() error {
	return m.store.Close()
}

// Project represents persistent project metadata
type Project struct {
	ID                    string              `cbor:"id"`
	BaseDir               string              `cbor:"baseDir"`
	CurrentDiscussionFile string              `cbor:"currentDiscussionFile"`
	DiscussionFiles       []DiscussionFileRef `cbor:"discussionFiles"`
	AuthorizedFiles       []string            `cbor:"authorizedFiles"`
	CreatedAt             time.Time           `cbor:"createdAt"`
	EmbeddingCount        int                 `cbor:"embeddingCount"`
	RoundHistory          []RoundEntry        `cbor:"roundHistory"`
}

// DiscussionFileRef tracks metadata about a discussion file
type DiscussionFileRef struct {
	Filepath   string    `cbor:"filepath"`
	CreatedAt  time.Time `cbor:"createdAt"`
	RoundCount int       `cbor:"roundCount"`
}

// RoundEntry tracks a query-response round
type RoundEntry struct {
	RoundID        string    `cbor:"roundID"`
	DiscussionFile string    `cbor:"discussionFile"`
	QueryID        string    `cbor:"queryID"`
	Timestamp      time.Time `cbor:"timestamp"`
	CIDs           []string  `cbor:"cids"`
}

// SaveProject persists a project to the KV store
func (m *Manager) SaveProject(project *Project) error {
	if project.ID == "" {
		return fmt.Errorf("cannot save project with empty ID")
	}
	if project.BaseDir == "" {
		return fmt.Errorf("cannot save project with empty BaseDir")
	}

	return m.store.Update(func(tx kv.WriteTx) error {
		data, err := MarshalCBOR(project)
		if err != nil {
			return fmt.Errorf("failed to marshal project: %w", err)
		}
		return tx.Put("projects", project.ID, data)
	})
}

// LoadProject retrieves a project by ID from the KV store
func (m *Manager) LoadProject(projectID string) (*Project, error) {
	var project *Project
	err := m.store.View(func(tx kv.ReadTx) error {
		data, ok := tx.Get("projects", projectID)
		if !ok {
			return fmt.Errorf("project %s not found", projectID)
		}
		project = &Project{}
		err := UnmarshalCBOR(data, project)
		if err != nil {
			return fmt.Errorf("failed to unmarshal project: %w", err)
		}
		return nil
	})
	return project, err
}

// LoadAllProjects retrieves all projects from the KV store
func (m *Manager) LoadAllProjects() (map[string]*Project, error) {
	projects := make(map[string]*Project)
	err := m.store.View(func(tx kv.ReadTx) error {
		return tx.ForEach("projects", func(k, v []byte) error {
			project := &Project{}
			err := UnmarshalCBOR(v, project)
			if err != nil {
				return fmt.Errorf("failed to unmarshal project: %w", err)
			}
			projects[project.ID] = project
			return nil
		})
	})
	return projects, err
}

// DeleteProject removes a project from the KV store
func (m *Manager) DeleteProject(projectID string) error {
	// Verify project exists before deletion
	err := m.store.View(func(tx kv.ReadTx) error {
		_, ok := tx.Get("projects", projectID)
		if !ok {
			return fmt.Errorf("project %s not found", projectID)
		}
		return nil
	})
	if err != nil {
		return err
	}

	return m.store.Update(func(tx kv.WriteTx) error {
		return tx.Delete("projects", projectID)
	})
}

// ListProjectIDs returns all project IDs from the KV store
func (m *Manager) ListProjectIDs() ([]string, error) {
	var ids []string
	err := m.store.View(func(tx kv.ReadTx) error {
		return tx.ForEach("projects", func(k, v []byte) error {
			ids = append(ids, string(k))
			return nil
		})
	})
	return ids, err
}
