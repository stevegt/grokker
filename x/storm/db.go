package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"go.etcd.io/bbolt"
)

// DBConfig holds Storm daemon configuration
type DBConfig struct {
	Port           int    `json:"port"`
	EmbeddingModel string `json:"embeddingModel"`
	OllamaEndpoint string `json:"ollamaEndpoint"`
	HNSW           struct {
		M              int `json:"M"`
		EfConstruction int `json:"efConstruction"`
		EfSearch       int `json:"efSearch"`
	} `json:"hnsw"`
}

// ProjectMetadata stores project information in the database
type ProjectMetadata struct {
	ID                  string   `json:"id"`
	BaseDir             string   `json:"baseDir"`
	CurrentDiscussionFile string `json:"currentDiscussionFile"`
	DiscussionFiles     []struct {
		Filepath  string `json:"filepath"`
		CreatedAt string `json:"createdAt"`
		RoundCount int   `json:"roundCount"`
	} `json:"discussionFiles"`
	AuthorizedFiles []string `json:"authorizedFiles"`
	CreatedAt       string   `json:"createdAt"`
	EmbeddingCount  int      `json:"embeddingCount"`
	RoundHistory    []struct {
		RoundID       string   `json:"roundID"`
		DiscussionFile string   `json:"discussionFile"`
		QueryID       string   `json:"queryID"`
		Timestamp     string   `json:"timestamp"`
		CIDs          []string `json:"CIDs"`
	} `json:"roundHistory"`
}

// DB wraps the BoltDB instance with helper methods
type DB struct {
	bolt  *bbolt.DB
	mutex sync.RWMutex
}

// NewDB opens or creates a BoltDB instance
func NewDB(filepath string) (*DB, error) {
	bdb, err := bbolt.Open(filepath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open BoltDB: %w", err)
	}
	return &DB{bolt: bdb}, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.bolt.Close()
}

// LoadConfig loads daemon configuration from the config/ bucket
func (d *DB) LoadConfig() (*DBConfig, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	var config *DBConfig
	err := d.bolt.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("config"))
		if bucket == nil {
			return nil // No config yet, return nil
		}
		data := bucket.Get([]byte("config"))
		if data == nil {
			return nil
		}
		config = &DBConfig{}
		if err := json.Unmarshal(data, config); err != nil {
			return fmt.Errorf("failed to unmarshal config: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return config, nil
}

// SaveConfig saves daemon configuration to the config/ bucket
func (d *DB) SaveConfig(config *DBConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.bolt.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("config"))
		if err != nil {
			return fmt.Errorf("failed to create config bucket: %w", err)
		}
		data, err := json.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		if err := bucket.Put([]byte("config"), data); err != nil {
			return fmt.Errorf("failed to store config: %w", err)
		}
		log.Printf("Saved config: port=%d, model=%s", config.Port, config.EmbeddingModel)
		return nil
	})
}

// LoadProjects loads all projects from the projects/ bucket
func (d *DB) LoadProjects() (map[string]*ProjectMetadata, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	projects := make(map[string]*ProjectMetadata)
	err := d.bolt.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("projects"))
		if bucket == nil {
			return nil // No projects yet
		}
		return bucket.ForEach(func(k, v []byte) error {
			var metadata ProjectMetadata
			if err := json.Unmarshal(v, &metadata); err != nil {
				log.Printf("Warning: failed to unmarshal project %s: %v", string(k), err)
				return nil
			}
			projects[metadata.ID] = &metadata
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	log.Printf("Loaded %d projects from database", len(projects))
	return projects, nil
}

// SaveProject saves a single project to the projects/ bucket
func (d *DB) SaveProject(metadata *ProjectMetadata) error {
	if metadata == nil {
		return fmt.Errorf("project metadata cannot be nil")
	}
	if metadata.ID == "" {
		return fmt.Errorf("project ID cannot be empty")
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.bolt.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("projects"))
		if err != nil {
			return fmt.Errorf("failed to create projects bucket: %w", err)
		}
		data, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal project: %w", err)
		}
		if err := bucket.Put([]byte(metadata.ID), data); err != nil {
			return fmt.Errorf("failed to store project: %w", err)
		}
		log.Printf("Saved project: id=%s, baseDir=%s", metadata.ID, metadata.BaseDir)
		return nil
	})
}

