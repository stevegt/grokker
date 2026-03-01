package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	. "github.com/stevegt/goadapt"
)

func TestModelsNoDBFound(t *testing.T) {
	// Commands that require a Grokker database should fail cleanly when
	// there is no `.grok` file in the current directory or any parent
	// directory.
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)

	dir, err := os.MkdirTemp("", "grokker-cli-nodb")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	cd(t, dir)
	defer cd(t, cwd)

	var emptyStdin bytes.Buffer
	stdout, stderr, err := grok(emptyStdin, "models")
	Tassert(t, err != nil, "expected error, got nil\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	Tassert(t, stdout.String() == "", "expected no stdout, got %q", stdout.String())
	Tassert(t, strings.Contains(stderr.String(), "no .grok found"), "expected stderr to mention missing .grok, got %q", stderr.String())
}
