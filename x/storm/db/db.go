package main

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
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

// LoadProjectRegistry loads all projects from persistent storage
func LoadProjectRegistry(store kv.KVStore) (map[string]*Project, error) {
	projects := make(map[string]*Project)
	
	err := store.View(func(tx kv.ReadTx) error {
		return tx.ForEach("projects", func(k, v []byte) error {
			var project Project
			if err := UnmarshalCBOR(v, &project); err != nil {
				return fmt.Errorf("failed to unmarshal project: %w", err)
			}
			projects[project.ID] = &project
			// Reinitialize Chat instance from markdown file
			project.Chat = NewChat(project.MarkdownFile)
			return nil
		})
	})
	
	return projects, err
}

// SaveProject persists a project to storage
func SaveProject(store kv.KVStore, project *Project) error {
	return store.Update(func(tx kv.WriteTx) error {
		data, err := MarshalCBOR(project)
		if err != nil {
			return fmt.Errorf("failed to marshal project: %w", err)
		}
		return tx.Put("projects", project.ID, data)
	})
}

// LoadConfig loads daemon configuration from storage
func LoadConfig(store kv.KVStore) (map[string]interface{}, error) {
	config := make(map[string]interface{})
	
	err := store.View(func(tx kv.ReadTx) error {
		data := tx.Get("config", "config")
		if data == nil {
			return nil // No config yet
		}
		return UnmarshalCBOR(data, &config)
	})
	
	return config, err
}

// SaveConfig persists daemon configuration to storage
func SaveConfig(store kv.KVStore, config map[string]interface{}) error {
	return store.Update(func(tx kv.WriteTx) error {
		data, err := MarshalCBOR(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		return tx.Put("config", "config", data)
	})
}
