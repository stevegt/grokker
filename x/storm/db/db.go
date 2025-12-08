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

// Manager provides database operations for Storm
type Manager struct {
	store kv.KVStore
}

// NewManager creates a new database manager
func NewManager(dbPath string) (*Manager, error) {
	store, err := NewStoreDefault(dbPath)
	if err != nil {
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
	ID                    string              `json:"id"`
	BaseDir               string              `json:"baseDir"`
	CurrentDiscussionFile string              `json:"currentDiscussionFile"`
	DiscussionFiles       []DiscussionFileRef `json:"discussionFiles"`
	AuthorizedFiles       []string            `json:"authorizedFiles"`
	CreatedAt             time.Time           `json:"createdAt"`
	EmbeddingCount        int                 `json:"embeddingCount"`
	RoundHistory          []RoundEntry        `json:"roundHistory"`
}

// DiscussionFileRef tracks metadata about a discussion file
type DiscussionFileRef struct {
	Filepath  string    `json:"filepath"`
	CreatedAt time.Time `json:"createdAt"`
	RoundCount int      `json:"roundCount"`
}

// RoundEntry tracks a query-response round
type RoundEntry struct {
	RoundID        string    `json:"roundID"`
	DiscussionFile string    `json:"discussionFile"`
	QueryID        string    `json:"queryID"`
	Timestamp      time.Time `json:"timestamp"`
	CIDs           []string  `json:"cids"`
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