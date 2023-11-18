package grokker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"

	. "github.com/stevegt/goadapt"
)

// mkFile creates a file with the given name, approximate chunk size in bytes, and chunk count.
// Each line of each chunk is a string of the form:
//
//	"file name: chunk number: chunk line number: sha256 hash hex digest of all previous lines"
//
// The file is created in the current directory.  If the file already
// exists, it is overwritten. The chunk size is approximate because
// each chunk will be slightly larger than the given size in order to
// complete the last line of the chunk.
func mkFile(name string, chunkCount, chunkSize int) {
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

// run executes the given command with the given arguments.
func run(t *testing.T, cmd string, args ...string) {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	Tassert(t, err == nil, "error running `%s`: %v", cmd, err)
}

// runOut executes the given command with the given arguments and returns the output.
func runOut(t *testing.T, cmd string, args ...string) string {
	c := exec.Command(cmd, args...)
	c.Stderr = os.Stderr
	out, err := c.Output()
	Tassert(t, err == nil, "error running `%s`: %v", cmd, err)
	return string(out)
}

// cd changes the current working directory to the given directory.
func cd(t *testing.T, dir string) {
	err := os.Chdir(dir)
	Tassert(t, err == nil, "error changing to directory %s: %v", dir, err)
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

func TestMigrationSetup(t *testing.T) {
	// get current working directory
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)

	// create temporary base directory
	tmpBaseDir, err = os.MkdirTemp("", "grokker-migration-test")
	Tassert(t, err == nil, "error creating temporary base directory: %v", err)
	tmpRepoDir = tmpBaseDir + "/grokker"
	tmpDataDir = tmpRepoDir + "/testdata/migration_tmp"

	// cd into temp base directory
	cd(t, tmpBaseDir)

	// clone repo into subdir of temporary base directory
	run(t, "git", "clone", cwd, "grokker")

	// create data directory
	err = os.Mkdir(tmpDataDir, 0755)
	Tassert(t, err == nil, "error creating testdata directory: %v", err)
}

func TestMigration_0_1_0(t *testing.T) {
	ckReady(t)

	// cd into temp repo directory
	cd(t, tmpRepoDir)

	// git checkout v0.1.0  # 2d3d3ff60feb2935b81035d44d944df8bc2c70f8
	run(t, "git", "checkout", "v0.1.0")

	// build grok and move to temp data directory
	cd(t, "cmd/grok")
	run(t, "go", "build")
	run(t, "mv", "grok", tmpDataDir)

	// cd into temp data directory
	cd(t, tmpDataDir)

	// grok init
	run(t, "./grok", "init")

	// grok upgrade gpt-4
	run(t, "./grok", "upgrade", "gpt-4")

	// simple test with all chunks small 'cause 0.1.0 can't
	// handle chunks larger than token limit
	//
	// create a file with 10 chunks of 1000 bytes
	mkFile("testfile-10-100.txt", 10, 1000)

	// grok add testfile-10-100.txt
	run(t, "./grok", "add", "testfile-10-100.txt")

	// run this and check the output for 5731294f1fbb4b48756f72a36838350d9353965ddad9f4fd6ad21a9daccd6dea
	out := runOut(t, "./grok", "q", "what is the hash after testfile-10-100.txt:9:10?")
	// search for the expected hash
	ok := strings.Contains(out, "5731294f1fbb4b48756f72a36838350d9353965ddad9f4fd6ad21a9daccd6dea")
	Tassert(t, ok, "expected hash not found in output: %s", out)
}

func TestMigrationHead(t *testing.T) {
	ckReady(t)

	// test with 1 chunk slightly larger than GPT-4 token size
	// create a file with 1 chunk of 20000 bytes
	// (about 11300 tokens each chunk depending on hash content)
	// mkFile("testfile-1-20000.txt", 1, 20000)

	// test with 3 chunks much larger than GPT-4 token size
	// create a file with 3 chunks of 300000 bytes
	// (about 167600 tokens each chunk depending on hash content)
	// mkFile("testfile-3-300000.txt", 3, 300000)
}
