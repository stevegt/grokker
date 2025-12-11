package split

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSplit(t *testing.T) {
	// Read test data from file instead of stdin
	testDataPath := filepath.Join("testdata", "example.md")
	input, err := os.ReadFile(testDataPath)
	if err != nil {
		t.Fatalf("Error reading test data: %v", err)
	}

	// Parse the storm file from the input
	roundTrips, err := Parse(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("Error parsing storm file: %v", err)
	}

	// Verify we got expected results
	if len(roundTrips) == 0 {
		t.Fatal("Expected at least one round trip, got zero")
	}

	// Verify each round trip has required fields
	for i, rt := range roundTrips {
		if rt.Query == "" {
			t.Errorf("Round trip %d missing query", i)
		}
		if rt.Response == "" {
			t.Errorf("Round trip %d missing response", i)
		}
	}
}