// GetProject retrieves a specific project from the projects/ bucket
func (d *DB) GetProject(projectID string) (*ProjectMetadata, error) {
	if projectID == "" {
		return nil, fmt.Errorf("projectID cannot be empty")
	}

	d.mutex.RLock()
	defer d.mutex.RUnlock()

	var metadata *ProjectMetadata
	err := d.bolt.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("projects"))
		if bucket == nil {
			return fmt.Errorf("project %s not found", projectID)
		}
		data := bucket.Get([]byte(projectID))
		if data == nil {
			return fmt.Errorf("project %s not found", projectID)
		}
		metadata = &ProjectMetadata{}
		if err := json.Unmarshal(data, metadata); err != nil {
			return fmt.Errorf("failed to unmarshal project: %w", err)
		}
		return nil
	})
	return metadata, err
}

// DeleteProject removes a project from the projects/ bucket
func (d *DB) DeleteProject(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("projectID cannot be empty")
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.bolt.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("projects"))
		if bucket == nil {
			return fmt.Errorf("project %s not found", projectID)
		}
		if err := bucket.Delete([]byte(projectID)); err != nil {
			return fmt.Errorf("failed to delete project: %w", err)
		}
		log.Printf("Deleted project: id=%s", projectID)
		return nil
	})
}

// ListProjectIDs returns all project IDs in the projects/ bucket
func (d *DB) ListProjectIDs() ([]string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	var ids []string
	err := d.bolt.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("projects"))
		if bucket == nil {
			return nil // No projects yet
		}
		return bucket.ForEach(func(k, v []byte) error {
			ids = append(ids, string(k[0:]))
			return nil
		})
	})
	return ids, err
}

// ProjectExists checks if a project exists in the projects/ bucket
func (d *DB) ProjectExists(projectID string) (bool, error) {
	if projectID == "" {
		return false, fmt.Errorf("projectID cannot be empty")
	}

	d.mutex.RLock()
	defer d.mutex.RUnlock()

	exists := false
	err := d.bolt.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("projects"))
		if bucket == nil {
			return nil
		}
		data := bucket.Get([]byte(projectID))
		exists = data != nil
		return nil
	})
	return exists, err
}

// SaveEmbedding saves an embedding vector to the embeddings/ bucket
func (d *DB) SaveEmbedding(cid string, vector []byte, modelName string) error {
	if cid == "" {
		return fmt.Errorf("CID cannot be empty")
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.bolt.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("embeddings"))
		if err != nil {
			return fmt.Errorf("failed to create embeddings bucket: %w", err)
		}

		embeddingData := map[string]interface{}{
			"vector": vector,
			"metadata": map[string]string{
				"modelName": modelName,
			},
		}

		data, err := json.Marshal(embeddingData)
		if err != nil {
			return fmt.Errorf("failed to marshal embedding: %w", err)
		}

		if err := bucket.Put([]byte(cid), data); err != nil {
			return fmt.Errorf("failed to store embedding: %w", err)
		}
		return nil
	})
}

// GetEmbedding retrieves an embedding vector from the embeddings/ bucket
func (d *DB) GetEmbedding(cid string) ([]byte, error) {
	if cid == "" {
		return nil, fmt.Errorf("CID cannot be empty")
	}

	d.mutex.RLock()
	defer d.mutex.RUnlock()

	var vector []byte
	err := d.bolt.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("embeddings"))
		if bucket == nil {
			return fmt.Errorf("embedding %s not found", cid)
		}

		data := bucket.Get([]byte(cid))
		if data == nil {
			return fmt.Errorf("embedding %s not found", cid)
		}

		embeddingData := make(map[string]interface{})
		if err := json.Unmarshal(data, &embeddingData); err != nil {
			return fmt.Errorf("failed to unmarshal embedding: %w", err)
		}

		if vectorData, ok := embeddingData["vector"]; ok {
			if v, ok := vectorData.([]byte); ok {
				vector = v
			}
		}
		return nil
	})
	return vector, err
}

