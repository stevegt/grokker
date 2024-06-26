package aidda

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

func TestRunTee(t *testing.T) {
	// Use 'echo' command for testing
	stdout, stderr, rc, err := RunTee("echo Hello, World!")
	if err != nil {
		t.Fatalf("RunTee failed: %v", err)
	}
	if rc != 0 {
		t.Fatalf("Expected return code 0, got: %d", rc)
	}
	if !bytes.Contains(stdout, []byte("Hello, World!")) {
		t.Errorf("Expected 'Hello, World!' in stdout, got: %s", stdout)
	}
	if len(stderr) != 0 {
		t.Errorf("Expected empty stderr, got: %s", stderr)
	}
}

func TestRun(t *testing.T) {
	// Use 'echo' command for testing
	stdout, stderr, rc, err := Run("echo Hello, Test!", nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if rc != 0 {
		t.Fatalf("Expected return code 0, got: %d", rc)
	}
	if !bytes.Contains(stdout, []byte("Hello, Test!")) {
		t.Errorf("Expected 'Hello, Test!' in stdout, got: %s", stdout)
	}
	if len(stderr) != 0 {
		t.Errorf("Expected empty stderr, got: %s", stderr)
	}
}

func TestRunInteractive(t *testing.T) {
	if os.Getenv("TEST_INTERACTIVE") == "1" {
		rc, err := RunInteractive("echo Hello, Interactive!")
		if err != nil {
			t.Fatalf("RunInteractive failed: %v", err)
		}
		if rc != 0 {
			t.Fatalf("Expected return code 0, got: %d", rc)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestRunInteractive")
	cmd.Env = append(os.Environ(), "TEST_INTERACTIVE=1")
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("RunInteractive command failed: %v", err)
	}
	if !bytes.Contains(stdout, []byte("Hello, Interactive!")) {
		t.Errorf("Expected 'Hello, Interactive!' in output, got: %s", stdout)
	}
}
