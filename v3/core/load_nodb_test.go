package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	. "github.com/stevegt/goadapt"
)

func TestLoadNoDBFound(t *testing.T) {
	// Load should return ErrNoDB when there is no `.grok` file in the
	// current directory or any parent directory.
	dir, err := os.MkdirTemp("", "grokker-load-nodb")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	restore := chdir(t, dir)
	defer restore()

	_, _, _, _, _, err = Load("", true)
	Tassert(t, err != nil, "expected an error, got nil")
	Tassert(t, errors.Is(err, ErrNoDB), "expected ErrNoDB, got %v", err)
}

func TestLoadFromEmptyPathNoLock(t *testing.T) {
	// LoadFrom should reject an empty path without creating a stray lock
	// file (previously this created `.lock` and then tried to open "").
	dir, err := os.MkdirTemp("", "grokker-loadfrom-empty")
	Tassert(t, err == nil, "error creating temp dir: %v", err)
	defer os.RemoveAll(dir)

	restore := chdir(t, dir)
	defer restore()

	_, _, _, _, _, err = LoadFrom("", "", true)
	Tassert(t, err != nil, "expected an error, got nil")
	Tassert(t, errors.Is(err, ErrNoDB), "expected ErrNoDB, got %v", err)

	_, statErr := os.Stat(filepath.Join(dir, ".lock"))
	Tassert(t, os.IsNotExist(statErr), "expected no .lock file, got err=%v", statErr)
}
