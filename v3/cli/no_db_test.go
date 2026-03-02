package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/core"
)

func TestModelsNoDBWorks(t *testing.T) {
	// The `models` command should work even when there is no `.grok` file
	// in the current directory or any parent directory.
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)

	dir, err := os.MkdirTemp("", "grokker-cli-nodb")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	cd(t, dir)
	defer cd(t, cwd)

	var emptyStdin bytes.Buffer
	stdout, stderr, err := grok(emptyStdin, "models")
	Tassert(t, err == nil, "CLI returned unexpected error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	Tassert(t, stderr.String() == "", "expected no stderr, got %q", stderr.String())
	Tassert(t, strings.Contains(stdout.String(), "o3-mini"), "expected stdout to mention a known model, got %q", stdout.String())
	Tassert(t, strings.Contains(stdout.String(), "*"), "expected stdout to mark an active model with '*', got %q", stdout.String())
}

func TestModelsWithDBShowsActiveModel(t *testing.T) {
	// When a `.grok` db exists, `models` should mark the db's current
	// model with `*`.
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)

	dir, err := os.MkdirTemp("", "grokker-cli-models-db")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	cd(t, dir)
	defer cd(t, cwd)

	var emptyStdin bytes.Buffer

	_, _, err = grok(emptyStdin, "init")
	Tassert(t, err == nil, "init returned unexpected error: %v", err)

	_, _, err = grok(emptyStdin, "model", "gpt-4")
	Tassert(t, err == nil, "model returned unexpected error: %v", err)

	stdout, stderr, err := grok(emptyStdin, "models")
	Tassert(t, err == nil, "models returned unexpected error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	Tassert(t, stderr.String() == "", "expected no stderr, got %q", stderr.String())
	Tassert(t, strings.Contains(stdout.String(), "* gpt-4"), "expected stdout to mark gpt-4 as active, got %q", stdout.String())
}

func TestVersionNoDBPrintsNone(t *testing.T) {
	// The `version` command should not require a `.grok` db.
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)

	dir, err := os.MkdirTemp("", "grokker-cli-version-nodb")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	cd(t, dir)
	defer cd(t, cwd)

	var emptyStdin bytes.Buffer
	stdout, stderr, err := grok(emptyStdin, "version")
	Tassert(t, err == nil, "version returned unexpected error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	Tassert(t, stderr.String() == "", "expected no stderr, got %q", stderr.String())
	Tassert(t, strings.Contains(stdout.String(), "grokker version "), "expected stdout to include code version, got %q", stdout.String())
	Tassert(t, strings.Contains(stdout.String(), "grok db version (none)"), "expected stdout to include '(none)', got %q", stdout.String())
}

func TestVersionWithDBPrintsDBVersion(t *testing.T) {
	// When a `.grok` db exists, `version` should include its version.
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)

	dir, err := os.MkdirTemp("", "grokker-cli-version-db")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	cd(t, dir)
	defer cd(t, cwd)

	var emptyStdin bytes.Buffer

	_, _, err = grok(emptyStdin, "init")
	Tassert(t, err == nil, "init returned unexpected error: %v", err)

	stdout, stderr, err := grok(emptyStdin, "version")
	Tassert(t, err == nil, "version returned unexpected error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	Tassert(t, stderr.String() == "", "expected no stderr, got %q", stderr.String())
	Tassert(t, strings.Contains(stdout.String(), Spf("grok db version %s", core.Version)), "expected stdout to include db version %s, got %q", core.Version, stdout.String())
}
