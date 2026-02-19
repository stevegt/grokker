package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/mock"
)

func TestInitNoDB(t *testing.T) {
	// InitNoDB should prepare a usable Grokker object without touching
	// the filesystem beyond verifying the root directory exists.
	dir, err := os.MkdirTemp("", "grokker-nodb")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	g, err := InitNoDB(dir, "")
	Tassert(t, err == nil, "InitNoDB returned unexpected error: %v", err)
	Tassert(t, g != nil, "expected non-nil Grokker")

	absDir, err := filepath.Abs(dir)
	Tassert(t, err == nil, "error getting abs dir: %v", err)
	Tassert(t, g.Root == absDir, "expected Root=%q, got %q", absDir, g.Root)

	_, err = os.Stat(filepath.Join(dir, ".grok"))
	Tassert(t, os.IsNotExist(err), "expected no .grok file, got err=%v", err)

	Tassert(t, g.models != nil, "expected models to be initialized")
	Tassert(t, g.Model != "", "expected default model to be set")
	Tassert(t, g.grokpath == "", "expected empty grokpath, got %q", g.grokpath)
}

func TestGitCommitMessageNoDB(t *testing.T) {
	// GitCommitMessage should not require a `.grok` database; it only
	// needs a Grokker object with models initialized.
	dir, err := os.MkdirTemp("", "grokker-commit-nodb")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	// Save/restore cwd because GitCommitMessage runs `git diff` in the
	// current directory.
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting cwd: %v", err)
	err = os.Chdir(dir)
	Tassert(t, err == nil, "error chdir to temp dir: %v", err)
	defer os.Chdir(cwd)

	// Create a minimal git repo with staged changes.
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		Tassert(t, err == nil, "error running %v: %v", args, err)
	}

	run("git", "init")
	err = os.WriteFile("file.txt", []byte("hello\n"), 0644)
	Tassert(t, err == nil, "error writing file.txt: %v", err)
	run("git", "add", "file.txt")

	g, err := InitNoDB(dir, "")
	Tassert(t, err == nil, "InitNoDB returned unexpected error: %v", err)

	// Use a mock model so the test doesn't make a network call.
	const modelName = "mock-commit-model"
	g.models.AddMockModel(modelName, 200000)
	m, ok := g.models.Available[modelName]
	Tassert(t, ok, "expected %q model to exist after AddMockModel", modelName)
	mockClient, ok := m.provider.(*mock.Client)
	Tassert(t, ok, "expected mock provider for %q model", modelName)

	want := "Write tests\n\n- file.txt: add hello"
	mockClient.SetResponse(modelName, want)

	got, err := g.GitCommitMessage(modelName, "--staged")
	Tassert(t, err == nil, "GitCommitMessage returned unexpected error: %v", err)
	Tassert(t, got == want, "unexpected commit message: got %q want %q", got, want)

	_, err = os.Stat(".grok")
	Tassert(t, os.IsNotExist(err), "expected no .grok file, got err=%v", err)
}
