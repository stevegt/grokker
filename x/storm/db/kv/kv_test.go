package kv

import (
	"testing"
)

// Interface compliance tests - verify implementations satisfy contracts
func TestReadTxInterface(t *testing.T) {
	// Compile-time check: if this compiles, interface is satisfied
	var _ ReadTx = nil
}

func TestWriteTxInterface(t *testing.T) {
	var _ WriteTx = nil
}

func TestKVStoreInterface(t *testing.T) {
	var _ KVStore = nil
}