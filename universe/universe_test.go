package universe

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/stevegt/goadapt"
)

var tmpDir string

func TestMain(m *testing.M) {
	// Create temporary directory
	var err error
	tmpDir, err = ioutil.TempDir("", "grokker")
	Ck(err)

	// Run tests
	exitCode := m.Run()

	// Remove temporary directory
	err = os.RemoveAll(tmpDir)
	Ck(err)

	// Exit with test status code
	os.Exit(exitCode)
}

func newUniverse(t *testing.T) (u *Universe) {
	fn := filepath.Join(tmpDir, "test.db")
	u, err := Open(fn)
	Tassert(t, err == nil)
	Tassert(t, u != nil)
	return
}

// As a caller, I want to be able to create a new universe.
func TestOpen(t *testing.T) {
	u := newUniverse(t)
	defer u.Close()
}

// As a caller, I want to be able to provide a document path to be
// added to the universe.
func TestAdd(t *testing.T) {
	u := newUniverse(t)
	defer u.Close()

	// Add a document
	err := u.AddDocument("testdata/doc1")
	Tassert(t, err == nil)

	// Get a chunk
	chunk, err := u.GetChunk("testdata/doc1", 0)
	Tassert(t, err == nil)
	Tassert(t, chunk != nil)

}
