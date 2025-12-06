package kv

import (
	"github.com/stevegt/grokker/x/storm/db/kv/bbolt"
)

// Re-export interface types from bbolt as type aliases
// This allows kv.go to define the abstraction layer while bbolt.go
// provides the concrete interface definitions without circular dependencies
type ReadTx = bbolt.ReadTx
type WriteTx = bbolt.WriteTx
type KVStore = bbolt.KVStore

// NewDBStore creates a new BoltDB-backed KV store
func NewDBStore(dbPath string) (KVStore, error) {
	return bbolt.NewBoltDBStore(dbPath)
}
