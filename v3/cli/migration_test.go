package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	. "github.com/stevegt/goadapt"
)

// mkDataFile creates a file with the given name, approximate chunk size in bytes, and chunk count.
// Each line of each chunk is a string of the form:
//
//	"file name: chunk number: chunk line number: sha256 hash hex digest of all previous lines"
//
// The file is created in the current directory.  If the file already
// exists, it is overwritten. The chunk size is approximate because
// each chunk will be slightly larger than the given size in order to
// complete the last line of the chunk.
func mkDataFile(name string, chunkCount, chunkSize int) {
	var buf bytes.Buffer
	hash := sha256.New()
	for i := 0; i < chunkCount; i++ {
		size := 0
		for j := 0; ; j++ {
			if size > chunkSize {
				break
			}
			// get hex digest of hash
			digest := hash.Sum(nil)
			hexDigest := hex.EncodeToString(digest)
			line := []byte(Spf("%s: %d: %d: %s\n", name, i, j, hexDigest))
			buf.Write(line)
			hash.Write(line)
			size += len(line)
		}
		// separate chunks with a blank line
		buf.WriteString("\n")
	}
	// write buf to file
	err := ioutil.WriteFile(name, buf.Bytes(), 0644)
	Ck(err)
}

// mkGrok builds the given version of grok and puts it in tmpDataDir
func mkGrok(t *testing.T, version, dir string) {
	ckReady(t)
	// cd into temp repo directory
	cd(t, tmpRepoDir)
	run(t, "git", "checkout", version)
	// build grok and move to temp data directory
	cd(t, dir)
	run(t, "go", "build")
	run(t, "mv", "grok", tmpDataDir)
	cd(t, tmpDataDir)
}

var (
	tmpBaseDir string
	tmpDataDir string
	tmpRepoDir string
)

// ckReady checks that the temporary base directory, temporary data directory,
// and temporary repo directory have been created.
func ckReady(t *testing.T) {
	Tassert(t, tmpBaseDir != "", "temporary base directory not created")
	Tassert(t, tmpDataDir != "", "temporary data directory not created")
	Tassert(t, tmpRepoDir != "", "temporary repo directory not created")
}

/*
XXX move setup to here after db is its own package and this test file is in there

// TestMain
func TestMain(m *testing.M) {
	// create a temporary directory
	var err error
	tmpBaseDir, err = os.MkdirTemp("", "grokker")
	if err != nil {
		panic(err)
	}
	// create a temporary data directory
	tmpDataDir, err = os.MkdirTemp(tmpBaseDir, "data")
	if err != nil {
		panic(err)
	}
	// create a temporary repo directory
	tmpRepoDir, err = os.MkdirTemp(tmpBaseDir, "repo")
	if err != nil {
		panic(err)
	}
	// run tests
	code := m.Run()
	// remove temporary directory
	os.RemoveAll(tmpBaseDir)
	// exit
	os.Exit(code)
}
*/

func TestMigrationSetup(t *testing.T) {
	// get current repo root directory
	// git rev-parse --show-toplevel
	out := runOut(t, "git", "rev-parse", "--show-toplevel")
	repoRoot := strings.TrimSpace(out)

	// create temporary base directory
	var err error
	tmpBaseDir, err = os.MkdirTemp("", "grokker-migration-test")
	Tassert(t, err == nil, "error creating temporary base directory: %v", err)
	tmpRepoDir = tmpBaseDir + "/grokker"
	tmpDataDir = tmpRepoDir + "/testdata/migration_tmp"

	// cd into temp base directory
	cd(t, tmpBaseDir)

	// clone repo into subdir of temporary base directory
	run(t, "git", "clone", repoRoot, "grokker")

	// create data directory
	err = os.MkdirAll(tmpDataDir, 0755)
	Tassert(t, err == nil, "error creating testdata directory: %v", err)
}

func TestMigration_0_1_0(t *testing.T) {
	// checkout v0.1.0, build grok, move to temp data directory, cd there
	mkGrok(t, "v0.1.0", "cmd/grok")

	// grok init
	run(t, "./grok", "init")

	// grok upgrade gpt-4
	run(t, "./grok", "upgrade", "gpt-4")

	// simple test with all chunks small 'cause 0.1.0 can't
	// handle chunks larger than token limit
	//
	// create a file with 10 chunks of 1000 bytes
	mkDataFile("testfile-10-100.txt", 10, 1000)

	// grok add testfile-10-100.txt
	run(t, "./grok", "add", "testfile-10-100.txt")

}

func TestMigration_1_1_1(t *testing.T) {
	mkGrok(t, "v1.1.1", "cmd/grok")
	// run ls to get upgraded to 1.1.1
	// - this is to ensure we're ignoring the patch version during
	//   subsequent migrations
	run(t, "./grok", "ls")
}

func TestMigration_2_1_2(t *testing.T) {
	mkGrok(t, "v2.1.2", "cmd/grok")

	// test with 1 chunk slightly larger than GPT-4 token size
	// create a file with 1 chunk of 20000 bytes
	// (about 11300 tokens each chunk depending on hash content)
	mkDataFile("testfile-1-20000.txt", 1, 20000)
	run(t, "./grok", "add", "testfile-1-20000.txt")

	// test with 3 chunks much larger than GPT-4 token size
	// create a file with 3 chunks of 300000 bytes
	// (about 167600 tokens each chunk depending on hash content)
	mkDataFile("testfile-3-300000.txt", 3, 300000)
	run(t, "./grok", "add", "testfile-3-300000.txt")
}

// test migration from v3.0.9 to 3.0.10 so we know integer ordering
// works as expected
func TestMigration_3_0_10(t *testing.T) {
	mkGrok(t, "v3.0.9", "v3/cmd/grok")
	// run ls to get upgraded to 3.0.9
	run(t, "./grok", "ls")
	// now build 3.0.10
	mkGrok(t, "v3.0.10", "v3/cmd/grok")
	// run ls to get upgraded to 3.0.10
	run(t, "./grok", "ls")
}

func TestMigrationHead(t *testing.T) {
	// mkGrok(t, "50635ed58e15af224ae118e762a4291cc0f54aa6")
	mkGrok(t, "main", "v3/cmd/grok")

	// run this and check the output for 5731294f1fbb4b48756f72a36838350d9353965ddad9f4fd6ad21a9daccd6dea
	out := runOut(t, "./grok", "q", "what is the hash after testfile-10-100.txt:9:10?")
	// search for the expected hash
	ok := strings.Contains(out, "5731294f1fbb4b48756f72a36838350d9353965ddad9f4fd6ad21a9daccd6dea")
	Tassert(t, ok, "expected hash not found in output: %s", out)

	// XXX check large file hashes
}
