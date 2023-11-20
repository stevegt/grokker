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
	config.Stdin = &stdin
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

	// create an empty emptyStdin buffer
	var emptyStdin bytes.Buffer

	// initialize a grokker repository
	stdout, stderr, err := grok(emptyStdin, "init")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// pass in a command line that should work
	stdout, stderr, err = grok(emptyStdin, "models")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match := strings.Contains(stdout.String(), "gpt-")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	// check that the stderr buffer is empty
	Tassert(t, stderr.String() == "", "CLI returned unexpected error: %s", stderr.String())

	// set model
	stdout, stderr, err = grok(emptyStdin, "model", "gpt-4")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// create a file
	f, err := os.Create("test.txt")
	Tassert(t, err == nil, "error creating file: %v", err)
	// put some content in the file
	_, err = f.WriteString("testing is good")
	Tassert(t, err == nil, "error writing to file: %v", err)
	// close the file
	err = f.Close()

	// add the file to the repository
	stdout, stderr, err = grok(emptyStdin, "add", "test.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// ask a question
	stdout, stderr, err = grok(emptyStdin, "q", "Does the context claim testing is good?  Answer yes or no.")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "yes") || strings.Contains(stdout.String(), "Yes")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// try adding the file again
	stdout, stderr, err = grok(emptyStdin, "add", "test.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer is empty
	Tassert(t, stdout.String() == "", "CLI returned unexpected output: %s", stdout.String())
	// check that the stderr buffer is empty
	Tassert(t, stderr.String() == "", "CLI returned unexpected error: %s", stderr.String())

	// try adding a file that doesn't exist
	stdout, stderr, err = grok(emptyStdin, "add", "test2.txt")
	// if the file doesn't exist, err will be an *fs.PathError
	Tassert(t, err.(*fs.PathError).Err == syscall.ENOENT, "CLI returned unexpected error: %#v", err)

	// create and add a couple of files we'll forget
	mkFile(t, "test2.txt", "forget daisies")
	mkFile(t, "test3.txt", "forget submarines")
	stdout, stderr, err = grok(emptyStdin, "add", "test2.txt", "test3.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// make sure the files are in the repository
	stdout, stderr, err = grok(emptyStdin, "q", "what should we forget?")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	match = strings.Contains(stdout.String(), "daisies") && strings.Contains(stdout.String(), "submarines")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// forget the files using relative and absolute paths
	stdout, stderr, err = grok(emptyStdin, "forget", "test2.txt", Spf("%s/test3.txt", dir))
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// make sure the files are no longer in the repository
	stdout, stderr, err = grok(emptyStdin, "q", "what should we forget?")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	match = !strings.Contains(stdout.String(), "daisies") && !strings.Contains(stdout.String(), "submarines")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test continuation
	mkFile(t, "continue.txt", "roses are red, violets are blue, grokker is great, and so are you!")
	stdout, stderr, err = grok(emptyStdin, "add", "continue.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// create a stdin buffer with the preface
	preface := "roses are red,"
	stdinContinue := bytes.Buffer{}
	stdinContinue.WriteString(preface)
	// run the CLI with the preface as stdin
	stdout, stderr, err = grok(stdinContinue, "qc")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// fmt.Println(stdout.String())
	match = strings.Contains(stdout.String(), "violets are blue")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test revision
	stdinRevision := bytes.Buffer{}
	stdinRevision.WriteString("roses are orange")
	// run the CLI with the incorrect text as stdin
	stdout, stderr, err = grok(stdinRevision, "qr")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// fmt.Println(stdout.String())
	match = strings.Contains(stdout.String(), "are red")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test revision with custom sysmsg
	stdinRevisionWithSysmsg := bytes.Buffer{}
	stdinRevisionWithSysmsg.WriteString("you are an expert botanist.  make corrections in the color of roses based on the provided context.\n\nroses are orange")
	// run the CLI with the incorrect text as stdin
	stdout, stderr, err = grok(stdinRevisionWithSysmsg, "qr", "-s")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	fmt.Println(stdout.String())
	match = strings.Contains(stdout.String(), "red")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test revision with custom sysmsg but one paragraph
	stdinRevisionWithSysmsg = bytes.Buffer{}
	stdinRevisionWithSysmsg.WriteString("this is a one-paragraph input that should not work.")
	// run the CLI with the incorrect text as stdin
	stdout, stderr, err = grok(stdinRevisionWithSysmsg, "qr", "-s")
	Tassert(t, err != nil, "CLI returned no error when it should have")

	// test backup
	stdout, stderr, err = grok(emptyStdin, "backup")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

}
