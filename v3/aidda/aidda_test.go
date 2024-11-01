package aidda

import (
	"bytes"
	// "io/ioutil"
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

func TestReadPrompt(t *testing.T) {
	// Create a temporary file
	promptContent := `This is a test prompt

Please make changes to the code.

Sysmsg: Test system message
In: input1.go input2.go
Out: output1.go output2.go
`
	tmpFile, err := os.CreateTemp("", "prompt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(promptContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	p, err := readPrompt(tmpFile.Name())
	if err != nil {
		t.Fatalf("readPrompt failed: %v", err)
	}

	expectedTxt := "Please make changes to the code.\n"
	if p.Txt != expectedTxt {
		t.Errorf("Expected Txt to be %q, got %q", expectedTxt, p.Txt)
	}

	if p.Sysmsg != "Test system message" {
		t.Errorf("Expected Sysmsg to be %q, got %q", "Test system message", p.Sysmsg)
	}

	expectedIn := []string{"input1.go", "input2.go"}
	if len(p.In) != len(expectedIn) {
		t.Errorf("Expected In to have %d items, got %d", len(expectedIn), len(p.In))
	}

	expectedOut := []string{"output1.go", "output2.go"}
	if len(p.Out) != len(expectedOut) {
		t.Errorf("Expected Out to have %d items, got %d", len(expectedOut), len(p.Out))
	}
}
