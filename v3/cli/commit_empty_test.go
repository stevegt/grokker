package cli

import (
	"bytes"
	"os"
	"testing"

	. "github.com/stevegt/goadapt"
)

func TestCommitEmptyDiffNoOutput(t *testing.T) {
	// If there is no diff for the args used by `commit` (default is
	// `--staged`), `grok commit` should exit successfully with no stdout.
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)

	dir, err := os.MkdirTemp("", "grokker-cli-commit-empty")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	cd(t, dir)
	defer cd(t, cwd)

	run(t, "git", "init")

	var emptyStdin bytes.Buffer
	stdout, stderr, err := grok(emptyStdin, "commit")
	Tassert(t, err == nil, "CLI returned unexpected error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	Tassert(t, stdout.String() == "", "expected no stdout, got %q", stdout.String())
}
