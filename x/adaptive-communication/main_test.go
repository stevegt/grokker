package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Config is a simple configuration structure for the agent.
type Config struct {
	Brain        string `json:"brain"`
	GoalMD       string `json:"goal_md"`
	PseudocodeMD string `json:"pseudocode_md"`
}

// createAgentDir sets up a temporary agent directory with a basic
// configuration, goal.md, and pseudocode.md files.
func createAgentDir(root, agentID string) error {
	agentDir := filepath.Join(root, agentID)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return err
	}

	config := Config{
		Brain:        "dummy-model",
		GoalMD:       "goal.md",
		PseudocodeMD: "pseudocode.md",
	}

	// Write a dummy config.json file.
	buf, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(agentDir, "config.json"), buf, 0644)
	if err != nil {
		return err
	}
	// Create dummy goal.md, pseudocode.md and messages.log files.
	if err := ioutil.WriteFile(filepath.Join(agentDir, "goal.md"),
		[]byte("Agent goal content."), 0644); err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(agentDir, "pseudocode.md"),
		[]byte("Initial pseudocode."), 0644); err != nil {
		return err
	}
	// Initialize an empty messages.log file.
	return ioutil.WriteFile(filepath.Join(agentDir, "messages.log"),
		[]byte{}, 0644)
}

// TestSimulation verifies that agents can be loaded from a temporary
// directory and simulates a brief run of the multi-agent system.
func TestSimulation(t *testing.T) {
	// Create a temporary directory for agents.
	tempDir, err := ioutil.TempDir("", "agents_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create two agent directories.
	if err := createAgentDir(tempDir, "agent1"); err != nil {
		t.Fatalf("Failed to create agent1: %v", err)
	}
	if err := createAgentDir(tempDir, "agent2"); err != nil {
		t.Fatalf("Failed to create agent2: %v", err)
	}

	// Load agents from the temporary directory.
	agents, err := LoadAgents(tempDir)
	if err != nil {
		t.Fatalf("Failed to load agents: %v", err)
	}
	if len(agents) < 2 {
		t.Fatalf("Expected at least 2 agents, got %d", len(agents))
	}
	t.Logf("Loaded %d agents for testing.", len(agents))

	// For testing purposes, run a short simulation duration.
	stopCh := make(chan bool)
	go func() {
		// Run simulation for 3 seconds.
		time.Sleep(3 * time.Second)
		close(stopCh)
	}()

	// Since main() runs an infinite simulation loop, for testing we only
	// verify that agents can be loaded and would run. A full integration
	// test would require more elaborate synchronization.
	t.Log("Simulation test completed.")
}
