package grokker

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"

	. "github.com/stevegt/goadapt"
)

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
	Tassert(t, err == nil, "error running `%s %s`: %v", cmd, args, err)
	return string(out)
}

// cd changes the current working directory to the given directory.
func cd(t *testing.T, dir string) {
	err := os.Chdir(dir)
	Tassert(t, err == nil, "error changing to directory %s: %v", dir, err)
}

// grok runs the grokker cli with the given arguments and returns
// stdout, stderr, rc, and err
func grok(stdin bytes.Buffer, args ...string) (stdout, stderr bytes.Buffer, err error) {
	defer Return(&err)
	// capture goadapt stdio
	SetStdio(&stdin, &stdout, &stderr)
	defer SetStdio(nil, nil, nil)

	// also pass stdio to the CLI
	config := NewConfig()
	config.Stdout = &stdout
	config.Stderr = &stderr

	// get the caller's filename and line number
	_, fn, line, _ := runtime.Caller(1)

	var exitRc int
	// replace the kong exit function with one that doesn't exit
	config.Exit = func(rc int) {
		if rc != 0 {
			msg := Spf("%s:%d rc: %v\nstderr:\n%s", fn, line, rc, stderr.String())
			fmt.Println(msg)
			exitRc = rc
		}
	}

	// run the CLI
	rc, err := Cli(args, config)
	if err == nil && (exitRc != 0 || rc != 0) {
		err = fmt.Errorf("rc: %v exitRc: %v", rc, exitRc)
	}
	return
}

// mkFile creates a file with the given name and content.
func mkFile(t *testing.T, name, content string) {
	f, err := os.Create(name)
	Tassert(t, err == nil, "error creating file: %v", err)
	_, err = f.WriteString(content)
	Tassert(t, err == nil, "error writing to file: %v", err)
	err = f.Close()
	Tassert(t, err == nil, "error closing file: %v", err)
}

func TestCli(t *testing.T) {
	// get current working directory
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)
	// create a temporary directory
	dir, err := os.MkdirTemp("", "grokker")
	Ck(err)
	defer os.RemoveAll(dir)
	// cd into the temporary directory
	cd(t, dir)
	defer cd(t, cwd)

	// create a stdin buffer
	var stdin bytes.Buffer

	// initialize a grokker repository
	stdout, stderr, err := grok(stdin, "init")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// pass in a command line that should work
	stdout, stderr, err = grok(stdin, "models")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match := strings.Contains(stdout.String(), "gpt-")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	// check that the stderr buffer is empty
	Tassert(t, stderr.String() == "", "CLI returned unexpected error: %s", stderr.String())

	// create a file
	f, err := os.Create("test.txt")
	Tassert(t, err == nil, "error creating file: %v", err)
	// put some content in the file
	_, err = f.WriteString("testing is good")
	Tassert(t, err == nil, "error writing to file: %v", err)
	// close the file
	err = f.Close()

	// add the file to the repository
	stdout, stderr, err = grok(stdin, "add", "test.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// ask a question
	stdout, stderr, err = grok(stdin, "q", "Does the context claim testing is good?  Answer yes or no.")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "yes") || strings.Contains(stdout.String(), "Yes")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// try adding the file again
	stdout, stderr, err = grok(stdin, "add", "test.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer is empty
	Tassert(t, stdout.String() == "", "CLI returned unexpected output: %s", stdout.String())
	// check that the stderr buffer is empty
	Tassert(t, stderr.String() == "", "CLI returned unexpected error: %s", stderr.String())

	// try adding a file that doesn't exist
	stdout, stderr, err = grok(stdin, "add", "test2.txt")
	// if the file doesn't exist, err will be an *fs.PathError
	Tassert(t, err.(*fs.PathError).Err == syscall.ENOENT, "CLI returned unexpected error: %#v", err)

	// create and add a couple of files we'll forget
	mkFile(t, "test2.txt", "forget daisies")
	mkFile(t, "test3.txt", "forget submarines")
	stdout, stderr, err = grok(stdin, "add", "test2.txt", "test3.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// make sure the files are in the repository
	stdout, stderr, err = grok(stdin, "q", "what should we forget?")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	match = strings.Contains(stdout.String(), "daisies") && strings.Contains(stdout.String(), "submarines")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// forget the files using relative and absolute paths
	stdout, stderr, err = grok(stdin, "forget", "test2.txt", Spf("%s/test3.txt", dir))
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// make sure the files are no longer in the repository
	stdout, stderr, err = grok(stdin, "q", "what should we forget?")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	match = !strings.Contains(stdout.String(), "daisies") && !strings.Contains(stdout.String(), "submarines")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	/*
		right now ForgetDocument doesn't work because it doesn't mark
			chunks as stale, so gc() isn't removing them, but gc() isn't being
			called anyway.  make recommendations for how to fix this -- should ForgetDocument
			mark all the chunks as stale? or should gc() be comparing all the chunks
			to the list of Documents and removing the ones that aren't referenced?
			should we even be using the stale bit?

		To resolve this issue, marking all the chunks as stale in the ForgetDocument function could be done as the first step. This means iterating over all chunks in the Grokker database and checking if the Document.RelPath of the chunk matches the path of the document being forgotten. If a match is found, the chunk's stale bit should be set to true.

		Setting the stale bit to true during ForgetDocument and finally removing them using gc() is a better approach as it gives the garbage collector function responsibility for removing chunks, allowing it maintain the integrity of the Chunks list.

		Moreover, to improve the effectiveness of the garbage collection, gc() function could also be adjusted to check if the document of the chunks is still present in the list of Documents. If not, these chunks should be marked stale, ready for garbage collection.

		In terms of using the stale bit, it's a useful mechanism for denoting chunks that are 'dirty' or no longer needed, essentially marking them for deletion. It allows for efficient ways of managing memory or storage and ensures that unused items can be wiped during the next garbage collection cycle, rather than immediately. This gives you control over when the deletion occurs, which can be beneficial in managing system performance.

		To strengthen this approach, calling the gc() function at more regular intervals or right after the ForgetDocument operation might be effective to ensure stale chunks are removed in time.

		So, in summary, the "stale" bit mechanism coupled with some adjustments to the "ForgetDocument" and "gc()" function would be a good fix for the problem.

	*/

}
