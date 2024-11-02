package aidda

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/stevegt/goadapt"
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

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "aidda-test")
	Ck(err)
	defer os.RemoveAll(tmpDir)
	// Create a .aidda directory in the temporary directory
	aiddaDir := tmpDir + "/.aidda"
	err = os.Mkdir(aiddaDir, 0755)
	Ck(err)
	// Create a prompt file in the .aidda directory
	tmpFile, err := os.Create(aiddaDir + "/prompt")
	Ck(err)
	defer tmpFile.Close()
	// Write the prompt content to the prompt file
	_, err = tmpFile.WriteString(promptContent)
	Ck(err)
	// Create input files in the temporary directory
	input1, err := os.Create(tmpDir + "/input1.go")
	Ck(err)
	defer input1.Close()
	input2, err := os.Create(tmpDir + "/input2.go")
	Ck(err)
	defer input2.Close()

	p, err := readPrompt(tmpFile.Name())
	if err != nil {
		t.Fatalf("readPrompt failed: %v", err)
	}

	expectedTxt := `This is a test prompt

Please make changes to the code.
`
	if p.Txt != expectedTxt {
		t.Errorf("Expected Txt to be %q, got %q", expectedTxt, p.Txt)
	}

	if p.Sysmsg != "Test system message" {
		t.Errorf("Expected Sysmsg to be %q, got %q", "Test system message", p.Sysmsg)
	}

	expectedIn := []string{filepath.Join(tmpDir, "input1.go"), filepath.Join(tmpDir, "input2.go")}
	if len(p.In) != len(expectedIn) {
		t.Errorf("Expected In to have %d items, got %d", len(expectedIn), len(p.In))
	}
	for i, in := range p.In {
		if in != expectedIn[i] {
			t.Errorf("Expected In[%d] to be %q, got %q", i, expectedIn[i], in)
		}
	}

	expectedOut := []string{filepath.Join(tmpDir, "output1.go"), filepath.Join(tmpDir, "output2.go")}
	if len(p.Out) != len(expectedOut) {
		t.Errorf("Expected Out to have %d items, got %d", len(expectedOut), len(p.Out))
	}
	for i, out := range p.Out {
		if out != expectedOut[i] {
			t.Errorf("Expected Out[%d] to be %q, got %q", i, expectedOut[i], out)
		}
	}
}

func TestReadPrompt_MultiLineHeaders(t *testing.T) {
	// Create a temporary file with multi-line headers
	promptContent := `This is a test prompt

Please make changes to the code.

Sysmsg: This is a test system message that
 continues on the next line
In: 
	input1.go input2.go
Out: output1.go
 output2.go
`

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "aidda-test-multiline")
	Ck(err)
	defer os.RemoveAll(tmpDir)
	// Create a .aidda directory in the temporary directory
	aiddaDir := tmpDir + "/.aidda"
	err = os.Mkdir(aiddaDir, 0755)
	Ck(err)
	// Create a prompt file in the .aidda directory
	tmpFile, err := os.Create(aiddaDir + "/prompt")
	Ck(err)
	defer tmpFile.Close()
	// Write the prompt content to the prompt file
	_, err = tmpFile.WriteString(promptContent)
	Ck(err)
	// Create input files in the temporary directory
	input1, err := os.Create(tmpDir + "/input1.go")
	Ck(err)
	defer input1.Close()
	input2, err := os.Create(tmpDir + "/input2.go")
	Ck(err)
	defer input2.Close()

	p, err := readPrompt(tmpFile.Name())
	if err != nil {
		t.Fatalf("readPrompt with multi-line headers failed: %v", err)
	}

	expectedTxt := `This is a test prompt

Please make changes to the code.
`
	if p.Txt != expectedTxt {
		t.Errorf("Expected Txt to be %q, got %q", expectedTxt, p.Txt)
	}

	expectedSysmsg := "This is a test system message that continues on the next line"
	if p.Sysmsg != expectedSysmsg {
		t.Errorf("Expected Sysmsg to be %q, got %q", expectedSysmsg, p.Sysmsg)
	}

	expectedIn := []string{filepath.Join(tmpDir, "input1.go"), filepath.Join(tmpDir, "input2.go")}
	if len(p.In) != len(expectedIn) {
		t.Errorf("Expected In to have %d items, got %d", len(expectedIn), len(p.In))
	}
	for i, in := range p.In {
		if in != expectedIn[i] {
			t.Errorf("Expected In[%d] to be %q, got %q", i, expectedIn[i], in)
		}
	}

	expectedOut := []string{filepath.Join(tmpDir, "output1.go"), filepath.Join(tmpDir, "output2.go")}
	if len(p.Out) != len(expectedOut) {
		t.Errorf("Expected Out to have %d items, got %d", len(expectedOut), len(p.Out))
	}
	for i, out := range p.Out {
		if out != expectedOut[i] {
			t.Errorf("Expected Out[%d] to be %q, got %q", i, expectedOut[i], out)
		}
	}
}

func TestReadPrompt_NoBlankLine(t *testing.T) {
	// Create a temporary file without a blank line after the first line
	promptContent := `This is a test prompt
Please make changes to the code.

Sysmsg: Test system message
In: input1.go input2.go
Out: output1.go output2.go
`
	tmpFile, err := os.CreateTemp("", "prompt_no_blank")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(promptContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	_, err = readPrompt(tmpFile.Name())
	if err == nil || err.Error() != "prompt file must have a blank line after the first line, just like a commit message" {
		t.Errorf("Expected error about missing blank line, got: %v", err)
	}
}
